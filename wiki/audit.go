package wiki

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// AuditOperation represents the type of write operation being logged
type AuditOperation string

const (
	// AuditOpEdit represents a page edit operation
	AuditOpEdit AuditOperation = "edit"
	// AuditOpCreate represents a new page creation
	AuditOpCreate AuditOperation = "create"
	// AuditOpUpload represents a file upload operation
	AuditOpUpload AuditOperation = "upload"
)

// AuditEntry represents a single auditable write operation
type AuditEntry struct {
	// Timestamp is when the operation occurred (RFC3339 format)
	Timestamp string `json:"timestamp"`

	// Operation is the type of write operation (edit, create, upload)
	Operation AuditOperation `json:"operation"`

	// Title is the page or file title that was modified
	Title string `json:"title"`

	// PageID is the MediaWiki page ID (0 for new pages)
	PageID int `json:"page_id,omitempty"`

	// RevisionID is the new revision ID after the edit
	RevisionID int `json:"revision_id,omitempty"`

	// ContentHash is a SHA-256 hash of the content for change tracking
	ContentHash string `json:"content_hash"`

	// ContentSize is the size of the content in bytes
	ContentSize int `json:"content_size"`

	// Summary is the edit summary provided
	Summary string `json:"summary,omitempty"`

	// Minor indicates if this was marked as a minor edit
	Minor bool `json:"minor,omitempty"`

	// Bot indicates if this was marked as a bot edit
	Bot bool `json:"bot,omitempty"`

	// WikiURL is the base URL of the wiki
	WikiURL string `json:"wiki_url"`

	// Success indicates if the operation succeeded
	Success bool `json:"success"`

	// Error contains error details if the operation failed
	Error string `json:"error,omitempty"`
}

// AuditLogger defines the interface for audit logging implementations
type AuditLogger interface {
	// Log records an audit entry
	Log(entry AuditEntry)

	// Close releases any resources held by the logger
	Close() error
}

// JSONAuditLogger writes audit entries as JSON lines to a file or writer
type JSONAuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
	file   *os.File // nil if using external writer
	logger *slog.Logger
}

// NewFileAuditLogger creates an audit logger that writes to a file.
// The file is created if it doesn't exist, or appended to if it does.
// The path is expected to come from a trusted source (environment variable).
func NewFileAuditLogger(path string, logger *slog.Logger) (*JSONAuditLogger, error) {
	// #nosec G304 -- path comes from trusted MEDIAWIKI_AUDIT_LOG env var set by admin
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	return &JSONAuditLogger{
		writer: file,
		file:   file,
		logger: logger,
	}, nil
}

// NewWriterAuditLogger creates an audit logger that writes to any io.Writer
// Useful for testing or custom output destinations
func NewWriterAuditLogger(w io.Writer, logger *slog.Logger) *JSONAuditLogger {
	return &JSONAuditLogger{
		writer: w,
		logger: logger,
	}
}

// Log writes an audit entry as a JSON line
func (l *JSONAuditLogger) Log(entry AuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		l.logger.Error("Failed to marshal audit entry", "error", err, "title", entry.Title)
		return
	}

	// Write JSON line with newline
	if _, err := l.writer.Write(append(data, '\n')); err != nil {
		l.logger.Error("Failed to write audit entry", "error", err, "title", entry.Title)
	}
}

// Close closes the underlying file if one was opened
func (l *JSONAuditLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// NullAuditLogger is a no-op logger that discards all entries
// Use when audit logging is disabled
type NullAuditLogger struct{}

// Log does nothing (discards the entry)
func (NullAuditLogger) Log(_ AuditEntry) {}

// Close does nothing
func (NullAuditLogger) Close() error { return nil }

// hashContent computes a SHA-256 hash of the content for audit tracking
func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// buildAuditEntry creates an AuditEntry for a page edit operation
func (c *Client) buildAuditEntry(operation AuditOperation, title, content, summary string, minor, bot, success bool, pageID, revisionID int, errMsg string) AuditEntry {
	return AuditEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Operation:   operation,
		Title:       title,
		PageID:      pageID,
		RevisionID:  revisionID,
		ContentHash: hashContent(content),
		ContentSize: len(content),
		Summary:     summary,
		Minor:       minor,
		Bot:         bot,
		WikiURL:     c.config.BaseURL,
		Success:     success,
		Error:       errMsg,
	}
}

// logAudit logs an audit entry if an audit logger is configured
func (c *Client) logAudit(entry AuditEntry) {
	if c.auditLogger != nil {
		c.auditLogger.Log(entry)
	}
}
