package wiki

import "time"

// ========== Recent Changes Types ==========

// RecentChangesArgs contains parameters for querying recent wiki changes.
type RecentChangesArgs struct {
	BaseArgs
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum changes to return (default 50, max 500)"`
	Namespace    int    `json:"namespace,omitempty" jsonschema:"Filter by namespace (-1 for all)"`
	Type         string `json:"type,omitempty" jsonschema:"Filter by type: 'edit', 'new', 'log', or empty for all"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema:"Continue token for pagination"`
	Start        string `json:"start,omitempty" jsonschema:"Lower time bound (ISO 8601). Returns changes on or after this timestamp."`
	End          string `json:"end,omitempty" jsonschema:"Upper time bound (ISO 8601). Returns changes on or before this timestamp."`
	AggregateBy  string `json:"aggregate_by,omitempty" jsonschema:"Aggregate results by: 'user', 'page', or 'type'. Returns counts instead of raw changes. Recommended for large result sets."`
}

// RecentChangesResult contains recent changes with optional aggregation.
type RecentChangesResult struct {
	Changes      []RecentChange     `json:"changes,omitempty"`
	HasMore      bool               `json:"has_more"`
	ContinueFrom string             `json:"continue_from,omitempty"`
	Aggregated   *AggregatedChanges `json:"aggregated,omitempty"`
}

// AggregatedChanges groups changes by user, page, or type for summaries.
type AggregatedChanges struct {
	By           string           `json:"by"`
	TotalChanges int              `json:"total_changes"`
	Items        []AggregateCount `json:"items"`
}

// AggregateCount holds a count for a single aggregation key.
type AggregateCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// RecentChange represents a single wiki edit, creation, or log event.
type RecentChange struct {
	Type       string    `json:"type"`
	Title      string    `json:"title"`
	PageID     int       `json:"page_id"`
	RevisionID int       `json:"revision_id"`
	User       string    `json:"user"`
	Timestamp  time.Time `json:"timestamp"`
	Comment    string    `json:"comment"`
	SizeDiff   int       `json:"size_diff"`
	New        bool      `json:"new"`
	Minor      bool      `json:"minor"`
	Bot        bool      `json:"bot"`
}

// ========== Revisions (Page History) Types ==========

// GetRevisionsArgs contains parameters for retrieving page revision history.
type GetRevisionsArgs struct {
	BaseArgs
	Title string `json:"title" jsonschema:"Page title to get revision history for"`
	Limit int    `json:"limit,omitempty" jsonschema:"Max revisions to return (default 20, max 100)"`
	Start string `json:"start,omitempty" jsonschema:"Lower time bound (ISO 8601). Returns revisions on or after this timestamp."`
	End   string `json:"end,omitempty" jsonschema:"Upper time bound (ISO 8601). Returns revisions on or before this timestamp."`
	User  string `json:"user,omitempty" jsonschema:"Filter to revisions by this user"`
}

// GetRevisionsResult contains the revision history for a page.
type GetRevisionsResult struct {
	Title     string         `json:"title"`
	PageID    int            `json:"page_id"`
	Revisions []RevisionInfo `json:"revisions"`
	Count     int            `json:"count"`
	HasMore   bool           `json:"has_more"`
}

// RevisionInfo describes a single revision in page history.
type RevisionInfo struct {
	RevID     int    `json:"revid"`
	ParentID  int    `json:"parentid"`
	User      string `json:"user"`
	Timestamp string `json:"timestamp"`
	Size      int    `json:"size"`
	SizeDiff  int    `json:"size_diff,omitempty"`
	Comment   string `json:"comment"`
	Minor     bool   `json:"minor,omitempty"`
}

// ========== Compare Revisions Types ==========

// CompareRevisionsArgs contains parameters for comparing two revisions.
type CompareRevisionsArgs struct {
	BaseArgs
	FromRev   int    `json:"from_rev,omitempty" jsonschema:"Source revision ID"`
	ToRev     int    `json:"to_rev,omitempty" jsonschema:"Target revision ID"`
	FromTitle string `json:"from_title,omitempty" jsonschema:"Source page title (uses latest revision)"`
	ToTitle   string `json:"to_title,omitempty" jsonschema:"Target page title (uses latest revision)"`
}

// CompareRevisionsResult contains the diff between two revisions.
type CompareRevisionsResult struct {
	FromTitle     string `json:"from_title"`
	FromRevID     int    `json:"from_revid"`
	ToTitle       string `json:"to_title"`
	ToRevID       int    `json:"to_revid"`
	Diff          string `json:"diff"`
	FromUser      string `json:"from_user,omitempty"`
	ToUser        string `json:"to_user,omitempty"`
	FromTimestamp string `json:"from_timestamp,omitempty"`
	ToTimestamp   string `json:"to_timestamp,omitempty"`
}

// ========== User Contributions Types ==========

// GetUserContributionsArgs contains parameters for retrieving a user's edits.
type GetUserContributionsArgs struct {
	BaseArgs
	User      string `json:"user" jsonschema:"Username to get contributions for"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max contributions to return (default 50, max 500)"`
	Namespace int    `json:"namespace,omitempty" jsonschema:"Filter by namespace (-1 for all)"`
	Start     string `json:"start,omitempty" jsonschema:"Lower time bound (ISO 8601). Returns contributions on or after this timestamp."`
	End       string `json:"end,omitempty" jsonschema:"Upper time bound (ISO 8601). Returns contributions on or before this timestamp."`
}

// GetUserContributionsResult contains a user's edit history.
type GetUserContributionsResult struct {
	User          string             `json:"user"`
	Contributions []UserContribution `json:"contributions"`
	Count         int                `json:"count"`
	HasMore       bool               `json:"has_more"`
}

// UserContribution represents a single edit by a user.
type UserContribution struct {
	PageID    int    `json:"page_id"`
	Title     string `json:"title"`
	Namespace int    `json:"namespace"`
	RevID     int    `json:"revid"`
	ParentID  int    `json:"parentid"`
	Timestamp string `json:"timestamp"`
	Comment   string `json:"comment"`
	Size      int    `json:"size"`
	SizeDiff  int    `json:"size_diff,omitempty"`
	Minor     bool   `json:"minor,omitempty"`
	New       bool   `json:"new,omitempty"`
}
