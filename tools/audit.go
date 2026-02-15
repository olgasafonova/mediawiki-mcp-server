package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

// ToolCallEntry represents a single tool invocation for handler-level audit logging.
// This is separate from wiki.AuditEntry which tracks content changes (edits, creates, uploads).
// Both entry types can coexist in the same JSONL file, distinguished by the Type field.
type ToolCallEntry struct {
	// Type is always "tool_call" to distinguish from content audit entries
	Type string `json:"type"`

	// Timestamp is when the tool was invoked (RFC3339 UTC)
	Timestamp string `json:"timestamp"`

	// Tool is the MCP tool name (e.g., "mediawiki_search")
	Tool string `json:"tool"`

	// Method is the wiki.Client method name (e.g., "Search")
	Method string `json:"method"`

	// Category groups tools logically (search, read, write, etc.)
	Category string `json:"category"`

	// DurationMs is the execution time in milliseconds
	DurationMs int64 `json:"duration_ms"`

	// Success indicates if the tool call succeeded
	Success bool `json:"success"`

	// Error contains a sanitized error message if the call failed
	Error string `json:"error,omitempty"`

	// ReadOnly indicates the tool doesn't modify wiki state
	ReadOnly bool `json:"readonly"`

	// Args contains a summary of key arguments (title, query) without content bodies
	Args string `json:"args,omitempty"`
}

// ToolAuditLogger defines the interface for handler-level audit logging.
type ToolAuditLogger interface {
	// Log records a tool call entry
	Log(entry ToolCallEntry)

	// Close releases any resources held by the logger
	Close() error
}

// JSONToolAuditLogger writes tool call entries as JSON lines to a file or writer.
type JSONToolAuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
	file   *os.File // nil if using external writer
	logger *slog.Logger
}

// NewFileToolAuditLogger creates a tool audit logger that writes to a file.
// The file is opened with O_APPEND for safe concurrent writes with other loggers.
func NewFileToolAuditLogger(path string, logger *slog.Logger) (*JSONToolAuditLogger, error) {
	// #nosec G304 -- path comes from trusted MEDIAWIKI_AUDIT_LOG env var set by admin
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open tool audit log file: %w", err)
	}

	return &JSONToolAuditLogger{
		writer: file,
		file:   file,
		logger: logger,
	}, nil
}

// NewWriterToolAuditLogger creates a tool audit logger that writes to any io.Writer.
func NewWriterToolAuditLogger(w io.Writer, logger *slog.Logger) *JSONToolAuditLogger {
	return &JSONToolAuditLogger{
		writer: w,
		logger: logger,
	}
}

// Log writes a tool call entry as a JSON line.
func (l *JSONToolAuditLogger) Log(entry ToolCallEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		l.logger.Error("Failed to marshal tool audit entry", "error", err, "tool", entry.Tool)
		return
	}

	if _, err := l.writer.Write(append(data, '\n')); err != nil {
		l.logger.Error("Failed to write tool audit entry", "error", err, "tool", entry.Tool)
	}
}

// Close closes the underlying file if one was opened.
func (l *JSONToolAuditLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// NullToolAuditLogger is a no-op logger used when audit logging is disabled.
type NullToolAuditLogger struct{}

// Log does nothing.
func (NullToolAuditLogger) Log(_ ToolCallEntry) {}

// Close does nothing.
func (NullToolAuditLogger) Close() error { return nil }

// extractArgsSummary extracts key fields from tool arguments for audit logging.
// Returns a concise summary like "title=API Reference" or "query=onboarding".
// Never includes content bodies or sensitive data.
func extractArgsSummary(args any) string {
	switch a := args.(type) {
	case wiki.SearchArgs:
		return fmt.Sprintf("query=%s", a.Query)
	case wiki.GetPageArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.SearchInPageArgs:
		return fmt.Sprintf("title=%s, query=%s", a.Title, a.Query)
	case wiki.SearchInFileArgs:
		return fmt.Sprintf("filename=%s, query=%s", a.Filename, a.Query)
	case wiki.ResolveTitleArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.ListPagesArgs:
		return fmt.Sprintf("prefix=%s", a.Prefix)
	case wiki.PageInfoArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.GetSectionsArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.GetRelatedArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.GetImagesArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.ParseArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.CategoryMembersArgs:
		return fmt.Sprintf("category=%s", a.Category)
	case wiki.GetRevisionsArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.CompareRevisionsArgs:
		return fmt.Sprintf("from_title=%s, to_title=%s", a.FromTitle, a.ToTitle)
	case wiki.GetExternalLinksArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.GetBacklinksArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.EditPageArgs:
		return fmt.Sprintf("title=%s", a.Title)
	case wiki.FindReplaceArgs:
		return fmt.Sprintf("title=%s, preview=%t", a.Title, a.Preview)
	case wiki.ApplyFormattingArgs:
		return fmt.Sprintf("title=%s, format=%s", a.Title, a.Format)
	case wiki.BulkReplaceArgs:
		return fmt.Sprintf("pages=%d, preview=%t", len(a.Pages), a.Preview)
	case wiki.FindSimilarPagesArgs:
		return fmt.Sprintf("page=%s", a.Page)
	case wiki.CompareTopicArgs:
		return fmt.Sprintf("topic=%s", a.Topic)
	default:
		return ""
	}
}

// errorString returns the error message or empty string for nil errors.
func errorString(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// newToolCallEntry creates a ToolCallEntry from execution context.
func newToolCallEntry(spec ToolSpec, args any, err error, start time.Time) ToolCallEntry {
	return ToolCallEntry{
		Type:       "tool_call",
		Timestamp:  start.UTC().Format(time.RFC3339),
		Tool:       spec.Name,
		Method:     spec.Method,
		Category:   spec.Category,
		DurationMs: time.Since(start).Milliseconds(),
		Success:    err == nil,
		Error:      errorString(err),
		ReadOnly:   spec.ReadOnly,
		Args:       extractArgsSummary(args),
	}
}
