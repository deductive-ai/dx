// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package upload

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/deductive-ai/dx/internal/api"
)

// UploadedFile represents an uploaded file with its metadata
type UploadedFile struct {
	FileName string // Original file name
	Key      string // S3 key
	S3URL    string // S3 URL (derived from upload URL)
}

// S3Uploader handles uploading files to S3 using presigned URLs
type S3Uploader struct {
	urls          []api.PresignedURL
	urlIndex      int
	uploadedKeys  []string
	uploadedFiles []UploadedFile
	httpClient    *http.Client
}

// NewS3Uploader creates a new S3 uploader with the given presigned URLs
func NewS3Uploader(urls []api.PresignedURL) *S3Uploader {
	return &S3Uploader{
		urls:          urls,
		urlIndex:      0,
		uploadedKeys:  make([]string, 0),
		uploadedFiles: make([]UploadedFile, 0),
		httpClient:    &http.Client{},
	}
}

// getNextURL returns the next available presigned URL
func (u *S3Uploader) getNextURL() (*api.PresignedURL, error) {
	if u.urlIndex >= len(u.urls) {
		return nil, fmt.Errorf("no more presigned URLs available (used %d of %d)", u.urlIndex, len(u.urls))
	}
	url := &u.urls[u.urlIndex]
	u.urlIndex++
	return url, nil
}

// UploadFile uploads a single file to S3
func (u *S3Uploader) UploadFile(filePath string) error {
	presignedURL, err := u.getNextURL()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	req, err := http.NewRequest("PUT", presignedURL.UploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return parseS3Error(resp.StatusCode, body, presignedURL.UploadURL)
	}

	// Extract base S3 URL from the presigned upload URL
	s3URL := extractS3BaseURL(presignedURL.UploadURL)

	u.uploadedKeys = append(u.uploadedKeys, presignedURL.Key)
	u.uploadedFiles = append(u.uploadedFiles, UploadedFile{
		FileName: filepath.Base(filePath),
		Key:      presignedURL.Key,
		S3URL:    s3URL,
	})
	return nil
}

// extractS3BaseURL removes query parameters from presigned URL to get base S3 URL
func extractS3BaseURL(presignedURL string) string {
	parsedURL, err := url.Parse(presignedURL)
	if err != nil {
		return presignedURL
	}
	// Remove query parameters (presigned signature)
	parsedURL.RawQuery = ""
	return parsedURL.String()
}

// FormatWorkspacePathsForMessage returns a formatted string of uploaded files and their workspace paths
// The paths are relative to the inference workspace directory
func (u *S3Uploader) FormatWorkspacePathsForMessage() string {
	if len(u.uploadedFiles) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, f := range u.uploadedFiles {
		// Extract workspace path from S3 key
		// Key format: {teamId}/thread_{sessionId}/uploads/file_{index}
		// Workspace path: uploads/file_{index}
		workspacePath := extractWorkspacePath(f.Key)
		sb.WriteString(fmt.Sprintf("- %s: %s\n", f.FileName, workspacePath))
	}
	return sb.String()
}

// FormatFileNamesForNotification returns a formatted list of uploaded files
// with their original names and workspace paths, suitable for a short notification.
func (u *S3Uploader) FormatFileNamesForNotification() string {
	if len(u.uploadedFiles) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, f := range u.uploadedFiles {
		workspacePath := extractWorkspacePath(f.Key)
		sb.WriteString(fmt.Sprintf("- %s (path: %s)\n", f.FileName, workspacePath))
	}
	return sb.String()
}

// extractWorkspacePath extracts the workspace-relative path from an S3 key
// S3 key format: {teamId}/thread_{sessionId}/uploads/file_{index}
// Returns: uploads/file_{index}
func extractWorkspacePath(key string) string {
	// Find "uploads/" in the key and return from there
	idx := strings.Index(key, "uploads/")
	if idx >= 0 {
		return key[idx:]
	}
	// Fallback: return the filename part
	parts := strings.Split(key, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return key
}

// UploadBytes uploads raw bytes to S3 with the given filename.
func (u *S3Uploader) UploadBytes(data []byte, fileName string) error {
	presignedURL, err := u.getNextURL()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", presignedURL.UploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return parseS3Error(resp.StatusCode, body, presignedURL.UploadURL)
	}

	s3URL := extractS3BaseURL(presignedURL.UploadURL)

	u.uploadedKeys = append(u.uploadedKeys, presignedURL.Key)
	u.uploadedFiles = append(u.uploadedFiles, UploadedFile{
		FileName: fileName,
		Key:      presignedURL.Key,
		S3URL:    s3URL,
	})
	return nil
}

// UploadDirectory recursively uploads all files in a directory
func (u *S3Uploader) UploadDirectory(dirPath string) (int, error) {
	count := 0

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if we have URLs available
		if u.urlIndex >= len(u.urls) {
			return fmt.Errorf("not enough presigned URLs for all files. Uploaded %d files before running out", count)
		}

		if err := u.UploadFile(path); err != nil {
			return fmt.Errorf("failed to upload %s: %w", path, err)
		}

		count++
		return nil
	})

	return count, err
}

// UploadedKeys returns the S3 keys of all uploaded files
func (u *S3Uploader) UploadedKeys() []string {
	return u.uploadedKeys
}

// UploadedFiles returns information about all uploaded files
func (u *S3Uploader) UploadedFiles() []UploadedFile {
	return u.uploadedFiles
}

// Count returns the number of files uploaded
func (u *S3Uploader) Count() int {
	return len(u.uploadedKeys)
}

// Available returns the number of presigned URLs still available
func (u *S3Uploader) Available() int {
	return len(u.urls) - u.urlIndex
}

// parseS3Error extracts meaningful error information from S3 error responses
func parseS3Error(statusCode int, body []byte, uploadURL string) error {
	bodyStr := string(body)

	// Parse common S3 error codes from XML response
	if strings.Contains(bodyStr, "<Code>NoSuchBucket</Code>") {
		return fmt.Errorf("upload bucket does not exist. This is a server configuration issue - please contact support")
	}
	if strings.Contains(bodyStr, "<Code>AccessDenied</Code>") {
		return fmt.Errorf("upload access denied. The presigned URL may have expired - try starting a new session with 'dx ask --new'")
	}
	if strings.Contains(bodyStr, "<Code>ExpiredToken</Code>") || strings.Contains(bodyStr, "<Code>RequestExpired</Code>") {
		return fmt.Errorf("upload URL expired. Start a new session with 'dx ask --new' to get fresh upload URLs")
	}
	if strings.Contains(bodyStr, "<Code>InvalidAccessKeyId</Code>") {
		return fmt.Errorf("upload credentials invalid. This is a server configuration issue - please contact support")
	}
	if strings.Contains(bodyStr, "<Code>SignatureDoesNotMatch</Code>") {
		return fmt.Errorf("upload signature mismatch. This may be a network or proxy issue - check your connection")
	}
	if strings.Contains(bodyStr, "<Code>EntityTooLarge</Code>") {
		return fmt.Errorf("file too large for upload. Try splitting into smaller files")
	}

	// Generic status code messages
	switch statusCode {
	case 403:
		return fmt.Errorf("upload forbidden (403). The presigned URL may have expired - try 'dx ask --new'")
	case 404:
		return fmt.Errorf("upload destination not found (404). This is a server configuration issue - please contact support")
	case 500, 502, 503, 504:
		return fmt.Errorf("upload service unavailable (%d). Please try again later", statusCode)
	}

	// Fallback to generic error with body content
	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200] + "..."
	}
	return fmt.Errorf("upload failed with status %d: %s", statusCode, bodyStr)
}
