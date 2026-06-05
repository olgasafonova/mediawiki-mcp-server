package wiki

// ========== External Links Types ==========

// GetExternalLinksArgs contains parameters for retrieving external URLs from a page.
type GetExternalLinksArgs struct {
	BaseArgs
	Title string `json:"title" jsonschema:"Page title to get external links from"`
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
	BaseArgs
	URLs    []string `json:"urls" jsonschema:"List of URLs to check (max 20)"`
	Timeout int      `json:"timeout,omitempty" jsonschema:"Timeout per URL in seconds (default 10, max 30)"`
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
	BaseArgs
	Titles []string `json:"titles" jsonschema:"Page titles to get external links from (max 10)"`
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

// ========== Broken Internal Links Types ==========

// FindBrokenInternalLinksArgs contains parameters for finding dead internal links.
type FindBrokenInternalLinksArgs struct {
	BaseArgs
	Pages    []string `json:"pages,omitempty" jsonschema:"Page titles to check for broken internal links"`
	Category string   `json:"category,omitempty" jsonschema:"Category to get pages from (alternative to pages)"`
	Limit    int      `json:"limit,omitempty" jsonschema:"Max pages to check (default 20, max 100)"`
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
	BaseArgs
	Namespace int    `json:"namespace,omitempty" jsonschema:"Namespace to check (0=main, default). Use -1 for all namespaces."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max pages to return (default 50, max 200)"`
	Prefix    string `json:"prefix,omitempty" jsonschema:"Only check pages starting with this prefix"`
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
	BaseArgs
	Title     string `json:"title" jsonschema:"Page title to find backlinks for"`
	Namespace int    `json:"namespace,omitempty" jsonschema:"Filter by namespace (-1 for all, 0 for main)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max backlinks to return (default 50, max 500)"`
	Redirect  bool   `json:"include_redirects,omitempty" jsonschema:"Include redirect pages in results"`
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
