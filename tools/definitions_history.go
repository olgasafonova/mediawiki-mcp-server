package tools

// historyLinkTools contains the HISTORY and LINK tool specifications.
var historyLinkTools = []ToolSpec{
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
- limit: Max pages to scan (default 20)

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
}
