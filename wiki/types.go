package wiki

import "time"

// Constants for response limits
const (
	DefaultLimit   = 50
	MaxLimit       = 500
	CharacterLimit = 250000 // 250KB - accommodates large documentation pages in HTML format

	// Edit limits
	MaxEditSize = 200000 // 200KB max for edits (larger than read to allow updates)
)

// ========== Search Types ==========

// SearchArgs contains parameters for full-text wiki search.
type SearchArgs struct {
	Query  string `json:"query" jsonschema:"required" jsonschema_description:"Search query text"`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum results to return (default 20, max 500)"`
	Offset int    `json:"offset,omitempty" jsonschema_description:"Offset for pagination"`
}

// SearchResult contains search results with pagination info.
type SearchResult struct {
	Query      string      `json:"query"`
	TotalHits  int         `json:"total_hits"`
	Results    []SearchHit `json:"results"`
	HasMore    bool        `json:"has_more"`
	NextOffset int         `json:"next_offset,omitempty"`
}

// SearchHit represents a single search result with snippet preview.
type SearchHit struct {
	PageID  int    `json:"page_id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Size    int    `json:"size"`
}

// ========== Page Content Types ==========

// GetPageArgs contains parameters for retrieving page content.
type GetPageArgs struct {
	Title  string `json:"title" jsonschema:"required" jsonschema_description:"Page title to retrieve"`
	Format string `json:"format,omitempty" jsonschema_description:"Output format: 'wikitext' (default) or 'html'"`
}

// PageContent holds the content of a wiki page in wikitext or HTML format.
type PageContent struct {
	Title     string `json:"title"`
	PageID    int    `json:"page_id"`
	Content   string `json:"content"`
	Format    string `json:"format"`
	Revision  int    `json:"revision_id"`
	Timestamp string `json:"timestamp"`
	Truncated bool   `json:"truncated,omitempty"`
	Message   string `json:"message,omitempty"`
}

// ========== List Pages Types ==========

// ListPagesArgs contains parameters for listing wiki pages.
type ListPagesArgs struct {
	Prefix       string `json:"prefix,omitempty" jsonschema_description:"Filter pages starting with this prefix"`
	Namespace    int    `json:"namespace,omitempty" jsonschema_description:"Namespace ID (0=main, 1=talk, etc.)"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum pages to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
}

// ListPagesResult contains a paginated list of wiki pages.
type ListPagesResult struct {
	Pages         []PageSummary `json:"pages"`
	ReturnedCount int           `json:"returned_count"`
	TotalCount    int           `json:"total_count,omitempty"`    // Deprecated: use returned_count. Shows returned count, not actual total.
	TotalEstimate int           `json:"total_estimate,omitempty"` // Estimated total pages in namespace (when available)
	HasMore       bool          `json:"has_more"`
	ContinueFrom  string        `json:"continue_from,omitempty"`
}

// PageSummary contains basic page identification info.
type PageSummary struct {
	PageID int    `json:"page_id"`
	Title  string `json:"title"`
}

// ========== Categories Types ==========

// ListCategoriesArgs contains parameters for listing wiki categories.
type ListCategoriesArgs struct {
	Prefix       string `json:"prefix,omitempty" jsonschema_description:"Filter categories starting with this prefix"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum categories to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
}

// ListCategoriesResult contains a paginated list of categories.
type ListCategoriesResult struct {
	Categories   []CategoryInfo `json:"categories"`
	HasMore      bool           `json:"has_more"`
	ContinueFrom string         `json:"continue_from,omitempty"`
}

// CategoryInfo describes a category and its member count.
type CategoryInfo struct {
	Title   string `json:"title"`
	Members int    `json:"members"`
}

// CategoryMembersArgs contains parameters for listing pages in a category.
type CategoryMembersArgs struct {
	Category     string `json:"category" jsonschema:"required" jsonschema_description:"Category name (with or without 'Category:' prefix)"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum members to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
	Type         string `json:"type,omitempty" jsonschema_description:"Filter by type: 'page', 'subcat', 'file', or empty for all"`
}

// CategoryMembersResult contains pages belonging to a category.
type CategoryMembersResult struct {
	Category     string        `json:"category"`
	Members      []PageSummary `json:"members"`
	HasMore      bool          `json:"has_more"`
	ContinueFrom string        `json:"continue_from,omitempty"`
}

// ========== Page Info Types ==========

// PageInfoArgs contains parameters for retrieving page metadata.
type PageInfoArgs struct {
	Title string `json:"title" jsonschema:"required" jsonschema_description:"Page title"`
}

