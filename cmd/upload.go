package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
)

// uploadFileToSession uploads a single text file to the active session.
// It requests a presigned URL on demand, uploads the file, then notifies the server.
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

	client := api.NewClient(cfg)

	urlResp, err := client.RequestUploadURL(state.SessionID, filepath.Base(path))
	if err != nil {
		return fmt.Errorf("failed to get upload URL: %w", err)
	}

	if err := putToS3(urlResp.UploadURL, data); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	resp, err := client.AttachFiles(state.SessionID, []string{urlResp.Key})
	if err != nil {
		return fmt.Errorf("could not attach file to session: %w", err)
	}
	if resp.Success {
		notifyMsg := fmt.Sprintf(
			"[System] 1 file uploaded to this session:\n\n- %s (path: uploads/%s)\n"+
				"This file is now available in the workspace for analysis when needed.",
			filepath.Base(path), filepath.Base(path),
		)
		if err := client.SendMessage(state.SessionID, notifyMsg, "", ""); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not notify about uploaded file: %v\n", err)
		}
	}

	return nil
}

// putToS3 uploads raw bytes to the given presigned PUT URL.
func putToS3(uploadURL string, data []byte) error {
	req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("S3 returned status %d", resp.StatusCode)
	}
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
