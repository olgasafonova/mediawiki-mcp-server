package wiki

// ========== Edit Types ==========

// EditPageArgs contains parameters for creating or editing a wiki page.
type EditPageArgs struct {
	BaseWriteArgs
	Title   string `json:"title" jsonschema:"Page title to edit or create"`
	Content string `json:"content" jsonschema:"New page content in wikitext format"`
	Summary string `json:"summary,omitempty" jsonschema:"Edit summary explaining the change"`
	Minor   bool   `json:"minor,omitempty" jsonschema:"Mark as minor edit"`
	Bot     bool   `json:"bot,omitempty" jsonschema:"Mark as bot edit (requires bot flag)"`
	Section string `json:"section,omitempty" jsonschema:"Section to edit ('new' for new section, number for existing)"`
}

// EditResult contains the result of a page edit operation.
type EditResult struct {
	Success    bool   `json:"success"`
	Title      string `json:"title"`
	PageID     int    `json:"page_id"`
	RevisionID int    `json:"revision_id"`
	NewPage    bool   `json:"new_page"`
	Message    string `json:"message"`
}

// EditRevisionInfo contains revision tracking info for edit operations
type EditRevisionInfo struct {
	OldRevision int64  `json:"old_revision,omitempty"`
	NewRevision int64  `json:"new_revision,omitempty"`
	DiffURL     string `json:"diff_url,omitempty"`
}

// UndoInfo provides instructions for undoing an edit
type UndoInfo struct {
	Instruction string `json:"instruction,omitempty"`
	WikiURL     string `json:"wiki_url,omitempty"`
}

// ========== Find and Replace Types ==========

// FindReplaceArgs contains parameters for text substitution in a page.
type FindReplaceArgs struct {
	BaseWriteArgs
	Title    string `json:"title" jsonschema:"Page title to edit"`
	Find     string `json:"find" jsonschema:"Text to find (exact match or regex if use_regex=true)"`
	Replace  string `json:"replace" jsonschema:"Replacement text"`
	UseRegex bool   `json:"use_regex,omitempty" jsonschema:"Treat 'find' as a Go RE2 regex. Characters like . [ ] * + ? ( ) have special meaning; escape with backslash for literal match. Max 500 chars."`
	All      bool   `json:"all,omitempty" jsonschema:"Replace all occurrences (default: first only)"`
	Preview  bool   `json:"preview,omitempty" jsonschema:"Preview changes without saving"`
	Summary  string `json:"summary,omitempty" jsonschema:"Edit summary"`
	Minor    bool   `json:"minor,omitempty" jsonschema:"Mark as minor edit"`
}

// FindReplaceResult contains the result of a find/replace operation.
type FindReplaceResult struct {
	Success      bool              `json:"success"`
	Title        string            `json:"title"`
	MatchCount   int               `json:"match_count"`
	ReplaceCount int               `json:"replace_count"`
	Preview      bool              `json:"preview"`
	Changes      []TextChange      `json:"changes,omitempty"`
	RevisionID   int               `json:"revision_id,omitempty"`
	Revision     *EditRevisionInfo `json:"revision,omitempty"`
	Undo         *UndoInfo         `json:"undo,omitempty"`
	Message      string            `json:"message"`
}

