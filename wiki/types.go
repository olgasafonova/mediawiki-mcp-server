package wiki

// Constants for response limits
const (
	DefaultLimit   = 50
	MaxLimit       = 500
	CharacterLimit = 250000 // 250KB - accommodates large documentation pages in HTML format

	// Edit limits
	MaxEditSize = 200000 // 200KB max for edits (larger than read to allow updates)
)

// BaseArgs holds parameters shared by every read-only tool call.
//
// Rationale is optional on reads. The post-hoc-debug value of "why did
// this search happen?" is low — search/get_page calls are almost always
// performative narration of the conversation. Required-on-reads paid a
// ceremony tax for ~zero audit signal. Writes are covered by
// [BaseWriteArgs]. Pattern from Teddy Riker, "Designing for Agents".
type BaseArgs struct {
	Rationale string `json:"rationale,omitempty" jsonschema:"Optional one-sentence explanation of why you are calling this tool. Used for audit trails when present."`
}

// BaseWriteArgs holds parameters shared by destructive / write tool calls.
//
// Rationale is required here. Edits, uploads, moves, and category changes
// are the calls worth reconstructing six weeks later, and where prompt-
// injected agents most need to surface intent.
type BaseWriteArgs struct {
	Rationale string `json:"rationale" jsonschema:"Required one-sentence explanation of why you are making this change. Stored in the audit log for post-hoc intent reconstruction."`
}

// GetRationale returns the rationale string. Both BaseArgs and BaseWriteArgs
// satisfy the same interface, so handlers extract rationale uniformly without
// per-type switches.
func (b BaseArgs) GetRationale() string {
	return b.Rationale
}

// GetRationale on BaseWriteArgs mirrors BaseArgs so the audit logger sees a
// single GetRationale() interface across reads and writes.
func (b BaseWriteArgs) GetRationale() string {
	return b.Rationale
}

// ========== Search Types ==========

// SearchArgs contains parameters for full-text wiki search.
type SearchArgs struct {
	BaseArgs
	Query  string `json:"query" jsonschema:"Search query text"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results to return (default 20, max 500)"`
	Offset int    `json:"offset,omitempty" jsonschema:"Offset for pagination"`
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
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet"`
	Size    int    `json:"size"`
}

// ========== Page Content Types ==========

