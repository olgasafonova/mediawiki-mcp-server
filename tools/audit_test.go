package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

func TestJSONToolAuditLogger_WritesValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	auditLogger := NewWriterToolAuditLogger(&buf, logger)

	entry := ToolCallEntry{
		Type:       "tool_call",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Tool:       "mediawiki_search",
		Method:     "Search",
		Category:   "search",
		DurationMs: 42,
		Success:    true,
		ReadOnly:   true,
		Args:       "query=onboarding",
	}

	auditLogger.Log(entry)

	// Verify valid JSON
	var decoded ToolCallEntry
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Failed to unmarshal audit entry: %v", err)
	}

	if decoded.Type != "tool_call" {
		t.Errorf("Type = %q, want %q", decoded.Type, "tool_call")
	}
	if decoded.Tool != "mediawiki_search" {
		t.Errorf("Tool = %q, want %q", decoded.Tool, "mediawiki_search")
	}
	if decoded.DurationMs != 42 {
		t.Errorf("DurationMs = %d, want 42", decoded.DurationMs)
	}
	if decoded.Args != "query=onboarding" {
		t.Errorf("Args = %q, want %q", decoded.Args, "query=onboarding")
	}
}

func TestJSONToolAuditLogger_ErrorEntry(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	auditLogger := NewWriterToolAuditLogger(&buf, logger)

	entry := ToolCallEntry{
		Type:       "tool_call",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Tool:       "mediawiki_edit_page",
		Method:     "EditPage",
		Category:   "write",
		DurationMs: 150,
		Success:    false,
		Error:      "authentication required",
		ReadOnly:   false,
		Args:       "title=Test Page",
	}

	auditLogger.Log(entry)

	var decoded ToolCallEntry
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Success {
		t.Error("Success should be false for error entry")
	}
	if decoded.Error != "authentication required" {
		t.Errorf("Error = %q, want %q", decoded.Error, "authentication required")
	}
}

func TestJSONToolAuditLogger_MultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	auditLogger := NewWriterToolAuditLogger(&buf, logger)

	for i := 0; i < 3; i++ {
		auditLogger.Log(ToolCallEntry{
			Type:       "tool_call",
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Tool:       "mediawiki_search",
			Method:     "Search",
			Category:   "search",
			DurationMs: int64(i * 10),
			Success:    true,
			ReadOnly:   true,
		})
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var entry ToolCallEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("Line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestJSONToolAuditLogger_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	auditLogger := NewWriterToolAuditLogger(&buf, logger)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			auditLogger.Log(ToolCallEntry{
				Type:       "tool_call",
				Timestamp:  time.Now().UTC().Format(time.RFC3339),
				Tool:       "mediawiki_search",
				Method:     "Search",
				Category:   "search",
				DurationMs: 1,
				Success:    true,
				ReadOnly:   true,
			})
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 20 {
		t.Fatalf("Expected 20 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var entry ToolCallEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("Line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestNullToolAuditLogger(t *testing.T) {
	logger := NullToolAuditLogger{}
	// Should not panic
	logger.Log(ToolCallEntry{Type: "tool_call", Tool: "test"})
	if err := logger.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestExtractArgsSummary(t *testing.T) {
	tests := []struct {
		name string
		args any
		want string
	}{
		{
			name: "SearchArgs",
			args: wiki.SearchArgs{Query: "onboarding"},
			want: "query=onboarding",
		},
		{
			name: "GetPageArgs",
			args: wiki.GetPageArgs{Title: "API Reference"},
			want: "title=API Reference",
		},
		{
			name: "SearchInPageArgs",
			args: wiki.SearchInPageArgs{Title: "FAQ", Query: "timeout"},
			want: "title=FAQ, query=timeout",
		},
		{
			name: "SearchInFileArgs",
			args: wiki.SearchInFileArgs{Filename: "Report.pdf", Query: "budget"},
			want: "filename=Report.pdf, query=budget",
		},
		{
			name: "EditPageArgs",
			args: wiki.EditPageArgs{Title: "Test Page", Content: "should not appear in summary"},
			want: "title=Test Page",
		},
		{
			name: "FindReplaceArgs",
			args: wiki.FindReplaceArgs{Title: "Release Notes", Preview: true},
			want: "title=Release Notes, preview=true",
		},
		{
			name: "BulkReplaceArgs",
			args: wiki.BulkReplaceArgs{Pages: []string{"A", "B", "C"}, Preview: false},
			want: "pages=3, preview=false",
		},
		{
			name: "CompareRevisionsArgs",
			args: wiki.CompareRevisionsArgs{FromTitle: "Page A", ToTitle: "Page B"},
			want: "from_title=Page A, to_title=Page B",
		},
		{
			name: "FindSimilarPagesArgs",
			args: wiki.FindSimilarPagesArgs{Page: "Installation Guide"},
			want: "page=Installation Guide",
		},
		{
			name: "CompareTopicArgs",
			args: wiki.CompareTopicArgs{Topic: "timeout"},
			want: "topic=timeout",
		},
		{
			name: "ApplyFormattingArgs",
			args: wiki.ApplyFormattingArgs{Title: "Team", Format: "strikethrough"},
			want: "title=Team, format=strikethrough",
		},
		{
			name: "unknown type returns empty",
			args: struct{ Foo string }{Foo: "bar"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractArgsSummary(tt.args)
			if got != tt.want {
				t.Errorf("extractArgsSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewToolCallEntry(t *testing.T) {
	spec := ToolSpec{
		Name:     "mediawiki_get_page",
		Method:   "GetPage",
		Category: "read",
		ReadOnly: true,
	}
	args := wiki.GetPageArgs{Title: "Test"}
	start := time.Now().Add(-50 * time.Millisecond)

	entry := newToolCallEntry(spec, args, nil, start)

	if entry.Type != "tool_call" {
		t.Errorf("Type = %q, want %q", entry.Type, "tool_call")
	}
	if entry.Tool != "mediawiki_get_page" {
		t.Errorf("Tool = %q, want %q", entry.Tool, "mediawiki_get_page")
	}
	if !entry.Success {
		t.Error("Success should be true for nil error")
	}
	if entry.Error != "" {
		t.Errorf("Error should be empty, got %q", entry.Error)
	}
	if !entry.ReadOnly {
		t.Error("ReadOnly should be true")
	}
	if entry.DurationMs < 50 {
		t.Errorf("DurationMs = %d, expected >= 50", entry.DurationMs)
	}
	if entry.Args != "title=Test" {
		t.Errorf("Args = %q, want %q", entry.Args, "title=Test")
	}
}

func TestNewToolCallEntry_WithError(t *testing.T) {
	spec := ToolSpec{
		Name:     "mediawiki_edit_page",
		Method:   "EditPage",
		Category: "write",
		ReadOnly: false,
	}
	args := wiki.EditPageArgs{Title: "Protected Page"}
	start := time.Now()

	entry := newToolCallEntry(spec, args, fmt.Errorf("permission denied"), start)

	if entry.Success {
		t.Error("Success should be false for non-nil error")
	}
	if entry.Error != "permission denied" {
		t.Errorf("Error = %q, want %q", entry.Error, "permission denied")
	}
}

func TestErrorString(t *testing.T) {
	if got := errorString(nil); got != "" {
		t.Errorf("errorString(nil) = %q, want empty", got)
	}
	if got := errorString(fmt.Errorf("test")); got != "test" {
		t.Errorf("errorString(err) = %q, want %q", got, "test")
	}
}