// TextChange describes a single text modification with before/after context.
type TextChange struct {
	Line    int    `json:"line"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Context string `json:"context,omitempty"`
}

// ========== Apply Formatting Types ==========

// ApplyFormattingArgs contains parameters for applying wiki markup formatting.
type ApplyFormattingArgs struct {
	BaseWriteArgs
	Title   string `json:"title" jsonschema:"Page title to edit"`
	Text    string `json:"text" jsonschema:"Text to find and format"`
	Format  string `json:"format" jsonschema:"Format to apply: 'strikethrough', 'bold', 'italic', 'underline', 'code', 'nowiki'"`
	All     bool   `json:"all,omitempty" jsonschema:"Apply to all occurrences (default: first only)"`
	Preview bool   `json:"preview,omitempty" jsonschema:"Preview changes without saving"`
	Summary string `json:"summary,omitempty" jsonschema:"Edit summary (auto-generated if empty)"`
}

// ApplyFormattingResult contains the result of a formatting operation.
type ApplyFormattingResult struct {
	Success     bool              `json:"success"`
	Title       string            `json:"title"`
	Format      string            `json:"format_applied"`
	MatchCount  int               `json:"match_count"`
	FormatCount int               `json:"format_count"`
	Preview     bool              `json:"preview"`
	Changes     []TextChange      `json:"changes,omitempty"`
	RevisionID  int               `json:"revision_id,omitempty"`
	Revision    *EditRevisionInfo `json:"revision,omitempty"`
	Undo        *UndoInfo         `json:"undo,omitempty"`
	Message     string            `json:"message"`
}

// ========== Bulk Replace Types ==========

// BulkReplaceArgs contains parameters for find/replace across multiple pages.
type BulkReplaceArgs struct {
	BaseWriteArgs
	Pages    []string `json:"pages,omitempty" jsonschema:"Page titles to process"`
	Category string   `json:"category,omitempty" jsonschema:"Category to get pages from (alternative to pages)"`
	Find     string   `json:"find" jsonschema:"Text to find"`
	Replace  string   `json:"replace" jsonschema:"Replacement text"`
	UseRegex bool     `json:"use_regex,omitempty" jsonschema:"Treat 'find' as a Go RE2 regex. Characters like . [ ] * + ? ( ) have special meaning; escape with backslash for literal match. Max 500 chars."`
	Preview  bool     `json:"preview,omitempty" jsonschema:"Preview changes without saving"`
	Summary  string   `json:"summary,omitempty" jsonschema:"Edit summary"`
	Limit    int      `json:"limit,omitempty" jsonschema:"Max pages to process (default 10, max 50)"`
}

// BulkReplaceResult summarizes find/replace results across multiple pages.
type BulkReplaceResult struct {
	PagesProcessed int                 `json:"pages_processed"`
	PagesModified  int                 `json:"pages_modified"`
	TotalChanges   int                 `json:"total_changes"`
	Preview        bool                `json:"preview"`
	Results        []PageReplaceResult `json:"results"`
	Message        string              `json:"message"`
}

// PageReplaceResult contains find/replace results for a single page.
type PageReplaceResult struct {
	Title        string            `json:"title"`
	MatchCount   int               `json:"match_count"`
	ReplaceCount int               `json:"replace_count"`
	Changes      []TextChange      `json:"changes,omitempty"`
	RevisionID   int               `json:"revision_id,omitempty"`
	Revision     *EditRevisionInfo `json:"revision,omitempty"`
	Undo         *UndoInfo         `json:"undo,omitempty"`
	Error        string            `json:"error,omitempty"`
}

// ========== Move Page Types ==========

// MovePageArgs contains parameters for moving (renaming) a wiki page.
type MovePageArgs struct {
	BaseWriteArgs
	From         string `json:"from" jsonschema:"Current page title"`
	To           string `json:"to" jsonschema:"New page title"`
	Reason       string `json:"reason,omitempty" jsonschema:"Reason for the move"`
	NoRedirect   bool   `json:"no_redirect,omitempty" jsonschema:"Don't create a redirect from the old title (requires suppressredirect right)"`
	MoveTalk     bool   `json:"move_talk,omitempty" jsonschema:"Also move the talk page if it exists (default true)"`
	MoveSubpages bool   `json:"move_subpages,omitempty" jsonschema:"Also move subpages if they exist"`
}

// MovePageResult contains the result of a page move operation.
type MovePageResult struct {
	Success     bool   `json:"success"`
	From        string `json:"from"`
	To          string `json:"to"`
	Reason      string `json:"reason,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
	TalkMoved   bool   `json:"talk_moved,omitempty"`
	Message     string `json:"message"`
}

// ========== Manage Categories Types ==========

// ManageCategoriesArgs contains parameters for adding or removing categories.
type ManageCategoriesArgs struct {
	BaseWriteArgs
	Title   string   `json:"title" jsonschema:"Page title to manage categories for"`
	Add     []string `json:"add,omitempty" jsonschema:"Category names to add (without 'Category:' prefix)"`
	Remove  []string `json:"remove,omitempty" jsonschema:"Category names to remove (without 'Category:' prefix)"`
	Summary string   `json:"summary,omitempty" jsonschema:"Edit summary"`
	Preview bool     `json:"preview,omitempty" jsonschema:"Preview changes without saving"`
}

// ManageCategoriesResult contains the result of category management.
type ManageCategoriesResult struct {
	Success           bool              `json:"success"`
	Title             string            `json:"title"`
	Added             []string          `json:"added,omitempty"`
	Removed           []string          `json:"removed,omitempty"`
	AlreadyPresent    []string          `json:"already_present,omitempty"`
	NotFound          []string          `json:"not_found,omitempty"`
	CurrentCategories []string          `json:"current_categories"`
	Preview           bool              `json:"preview"`
	RevisionID        int               `json:"revision_id,omitempty"`
	Revision          *EditRevisionInfo `json:"revision,omitempty"`
	Undo              *UndoInfo         `json:"undo,omitempty"`
	Message           string            `json:"message"`
}
