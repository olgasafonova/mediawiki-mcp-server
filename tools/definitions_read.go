package tools

// readTools contains the READ and CATEGORY tool specifications.
var readTools = []ToolSpec{
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
}
