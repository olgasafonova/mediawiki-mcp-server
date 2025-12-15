package wiki

import "time"

// Constants for response limits
const (
	DefaultLimit    = 50
	MaxLimit        = 500
	CharacterLimit  = 25000
)

// ========== Search Types ==========

type SearchArgs struct {
	Query  string `json:"query" jsonschema:"required" jsonschema_description:"Search query text"`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum results to return (default 20, max 500)"`
	Offset int    `json:"offset,omitempty" jsonschema_description:"Offset for pagination"`
}

type SearchResult struct {
	Query      string       `json:"query"`
	TotalHits  int          `json:"total_hits"`
	Results    []SearchHit  `json:"results"`
	HasMore    bool         `json:"has_more"`
	NextOffset int          `json:"next_offset,omitempty"`
}

type SearchHit struct {
	PageID  int    `json:"page_id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Size    int    `json:"size"`
}

// ========== Page Content Types ==========

type GetPageArgs struct {
	Title  string `json:"title" jsonschema:"required" jsonschema_description:"Page title to retrieve"`
	Format string `json:"format,omitempty" jsonschema_description:"Output format: 'wikitext' (default) or 'html'"`
}

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

type ListPagesArgs struct {
	Prefix       string `json:"prefix,omitempty" jsonschema_description:"Filter pages starting with this prefix"`
	Namespace    int    `json:"namespace,omitempty" jsonschema_description:"Namespace ID (0=main, 1=talk, etc.)"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum pages to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
}

type ListPagesResult struct {
	Pages        []PageSummary `json:"pages"`
	TotalCount   int           `json:"total_count"`
	HasMore      bool          `json:"has_more"`
	ContinueFrom string        `json:"continue_from,omitempty"`
}

type PageSummary struct {
	PageID int    `json:"page_id"`
	Title  string `json:"title"`
}

// ========== Categories Types ==========

type ListCategoriesArgs struct {
	Prefix       string `json:"prefix,omitempty" jsonschema_description:"Filter categories starting with this prefix"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum categories to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
}

type ListCategoriesResult struct {
	Categories   []CategoryInfo `json:"categories"`
	HasMore      bool           `json:"has_more"`
	ContinueFrom string         `json:"continue_from,omitempty"`
}

type CategoryInfo struct {
	Title   string `json:"title"`
	Members int    `json:"members"`
}

type CategoryMembersArgs struct {
	Category     string `json:"category" jsonschema:"required" jsonschema_description:"Category name (with or without 'Category:' prefix)"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum members to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
	Type         string `json:"type,omitempty" jsonschema_description:"Filter by type: 'page', 'subcat', 'file', or empty for all"`
}

type CategoryMembersResult struct {
	Category     string        `json:"category"`
	Members      []PageSummary `json:"members"`
	HasMore      bool          `json:"has_more"`
	ContinueFrom string        `json:"continue_from,omitempty"`
}

// ========== Page Info Types ==========

type PageInfoArgs struct {
	Title string `json:"title" jsonschema:"required" jsonschema_description:"Page title"`
}

type PageInfo struct {
	Title         string     `json:"title"`
	PageID        int        `json:"page_id"`
	Namespace     int        `json:"namespace"`
	ContentModel  string     `json:"content_model"`
	PageLanguage  string     `json:"page_language"`
	Length        int        `json:"length"`
	Touched       string     `json:"touched"`
	LastRevision  int        `json:"last_revision_id"`
	Categories    []string   `json:"categories,omitempty"`
	Links         int        `json:"links_count"`
	Exists        bool       `json:"exists"`
	Redirect      bool       `json:"redirect"`
	RedirectTo    string     `json:"redirect_to,omitempty"`
	Protection    []string   `json:"protection,omitempty"`
}

// ========== Edit Types ==========

type EditPageArgs struct {
	Title   string `json:"title" jsonschema:"required" jsonschema_description:"Page title to edit or create"`
	Content string `json:"content" jsonschema:"required" jsonschema_description:"New page content in wikitext format"`
	Summary string `json:"summary,omitempty" jsonschema_description:"Edit summary explaining the change"`
	Minor   bool   `json:"minor,omitempty" jsonschema_description:"Mark as minor edit"`
	Bot     bool   `json:"bot,omitempty" jsonschema_description:"Mark as bot edit (requires bot flag)"`
	Section string `json:"section,omitempty" jsonschema_description:"Section to edit ('new' for new section, number for existing)"`
}

type EditResult struct {
	Success    bool   `json:"success"`
	Title      string `json:"title"`
	PageID     int    `json:"page_id"`
	RevisionID int    `json:"revision_id"`
	NewPage    bool   `json:"new_page"`
	Message    string `json:"message"`
}

// ========== Recent Changes Types ==========

