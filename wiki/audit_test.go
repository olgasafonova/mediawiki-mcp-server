package wiki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHashContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // First few chars of SHA-256 hash
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "e3b0c442", // SHA-256 of empty string
		},
		{
			name:     "simple text",
			input:    "Hello, World!",
			expected: "dffd6021", // SHA-256 of "Hello, World!"
		},
		{
			name:     "same input same hash",
			input:    "test content",
			expected: "6ae8a75555",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashContent(tt.input)
			if !strings.HasPrefix(result, tt.expected) {
				t.Errorf("hashContent(%q) = %s, want prefix %s", tt.input, result, tt.expected)
			}
			// Verify consistent hashing
			result2 := hashContent(tt.input)
			if result != result2 {
				t.Errorf("hashContent not deterministic: got %s and %s", result, result2)
			}
		})
	}
}

func TestJSONAuditLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	auditLogger := NewWriterAuditLogger(&buf, logger)

	entry := AuditEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Operation:   AuditOpEdit,
		Title:       "Test Page",
		PageID:      123,
		RevisionID:  456,
		ContentHash: "abc123",
		ContentSize: 100,
		Summary:     "Test edit",
		Minor:       true,
		Bot:         false,
		WikiURL:     "https://wiki.example.com/api.php",
		Success:     true,
	}

	auditLogger.Log(entry)

	// Verify JSON was written
	output := buf.String()
	if output == "" {
		t.Fatal("Expected output, got empty string")
	}

	// Parse back to verify structure
	var parsed AuditEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v\nOutput: %s", err, output)
	}

	if parsed.Title != entry.Title {
		t.Errorf("Title = %q, want %q", parsed.Title, entry.Title)
	}
	if parsed.Operation != entry.Operation {
		t.Errorf("Operation = %q, want %q", parsed.Operation, entry.Operation)
	}
	if parsed.PageID != entry.PageID {
		t.Errorf("PageID = %d, want %d", parsed.PageID, entry.PageID)
	}
	if parsed.Success != entry.Success {
		t.Errorf("Success = %v, want %v", parsed.Success, entry.Success)
	}
}

func TestJSONAuditLoggerMultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	auditLogger := NewWriterAuditLogger(&buf, logger)

	entries := []AuditEntry{
		{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Operation: AuditOpEdit,
			Title:     "Page 1",
			Success:   true,
		},
		{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Operation: AuditOpCreate,
			Title:     "Page 2",
			Success:   true,
		},
		{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Operation: AuditOpUpload,
			Title:     "File:Image.png",
			Success:   false,
			Error:     "Upload failed",
		},
	}

	for _, e := range entries {
		auditLogger.Log(e)
	}

	// Verify we got 3 JSON lines
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i+1, err)
		}
	}
}

func TestNullAuditLogger(t *testing.T) {
	logger := NullAuditLogger{}

	// Should not panic
	logger.Log(AuditEntry{Title: "Test"})

	// Close should return nil
	if err := logger.Close(); err != nil {
		t.Errorf("NullAuditLogger.Close() = %v, want nil", err)
	}
}

func TestAuditOperationConstants(t *testing.T) {
	// Verify operation constants are distinct
	ops := []AuditOperation{AuditOpEdit, AuditOpCreate, AuditOpUpload}
	seen := make(map[AuditOperation]bool)

	for _, op := range ops {
		if seen[op] {
			t.Errorf("Duplicate operation constant: %s", op)
		}
		seen[op] = true

		// Verify they're valid non-empty strings
		if string(op) == "" {
			t.Error("Operation constant is empty string")
		}
	}
}

func TestFileAuditLogger(t *testing.T) {
	// Create a temp file
	tmpFile, err := os.CreateTemp("", "audit_test_*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	auditLogger, err := NewFileAuditLogger(tmpPath, logger)
	if err != nil {
		t.Fatalf("NewFileAuditLogger failed: %v", err)
	}

	entry := AuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Operation: AuditOpEdit,
		Title:     "File Test Page",
		Success:   true,
	}

	auditLogger.Log(entry)

	// Close and verify file was written
	if err := auditLogger.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Read file back
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	if len(content) == 0 {
		t.Error("File is empty")
	}

	// Verify it's valid JSON
	var parsed AuditEntry
	if err := json.Unmarshal(bytes.TrimSpace(content), &parsed); err != nil {
		t.Errorf("File content is not valid JSON: %v", err)
	}

	if parsed.Title != entry.Title {
		t.Errorf("Title = %q, want %q", parsed.Title, entry.Title)
	}
}