// PageInfo contains metadata about a wiki page without its content.
type PageInfo struct {
	Title        string   `json:"title"`
	PageID       int      `json:"page_id"`
	Namespace    int      `json:"namespace"`
	ContentModel string   `json:"content_model"`
	PageLanguage string   `json:"page_language"`
	Length       int      `json:"length"`
	Touched      string   `json:"touched"`
	LastRevision int      `json:"last_revision_id"`
	Categories   []string `json:"categories,omitempty"`
	Links        int      `json:"links_count"`
	Exists       bool     `json:"exists"`
	Redirect     bool     `json:"redirect"`
	RedirectTo   string   `json:"redirect_to,omitempty"`
	Protection   []string `json:"protection,omitempty"`
}

// ========== Edit Types ==========

// EditPageArgs contains parameters for creating or editing a wiki page.
type EditPageArgs struct {
	Title   string `json:"title" jsonschema:"required" jsonschema_description:"Page title to edit or create"`
	Content string `json:"content" jsonschema:"required" jsonschema_description:"New page content in wikitext format"`
	Summary string `json:"summary,omitempty" jsonschema_description:"Edit summary explaining the change"`
	Minor   bool   `json:"minor,omitempty" jsonschema_description:"Mark as minor edit"`
	Bot     bool   `json:"bot,omitempty" jsonschema_description:"Mark as bot edit (requires bot flag)"`
	Section string `json:"section,omitempty" jsonschema_description:"Section to edit ('new' for new section, number for existing)"`
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

// ========== Recent Changes Types ==========

// RecentChangesArgs contains parameters for querying recent wiki changes.
type RecentChangesArgs struct {
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum changes to return (default 50, max 500)"`
	Namespace    int    `json:"namespace,omitempty" jsonschema_description:"Filter by namespace (-1 for all)"`
	Type         string `json:"type,omitempty" jsonschema_description:"Filter by type: 'edit', 'new', 'log', or empty for all"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
	Start        string `json:"start,omitempty" jsonschema_description:"Start timestamp (ISO 8601)"`
	End          string `json:"end,omitempty" jsonschema_description:"End timestamp (ISO 8601)"`
	AggregateBy  string `json:"aggregate_by,omitempty" jsonschema_description:"Aggregate results by: 'user', 'page', or 'type'. Returns counts instead of raw changes. Recommended for large result sets."`
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

// ========== Parse Types ==========

// ParseArgs contains parameters for parsing wikitext to HTML.
type ParseArgs struct {
	Wikitext string `json:"wikitext" jsonschema:"required" jsonschema_description:"Wikitext content to parse"`
	Title    string `json:"title,omitempty" jsonschema_description:"Page title for context (affects template expansion)"`
}

// ParseResult contains HTML output and extracted metadata from parsed wikitext.
type ParseResult struct {
	HTML       string   `json:"html"`
	Categories []string `json:"categories,omitempty"`
	Links      []string `json:"links,omitempty"`
	Truncated  bool     `json:"truncated,omitempty"`
	Message    string   `json:"message,omitempty"`
}

// ========== Wiki Info Types ==========

// WikiInfoArgs contains parameters for retrieving wiki site info (none required).
type WikiInfoArgs struct {
	// No arguments needed
}

// WikiInfo describes the MediaWiki installation and its statistics.
type WikiInfo struct {
	SiteName    string     `json:"site_name"`
	MainPage    string     `json:"main_page"`
	Base        string     `json:"base_url"`
	Generator   string     `json:"generator"`
	PHPVersion  string     `json:"php_version"`
	Language    string     `json:"language"`
	ArticlePath string     `json:"article_path"`
	Server      string     `json:"server"`
	Timezone    string     `json:"timezone"`
	WriteAPI    bool       `json:"write_api_enabled"`
	Statistics  *WikiStats `json:"statistics,omitempty"`
}

// WikiStats contains numerical statistics about the wiki.
type WikiStats struct {
	Pages       int `json:"pages"`
	Articles    int `json:"articles"`
	Edits       int `json:"edits"`
	Images      int `json:"images"`
	Users       int `json:"users"`
	ActiveUsers int `json:"active_users"`
	Admins      int `json:"admins"`
}

// ========== External Links Types ==========

// GetExternalLinksArgs contains parameters for retrieving external URLs from a page.
type GetExternalLinksArgs struct {
	Title string `json:"title" jsonschema:"required" jsonschema_description:"Page title to get external links from"`
}

// ExternalLinksResult contains external URLs found on a wiki page.
type ExternalLinksResult struct {
	Title string         `json:"title"`
	Links []ExternalLink `json:"links"`
	Count int            `json:"count"`
}

// ExternalLink represents a URL link from a wiki page.
type ExternalLink struct {
	URL      string `json:"url"`
	Protocol string `json:"protocol,omitempty"`
}

// ========== Check Links Types ==========

// CheckLinksArgs contains parameters for checking URL accessibility.
type CheckLinksArgs struct {
	URLs    []string `json:"urls" jsonschema:"required" jsonschema_description:"List of URLs to check (max 20)"`
	Timeout int      `json:"timeout,omitempty" jsonschema_description:"Timeout per URL in seconds (default 10, max 30)"`
}

// CheckLinksResult summarizes broken link detection results.
type CheckLinksResult struct {
	Results     []LinkCheckResult `json:"results"`
	TotalLinks  int               `json:"total_links"`
	BrokenCount int               `json:"broken_count"`
	ValidCount  int               `json:"valid_count"`
}

// LinkCheckResult contains the status of a single URL check.
type LinkCheckResult struct {
	URL        string `json:"url"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	Broken     bool   `json:"broken"`
}

// ========== Batch External Links Types ==========

// GetExternalLinksBatchArgs contains parameters for retrieving links from multiple pages.
type GetExternalLinksBatchArgs struct {
	Titles []string `json:"titles" jsonschema:"required" jsonschema_description:"Page titles to get external links from (max 10)"`
}

// ExternalLinksBatchResult contains links from multiple pages.
type ExternalLinksBatchResult struct {
	Pages      []PageExternalLinks `json:"pages"`
	TotalLinks int                 `json:"total_links"`
}

// PageExternalLinks contains external links for a single page.
type PageExternalLinks struct {
	Title string         `json:"title"`
	Links []ExternalLink `json:"links"`
	Count int            `json:"count"`
	Error string         `json:"error,omitempty"`
}

// ========== Terminology Check Types ==========

// CheckTerminologyArgs contains parameters for checking terminology consistency.
type CheckTerminologyArgs struct {
	Pages             []string `json:"pages,omitempty" jsonschema_description:"Page titles to check. If empty, uses pages from category."`
	Category          string   `json:"category,omitempty" jsonschema_description:"Category to get pages from (alternative to pages list)"`
	GlossaryPage      string   `json:"glossary_page,omitempty" jsonschema_description:"Wiki page containing the glossary table (default: 'Brand Terminology Glossary')"`
	Limit             int      `json:"limit,omitempty" jsonschema_description:"Max pages to check (default 10, max 50)"`
	ExcludeCodeBlocks *bool    `json:"exclude_code_blocks,omitempty" jsonschema_description:"Skip code blocks (syntaxhighlight, source, pre, code tags) to avoid false positives on code paths. Default: true"`
}

// CheckTerminologyResult contains terminology violations found across pages.
type CheckTerminologyResult struct {
	PagesChecked int                     `json:"pages_checked"`
	IssuesFound  int                     `json:"issues_found"`
	GlossaryPage string                  `json:"glossary_page"`
	TermsLoaded  int                     `json:"terms_loaded"`
	Pages        []PageTerminologyResult `json:"pages"`
}

// PageTerminologyResult contains terminology issues for a single page.
type PageTerminologyResult struct {
	Title      string             `json:"title"`
	IssueCount int                `json:"issue_count"`
	Issues     []TerminologyIssue `json:"issues"`
	Error      string             `json:"error,omitempty"`
}

// TerminologyIssue describes a single terminology violation.
type TerminologyIssue struct {
	Incorrect string `json:"incorrect"`
	Correct   string `json:"correct"`
	Line      int    `json:"line"`
	Context   string `json:"context"`
	Notes     string `json:"notes,omitempty"`
}

// GlossaryTerm defines correct terminology with optional pattern matching.
type GlossaryTerm struct {
	Incorrect string `json:"incorrect"`
	Correct   string `json:"correct"`
	Pattern   string `json:"pattern,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

// ========== Translation Check Types ==========

// CheckTranslationsArgs contains parameters for checking translation coverage.
type CheckTranslationsArgs struct {
	BasePages []string `json:"base_pages,omitempty" jsonschema_description:"Base page names to check for translations (without language suffix)"`
	Category  string   `json:"category,omitempty" jsonschema_description:"Category to get base pages from (alternative to base_pages)"`
	Languages []string `json:"languages" jsonschema:"required" jsonschema_description:"Language codes to check (e.g., ['en', 'no', 'sv'])"`
	Pattern   string   `json:"pattern,omitempty" jsonschema_description:"Pattern for language pages: 'subpage' (Page/lang), 'suffix' (Page (lang)), or 'prefix' (lang:Page). Default: 'subpage'"`
	Limit     int      `json:"limit,omitempty" jsonschema_description:"Max base pages to check (default 20, max 100)"`
}

// CheckTranslationsResult shows which pages have translations in each language.
type CheckTranslationsResult struct {
	PagesChecked     int                     `json:"pages_checked"`
	LanguagesChecked []string                `json:"languages_checked"`
	MissingCount     int                     `json:"missing_count"`
	Pattern          string                  `json:"pattern"`
	Pages            []PageTranslationResult `json:"pages"`
}

// PageTranslationResult shows translation status for a single base page.
type PageTranslationResult struct {
	BasePage     string                       `json:"base_page"`
	Translations map[string]TranslationStatus `json:"translations"`
	MissingLangs []string                     `json:"missing_languages,omitempty"`
	Complete     bool                         `json:"complete"`
}

// TranslationStatus indicates whether a language version exists.
type TranslationStatus struct {
	Exists    bool   `json:"exists"`
	PageTitle string `json:"page_title"`
	PageID    int    `json:"page_id,omitempty"`
	Length    int    `json:"length,omitempty"`
}

// ========== Broken Internal Links Types ==========

// FindBrokenInternalLinksArgs contains parameters for finding dead internal links.
type FindBrokenInternalLinksArgs struct {
	Pages    []string `json:"pages,omitempty" jsonschema_description:"Page titles to check for broken internal links"`
	Category string   `json:"category,omitempty" jsonschema_description:"Category to get pages from (alternative to pages)"`
	Limit    int      `json:"limit,omitempty" jsonschema_description:"Max pages to check (default 20, max 100)"`
}

// FindBrokenInternalLinksResult contains broken wiki links found across pages.
type FindBrokenInternalLinksResult struct {
	PagesChecked int                     `json:"pages_checked"`
	BrokenCount  int                     `json:"broken_count"`
	Pages        []PageBrokenLinksResult `json:"pages"`
}

// PageBrokenLinksResult contains broken links for a single page.
type PageBrokenLinksResult struct {
	Title       string       `json:"title"`
	BrokenLinks []BrokenLink `json:"broken_links"`
	BrokenCount int          `json:"broken_count"`
	Error       string       `json:"error,omitempty"`
}

// BrokenLink describes a link pointing to a non-existent page.
type BrokenLink struct {
	Target  string `json:"target"`
	Context string `json:"context,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// ========== Orphaned Pages Types ==========

// FindOrphanedPagesArgs contains parameters for finding pages with no incoming links.
type FindOrphanedPagesArgs struct {
	Namespace int    `json:"namespace,omitempty" jsonschema_description:"Namespace to check (0=main, default). Use -1 for all namespaces."`
	Limit     int    `json:"limit,omitempty" jsonschema_description:"Max pages to return (default 50, max 200)"`
	Prefix    string `json:"prefix,omitempty" jsonschema_description:"Only check pages starting with this prefix"`
}

// FindOrphanedPagesResult contains pages that have no incoming wiki links.
type FindOrphanedPagesResult struct {
	OrphanedPages []OrphanedPage `json:"orphaned_pages"`
	TotalChecked  int            `json:"total_checked"`
	OrphanedCount int            `json:"orphaned_count"`
}

// OrphanedPage represents a page with no incoming links.
type OrphanedPage struct {
	Title      string `json:"title"`
	PageID     int    `json:"page_id"`
	Length     int    `json:"length"`
	LastEdited string `json:"last_edited,omitempty"`
}

// ========== Backlinks Types ==========

// GetBacklinksArgs contains parameters for finding pages that link to a target.
type GetBacklinksArgs struct {
	Title     string `json:"title" jsonschema:"required" jsonschema_description:"Page title to find backlinks for"`
	Namespace int    `json:"namespace,omitempty" jsonschema_description:"Filter by namespace (-1 for all, 0 for main)"`
	Limit     int    `json:"limit,omitempty" jsonschema_description:"Max backlinks to return (default 50, max 500)"`
	Redirect  bool   `json:"include_redirects,omitempty" jsonschema_description:"Include redirect pages in results"`
}

// GetBacklinksResult contains pages that link to the target page.
type GetBacklinksResult struct {
	Title     string         `json:"title"`
	Backlinks []BacklinkInfo `json:"backlinks"`
	Count     int            `json:"count"`
	HasMore   bool           `json:"has_more"`
}

// BacklinkInfo describes a page that links to the target.
type BacklinkInfo struct {
	PageID     int    `json:"page_id"`
	Title      string `json:"title"`
	Namespace  int    `json:"namespace"`
	IsRedirect bool   `json:"is_redirect,omitempty"`
}

// ========== Revisions (Page History) Types ==========

// GetRevisionsArgs contains parameters for retrieving page revision history.
type GetRevisionsArgs struct {
	Title string `json:"title" jsonschema:"required" jsonschema_description:"Page title to get revision history for"`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Max revisions to return (default 20, max 100)"`
	Start string `json:"start,omitempty" jsonschema_description:"Start from this timestamp (ISO 8601, newer first)"`
	End   string `json:"end,omitempty" jsonschema_description:"End at this timestamp (ISO 8601)"`
	User  string `json:"user,omitempty" jsonschema_description:"Filter to revisions by this user"`
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
	FromRev   int    `json:"from_rev,omitempty" jsonschema_description:"Source revision ID"`
	ToRev     int    `json:"to_rev,omitempty" jsonschema_description:"Target revision ID"`
	FromTitle string `json:"from_title,omitempty" jsonschema_description:"Source page title (uses latest revision)"`
	ToTitle   string `json:"to_title,omitempty" jsonschema_description:"Target page title (uses latest revision)"`
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
	User      string `json:"user" jsonschema:"required" jsonschema_description:"Username to get contributions for"`
	Limit     int    `json:"limit,omitempty" jsonschema_description:"Max contributions to return (default 50, max 500)"`
	Namespace int    `json:"namespace,omitempty" jsonschema_description:"Filter by namespace (-1 for all)"`
	Start     string `json:"start,omitempty" jsonschema_description:"Start from this timestamp (ISO 8601, newer first)"`
	End       string `json:"end,omitempty" jsonschema_description:"End at this timestamp (ISO 8601)"`
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

// ========== Find and Replace Types ==========

// FindReplaceArgs contains parameters for text substitution in a page.
type FindReplaceArgs struct {
	Title    string `json:"title" jsonschema:"required" jsonschema_description:"Page title to edit"`
	Find     string `json:"find" jsonschema:"required" jsonschema_description:"Text to find (exact match or regex if use_regex=true)"`
	Replace  string `json:"replace" jsonschema:"required" jsonschema_description:"Replacement text"`
	UseRegex bool   `json:"use_regex,omitempty" jsonschema_description:"Treat 'find' as a regular expression"`
	All      bool   `json:"all,omitempty" jsonschema_description:"Replace all occurrences (default: first only)"`
	Preview  bool   `json:"preview,omitempty" jsonschema_description:"Preview changes without saving"`
	Summary  string `json:"summary,omitempty" jsonschema_description:"Edit summary"`
	Minor    bool   `json:"minor,omitempty" jsonschema_description:"Mark as minor edit"`
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
	Title   string `json:"title" jsonschema:"required" jsonschema_description:"Page title to edit"`
	Text    string `json:"text" jsonschema:"required" jsonschema_description:"Text to find and format"`
	Format  string `json:"format" jsonschema:"required" jsonschema_description:"Format to apply: 'strikethrough', 'bold', 'italic', 'underline', 'code', 'nowiki'"`
	All     bool   `json:"all,omitempty" jsonschema_description:"Apply to all occurrences (default: first only)"`
	Preview bool   `json:"preview,omitempty" jsonschema_description:"Preview changes without saving"`
	Summary string `json:"summary,omitempty" jsonschema_description:"Edit summary (auto-generated if empty)"`
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
	Pages    []string `json:"pages,omitempty" jsonschema_description:"Page titles to process"`
	Category string   `json:"category,omitempty" jsonschema_description:"Category to get pages from (alternative to pages)"`
	Find     string   `json:"find" jsonschema:"required" jsonschema_description:"Text to find"`
	Replace  string   `json:"replace" jsonschema:"required" jsonschema_description:"Replacement text"`
	UseRegex bool     `json:"use_regex,omitempty" jsonschema_description:"Treat 'find' as regex"`
	Preview  bool     `json:"preview,omitempty" jsonschema_description:"Preview changes without saving"`
	Summary  string   `json:"summary,omitempty" jsonschema_description:"Edit summary"`
	Limit    int      `json:"limit,omitempty" jsonschema_description:"Max pages to process (default 10, max 50)"`
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

// ========== Search in Page Types ==========

// SearchInPageArgs contains parameters for searching within a specific page.
type SearchInPageArgs struct {
	Title        string `json:"title" jsonschema:"required" jsonschema_description:"Page title to search in"`
	Query        string `json:"query" jsonschema:"required" jsonschema_description:"Text to search for"`
	UseRegex     bool   `json:"use_regex,omitempty" jsonschema_description:"Treat query as regex"`
	ContextLines int    `json:"context_lines,omitempty" jsonschema_description:"Lines of context around matches (default 2)"`
}

// SearchInPageResult contains text matches found within a page.
type SearchInPageResult struct {
	Title      string      `json:"title"`
	Query      string      `json:"query"`
	MatchCount int         `json:"match_count"`
	Matches    []PageMatch `json:"matches"`
}

// PageMatch represents a single text match with location and context.
type PageMatch struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Text    string `json:"text"`
	Context string `json:"context"`
}

// ========== Resolve Title Types ==========

// ResolveTitleArgs contains parameters for fuzzy page title matching.
type ResolveTitleArgs struct {
	Title      string `json:"title" jsonschema:"required" jsonschema_description:"Page title to resolve (can be inexact)"`
	Fuzzy      bool   `json:"fuzzy,omitempty" jsonschema_description:"Enable fuzzy matching for similar titles"`
	MaxResults int    `json:"max_results,omitempty" jsonschema_description:"Max suggestions to return (default 5)"`
}

// ResolveTitleResult contains the resolved page or similar suggestions.
type ResolveTitleResult struct {
	ExactMatch    bool              `json:"exact_match"`
	ResolvedTitle string            `json:"resolved_title,omitempty"`
	PageID        int               `json:"page_id,omitempty"`
	Suggestions   []TitleSuggestion `json:"suggestions,omitempty"`
	Message       string            `json:"message"`
}

// TitleSuggestion represents a possible title match with similarity score.
type TitleSuggestion struct {
	Title      string  `json:"title"`
	PageID     int     `json:"page_id"`
	Similarity float64 `json:"similarity,omitempty"`
}

// ========== List Users Types ==========

// ListUsersArgs contains parameters for listing wiki users.
type ListUsersArgs struct {
	Group        string `json:"group,omitempty" jsonschema_description:"Filter by user group: 'sysop' (admins), 'bureaucrat', 'bot', or empty for all users"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum users to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
	ActiveOnly   bool   `json:"active_only,omitempty" jsonschema_description:"Only return users active in the last 30 days"`
}

// ListUsersResult contains a paginated list of wiki users.
type ListUsersResult struct {
	Users        []UserInfo `json:"users"`
	TotalCount   int        `json:"total_count"`
	HasMore      bool       `json:"has_more"`
	ContinueFrom string     `json:"continue_from,omitempty"`
	Group        string     `json:"group,omitempty"`
}

// UserInfo describes a wiki user account.
type UserInfo struct {
	UserID       int      `json:"user_id"`
	Name         string   `json:"name"`
	Groups       []string `json:"groups,omitempty"`
	EditCount    int      `json:"edit_count,omitempty"`
	Registration string   `json:"registration,omitempty"`
}

// ========== Get Sections Types ==========

// GetSectionsArgs contains parameters for retrieving page section structure.
type GetSectionsArgs struct {
	Title   string `json:"title" jsonschema:"required" jsonschema_description:"Page title to get sections from"`
	Section int    `json:"section,omitempty" jsonschema_description:"Specific section number to retrieve content for (0 = intro, 1+ = sections). Omit to list all sections."`
	Format  string `json:"format,omitempty" jsonschema_description:"Output format for section content: 'wikitext' (default) or 'html'"`
}

// GetSectionsResult contains section headings and optional section content.
type GetSectionsResult struct {
	Title          string        `json:"title"`
	PageID         int           `json:"page_id"`
	Sections       []SectionInfo `json:"sections,omitempty"`
	SectionContent string        `json:"section_content,omitempty"`
	SectionTitle   string        `json:"section_title,omitempty"`
	Format         string        `json:"format,omitempty"`
	Message        string        `json:"message,omitempty"`
}

// SectionInfo describes a single section heading in a page.
type SectionInfo struct {
	Index   int    `json:"index"`
	Level   int    `json:"level"`
	Title   string `json:"title"`
	Anchor  string `json:"anchor"`
	LineNum int    `json:"line_number,omitempty"`
}

// ========== Related Pages Types ==========

// GetRelatedArgs contains parameters for finding related pages.
type GetRelatedArgs struct {
	Title  string `json:"title" jsonschema:"required" jsonschema_description:"Page title to find related pages for"`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum related pages to return (default 20, max 50)"`
	Method string `json:"method,omitempty" jsonschema_description:"Method to find related: 'categories' (default), 'links', 'backlinks', or 'all'"`
}

// GetRelatedResult contains pages related to the source page.
type GetRelatedResult struct {
	Title        string        `json:"title"`
	RelatedPages []RelatedPage `json:"related_pages"`
	Count        int           `json:"count"`
	Method       string        `json:"method"`
	Categories   []string      `json:"categories_used,omitempty"`
}

// RelatedPage represents a page related by category, link, or backlink.
type RelatedPage struct {
	Title      string   `json:"title"`
	PageID     int      `json:"page_id"`
	Relation   string   `json:"relation"`
	Categories []string `json:"shared_categories,omitempty"`
	Score      int      `json:"relevance_score,omitempty"`
}

// ========== Upload File Types ==========

// UploadFileArgs contains parameters for uploading a file to the wiki.
type UploadFileArgs struct {
	Filename       string `json:"filename" jsonschema:"required" jsonschema_description:"Target filename on the wiki (e.g., 'Example.png')"`
	FilePath       string `json:"file_path,omitempty" jsonschema_description:"Local file path to upload"`
	FileURL        string `json:"file_url,omitempty" jsonschema_description:"URL to fetch and upload (alternative to file_path)"`
	Text           string `json:"text,omitempty" jsonschema_description:"File description page content (wikitext)"`
	Comment        string `json:"comment,omitempty" jsonschema_description:"Upload comment for the log"`
	IgnoreWarnings bool   `json:"ignore_warnings,omitempty" jsonschema_description:"Ignore duplicate/overwrite warnings"`
}

// UploadFileResult contains the result of a file upload operation.
type UploadFileResult struct {
	Success  bool     `json:"success"`
	Filename string   `json:"filename"`
	PageID   int      `json:"page_id,omitempty"`
	URL      string   `json:"url,omitempty"`
	Size     int      `json:"size,omitempty"`
	Message  string   `json:"message"`
	Warnings []string `json:"warnings,omitempty"`
}

// ========== Get Images Types ==========

// GetImagesArgs contains parameters for retrieving images used on a page.
type GetImagesArgs struct {
	Title string `json:"title" jsonschema:"required" jsonschema_description:"Page title to get images from"`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum images to return (default 50, max 500)"`
}

// GetImagesResult contains images and files embedded in a page.
type GetImagesResult struct {
	Title  string      `json:"title"`
	Images []ImageInfo `json:"images"`
	Count  int         `json:"count"`
}

// ImageInfo describes an image or file used on a page.
type ImageInfo struct {
	Title    string `json:"title"`
	URL      string `json:"url,omitempty"`
	ThumbURL string `json:"thumb_url,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Size     int    `json:"size,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// ========== File Search Types ==========

// SearchInFileArgs contains parameters for searching within uploaded files.
type SearchInFileArgs struct {
	Filename string `json:"filename" jsonschema:"required" jsonschema_description:"File page name (e.g., 'File:Report.pdf' or just 'Report.pdf')"`
	Query    string `json:"query" jsonschema:"required" jsonschema_description:"Text to search for in the file"`
}

// SearchInFileResult contains text matches found in an uploaded file.
type SearchInFileResult struct {
	Filename   string            `json:"filename"`
	FileType   string            `json:"file_type"`
	Matches    []FileSearchMatch `json:"matches"`
	MatchCount int               `json:"match_count"`
	Searchable bool              `json:"searchable"`
	Message    string            `json:"message,omitempty"`
}

// FileSearchMatch represents a text match within an uploaded file.
type FileSearchMatch struct {
	Page    int    `json:"page,omitempty"`
	Line    int    `json:"line,omitempty"`
	Context string `json:"context"`
}

// ========== Find Similar Pages Types ==========

// FindSimilarPagesArgs contains parameters for finding content-similar pages.
type FindSimilarPagesArgs struct {
	Page     string  `json:"page" jsonschema:"required" jsonschema_description:"Page title to find similar pages for"`
	Limit    int     `json:"limit,omitempty" jsonschema_description:"Maximum similar pages to return (default 10, max 50)"`
	Category string  `json:"category,omitempty" jsonschema_description:"Limit search to pages in this category"`
	MinScore float64 `json:"min_score,omitempty" jsonschema_description:"Minimum similarity score 0-1 (default 0.1)"`
}

// FindSimilarPagesResult contains pages with similar content to the source.
type FindSimilarPagesResult struct {
	SourcePage    string        `json:"source_page"`
	SimilarPages  []SimilarPage `json:"similar_pages"`
	TotalCompared int           `json:"total_compared"`
	Message       string        `json:"message,omitempty"`
}

// SimilarPage describes a page similar to the source with comparison metrics.
type SimilarPage struct {
	Title           string   `json:"title"`
	SimilarityScore float64  `json:"similarity_score"`
	CommonTerms     []string `json:"common_terms"`
	IsLinked        bool     `json:"is_linked"`
	LinksBack       bool     `json:"links_back"`
	Recommendation  string   `json:"recommendation"`
}

// ========== Compare Topic Types ==========

// CompareTopicArgs contains parameters for comparing topic mentions across pages.
type CompareTopicArgs struct {
	Topic    string `json:"topic" jsonschema:"required" jsonschema_description:"Topic or term to compare across pages"`
	Category string `json:"category,omitempty" jsonschema_description:"Limit search to pages in this category"`
	Limit    int    `json:"limit,omitempty" jsonschema_description:"Maximum pages to compare (default 20, max 50)"`
}

// CompareTopicResult shows how a topic is described across different pages.
type CompareTopicResult struct {
	Topic           string          `json:"topic"`
	PagesFound      int             `json:"pages_found"`
	PageMentions    []TopicMention  `json:"page_mentions"`
	Inconsistencies []Inconsistency `json:"inconsistencies,omitempty"`
	Summary         string          `json:"summary"`
}

// TopicMention describes how a page mentions and describes a topic.
type TopicMention struct {
	PageTitle  string   `json:"page_title"`
	Mentions   int      `json:"mentions"`
	Contexts   []string `json:"contexts"`
	LastEdited string   `json:"last_edited"`
}

// Inconsistency describes conflicting information between two pages.
type Inconsistency struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	PageA       string `json:"page_a"`
	PageB       string `json:"page_b"`
	ValueA      string `json:"value_a"`
	ValueB      string `json:"value_b"`
}

// ========== Wiki Health Audit Types ==========

// WikiHealthAuditArgs contains parameters for a comprehensive wiki health audit.
type WikiHealthAuditArgs struct {
	Pages    []string `json:"pages,omitempty" jsonschema_description:"Specific pages to audit"`
	Category string   `json:"category,omitempty" jsonschema_description:"Category to audit (alternative to pages)"`
	Limit    int      `json:"limit,omitempty" jsonschema_description:"Max pages to audit (default 20, max 50)"`
	Checks   []string `json:"checks,omitempty" jsonschema_description:"Which checks to run: 'links', 'terminology', 'orphans', 'external', 'activity'. Default: all except 'external'"`
}

// WikiHealthAuditResult contains the aggregated results of a wiki health audit.
type WikiHealthAuditResult struct {
	WikiName       string                         `json:"wiki_name"`
	AuditedAt      string                         `json:"audited_at"`
	PagesAudited   int                            `json:"pages_audited"`
	HealthScore    int                            `json:"health_score"`
	Summary        WikiHealthAuditSummary         `json:"summary"`
	BrokenLinks    *FindBrokenInternalLinksResult `json:"broken_links,omitempty"`
	Terminology    *CheckTerminologyResult        `json:"terminology,omitempty"`
	OrphanedPages  *FindOrphanedPagesResult       `json:"orphaned_pages,omitempty"`
	ExternalLinks  *CheckLinksResult              `json:"external_links,omitempty"`
	RecentActivity *AggregatedChanges             `json:"recent_activity,omitempty"`
	Errors         []string                       `json:"errors,omitempty"`
}

// WikiHealthAuditSummary provides a quick overview of audit findings.
type WikiHealthAuditSummary struct {
	BrokenLinksCount    int `json:"broken_links_count"`
	TerminologyIssues   int `json:"terminology_issues"`
	OrphanedPagesCount  int `json:"orphaned_pages_count"`
	ExternalBrokenCount int `json:"external_broken_count"`
}
