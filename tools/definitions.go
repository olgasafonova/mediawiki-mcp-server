package tools

// AllTools contains all tool specifications for the MediaWiki MCP server.
// Tools are organized by category for easier maintenance.
// Tool descriptions follow a structured format for optimal LLM tool selection:
// - USE WHEN: Natural language triggers
// - NOT FOR: Disambiguation from similar tools
// - PARAMETERS: Key arguments with defaults
// - RETURNS: What the tool returns
//
// The tool specs are grouped across sibling files (definitions_*.go) by
// category. AllTools concatenates the groups in their original order so that
// registration order and tool_count are unchanged.
var AllTools = concatToolSpecs(
	searchTools,
	readTools,
	historyLinkTools,
	qualityTools,
	writeTools,
)

// concatToolSpecs joins the per-category tool spec groups into a single slice,
// preserving the order in which the groups are passed.
func concatToolSpecs(groups ...[]ToolSpec) []ToolSpec {
	var all []ToolSpec
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}

// searchTools contains the SEARCH tool specifications.
var searchTools = []ToolSpec{
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
- limit: Max results (default 20)

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
}