func TestAuditEntryJSONFormat(t *testing.T) {
	entry := AuditEntry{
		Timestamp:   "2024-01-15T10:30:00Z",
		Operation:   AuditOpEdit,
		Title:       "Test Page",
		PageID:      123,
		RevisionID:  456,
		ContentHash: "abc123def456",
		ContentSize: 1024,
		Summary:     "Fixed typo",
		Minor:       true,
		Bot:         true,
		WikiURL:     "https://wiki.example.com/api.php",
		Success:     true,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify required fields are present in JSON
	jsonStr := string(data)
	requiredFields := []string{
		`"timestamp"`,
		`"operation"`,
		`"title"`,
		`"content_hash"`,
		`"wiki_url"`,
		`"success"`,
	}

	for _, field := range requiredFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("JSON missing field %s: %s", field, jsonStr)
		}
	}

	// Verify omitempty works for zero values
	entryMinimal := AuditEntry{
		Timestamp:   "2024-01-15T10:30:00Z",
		Operation:   AuditOpEdit,
		Title:       "Test",
		ContentHash: "abc",
		WikiURL:     "https://example.com",
		Success:     true,
	}

	dataMinimal, _ := json.Marshal(entryMinimal)
	minimalStr := string(dataMinimal)

	// These should be omitted when zero
	omittedFields := []string{`"page_id"`, `"revision_id"`, `"error"`}
	for _, field := range omittedFields {
		if strings.Contains(minimalStr, field) {
			t.Errorf("JSON should omit zero-value field %s: %s", field, minimalStr)
		}
	}
}

func TestNullAuditLogger_Log(t *testing.T) {
	logger := NullAuditLogger{}

	// Should not panic and should do nothing
	entry := AuditEntry{
		Timestamp:   "2024-01-15T10:30:00Z",
		Operation:   AuditOpEdit,
		Title:       "Test Page",
		ContentHash: "abc123",
		WikiURL:     "https://example.com",
		Success:     true,
	}

	// This should be a no-op
	logger.Log(entry)

	// Test Close as well
	err := logger.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestJSONAuditLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger := NewWriterAuditLogger(&buf, logger)

	entry := AuditEntry{
		Timestamp:   "2024-01-15T10:30:00Z",
		Operation:   AuditOpEdit,
		Title:       "Test Page",
		ContentHash: "abc123",
		WikiURL:     "https://example.com",
		Success:     true,
	}

	auditLogger.Log(entry)

	output := buf.String()
	if !strings.Contains(output, "Test Page") {
		t.Errorf("Expected output to contain 'Test Page', got: %s", output)
	}
	if !strings.Contains(output, "abc123") {
		t.Errorf("Expected output to contain 'abc123', got: %s", output)
	}
}

