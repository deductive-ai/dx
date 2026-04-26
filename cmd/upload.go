// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/deductive-ai/dx/internal/telemetry"
	"github.com/deductive-ai/dx/internal/upload"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
)

var (
	uploadFileFlag      string
	uploadRecursiveFlag bool
	uploadStdinFlag     bool
	uploadNameFlag      string
)

var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload files to the current session",
	Long: `Upload files or directories to the current Deductive AI session.

Files are uploaded to S3 and attached to the session for analysis.
A session must exist first — run 'dx ask' to start one.

Examples:
  # Upload a single file
  dx upload -f /tmp/app.log

  # Upload a directory recursively
  dx upload -f /var/log/myapp -r

  # Upload from stdin
  cat /tmp/app.log | dx upload --stdin

  # Upload from stdin with a custom name
  cat /tmp/app.log | dx upload --stdin --name=app.log

  # Upload using a specific profile
  dx upload -f /tmp/app.log --profile=staging`,
	Example: `  # Upload a log file
  dx upload -f /tmp/app.log

  # Upload a directory
  dx upload -f /var/log/myapp -r

  # Pipe from stdin
  kubectl logs deploy/api | dx upload --stdin --name=api.log`,
	Hidden: true,
	Run:    runUpload,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if !uploadStdinFlag && uploadFileFlag == "" {
			return fmt.Errorf("required flag \"file\" not set (use -f or --stdin)")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uploadCmd)
	uploadCmd.Flags().StringVarP(&uploadFileFlag, "file", "f", "", "File or directory to upload")
	uploadCmd.Flags().BoolVarP(&uploadRecursiveFlag, "recursive", "r", false, "Upload directory contents recursively")
	uploadCmd.Flags().BoolVar(&uploadStdinFlag, "stdin", false, "Read data from stdin")
	uploadCmd.Flags().StringVar(&uploadNameFlag, "name", "", "Custom filename for stdin upload (default: stdin.txt)")
}

