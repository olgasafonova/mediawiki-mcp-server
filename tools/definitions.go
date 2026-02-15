package tools

// AllTools contains all tool specifications for the MediaWiki MCP server.
// Tools are organized by category for easier maintenance.
// Tool descriptions follow a structured format for optimal LLM tool selection:
// - USE WHEN: Natural language triggers
// - NOT FOR: Disambiguation from similar tools
// - PARAMETERS: Key arguments with defaults
// - RETURNS: What the tool returns
var AllTools = []ToolSpec{
	// ==========================================================================
	// SEARCH TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_search",
		Method:   "Search",
		Title:    "Search Wiki",
		Category: "search",
		Description: `Search ACROSS the entire wiki for pages containing specific text.

USE WHEN: User asks "find pages about X", "where is X documented", "search for X", or doesn't know which page contains information.

NOT FOR: Searching within a specific known page (use mediawiki_search_in_page instead).

PARAMETERS:
- query: Search text (required)
- limit: Max results (default 10)

RETURNS: Page titles, snippets with highlights, and relevance scores.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_search_in_page",
		Method:   "SearchInPage",
		Title:    "Search in Page",
		Category: "search",
		Description: `Search WITHIN a known page (not across wiki).

USE WHEN: User says "find X on page Y", "does page Y mention X", "search for X in the Configuration page".

NOT FOR: Finding which page contains info (use mediawiki_search instead).

PARAMETERS:
- title: Page name (required)
- query: Text to find (required)
- use_regex: Enable regex matching (optional)
- context_lines: Lines of context around matches (default 2)

RETURNS: Matches with line numbers and surrounding context.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_search_in_file",
		Method:   "SearchInFile",
		Title:    "Search in File",
		Category: "search",
		Description: `Search for text within wiki-hosted files (PDFs, text files).

USE WHEN: User asks "search the PDF for X", "find X in the uploaded document".

NOT FOR: Searching wiki pages (use mediawiki_search or mediawiki_search_in_page).

PARAMETERS:
- filename: File name on wiki (required)
- query: Text to search for (required)

RETURNS: Matches with page numbers (for PDFs) or line numbers.

NOTE: Supports text-based PDFs and text files (TXT, MD, CSV, JSON, XML, HTML). Scanned/image PDFs require OCR and are not supported.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_resolve_title",
		Method:   "ResolveTitle",
		Title:    "Resolve Title",
		Category: "search",
		Description: `RECOVERY tool when page not found due to case sensitivity or typos.

USE WHEN: User got "page not found" and suspects wrong capitalization or spelling. E.g., "module overview" should be "Module Overview".

NOT FOR: Finding pages about a topic (use mediawiki_search instead).

PARAMETERS:
- title: Approximate page name (required)
- fuzzy: Enable fuzzy matching for typos (default true)
- max_results: Max suggestions (default 5)

RETURNS: Suggested correct page titles with confidence scores.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// READ TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_get_page",
		Method:   "GetPage",
		Title:    "Get Page Content",
		Category: "read",
		Description: `Retrieve full wiki page content.

USE WHEN: User says "show me the X page", "what's on the Main Page", "read the FAQ".

NOT FOR: Getting page structure/TOC (use mediawiki_get_sections). Not for searching content (use mediawiki_search_in_page). Not for metadata only (use mediawiki_get_page_info). If title not found, use mediawiki_resolve_title to handle typos and case sensitivity.

PARAMETERS:
- title: Page name (required)
- format: "wikitext" (default) or "html"

RETURNS: Page content in requested format. Large pages truncated at 25KB.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_list_pages",
		Method:   "ListPages",
		Title:    "List Pages",
		Category: "read",
		Description: `List wiki pages with optional prefix filter.

USE WHEN: User asks "list all pages", "show pages starting with API", "what pages exist".

NOT FOR: Finding pages by content (use mediawiki_search).

PARAMETERS:
- prefix: Filter by title prefix (optional)
- namespace: Namespace ID (default 0 = main)
- limit: Max pages (default 50)
- continue_from: Pagination token from previous response

RETURNS: Page titles and IDs.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_page_info",
		Method:   "GetPageInfo",
		Title:    "Get Page Info",
		Category: "read",
		Description: `Get page metadata without content.

USE WHEN: User asks "when was X last edited", "who created the FAQ", "is the page protected".

NOT FOR: Getting page content (use mediawiki_get_page). Not for full edit history (use mediawiki_get_revisions).

PARAMETERS:
- title: Page name (required)

RETURNS: Last edit timestamp, page size, protection status, creator.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_sections",
		Method:   "GetSections",
		Title:    "Get Sections",
		Category: "read",
		Description: `Get page section structure (TOC) or specific section content.

USE WHEN: User asks "what sections does X have", "show the table of contents", "get the Installation section".

NOT FOR: Full page content (use mediawiki_get_page).

PARAMETERS:
- title: Page name (required)
- section: Section index to retrieve content (optional; omit for TOC only)
- format: "wikitext" (default) or "html" (for section content)

RETURNS: Section headings with indices, or specific section content.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_related",
		Method:   "GetRelated",
		Title:    "Get Related Pages",
		Category: "read",
		Description: `Find pages related to a given page via links and categories.

USE WHEN: User asks "what pages are related to X", "show linked pages", "find associated content".

NOT FOR: Finding content-similar pages (use mediawiki_find_similar_pages for duplicate detection).

PARAMETERS:
- title: Page name (required)
- method: "categories" (default), "links", "backlinks", or "all"
- limit: Max related pages (default 20)

RETURNS: Related page titles with relationship type.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_images",
		Method:   "GetImages",
		Title:    "Get Images",
		Category: "read",
		Description: `Get all images and files used on a wiki page.

USE WHEN: User asks "what images are on X", "show files used in the article", "list media on this page".

PARAMETERS:
- title: Page name (required)
- limit: Max images (default 50)

RETURNS: Image titles, URLs, dimensions, and file sizes.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_parse",
		Method:   "Parse",
		Title:    "Parse Wikitext",
		Category: "read",
		Description: `Parse wikitext and return rendered HTML.

USE WHEN: User wants to preview wikitext rendering, test markup syntax.

PARAMETERS:
- wikitext: Wikitext content to parse (required)
- title: Context page title for link resolution (optional)

RETURNS: Rendered HTML output.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_wiki_info",
		Method:   "GetWikiInfo",
		Title:    "Get Wiki Info",
		Category: "read",
		Description: `Get information about the wiki itself.

USE WHEN: User asks "what wiki is this", "wiki statistics", "MediaWiki version".

PARAMETERS: None

RETURNS: Wiki name, version, statistics (pages, users, edits).`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// CATEGORY TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_list_categories",
		Method:   "ListCategories",
		Title:    "List Categories",
		Category: "categories",
		Description: `List all categories in the wiki.

USE WHEN: User asks "what categories exist", "show all categories", "list available categories".

NOT FOR: Getting pages in a category (use mediawiki_get_category_members).

PARAMETERS:
- prefix: Filter by category name prefix (optional)
- limit: Max categories (default 50)
- continue_from: Pagination token

RETURNS: Category names and page counts.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_category_members",
		Method:   "GetCategoryMembers",
		Title:    "Get Category Members",
		Category: "categories",
		Description: `Get all pages that belong to a specific category.

USE WHEN: User asks "show pages in Documentation category", "list all tutorials", "what's in Category:API".

NOT FOR: Listing categories themselves (use mediawiki_list_categories).

PARAMETERS:
- category: Category name without "Category:" prefix (required)
- type: Filter by type - "page", "subcat", "file", or all (default)
- limit: Max members (default 50)
- continue_from: Pagination token

RETURNS: Page titles in the category.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// HISTORY TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_get_recent_changes",
		Method:   "GetRecentChanges",
		Title:    "Get Recent Changes",
		Category: "history",
		Description: `Get recent changes across the entire wiki.

USE WHEN: User asks "what's been changed recently", "show wiki activity", "who's been editing".

NOT FOR: Single page history (use mediawiki_get_revisions). Not for user-specific edits (use mediawiki_get_user_contributions).

PARAMETERS:
- limit: Max changes (default 50)
- start, end: Time range (ISO 8601)
- namespace: Filter by namespace
- type: Filter by change type (edit, new, log)
- aggregate_by: Group results - "user", "page", or "type"

RETURNS: Recent changes with timestamps, users, and summaries. Aggregation returns counts.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_revisions",
		Method:   "GetRevisions",
		Title:    "Get Revisions",
		Category: "history",
		Description: `Get revision history for a specific page.

USE WHEN: User asks "who edited the FAQ", "show edit history of X", "when was this page last changed".

NOT FOR: Wiki-wide activity (use mediawiki_get_recent_changes). Not for comparing versions (use mediawiki_compare_revisions).

PARAMETERS:
- title: Page name (required)
- limit: Max revisions (default 50)
- start, end: Time range (ISO 8601)
- user: Filter by user

RETURNS: Revision list with timestamps, users, sizes, and edit summaries.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_compare_revisions",
		Method:   "CompareRevisions",
		Title:    "Compare Revisions",
		Category: "history",
		Description: `Compare two revisions and show the diff.

USE WHEN: User asks "what changed between versions", "show the diff", "compare old and new".

NOT FOR: Just listing revisions (use mediawiki_get_revisions). Not for comparing a topic across pages (use mediawiki_compare_topic).

PARAMETERS:
- from_rev: Source revision ID, OR
- from_title: Source page title (uses latest revision)
- to_rev: Target revision ID, OR
- to_title: Target page title

RETURNS: HTML-formatted diff showing additions (green) and deletions (red).`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_user_contributions",
		Method:   "GetUserContributions",
		Title:    "Get User Contributions",
		Category: "history",
		Description: `Get all edits made by a specific user.

USE WHEN: User asks "what did John edit", "show user's contributions", "list edits by admin".

NOT FOR: Page-specific history (use mediawiki_get_revisions). Not for wiki-wide activity (use mediawiki_get_recent_changes).

PARAMETERS:
- user: Username (required)
- limit: Max contributions (default 50)
- start, end: Time range (ISO 8601)
- namespace: Filter by namespace

RETURNS: List of pages edited with timestamps and summaries.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// LINK TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_get_external_links",
		Method:   "GetExternalLinks",
		Title:    "Get External Links",
		Category: "links",
		Description: `Get all external URLs from a wiki page.

USE WHEN: User asks "what external links are on X", "show outgoing URLs", "list http links".

NOT FOR: Incoming wiki links (use mediawiki_get_backlinks). Not for verifying links work (use mediawiki_check_links).

PARAMETERS:
- title: Page name (required)

RETURNS: List of external URLs on the page.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_external_links_batch",
		Method:   "GetExternalLinksBatch",
		Title:    "Get External Links (Batch)",
		Category: "links",
		Description: `Batch retrieve external URLs from multiple pages at once.

USE WHEN: User asks "get links from these 5 pages", "collect URLs from multiple articles".

NOT FOR: Single page (use mediawiki_get_external_links - more efficient).

PARAMETERS:
- titles: Array of page names (required, max 10)

RETURNS: External URLs grouped by source page.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_check_links",
		Method:   "CheckLinks",
		Title:    "Check Links",
		Category: "links",
		Description: `Verify external URL accessibility via HTTP requests.

USE WHEN: User asks "check if these links work", "find broken URLs", "verify external links".

NOT FOR: Finding broken internal wiki links (use mediawiki_find_broken_internal_links).

PARAMETERS:
- urls: Array of URLs to check (required, max 20)
- timeout: Request timeout in seconds (default 10)

RETURNS: URL status codes, response times, and broken link identification.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_backlinks",
		Method:   "GetBacklinks",
		Title:    "Get Backlinks",
		Category: "links",
		Description: `Get pages that link TO a specific page ("What links here").

USE WHEN: User asks "what links to X", "which pages reference the API", "show incoming links".

NOT FOR: Outgoing external links (use mediawiki_get_external_links).

PARAMETERS:
- title: Page name (required)
- namespace: Filter by namespace (optional)
- limit: Max backlinks (default 50)
- include_redirects: Include redirect pages (default false)

RETURNS: List of pages that link to the target page.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_find_broken_internal_links",
		Method:   "FindBrokenInternalLinks",
		Title:    "Find Broken Internal Links",
		Category: "links",
		Description: `Find internal wiki [[links]] pointing to non-existent pages.

USE WHEN: User asks "find broken wiki links", "check for dead internal links", "find [[links]] to missing pages".

NOT FOR: Checking external HTTP URLs (use mediawiki_check_links).

PARAMETERS:
- pages: Array of pages to scan (optional)
- category: Scan all pages in category (optional)
- limit: Max pages to scan (default 50)

RETURNS: Broken links with source page, line number, and context.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_find_orphaned_pages",
		Method:   "FindOrphanedPages",
		Title:    "Find Orphaned Pages",
		Category: "links",
		Description: `Find pages with no incoming links from other pages.

USE WHEN: User asks "find orphan pages", "which pages have no links", "find undiscoverable content".

PARAMETERS:
- namespace: Filter by namespace (default 0 = main)
- prefix: Filter by title prefix (optional)
- limit: Max orphans to return (default 50)

RETURNS: List of orphaned page titles.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// QUALITY TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_check_terminology",
		Method:   "CheckTerminology",
		Title:    "Check Terminology",
		Category: "quality",
		Description: `Scan pages for terminology violations against a glossary.

USE WHEN: User asks "check brand terminology", "find incorrect terms", "verify consistent naming".

PARAMETERS:
- pages: Array of pages to check (optional)
- category: Check all pages in category (optional)
- glossary_page: Wiki page with term mappings (default "Brand Terminology Glossary")
- exclude_code_blocks: Skip code blocks (default true)
- limit: Max pages (default 50)

RETURNS: Violations with page, line, wrong term, and correct term.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_check_translations",
		Method:   "CheckTranslations",
		Title:    "Check Translations",
		Category: "quality",
		Description: `Find pages missing translations in specified languages.

USE WHEN: User asks "which pages need German translation", "find missing translations", "check language coverage".

PARAMETERS:
- languages: Array of language codes (required, e.g., ["de", "fr", "es"])
- base_pages: Specific pages to check (optional)
- category: Check pages in category (optional)
- pattern: Naming pattern - "subpages" (Page/de), "suffixes" (Page (de)), or "prefixes" (de:Page)
- limit: Max pages (default 50)

RETURNS: Missing translations grouped by language.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_audit",
		Method:   "HealthAudit",
		Title:    "Wiki Health Audit",
		Category: "quality",
		Description: `Run comprehensive wiki health audit with multiple checks.

USE WHEN: User asks "run health check", "audit the wiki", "check wiki quality".

PARAMETERS:
- checks: Array of checks to run (default all):
  - "links": Broken internal links
  - "terminology": Glossary violations
  - "orphans": Unlinked pages
  - "activity": Recent changes
  - "external": Broken external links (slow)
- limit: Max items per check (default 20)

RETURNS: Health score (0-100), detailed results per check, and recommendations.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// DISCOVERY TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_find_similar_pages",
		Method:   "FindSimilarPages",
		Title:    "Find Similar Pages",
		Category: "discovery",
		Description: `Find pages with similar content (potential duplicates or overlaps).

USE WHEN: User asks "find similar pages", "are there duplicates", "what pages overlap with X".

NOT FOR: Finding related pages by links (use mediawiki_get_related).

PARAMETERS:
- page: Source page name (required)
- category: Limit search to category (optional)
- min_score: Minimum similarity threshold 0-1 (default 0.5)
- limit: Max similar pages (default 10)

RETURNS: Similar pages with similarity scores and linking recommendations.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_compare_topic",
		Method:   "CompareTopic",
		Title:    "Compare Topic",
		Category: "discovery",
		Description: `Compare how a topic is described across multiple pages.

USE WHEN: User asks "how is X described on different pages", "find inconsistencies about timeout", "compare definitions of Y".

NOT FOR: Comparing page revisions (use mediawiki_compare_revisions).

PARAMETERS:
- topic: Topic or term to compare (required)
- category: Limit to pages in category (optional)
- limit: Max pages to check (default 20)

RETURNS: Page mentions with context, detected value mismatches, and inconsistencies.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// USER TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_list_users",
		Method:   "ListUsers",
		Title:    "List Users",
		Category: "users",
		Description: `List wiki users with optional group filtering.

USE WHEN: User asks "who are the admins", "list all users", "show active editors".

PARAMETERS:
- group: Filter by group - "sysop" (admins), "bureaucrat", "bot" (optional)
- active_only: Only show recently active users (default false)
- limit: Max users (default 50)
- continue_from: Pagination token

RETURNS: User names, groups, edit counts, and registration dates.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// WRITE TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_edit_page",
		Method:   "EditPage",
		Title:    "Edit Page",
		Category: "write",
		Description: `Create new pages or rewrite entire page content.

USE WHEN: User says "create a new page", "rewrite the entire About page", "replace all content".

NOT FOR: Simple text changes (use mediawiki_find_replace). Not for formatting (use mediawiki_apply_formatting).

PARAMETERS:
- title: Page name (required)
- content: New page content (required)
- section: Edit specific section only (optional)
- summary: Edit summary (required for good practice)
- minor: Mark as minor edit (default false)
- bot: Mark as bot edit (default false)

WARNING: This overwrites entire page content unless section is specified.`,
		ReadOnly:    false,
		Destructive: true,
		Idempotent:  false,
		OpenWorld:   true,
	},
	{
		Name:     "mediawiki_find_replace",
		Method:   "FindReplace",
		Title:    "Find and Replace",
		Category: "write",
		Description: `PREFERRED for simple text changes on a single page.

USE WHEN: User says "replace X with Y", "fix the typo", "change the version number", "update the name".

NOT FOR: Creating/rewriting pages (use mediawiki_edit_page). Not for multi-page updates (use mediawiki_bulk_replace). Not for formatting (use mediawiki_apply_formatting).

PARAMETERS:
- title: Page name (required)
- find: Text to find (required)
- replace: Replacement text (required)
- all: Replace all occurrences (default false = first only)
- use_regex: Treat find as regex (default false)
- preview: Preview changes without saving (default true for safety)
- summary: Edit summary

RETURNS: Match count and preview of changes. Set preview=false to apply.`,
		ReadOnly:    false,
		Destructive: true,
		Idempotent:  false,
		OpenWorld:   true,
	},
	{
		Name:     "mediawiki_apply_formatting",
		Method:   "ApplyFormatting",
		Title:    "Apply Formatting",
		Category: "write",
		Description: `BEST for adding formatting markup to specific text.

USE WHEN: User says "strike out X", "cross out the name", "make X bold", "italicize Y", "mark as code".

NOT FOR: Replacing text (use mediawiki_find_replace).

PARAMETERS:
- title: Page name (required)
- text: Text to format (required)
- format: Formatting type (required):
  - "strikethrough": ~~text~~ (for removed/former items)
  - "bold": '''text'''
  - "italic": ''text''
  - "underline": <u>text</u>
  - "code": <code>text</code>
- all: Format all occurrences (default false)
- preview: Preview changes (default true)
- summary: Edit summary

RETURNS: Preview of formatting applied. Set preview=false to apply.`,
		ReadOnly:    false,
		Destructive: true,
		Idempotent:  false,
		OpenWorld:   true,
	},
	{
		Name:     "mediawiki_bulk_replace",
		Method:   "BulkReplace",
		Title:    "Bulk Replace",
		Category: "write",
		Description: `Update text across MULTIPLE pages at once.

USE WHEN: User says "update everywhere", "fix on all pages", "change brand name across docs", "update in all documentation".

NOT FOR: Single page changes (use mediawiki_find_replace - more efficient).

PARAMETERS:
- find: Text to find (required)
- replace: Replacement text (required)
- pages: Array of specific pages (optional)
- category: Update all pages in category (optional)
- use_regex: Treat find as regex (default false)
- preview: Preview changes (ALWAYS use true first!)
- limit: Max pages to update (default 50)
- summary: Edit summary

WARNING: Always use preview=true first to verify matches before applying.

RETURNS: Changes per page. Set preview=false to apply all changes.`,
		ReadOnly:    false,
		Destructive: true,
		Idempotent:  false,
		OpenWorld:   true,
	},
	{
		Name:     "mediawiki_upload_file",
		Method:   "UploadFile",
		Title:    "Upload File",
		Category: "write",
		Description: `Upload a file to the wiki from a URL.

USE WHEN: User says "upload this image", "add file to wiki", "import document".

PARAMETERS:
- filename: Target filename on wiki (required)
- file_url: Source URL to fetch file from (required)
- text: File description page content (optional)
- comment: Upload comment (optional)
- ignore_warnings: Overwrite existing file (default false)

RETURNS: Upload status and file page URL.

NOTE: Requires authentication. URL must be publicly accessible.`,
		ReadOnly:    false,
		Destructive: false,
		Idempotent:  false,
		OpenWorld:   true,
	},
}
