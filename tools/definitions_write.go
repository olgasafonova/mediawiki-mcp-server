package tools

// writeTools contains the WRITE, BATCH, COMPOSITE, PAGE MANAGEMENT, and WIKI
// HYGIENE tool specifications.
var writeTools = []ToolSpec{
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

RETURNS: Includes revision ID, diff URL, and undo instructions.

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

RETURNS: Match count and preview of changes. Set preview=false to apply. Includes revision ID, diff URL, and undo instructions.`,
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

RETURNS: Preview of formatting applied. Set preview=false to apply. Includes revision ID, diff URL, and undo instructions.`,
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

RETURNS: Changes per page. Set preview=false to apply all changes. Includes revision ID, diff URL, and undo instructions.`,
		ReadOnly:    false,
		Destructive: true,
		Idempotent:  false,
		OpenWorld:   true,
	},
	// ==========================================================================
	// BATCH TOOLS (Performance)
	// ==========================================================================
	{
		Name:     "mediawiki_batch_get_pages",
		Method:   "GetPagesBatch",
		Title:    "Batch Get Pages",
		Category: "read",
		Description: `Retrieve content from MULTIPLE pages in a single API call.

USE WHEN: You need content from 2+ pages. Much faster than individual mediawiki_get_page calls.

NOT FOR: Single page (use mediawiki_get_page). Not for metadata only (use mediawiki_batch_get_pages_info).

PARAMETERS:
- titles: Array of page titles (required, max 50)
- format: "wikitext" (default) or "html"

RETURNS: Page content for each title, with exists/missing status. Missing pages are reported, not errors.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_batch_get_pages_info",
		Method:   "GetPagesInfoBatch",
		Title:    "Batch Get Pages Info",
		Category: "read",
		Description: `Get metadata for MULTIPLE pages in a single API call.

USE WHEN: You need info (last edit, size, categories) for 2+ pages without their content.

NOT FOR: Single page (use mediawiki_get_page_info). Not for content (use mediawiki_batch_get_pages).

PARAMETERS:
- titles: Array of page titles (required, max 50)

RETURNS: Metadata (size, last edit, categories, protection) per page. Missing pages reported with exists=false.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// COMPOSITE TOOLS (UX)
	// ==========================================================================
	{
		Name:     "mediawiki_search_and_read",
		Method:   "SearchAndRead",
		Title:    "Search and Read",
		Category: "search",
		Description: `Search wiki AND read the top result(s) in a single call.

USE WHEN: User asks a question about wiki content. This is the fastest path from question to answer — eliminates the search-then-read round trip.

NOT FOR: Known page titles (use mediawiki_get_page directly). Not for listing search results without reading (use mediawiki_search).

PARAMETERS:
- query: Search text (required)
- read_count: How many top results to read (default 1, max 5)
- format: "wikitext" (default) or "html"

RETURNS: Full content of top result(s) plus remaining search hits as summaries.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},
	{
		Name:     "mediawiki_get_page_summary",
		Method:   "GetPageSummary",
		Title:    "Get Page Summary",
		Category: "read",
		Description: `Get lead section + key metadata without loading the full page.

USE WHEN: User asks "what is X about", "quick overview of X", "summarize the X page". Much lighter than mediawiki_get_page for large pages.

NOT FOR: Full page content (use mediawiki_get_page). Not for specific sections (use mediawiki_get_sections with section parameter).

PARAMETERS:
- title: Page name (required)

RETURNS: Lead section (intro before first heading), page size, categories, section list, last edit timestamp.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	// ==========================================================================
	// PAGE MANAGEMENT TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_move_page",
		Method:   "MovePage",
		Title:    "Move Page",
		Category: "write",
		Description: `Move (rename) a wiki page. Creates a redirect from the old title.

USE WHEN: User says "rename the page", "move X to Y", "change the page title".

NOT FOR: Editing page content (use mediawiki_edit_page or mediawiki_find_replace).

PARAMETERS:
- from: Current page title (required)
- to: New page title (required)
- reason: Reason for the move (optional)
- no_redirect: Don't create redirect from old title (default false)
- move_talk: Also move the talk page (default true)
- move_subpages: Also move subpages (default false)

RETURNS: Includes revision ID, diff URL, and undo instructions.

WARNING: Requires move permissions. Creates a redirect from the old title by default.`,
		ReadOnly:    false,
		Destructive: true,
		Idempotent:  false,
		OpenWorld:   true,
	},
	{
		Name:     "mediawiki_manage_categories",
		Method:   "ManageCategories",
		Title:    "Manage Categories",
		Category: "write",
		Description: `Add or remove categories from a page without editing the full content.

USE WHEN: User says "add category X to this page", "remove this from category Y", "categorize this page".

NOT FOR: Listing categories (use mediawiki_list_categories). Not for viewing category members (use mediawiki_get_category_members).

PARAMETERS:
- title: Page name (required)
- add: Array of category names to add (without "Category:" prefix)
- remove: Array of category names to remove (without "Category:" prefix)
- preview: Preview changes without saving (default false)
- summary: Edit summary

RETURNS: Which categories were added, removed, already present, or not found. Includes revision ID, diff URL, and undo instructions.`,
		ReadOnly:    false,
		Destructive: true, // HG-3: edits page content (changes category set via EditPage)
		Idempotent:  false,
		OpenWorld:   true,
	},

	// ==========================================================================
	// WIKI HYGIENE TOOLS
	// ==========================================================================
	{
		Name:     "mediawiki_get_stale_pages",
		Method:   "GetStalePages",
		Title:    "Get Stale Pages",
		Category: "quality",
		Description: `Find pages that haven't been edited in a specified number of days.

USE WHEN: User asks "find outdated pages", "which pages need review", "show stale content", "wiki hygiene check".

NOT FOR: Recent activity (use mediawiki_get_recent_changes). Not for orphaned pages (use mediawiki_find_orphaned_pages).

PARAMETERS:
- days: Staleness threshold in days (default 90)
- category: Limit to pages in this category (optional)
- namespace: Namespace to check (default 0 = main)
- limit: Max pages to return (default 50, max 200)

RETURNS: Stale pages sorted by last edit (oldest first), with days since edit and last editor.`,
		ReadOnly:   true,
		Idempotent: true,
		OpenWorld:  true,
	},

	{
		Name:     "mediawiki_upload_file",
		Method:   "UploadFile",
		Title:    "Upload File",
		Category: "write",
		Description: `Upload a file to the wiki from a URL or local path.

USE WHEN: User says "upload this image", "add file to wiki", "import document".

PARAMETERS:
- filename: Target filename on wiki (required)
- file_url: Source URL to fetch file from (one of file_url or file_path required)
- file_path: Local file path (alternative to file_url)
- text: File description page content (optional)
- comment: Upload comment (optional)
- ignore_warnings: Overwrite existing file (default false)

RETURNS: Upload status and file page URL. Includes revision ID, diff URL, and undo instructions.

NOTE: Requires authentication. URL must be publicly accessible.

SECURITY: Source URL must be on the MEDIAWIKI_UPLOAD_ALLOWED_DOMAINS env-var allowlist (fail-closed when unset). Private/internal IPs are blocked unconditionally. ignore_warnings=true overwrites existing files; the destructive-hint annotation is set so hosts that gate destructive operations will prompt before this runs.`,
		ReadOnly:    false,
		Destructive: true, // HG-3: writes attacker-controllable bytes to the wiki and (with ignore_warnings) overwrites existing files
		Idempotent:  false,
		OpenWorld:   true,
	},
}