func TestJSONAuditLogger_Close_WithFile(t *testing.T) {
	// Create a temp file
	tmpFile, err := os.CreateTemp("", "audit_test_*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger, err := NewFileAuditLogger(tmpPath, logger)
	if err != nil {
		t.Fatalf("Failed to create file audit logger: %v", err)
	}

	entry := AuditEntry{
		Timestamp: "2024-01-15T10:30:00Z",
		Operation: AuditOpEdit,
		Title:     "Test Page",
		Success:   true,
	}
	auditLogger.Log(entry)

	err = auditLogger.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestClientLogAudit(t *testing.T) {
	// Test logAudit with a configured audit logger
	var buf bytes.Buffer
	slogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger := NewWriterAuditLogger(&buf, slogger)

	client := &Client{
		auditLogger: auditLogger,
	}

	entry := AuditEntry{
		Timestamp:   "2024-01-15T10:30:00Z",
		Operation:   AuditOpEdit,
		Title:       "Test Page",
		ContentHash: "abc123",
		WikiURL:     "https://example.com",
		Success:     true,
	}

	client.logAudit(entry)

	output := buf.String()
	if !strings.Contains(output, "Test Page") {
		t.Errorf("Expected audit log to contain 'Test Page', got: %s", output)
	}
}

func TestClientLogAudit_NoLogger(t *testing.T) {
	// Test logAudit without an audit logger (should not panic)
	client := &Client{
		auditLogger: nil,
	}

	entry := AuditEntry{
		Timestamp: "2024-01-15T10:30:00Z",
		Operation: AuditOpEdit,
		Title:     "Test Page",
		Success:   true,
	}

	// Should not panic
	client.logAudit(entry)
}

func TestBuildAuditEntry(t *testing.T) {
	client := &Client{
		config: &Config{
			BaseURL: "https://wiki.example.com/api.php",
		},
	}

	entry := client.buildAuditEntry(
		AuditOpEdit,
		"Test Page",
		"Content here",
		"Edit summary",
		true,  // minor
		false, // bot
		true,  // success
		123,   // pageID
		456,   // revisionID
		"",    // errMsg
	)

	if entry.Title != "Test Page" {
		t.Errorf("Title = %q, want 'Test Page'", entry.Title)
	}
	if entry.Operation != AuditOpEdit {
		t.Errorf("Operation = %q, want %q", entry.Operation, AuditOpEdit)
	}
	if entry.PageID != 123 {
		t.Errorf("PageID = %d, want 123", entry.PageID)
	}
	if entry.Minor != true {
		t.Error("Expected Minor = true")
	}
	if entry.Success != true {
		t.Error("Expected Success = true")
	}
	if entry.WikiURL != "https://wiki.example.com/api.php" {
		t.Errorf("WikiURL = %q, want 'https://wiki.example.com/api.php'", entry.WikiURL)
	}
	if entry.ContentHash == "" {
		t.Error("Expected non-empty ContentHash")
	}
}

func TestBuildAuditEntry_WithError(t *testing.T) {
	client := &Client{
		config: &Config{
			BaseURL: "https://wiki.example.com/api.php",
		},
	}

	entry := client.buildAuditEntry(
		AuditOpUpload,
		"File:Test.png",
		"",
		"Upload test",
		false, // minor
		true,  // bot
		false, // success
		0,     // pageID
		0,     // revisionID
		"Upload failed: file too large",
	)

	if entry.Operation != AuditOpUpload {
		t.Errorf("Operation = %q, want %q", entry.Operation, AuditOpUpload)
	}
	if entry.Success != false {
		t.Error("Expected Success = false")
	}
	if entry.Error != "Upload failed: file too large" {
		t.Errorf("Error = %q, want 'Upload failed: file too large'", entry.Error)
	}
	if entry.Bot != true {
		t.Error("Expected Bot = true")
	}
}

// failingWriter is a writer that always fails
type failingWriter struct{}

func (f failingWriter) Write(_ []byte) (n int, err error) {
	return 0, fmt.Errorf("intentional write failure")
}

func TestJSONAuditLogger_WriteError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger := NewWriterAuditLogger(failingWriter{}, logger)

	entry := AuditEntry{
		Timestamp:   "2024-01-15T10:30:00Z",
		Operation:   AuditOpEdit,
		Title:       "Test Page",
		ContentHash: "abc123",
		WikiURL:     "https://example.com",
		Success:     true,
	}

	// Should not panic, just log the error
	auditLogger.Log(entry)
}

func TestNullAuditLogger_Complete(t *testing.T) {
	logger := NullAuditLogger{}

	// Create a fully populated entry
	entry := AuditEntry{
		Timestamp:   "2024-01-15T10:30:00Z",
		Operation:   AuditOpCreate,
		Title:       "Complete Test Page",
		PageID:      999,
		RevisionID:  888,
		ContentHash: "fullhash123",
		ContentSize: 2048,
		Summary:     "Complete test",
		Minor:       true,
		Bot:         true,
		WikiURL:     "https://complete.example.com",
		Success:     true,
		Error:       "",
	}

	// Log should be no-op
	logger.Log(entry)

	// Multiple logs should still work
	logger.Log(entry)
	logger.Log(entry)

	// Close should return nil
	err := logger.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}