type RecentChangesArgs struct {
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Maximum changes to return (default 50, max 500)"`
	Namespace    int    `json:"namespace,omitempty" jsonschema_description:"Filter by namespace (-1 for all)"`
	Type         string `json:"type,omitempty" jsonschema_description:"Filter by type: 'edit', 'new', 'log', or empty for all"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema_description:"Continue token for pagination"`
	Start        string `json:"start,omitempty" jsonschema_description:"Start timestamp (ISO 8601)"`
	End          string `json:"end,omitempty" jsonschema_description:"End timestamp (ISO 8601)"`
}

type RecentChangesResult struct {
	Changes      []RecentChange `json:"changes"`
	HasMore      bool           `json:"has_more"`
	ContinueFrom string         `json:"continue_from,omitempty"`
}

type RecentChange struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	PageID    int       `json:"page_id"`
	RevisionID int      `json:"revision_id"`
	User      string    `json:"user"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment"`
	SizeDiff  int       `json:"size_diff"`
	New       bool      `json:"new"`
	Minor     bool      `json:"minor"`
	Bot       bool      `json:"bot"`
}

// ========== Parse Types ==========

type ParseArgs struct {
	Wikitext string `json:"wikitext" jsonschema:"required" jsonschema_description:"Wikitext content to parse"`
	Title    string `json:"title,omitempty" jsonschema_description:"Page title for context (affects template expansion)"`
}

type ParseResult struct {
	HTML       string   `json:"html"`
	Categories []string `json:"categories,omitempty"`
	Links      []string `json:"links,omitempty"`
	Truncated  bool     `json:"truncated,omitempty"`
	Message    string   `json:"message,omitempty"`
}

// ========== Wiki Info Types ==========

type WikiInfoArgs struct {
	// No arguments needed
}

type WikiInfo struct {
	SiteName    string `json:"site_name"`
	MainPage    string `json:"main_page"`
	Base        string `json:"base_url"`
	Generator   string `json:"generator"`
	PHPVersion  string `json:"php_version"`
	Language    string `json:"language"`
	ArticlePath string `json:"article_path"`
	Server      string `json:"server"`
	Timezone    string `json:"timezone"`
	WriteAPI    bool   `json:"write_api_enabled"`
	Statistics  *WikiStats `json:"statistics,omitempty"`
}

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

type GetExternalLinksArgs struct {
	Title string `json:"title" jsonschema:"required" jsonschema_description:"Page title to get external links from"`
}

type ExternalLinksResult struct {
	Title string         `json:"title"`
	Links []ExternalLink `json:"links"`
	Count int            `json:"count"`
}

type ExternalLink struct {
	URL      string `json:"url"`
	Protocol string `json:"protocol,omitempty"`
}

// ========== Check Links Types ==========

type CheckLinksArgs struct {
	URLs    []string `json:"urls" jsonschema:"required" jsonschema_description:"List of URLs to check (max 20)"`
	Timeout int      `json:"timeout,omitempty" jsonschema_description:"Timeout per URL in seconds (default 10, max 30)"`
}

type CheckLinksResult struct {
	Results     []LinkCheckResult `json:"results"`
	TotalLinks  int               `json:"total_links"`
	BrokenCount int               `json:"broken_count"`
	ValidCount  int               `json:"valid_count"`
}