// GetPageArgs contains parameters for retrieving page content.
type GetPageArgs struct {
	BaseArgs
	Title  string `json:"title" jsonschema:"Page title to retrieve"`
	Format string `json:"format,omitempty" jsonschema:"Output format: 'wikitext' (default) or 'html'"`
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

// ========== Batch Page Types ==========

// GetPagesBatchArgs contains parameters for retrieving multiple pages at once.
// This is more efficient than individual GetPage calls for bulk operations.
type GetPagesBatchArgs struct {
	BaseArgs
	Titles []string `json:"titles" jsonschema:"List of page titles to retrieve (max 50)"`
	Format string   `json:"format,omitempty" jsonschema:"Output format: 'wikitext' (default) or 'html'"`
}

// GetPagesBatchResult contains content from multiple pages.
type GetPagesBatchResult struct {
	Pages        []PageContentResult `json:"pages"`
	TotalCount   int                 `json:"total_count"`
	FoundCount   int                 `json:"found_count"`
	MissingCount int                 `json:"missing_count"`
}

// PageContentResult contains content for a single page in batch results.
type PageContentResult struct {
	Title     string `json:"title"`
	PageID    int    `json:"page_id,omitempty"`
	Content   string `json:"content,omitempty"`
	Format    string `json:"format,omitempty"`
	Revision  int    `json:"revision_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Exists    bool   `json:"exists"`
	Error     string `json:"error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// GetPagesInfoBatchArgs contains parameters for retrieving metadata for multiple pages.
type GetPagesInfoBatchArgs struct {
	BaseArgs
	Titles []string `json:"titles" jsonschema:"List of page titles to get info for (max 50)"`
}

// GetPagesInfoBatchResult contains metadata for multiple pages.
type GetPagesInfoBatchResult struct {
	Pages        []PageInfo `json:"pages"`
	TotalCount   int        `json:"total_count"`
	ExistsCount  int        `json:"exists_count"`
	MissingCount int        `json:"missing_count"`
}

// ========== List Pages Types ==========

// ListPagesArgs contains parameters for listing wiki pages.
type ListPagesArgs struct {
	BaseArgs
	Prefix       string `json:"prefix,omitempty" jsonschema:"Filter pages starting with this prefix"`
	Namespace    int    `json:"namespace,omitempty" jsonschema:"Namespace ID (0=main, 1=talk, etc.)"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum pages to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema:"Continue token for pagination"`
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
	BaseArgs
	Prefix       string `json:"prefix,omitempty" jsonschema:"Filter categories starting with this prefix"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum categories to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema:"Continue token for pagination"`
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
	BaseArgs
	Category     string `json:"category" jsonschema:"Category name (with or without 'Category:' prefix)"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum members to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema:"Continue token for pagination"`
	Type         string `json:"type,omitempty" jsonschema:"Filter by type: 'page', 'subcat', 'file', or empty for all"`
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
	BaseArgs
	Title string `json:"title" jsonschema:"Page title"`
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

// ========== Parse Types ==========

// ParseArgs contains parameters for parsing wikitext to HTML.
type ParseArgs struct {
	BaseArgs
	Wikitext string `json:"wikitext" jsonschema:"Wikitext content to parse"`
	Title    string `json:"title,omitempty" jsonschema:"Page title for context (affects template expansion)"`
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
	BaseArgs
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

// ========== Search in Page Types ==========

// SearchInPageArgs contains parameters for searching within a specific page.
type SearchInPageArgs struct {
	BaseArgs
	Title        string `json:"title" jsonschema:"Page title to search in"`
	Query        string `json:"query" jsonschema:"Text to search for"`
	UseRegex     bool   `json:"use_regex,omitempty" jsonschema:"Treat query as a Go RE2 regex. Characters like . [ ] * + ? ( ) have special meaning; escape with backslash for literal match. Max 500 chars."`
	ContextLines int    `json:"context_lines,omitempty" jsonschema:"Lines of context around matches (default 2)"`
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
	BaseArgs
	Title      string `json:"title" jsonschema:"Page title to resolve (can be inexact)"`
	Fuzzy      bool   `json:"fuzzy,omitempty" jsonschema:"Enable fuzzy matching for similar titles"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"Max suggestions to return (default 5)"`
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

// ========== Search and Read Types ==========

// SearchAndReadArgs contains parameters for searching and reading top results in one call.
type SearchAndReadArgs struct {
	BaseArgs
	Query     string `json:"query" jsonschema:"Search query text"`
	ReadCount int    `json:"read_count,omitempty" jsonschema:"Number of top results to read (default 1, max 5)"`
	Format    string `json:"format,omitempty" jsonschema:"Content format: 'wikitext' (default) or 'html'"`
}

// SearchAndReadResult contains search results with page content for top hits.
type SearchAndReadResult struct {
	Query     string              `json:"query"`
	TotalHits int                 `json:"total_hits"`
	Pages     []SearchAndReadPage `json:"pages"`
	OtherHits []SearchHit         `json:"other_hits,omitempty"`
	Message   string              `json:"message"`
}

// SearchAndReadPage contains a search hit with its full page content.
type SearchAndReadPage struct {
	Title     string `json:"title"`
	PageID    int    `json:"page_id"`
	Snippet   string `json:"snippet"`
	Content   string `json:"content"`
	Format    string `json:"format"`
	Revision  int    `json:"revision_id"`
	Timestamp string `json:"timestamp,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ========== Page Summary Types ==========

// GetPageSummaryArgs contains parameters for retrieving a page summary.
type GetPageSummaryArgs struct {
	BaseArgs
	Title string `json:"title" jsonschema:"Page title to get summary for"`
}

// PageSummaryResult contains the lead section and key metadata for a page.
type PageSummaryResult struct {
	Title        string   `json:"title"`
	PageID       int      `json:"page_id"`
	LeadContent  string   `json:"lead_content"`
	Format       string   `json:"format"`
	Length       int      `json:"length"`
	Revision     int      `json:"revision_id"`
	LastEdited   string   `json:"last_edited"`
	Categories   []string `json:"categories,omitempty"`
	SectionCount int      `json:"section_count"`
	Sections     []string `json:"sections,omitempty"`
	Redirect     bool     `json:"redirect,omitempty"`
	RedirectTo   string   `json:"redirect_to,omitempty"`
	Message      string   `json:"message,omitempty"`
}
