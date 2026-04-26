package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/deductive-ai/dx/internal/upload"
)

// uploadFileToSession uploads a single text file to the active session.
// It rejects directories and binary files.
func uploadFileToSession(cfg *config.Config, state *session.State, path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", path, err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("%s is a directory; only single files are supported", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	if isBinary(data) {
		return fmt.Errorf("%s appears to be a binary file; only text files are supported", filepath.Base(path))
	}

	if state.GetAvailableURLCount() <= 0 {
		return fmt.Errorf("no upload slots available; start a new session with /new or dx ask --new")
	}

	startIdx := state.URLsUsed
	if startIdx > len(state.PresignedURLs) {
		startIdx = len(state.PresignedURLs)
	}
	uploader := upload.NewS3Uploader(state.PresignedURLs[startIdx:])

	if err := uploader.UploadFile(path); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	client := api.NewClient(cfg)
	uploadedKeys := uploader.UploadedKeys()

	resp, err := client.AttachFiles(state.SessionID, uploadedKeys)
	if err != nil {
		return fmt.Errorf("could not attach file to session: %w", err)
	}
	if resp.Success {
		fileList := uploader.FormatFileNamesForNotification()
		notifyMsg := fmt.Sprintf(
			"[System] 1 file uploaded to this session:\n\n%s\n"+
				"This file is now available in the workspace for analysis when needed.",
			fileList,
		)
		if err := client.SendMessage(state.SessionID, notifyMsg, "", ""); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not notify about uploaded file: %v\n", err)
		}
	}

	state.URLsUsed++
	_ = session.Save(state)

	return nil
}

// isBinary returns true if data contains a null byte in the first 512 bytes.
func isBinary(data []byte) bool {
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	return bytes.ContainsRune(check, 0)
}