func runUpload(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	_, span := telemetry.StartSpan(context.Background(), "dx.upload",
		attribute.String("profile", profile),
	)
	defer span.End()

	// Check config and auth (env vars → config file → interactive bootstrap)
	cfg := LoadOrBootstrap(profile)

	var err error
	cfg, err = EnsureAuth(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check for active session
	state, err := session.LoadCurrent(profile)
	if err != nil || state == nil {
		fmt.Fprintln(os.Stderr, "Error: No active session. Run 'dx ask' first.")
		os.Exit(1)
	}

	// Handle stdin upload
	if uploadStdinFlag {
		if state.GetAvailableURLCount() <= 0 {
			fmt.Fprintln(os.Stderr, "Error: No upload slots available. Start a new session with 'dx ask --new'.")
			os.Exit(1)
		}

		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		if len(data) == 0 {
			fmt.Fprintln(os.Stderr, "Error: No data received from stdin")
			os.Exit(1)
		}

		stdinName := "stdin.txt"
		if uploadNameFlag != "" {
			stdinName = uploadNameFlag
		}
		fmt.Printf("Uploading %s (%s)...\n", stdinName, formatByteSize(len(data)))

		startIdx := state.URLsUsed
		if startIdx > len(state.PresignedURLs) {
			startIdx = len(state.PresignedURLs)
		}
		uploader := upload.NewS3Uploader(state.PresignedURLs[startIdx:])
		if err := uploader.UploadBytes(data, stdinName); err != nil {
			fmt.Fprintf(os.Stderr, "Error uploading stdin data: %v\n", err)
			os.Exit(1)
		}

		client := api.NewClient(cfg)
		uploadedKeys := uploader.UploadedKeys()

		resp, err := client.AttachFiles(state.SessionID, uploadedKeys)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not attach files to session: %v\n", err)
		} else if resp.Success {
			fmt.Printf("✓ Attached %d file(s) to session\n", len(resp.AttachedFiles))

			fileList := uploader.FormatFileNamesForNotification()
			notifyMsg := fmt.Sprintf(
				"[System] 1 file uploaded to this session (from stdin):\n\n%s\n"+
					"This file is now available in the workspace for analysis when needed.",
				fileList,
			)
			if err := client.SendMessage(state.SessionID, notifyMsg, "", ""); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Could not notify about uploaded file: %v\n", err)
			}
		}

		state.URLsUsed++
		if err := session.Save(state); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not update session state: %v\n", err)
		}

		fmt.Println()
		fmt.Printf("✓ Uploaded 1 file(s)\n")
		fmt.Printf("  Upload slots remaining: %d\n", state.GetAvailableURLCount())
		return
	}

	logging.Debug("Upload started", "file", uploadFileFlag, "stdin", uploadStdinFlag, "recursive", uploadRecursiveFlag)

	// Validate file/directory exists
	path := uploadFileFlag
	fileInfo, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot access %s: %v\n", path, err)
		os.Exit(1)
	}

	// Validate directory requires -r flag (check before slot count)
	if fileInfo.IsDir() && !uploadRecursiveFlag {
		fmt.Fprintln(os.Stderr, "Error: Path is a directory. Use -r flag to upload recursively.")
		os.Exit(1)
	}

	// Check if we have enough presigned URLs
	availableURLs := state.GetAvailableURLCount()
	if availableURLs <= 0 {
		fmt.Fprintln(os.Stderr, "Error: No upload slots available. Start a new session with 'dx ask --new'.")
		os.Exit(1)
	}

	// Create uploader (bounds-check URLsUsed to prevent slice panic)
	startIdx := state.URLsUsed
	if startIdx > len(state.PresignedURLs) {
		startIdx = len(state.PresignedURLs)
	}
	uploader := upload.NewS3Uploader(state.PresignedURLs[startIdx:])

	var uploadCount int

	if fileInfo.IsDir() {

		// Count files and total size
		fileCount := 0
		var totalSize int64
		_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				fileCount++
				totalSize += info.Size()
			}
			return nil
		})

		if fileCount > availableURLs {
			fmt.Fprintf(os.Stderr, "Warning: Directory contains %d files but only %d upload slots available.\n", fileCount, availableURLs)
			fmt.Fprintln(os.Stderr, "Only the first files will be uploaded.")
		}

		fmt.Printf("Uploading directory %s (%d files, %s)...\n", path, fileCount, formatByteSize(int(totalSize)))

		count, err := uploader.UploadDirectory(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error uploading directory: %v\n", err)
		}
		uploadCount = count

	} else {
		fmt.Printf("Uploading %s (%s)...\n", path, formatByteSize(int(fileInfo.Size())))

		if err := uploader.UploadFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error uploading file: %v\n", err)
			os.Exit(1)
		}
		uploadCount = 1
	}

	// Attach uploaded files to the session and update slot count
	if uploadCount > 0 {
		client := api.NewClient(cfg)
		uploadedKeys := uploader.UploadedKeys()

		resp, err := client.AttachFiles(state.SessionID, uploadedKeys)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not attach files to session: %v\n", err)
		} else if resp.Success {
			fmt.Printf("✓ Attached %d file(s) to session\n", len(resp.AttachedFiles))

			fileList := uploader.FormatFileNamesForNotification()
			notifyMsg := fmt.Sprintf(
				"[System] %d file(s) uploaded to this session:\n\n%s\n"+
					"These files are now available in the workspace for analysis when needed.",
				uploadCount,
				fileList,
			)

			if err := client.SendMessage(state.SessionID, notifyMsg, "", ""); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Could not notify about uploaded files: %v\n", err)
			}
		}

		// Only consume slots after successful S3 upload (attach failure is non-fatal)
		state.URLsUsed += uploadCount
		if err := session.Save(state); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not update session state: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Printf("✓ Uploaded %d file(s)\n", uploadCount)
	fmt.Printf("  Upload slots remaining: %d\n", state.GetAvailableURLCount())

	if state.GetAvailableURLCount() == 0 {
		fmt.Println()
		fmt.Println("No more upload slots. Start a new session with 'dx ask --new'.")
	}
}
