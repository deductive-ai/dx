package upload

import (
	"testing"

	"github.com/deductive-ai/dx/internal/api"
)

func TestExtractS3BaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			"removes query params",
			"https://bucket.s3.amazonaws.com/key/path?X-Amz-Algorithm=AWS4&X-Amz-Signature=abc123",
			"https://bucket.s3.amazonaws.com/key/path",
		},
		{
			"no query params",
			"https://bucket.s3.amazonaws.com/key/path",
			"https://bucket.s3.amazonaws.com/key/path",
		},
		{
			"complex path",
			"https://bucket.s3.us-west-2.amazonaws.com/team1/thread_abc/uploads/file_0?sig=xyz&expires=123",
			"https://bucket.s3.us-west-2.amazonaws.com/team1/thread_abc/uploads/file_0",
		},
		{
			"localhost url",
			"http://localhost:4566/bucket/key?query=1",
			"http://localhost:4566/bucket/key",
		},
		{
			"invalid url returns as-is",
			"://not-valid",
			"://not-valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractS3BaseURL(tt.url)
			if got != tt.want {
				t.Errorf("extractS3BaseURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExtractWorkspacePath(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			"standard key format",
			"team1/thread_sess123/uploads/file_0",
			"uploads/file_0",
		},
		{
			"nested uploads path",
			"deep/nested/path/uploads/subdir/file.txt",
			"uploads/subdir/file.txt",
		},
		{
			"no uploads prefix - returns last segment",
			"team1/thread_sess/other/file.txt",
			"file.txt",
		},
		{
			"single segment",
			"standalone-file",
			"standalone-file",
		},
		{
			"empty key",
			"",
			"",
		},
		{
			"just uploads/",
			"uploads/myfile",
			"uploads/myfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractWorkspacePath(tt.key)
			if got != tt.want {
				t.Errorf("extractWorkspacePath(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestParseS3Error(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantSubstr string
	}{
		{"NoSuchBucket", 404, "<Code>NoSuchBucket</Code>", "bucket does not exist"},
		{"AccessDenied", 403, "<Code>AccessDenied</Code>", "access denied"},
		{"ExpiredToken", 403, "<Code>ExpiredToken</Code>", "expired"},
		{"RequestExpired", 403, "<Code>RequestExpired</Code>", "expired"},
		{"InvalidAccessKeyId", 403, "<Code>InvalidAccessKeyId</Code>", "credentials invalid"},
		{"SignatureDoesNotMatch", 403, "<Code>SignatureDoesNotMatch</Code>", "signature mismatch"},
		{"EntityTooLarge", 413, "<Code>EntityTooLarge</Code>", "too large"},
		{"generic 403", 403, "some other body", "forbidden (403)"},
		{"generic 404", 404, "not found body", "not found (404)"},
		{"server error 500", 500, "", "unavailable (500)"},
		{"server error 503", 503, "", "unavailable (503)"},
		{"unknown status", 418, "I'm a teapot", "status 418"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseS3Error(tt.statusCode, []byte(tt.body), "https://example.com/upload")
			if err == nil {
				t.Fatal("parseS3Error() should return error")
			}
			errMsg := err.Error()
			if !containsSubstring(errMsg, tt.wantSubstr) {
				t.Errorf("parseS3Error() = %q, want substring %q", errMsg, tt.wantSubstr)
			}
		})
	}
}

func TestParseS3Error_TruncatesLongBody(t *testing.T) {
	longBody := make([]byte, 300)
	for i := range longBody {
		longBody[i] = 'x'
	}

	err := parseS3Error(400, longBody, "https://example.com/upload")
	errMsg := err.Error()
	if len(errMsg) > 300 {
		// The error message includes prefix text, but the body portion should be truncated
		if !containsSubstring(errMsg, "...") {
			t.Error("long body should be truncated with ...")
		}
	}
}

func TestNewS3Uploader(t *testing.T) {
	urls := []api.PresignedURL{
		{UploadURL: "url1", Key: "key1"},
		{UploadURL: "url2", Key: "key2"},
		{UploadURL: "url3", Key: "key3"},
	}

	uploader := NewS3Uploader(urls)
	if uploader == nil {
		t.Fatal("NewS3Uploader() returned nil")
	}
	if uploader.Available() != 3 {
		t.Errorf("Available() = %d, want 3", uploader.Available())
	}
	if uploader.Count() != 0 {
		t.Errorf("Count() = %d, want 0", uploader.Count())
	}
}

func TestS3Uploader_Available(t *testing.T) {
	urls := []api.PresignedURL{
		{UploadURL: "url1", Key: "key1"},
		{UploadURL: "url2", Key: "key2"},
	}
	uploader := NewS3Uploader(urls)

	if uploader.Available() != 2 {
		t.Errorf("Available() = %d, want 2", uploader.Available())
	}

	// Simulate using one URL internally
	uploader.urlIndex = 1
	if uploader.Available() != 1 {
		t.Errorf("Available() = %d, want 1", uploader.Available())
	}

	uploader.urlIndex = 2
	if uploader.Available() != 0 {
		t.Errorf("Available() = %d, want 0", uploader.Available())
	}
}

func TestS3Uploader_EmptyURLs(t *testing.T) {
	uploader := NewS3Uploader([]api.PresignedURL{})
	if uploader.Available() != 0 {
		t.Errorf("Available() = %d, want 0", uploader.Available())
	}
}

func TestFormatFileNamesForNotification(t *testing.T) {
	uploader := &S3Uploader{
		uploadedFiles: []UploadedFile{
			{FileName: "report.csv", Key: "team1/thread_abc/uploads/file_0"},
			{FileName: "data.json", Key: "team1/thread_abc/uploads/file_1"},
		},
	}

	got := uploader.FormatFileNamesForNotification()
	if !containsSubstring(got, "report.csv") {
		t.Error("should contain report.csv")
	}
	if !containsSubstring(got, "data.json") {
		t.Error("should contain data.json")
	}
	if !containsSubstring(got, "uploads/file_0") {
		t.Error("should contain workspace path for first file")
	}
}

func TestFormatFileNamesForNotification_Empty(t *testing.T) {
	uploader := &S3Uploader{uploadedFiles: []UploadedFile{}}
	got := uploader.FormatFileNamesForNotification()
	if got != "" {
		t.Errorf("FormatFileNamesForNotification() = %q, want empty string", got)
	}
}

func TestFormatWorkspacePathsForMessage(t *testing.T) {
	uploader := &S3Uploader{
		uploadedFiles: []UploadedFile{
			{FileName: "log.txt", Key: "team1/thread_abc/uploads/file_0"},
		},
	}

	got := uploader.FormatWorkspacePathsForMessage()
	if !containsSubstring(got, "log.txt") {
		t.Error("should contain filename")
	}
	if !containsSubstring(got, "uploads/file_0") {
		t.Error("should contain workspace path")
	}
}

func TestFormatWorkspacePathsForMessage_Empty(t *testing.T) {
	uploader := &S3Uploader{uploadedFiles: []UploadedFile{}}
	got := uploader.FormatWorkspacePathsForMessage()
	if got != "" {
		t.Errorf("FormatWorkspacePathsForMessage() = %q, want empty string", got)
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
