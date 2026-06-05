package tools

// qualityTools contains the QUALITY, DISCOVERY, and USER tool specifications.
var qualityTools = []ToolSpec{
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
- limit: Max pages (default 10)

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
- min_score: Minimum similarity threshold 0-1 (default 0.1)
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
}
