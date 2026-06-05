package wiki

// ========== Terminology Check Types ==========

// CheckTerminologyArgs contains parameters for checking terminology consistency.
type CheckTerminologyArgs struct {
	BaseArgs
	Pages             []string `json:"pages,omitempty" jsonschema:"Page titles to check. If empty, uses pages from category."`
	Category          string   `json:"category,omitempty" jsonschema:"Category to get pages from (alternative to pages list)"`
	GlossaryPage      string   `json:"glossary_page,omitempty" jsonschema:"Wiki page containing the glossary table (default: 'Brand Terminology Glossary')"`
	Limit             int      `json:"limit,omitempty" jsonschema:"Max pages to check (default 10, max 50)"`
	ExcludeCodeBlocks *bool    `json:"exclude_code_blocks,omitempty" jsonschema:"Skip code blocks (syntaxhighlight, source, pre, code tags) to avoid false positives on code paths. Default: true"`
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
	BaseArgs
	BasePages []string `json:"base_pages,omitempty" jsonschema:"Base page names to check for translations (without language suffix)"`
	Category  string   `json:"category,omitempty" jsonschema:"Category to get base pages from (alternative to base_pages)"`
	Languages []string `json:"languages" jsonschema:"Language codes to check (e.g., ['en', 'no', 'sv'])"`
	Pattern   string   `json:"pattern,omitempty" jsonschema:"Pattern for language pages: 'subpage' (Page/lang), 'suffix' (Page (lang)), or 'prefix' (lang:Page). Default: 'subpage'"`
	Limit     int      `json:"limit,omitempty" jsonschema:"Max base pages to check (default 20, max 100)"`
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

// ========== Find Similar Pages Types ==========

// FindSimilarPagesArgs contains parameters for finding content-similar pages.
type FindSimilarPagesArgs struct {
	BaseArgs
	Page     string  `json:"page" jsonschema:"Page title to find similar pages for"`
	Limit    int     `json:"limit,omitempty" jsonschema:"Maximum similar pages to return (default 10, max 50)"`
	Category string  `json:"category,omitempty" jsonschema:"Limit search to pages in this category"`
	MinScore float64 `json:"min_score,omitempty" jsonschema:"Minimum similarity score 0-1 (default 0.1)"`
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
	BaseArgs
	Topic    string `json:"topic" jsonschema:"Topic or term to compare across pages"`
	Category string `json:"category,omitempty" jsonschema:"Limit search to pages in this category"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum pages to compare (default 20, max 50)"`
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
	BaseArgs
	Pages    []string `json:"pages,omitempty" jsonschema:"Specific pages to audit"`
	Category string   `json:"category,omitempty" jsonschema:"Category to audit (alternative to pages)"`
	Limit    int      `json:"limit,omitempty" jsonschema:"Max pages to audit (default 20, max 50)"`
	Checks   []string `json:"checks,omitempty" jsonschema:"Which checks to run: 'links', 'terminology', 'orphans', 'external', 'activity'. Default: all except 'external'"`
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

// ========== Stale Pages Types ==========

// GetStalePagesArgs contains parameters for finding pages not recently updated.
type GetStalePagesArgs struct {
	BaseArgs
	Days      int    `json:"days,omitempty" jsonschema:"Pages not edited in this many days (default 90)"`
	Category  string `json:"category,omitempty" jsonschema:"Limit to pages in this category"`
	Namespace int    `json:"namespace,omitempty" jsonschema:"Namespace to check (default 0 = main)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max pages to return (default 50, max 200)"`
}

// GetStalePagesResult contains pages that haven't been updated recently.
type GetStalePagesResult struct {
	Days         int         `json:"days_threshold"`
	StalePages   []StalePage `json:"stale_pages"`
	StaleCount   int         `json:"stale_count"`
	TotalScanned int         `json:"total_scanned"`
	Message      string      `json:"message"`
}

// StalePage represents a page that hasn't been edited recently.
type StalePage struct {
	Title      string `json:"title"`
	PageID     int    `json:"page_id"`
	LastEdited string `json:"last_edited"`
	DaysStale  int    `json:"days_stale"`
	Length     int    `json:"length"`
	Editor     string `json:"last_editor,omitempty"`
}