type LinkCheckResult struct {
	URL        string `json:"url"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	Broken     bool   `json:"broken"`
}

// ========== Batch External Links Types ==========

type GetExternalLinksBatchArgs struct {
	Titles []string `json:"titles" jsonschema:"required" jsonschema_description:"Page titles to get external links from (max 10)"`
}

type ExternalLinksBatchResult struct {
	Pages      []PageExternalLinks `json:"pages"`
	TotalLinks int                 `json:"total_links"`
}

type PageExternalLinks struct {
	Title string         `json:"title"`
	Links []ExternalLink `json:"links"`
	Count int            `json:"count"`
	Error string         `json:"error,omitempty"`
}

// ========== Terminology Check Types ==========

type CheckTerminologyArgs struct {
	Pages        []string `json:"pages,omitempty" jsonschema_description:"Page titles to check. If empty, uses pages from category."`
	Category     string   `json:"category,omitempty" jsonschema_description:"Category to get pages from (alternative to pages list)"`
	GlossaryPage string   `json:"glossary_page,omitempty" jsonschema_description:"Wiki page containing the glossary table (default: 'Brand Terminology Glossary')"`
	Limit        int      `json:"limit,omitempty" jsonschema_description:"Max pages to check (default 10, max 50)"`
}

type CheckTerminologyResult struct {
	PagesChecked  int                   `json:"pages_checked"`
	IssuesFound   int                   `json:"issues_found"`
	GlossaryPage  string                `json:"glossary_page"`
	TermsLoaded   int                   `json:"terms_loaded"`
	Pages         []PageTerminologyResult `json:"pages"`
}

type PageTerminologyResult struct {
	Title       string             `json:"title"`
	IssueCount  int                `json:"issue_count"`
	Issues      []TerminologyIssue `json:"issues"`
	Error       string             `json:"error,omitempty"`
}

type TerminologyIssue struct {
	Incorrect   string `json:"incorrect"`
	Correct     string `json:"correct"`
	Line        int    `json:"line"`
	Context     string `json:"context"`
	Notes       string `json:"notes,omitempty"`
}

type GlossaryTerm struct {
	Incorrect string `json:"incorrect"`
	Correct   string `json:"correct"`
	Pattern   string `json:"pattern,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

// ========== Translation Check Types ==========

type CheckTranslationsArgs struct {
	BasePages  []string `json:"base_pages,omitempty" jsonschema_description:"Base page names to check for translations (without language suffix)"`
	Category   string   `json:"category,omitempty" jsonschema_description:"Category to get base pages from (alternative to base_pages)"`
	Languages  []string `json:"languages" jsonschema:"required" jsonschema_description:"Language codes to check (e.g., ['en', 'no', 'sv'])"`
	Pattern    string   `json:"pattern,omitempty" jsonschema_description:"Pattern for language pages: 'subpage' (Page/lang), 'suffix' (Page (lang)), or 'prefix' (lang:Page). Default: 'subpage'"`
	Limit      int      `json:"limit,omitempty" jsonschema_description:"Max base pages to check (default 20, max 100)"`
}

type CheckTranslationsResult struct {
	PagesChecked    int                        `json:"pages_checked"`
	LanguagesChecked []string                  `json:"languages_checked"`
	MissingCount    int                        `json:"missing_count"`
	Pattern         string                     `json:"pattern"`
	Pages           []PageTranslationResult    `json:"pages"`
}

type PageTranslationResult struct {
	BasePage    string              `json:"base_page"`
	Translations map[string]TranslationStatus `json:"translations"`
	MissingLangs []string           `json:"missing_languages,omitempty"`
	Complete    bool                `json:"complete"`
}

type TranslationStatus struct {
	Exists   bool   `json:"exists"`
	PageTitle string `json:"page_title"`
	PageID   int    `json:"page_id,omitempty"`
	Length   int    `json:"length,omitempty"`
}

// ========== Broken Internal Links Types ==========

type FindBrokenInternalLinksArgs struct {
	Pages    []string `json:"pages,omitempty" jsonschema_description:"Page titles to check for broken internal links"`
	Category string   `json:"category,omitempty" jsonschema_description:"Category to get pages from (alternative to pages)"`
	Limit    int      `json:"limit,omitempty" jsonschema_description:"Max pages to check (default 20, max 100)"`
}

type FindBrokenInternalLinksResult struct {
	PagesChecked   int                    `json:"pages_checked"`
	BrokenCount    int                    `json:"broken_count"`
	Pages          []PageBrokenLinksResult `json:"pages"`
}

type PageBrokenLinksResult struct {
	Title         string        `json:"title"`
	BrokenLinks   []BrokenLink  `json:"broken_links"`
	BrokenCount   int           `json:"broken_count"`
	Error         string        `json:"error,omitempty"`
}

type BrokenLink struct {
	Target    string `json:"target"`
	Context   string `json:"context,omitempty"`
	Line      int    `json:"line,omitempty"`
}

// ========== Orphaned Pages Types ==========

type FindOrphanedPagesArgs struct {
	Namespace int    `json:"namespace,omitempty" jsonschema_description:"Namespace to check (0=main, default). Use -1 for all namespaces."`
	Limit     int    `json:"limit,omitempty" jsonschema_description:"Max pages to return (default 50, max 200)"`
	Prefix    string `json:"prefix,omitempty" jsonschema_description:"Only check pages starting with this prefix"`
}

type FindOrphanedPagesResult struct {
	OrphanedPages []OrphanedPage `json:"orphaned_pages"`
	TotalChecked  int            `json:"total_checked"`
	OrphanedCount int            `json:"orphaned_count"`
}

type OrphanedPage struct {
	Title      string `json:"title"`
	PageID     int    `json:"page_id"`
	Length     int    `json:"length"`
	LastEdited string `json:"last_edited,omitempty"`
}

// ========== Backlinks Types ==========

type GetBacklinksArgs struct {
	Title     string `json:"title" jsonschema:"required" jsonschema_description:"Page title to find backlinks for"`
	Namespace int    `json:"namespace,omitempty" jsonschema_description:"Filter by namespace (-1 for all, 0 for main)"`
	Limit     int    `json:"limit,omitempty" jsonschema_description:"Max backlinks to return (default 50, max 500)"`
	Redirect  bool   `json:"include_redirects,omitempty" jsonschema_description:"Include redirect pages in results"`
}

type GetBacklinksResult struct {
	Title       string        `json:"title"`
	Backlinks   []BacklinkInfo `json:"backlinks"`
	Count       int           `json:"count"`
	HasMore     bool          `json:"has_more"`
}

type BacklinkInfo struct {
	PageID     int    `json:"page_id"`
	Title      string `json:"title"`
	Namespace  int    `json:"namespace"`
	IsRedirect bool   `json:"is_redirect,omitempty"`
}
