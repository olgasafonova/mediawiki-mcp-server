// MediaWiki MCP Server - A Model Context Protocol server for MediaWiki wikis
// Provides tools for searching, reading, and editing MediaWiki content
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"runtime/debug"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

// recoverPanic wraps a function with panic recovery and returns an error instead of crashing
func recoverPanic(logger *slog.Logger, operation string) {
	if r := recover(); r != nil {
		logger.Error("Panic recovered",
			"operation", operation,
			"panic", r,
			"stack", string(debug.Stack()))
	}
}

const (
	ServerName    = "mediawiki-mcp-server"
	ServerVersion = "1.5.0" // Added revision history, diff, and user contributions tools
)

func main() {
	// Configure logging to stderr (stdout is used for MCP protocol)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration from environment
	config, err := wiki.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create MediaWiki client
	client := wiki.NewClient(config, logger)

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Logger: logger,
		Instructions: `MediaWiki MCP Server provides tools and resources for interacting with MediaWiki wikis.

Resources (direct context access):
- wiki://page/{title}: Access wiki page content directly
- wiki://category/{name}: List pages in a category

Tools:
- mediawiki_search: Search for pages by text
- mediawiki_get_page: Get page content (wikitext or HTML)
- mediawiki_list_pages: List all pages with pagination
- mediawiki_list_categories: List all categories
- mediawiki_get_category_members: Get pages in a category
- mediawiki_get_page_info: Get metadata about a page
- mediawiki_edit_page: Create or edit a page (requires authentication)
- mediawiki_get_recent_changes: Get recent wiki changes
- mediawiki_get_external_links: Get external URLs from a page
- mediawiki_get_external_links_batch: Get external URLs from multiple pages
- mediawiki_check_links: Check if URLs are accessible (broken link detection)
- mediawiki_check_terminology: Check pages for terminology inconsistencies using a wiki glossary
- mediawiki_check_translations: Find pages missing in specific languages (localization gaps)
- mediawiki_find_broken_internal_links: Find internal wiki links pointing to non-existent pages
- mediawiki_find_orphaned_pages: Find pages with no incoming links
- mediawiki_get_backlinks: Get pages linking to a specific page ("What links here")
- mediawiki_get_revisions: Get revision history (edit log) for a page
- mediawiki_compare_revisions: Compare two revisions and get the diff
- mediawiki_get_user_contributions: Get edit history for a specific user

Configure via environment variables:
- MEDIAWIKI_URL: Wiki API URL (e.g., https://wiki.example.com/api.php)
- MEDIAWIKI_USERNAME: Bot username (for editing)
- MEDIAWIKI_PASSWORD: Bot password (for editing)`,
	})

	// Register all tools
	registerTools(server, client, logger)

	// Register resources for direct wiki page access
	registerResources(server, client, logger)

	// Run server on stdio transport
	ctx := context.Background()
	logger.Info("Starting MediaWiki MCP Server",
		"name", ServerName,
		"version", ServerVersion,
		"wiki_url", config.BaseURL,
	)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func registerTools(server *mcp.Server, client *wiki.Client, logger *slog.Logger) {
	// Search tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_search",
		Description: "Full-text search across wiki pages. Returns titles, snippets, and page IDs. Use 'offset' parameter for pagination when results exceed limit.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Search Wiki",
			ReadOnlyHint:  true,
			IdempotentHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.SearchArgs) (*mcp.CallToolResult, wiki.SearchResult, error) {
		defer recoverPanic(logger, "search")
		result, err := client.Search(ctx, args)
		if err != nil {
			return nil, wiki.SearchResult{}, fmt.Errorf("search failed: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_search",
			"query", args.Query,
			"results_count", len(result.Results),
			"total_hits", result.TotalHits,
		)
		return nil, result, nil
	})

	// Get page content
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_page",
		Description: "Retrieve wiki page content. Returns wikitext by default; set format='html' for rendered HTML. Large pages are truncated at 25KB.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get Page Content",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetPageArgs) (*mcp.CallToolResult, wiki.PageContent, error) {
		defer recoverPanic(logger, "get_page")
		result, err := client.GetPage(ctx, args)
		if err != nil {
			return nil, wiki.PageContent{}, fmt.Errorf("failed to get page: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_page",
			"title", args.Title,
			"format", args.Format,
			"output_chars", len(result.Content),
			"approx_tokens", len(result.Content)/4,
		)
		return nil, result, nil
	})

	// List pages
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_list_pages",
		Description: "List wiki pages with optional prefix filter. Returns page titles and IDs. Use 'continue_from' token from previous response for pagination.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "List Pages",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ListPagesArgs) (*mcp.CallToolResult, wiki.ListPagesResult, error) {
		defer recoverPanic(logger, "list_pages")
		result, err := client.ListPages(ctx, args)
		if err != nil {
			return nil, wiki.ListPagesResult{}, fmt.Errorf("failed to list pages: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_list_pages",
			"prefix", args.Prefix,
			"pages_returned", len(result.Pages),
			"has_more", result.HasMore,
		)
		return nil, result, nil
	})

	// List categories
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_list_categories",
		Description: "List all categories in the wiki with pagination.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "List Categories",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ListCategoriesArgs) (*mcp.CallToolResult, wiki.ListCategoriesResult, error) {
		defer recoverPanic(logger, "list_categories")
		result, err := client.ListCategories(ctx, args)
		if err != nil {
			return nil, wiki.ListCategoriesResult{}, fmt.Errorf("failed to list categories: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_list_categories",
			"prefix", args.Prefix,
			"categories_returned", len(result.Categories),
			"has_more", result.HasMore,
		)
		return nil, result, nil
	})

	// Get category members
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_category_members",
		Description: "Get all pages that belong to a specific category.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get Category Members",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CategoryMembersArgs) (*mcp.CallToolResult, wiki.CategoryMembersResult, error) {
		defer recoverPanic(logger, "get_category_members")
		result, err := client.GetCategoryMembers(ctx, args)
		if err != nil {
			return nil, wiki.CategoryMembersResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_category_members",
			"category", args.Category,
			"members_returned", len(result.Members),
			"has_more", result.HasMore,
		)
		return nil, result, nil
	})

	// Get page info
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_page_info",
		Description: "Get metadata about a page including last edit, size, and protection status.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get Page Info",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.PageInfoArgs) (*mcp.CallToolResult, wiki.PageInfo, error) {
		defer recoverPanic(logger, "get_page_info")
		result, err := client.GetPageInfo(ctx, args)
		if err != nil {
			return nil, wiki.PageInfo{}, fmt.Errorf("failed to get page info: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_page_info",
			"title", args.Title,
			"exists", result.Exists,
			"page_length", result.Length,
		)
		return nil, result, nil
	})

	// Edit page (requires authentication)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_edit_page",
		Description: "Create or update wiki page content. WARNING: Overwrites existing content unless 'section' is specified. Requires MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD environment variables.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Edit Page",
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
			IdempotentHint:  false,
			OpenWorldHint:   ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.EditPageArgs) (*mcp.CallToolResult, wiki.EditResult, error) {
		defer recoverPanic(logger, "edit_page")
		result, err := client.EditPage(ctx, args)
		if err != nil {
			return nil, wiki.EditResult{}, fmt.Errorf("failed to edit page: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_edit_page",
			"title", args.Title,
			"input_chars", len(args.Content),
			"approx_input_tokens", len(args.Content)/4,
			"success", result.Success,
			"new_page", result.NewPage,
		)
		return nil, result, nil
	})

	// Get recent changes
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_recent_changes",
		Description: "Get recent changes to the wiki. Useful for monitoring activity.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get Recent Changes",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.RecentChangesArgs) (*mcp.CallToolResult, wiki.RecentChangesResult, error) {
		defer recoverPanic(logger, "get_recent_changes")
		result, err := client.GetRecentChanges(ctx, args)
		if err != nil {
			return nil, wiki.RecentChangesResult{}, fmt.Errorf("failed to get recent changes: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_recent_changes",
			"changes_returned", len(result.Changes),
			"has_more", result.HasMore,
		)
		return nil, result, nil
	})

	// Parse wikitext
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_parse",
		Description: "Parse wikitext and return rendered HTML. Useful for previewing content before saving.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Parse Wikitext",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ParseArgs) (*mcp.CallToolResult, wiki.ParseResult, error) {
		defer recoverPanic(logger, "parse")
		result, err := client.Parse(ctx, args)
		if err != nil {
			return nil, wiki.ParseResult{}, fmt.Errorf("failed to parse wikitext: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_parse",
			"input_chars", len(args.Wikitext),
			"output_chars", len(result.HTML),
			"approx_input_tokens", len(args.Wikitext)/4,
			"approx_output_tokens", len(result.HTML)/4,
		)
		return nil, result, nil
	})

	// Get wiki info
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_wiki_info",
		Description: "Get information about the wiki including name, version, and statistics.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get Wiki Info",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.WikiInfoArgs) (*mcp.CallToolResult, wiki.WikiInfo, error) {
		defer recoverPanic(logger, "get_wiki_info")
		result, err := client.GetWikiInfo(ctx, args)
		if err != nil {
			return nil, wiki.WikiInfo{}, fmt.Errorf("failed to get wiki info: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_wiki_info",
			"site_name", result.SiteName,
		)
		return nil, result, nil
	})

	// Get external links from a page
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_external_links",
		Description: "Get all external links (URLs) from a wiki page. Useful for finding outbound links and checking for broken links.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get External Links",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetExternalLinksArgs) (*mcp.CallToolResult, wiki.ExternalLinksResult, error) {
		defer recoverPanic(logger, "get_external_links")
		result, err := client.GetExternalLinks(ctx, args)
		if err != nil {
			return nil, wiki.ExternalLinksResult{}, fmt.Errorf("failed to get external links: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_external_links",
			"title", args.Title,
			"links_found", result.Count,
		)
		return nil, result, nil
	})

	// Get external links from multiple pages
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_external_links_batch",
		Description: "Batch retrieve external URLs from up to 10 wiki pages. More efficient than multiple single-page calls. Returns links grouped by source page.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get External Links (Batch)",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetExternalLinksBatchArgs) (*mcp.CallToolResult, wiki.ExternalLinksBatchResult, error) {
		defer recoverPanic(logger, "get_external_links_batch")
		result, err := client.GetExternalLinksBatch(ctx, args)
		if err != nil {
			return nil, wiki.ExternalLinksBatchResult{}, fmt.Errorf("failed to get external links batch: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_external_links_batch",
			"pages_requested", len(args.Titles),
			"total_links_found", result.TotalLinks,
		)
		return nil, result, nil
	})

	// Check if URLs are broken
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_check_links",
		Description: "Verify URL accessibility via HTTP HEAD/GET requests. Returns status codes and identifies broken links. Max 20 URLs per call, 10s default timeout.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Check Links",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CheckLinksArgs) (*mcp.CallToolResult, wiki.CheckLinksResult, error) {
		defer recoverPanic(logger, "check_links")
		result, err := client.CheckLinks(ctx, args)
		if err != nil {
			return nil, wiki.CheckLinksResult{}, fmt.Errorf("failed to check links: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_check_links",
			"urls_checked", len(args.URLs),
			"broken_count", result.BrokenCount,
			"valid_count", result.ValidCount,
		)
		return nil, result, nil
	})

	// Check terminology consistency
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_check_terminology",
		Description: "Scan pages for terminology violations using a wiki-hosted glossary table. Specify pages directly or scan entire category. Default glossary: 'Brand Terminology Glossary'.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Check Terminology",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CheckTerminologyArgs) (*mcp.CallToolResult, wiki.CheckTerminologyResult, error) {
		defer recoverPanic(logger, "check_terminology")
		result, err := client.CheckTerminology(ctx, args)
		if err != nil {
			return nil, wiki.CheckTerminologyResult{}, fmt.Errorf("failed to check terminology: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_check_terminology",
			"pages_checked", result.PagesChecked,
			"issues_found", result.IssuesFound,
			"terms_loaded", result.TermsLoaded,
		)
		return nil, result, nil
	})

	// Check translations (find missing localized pages)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_check_translations",
		Description: "Find pages missing in specific languages. Check if base pages have translations in all required languages. Supports different naming patterns: subpages (Page/lang), suffixes (Page (lang)), or prefixes (lang:Page).",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Check Translations",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CheckTranslationsArgs) (*mcp.CallToolResult, wiki.CheckTranslationsResult, error) {
		defer recoverPanic(logger, "check_translations")
		result, err := client.CheckTranslations(ctx, args)
		if err != nil {
			return nil, wiki.CheckTranslationsResult{}, fmt.Errorf("failed to check translations: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_check_translations",
			"pages_checked", result.PagesChecked,
			"languages_checked", len(result.LanguagesChecked),
			"missing_count", result.MissingCount,
		)
		return nil, result, nil
	})

	// Find broken internal links
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_find_broken_internal_links",
		Description: "Find internal wiki links that point to non-existent pages. Scans page content for [[links]] and verifies each target exists. Returns broken links with line numbers and context.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Find Broken Internal Links",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.FindBrokenInternalLinksArgs) (*mcp.CallToolResult, wiki.FindBrokenInternalLinksResult, error) {
		defer recoverPanic(logger, "find_broken_internal_links")
		result, err := client.FindBrokenInternalLinks(ctx, args)
		if err != nil {
			return nil, wiki.FindBrokenInternalLinksResult{}, fmt.Errorf("failed to find broken internal links: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_find_broken_internal_links",
			"pages_checked", result.PagesChecked,
			"broken_count", result.BrokenCount,
		)
		return nil, result, nil
	})

	// Find orphaned pages
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_find_orphaned_pages",
		Description: "Find pages with no incoming links from other pages. These 'lonely pages' may be hard to discover through normal wiki navigation.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Find Orphaned Pages",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.FindOrphanedPagesArgs) (*mcp.CallToolResult, wiki.FindOrphanedPagesResult, error) {
		defer recoverPanic(logger, "find_orphaned_pages")
		result, err := client.FindOrphanedPages(ctx, args)
		if err != nil {
			return nil, wiki.FindOrphanedPagesResult{}, fmt.Errorf("failed to find orphaned pages: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_find_orphaned_pages",
			"total_checked", result.TotalChecked,
			"orphaned_count", result.OrphanedCount,
		)
		return nil, result, nil
	})

	// Get backlinks ("What links here")
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_backlinks",
		Description: "Get pages that link to a specific page ('What links here'). Useful for understanding page relationships and impact of changes.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get Backlinks",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetBacklinksArgs) (*mcp.CallToolResult, wiki.GetBacklinksResult, error) {
		defer recoverPanic(logger, "get_backlinks")
		result, err := client.GetBacklinks(ctx, args)
		if err != nil {
			return nil, wiki.GetBacklinksResult{}, fmt.Errorf("failed to get backlinks: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_backlinks",
			"title", args.Title,
			"backlinks_found", result.Count,
			"has_more", result.HasMore,
		)
		return nil, result, nil
	})

	// Get revision history
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_revisions",
		Description: "Get revision history (edit log) for a page. Shows who edited the page, when, and edit summaries. Useful for tracking changes and reviewing history.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get Revisions",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetRevisionsArgs) (*mcp.CallToolResult, wiki.GetRevisionsResult, error) {
		defer recoverPanic(logger, "get_revisions")
		result, err := client.GetRevisions(ctx, args)
		if err != nil {
			return nil, wiki.GetRevisionsResult{}, fmt.Errorf("failed to get revisions: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_revisions",
			"title", args.Title,
			"revisions_found", result.Count,
			"has_more", result.HasMore,
		)
		return nil, result, nil
	})

	// Compare revisions (diff)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_compare_revisions",
		Description: "Compare two revisions and get the diff. Can compare by revision IDs or page titles (uses latest revision). Returns HTML-formatted diff showing additions and deletions.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Compare Revisions",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CompareRevisionsArgs) (*mcp.CallToolResult, wiki.CompareRevisionsResult, error) {
		defer recoverPanic(logger, "compare_revisions")
		result, err := client.CompareRevisions(ctx, args)
		if err != nil {
			return nil, wiki.CompareRevisionsResult{}, fmt.Errorf("failed to compare revisions: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_compare_revisions",
			"from_rev", result.FromRevID,
			"to_rev", result.ToRevID,
		)
		return nil, result, nil
	})

	// Get user contributions
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_user_contributions",
		Description: "Get edit history for a specific user. Shows all pages they've edited, with timestamps and edit summaries. Useful for reviewing user activity.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Get User Contributions",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetUserContributionsArgs) (*mcp.CallToolResult, wiki.GetUserContributionsResult, error) {
		defer recoverPanic(logger, "get_user_contributions")
		result, err := client.GetUserContributions(ctx, args)
		if err != nil {
			return nil, wiki.GetUserContributionsResult{}, fmt.Errorf("failed to get user contributions: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_get_user_contributions",
			"user", args.User,
			"contributions_found", result.Count,
			"has_more", result.HasMore,
		)
		return nil, result, nil
	})
}

// registerResources adds MCP resources for direct wiki page access
func registerResources(server *mcp.Server, client *wiki.Client, logger *slog.Logger) {
	// Resource template for wiki pages
	// URI format: wiki://page/{title}
	// Example: wiki://page/Main_Page
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "wiki://page/{title}",
		Name:        "Wiki Page",
		Description: "Access MediaWiki page content directly. Use URL-encoded page titles (e.g., 'Main_Page' or 'Category%3AHelp').",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic recovered in resource handler",
					"panic", r,
					"stack", string(debug.Stack()))
			}
		}()

		// Extract page title from URI
		// URI format: wiki://page/{title}
		uri := req.Params.URI
		if !strings.HasPrefix(uri, "wiki://page/") {
			return nil, mcp.ResourceNotFoundError(uri)
		}

		encodedTitle := strings.TrimPrefix(uri, "wiki://page/")
		title, err := url.PathUnescape(encodedTitle)
		if err != nil {
			return nil, fmt.Errorf("invalid page title encoding: %w", err)
		}

		if title == "" {
			return nil, fmt.Errorf("page title cannot be empty")
		}

		// Fetch page content (wikitext format for better context)
		result, err := client.GetPage(ctx, wiki.GetPageArgs{
			Title:  title,
			Format: "wikitext",
		})
		if err != nil {
			logger.Warn("Failed to read wiki page resource",
				"uri", uri,
				"title", title,
				"error", err,
			)
			return nil, mcp.ResourceNotFoundError(uri)
		}

		logger.Info("Resource accessed",
			"uri", uri,
			"title", result.Title,
			"page_id", result.PageID,
		)

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      uri,
				MIMEType: "text/plain",
				Text:     result.Content,
			}},
		}, nil
	})

	// Resource template for wiki categories
	// URI format: wiki://category/{name}
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "wiki://category/{name}",
		Name:        "Wiki Category",
		Description: "List pages in a MediaWiki category. Use URL-encoded category names without 'Category:' prefix.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic recovered in category resource handler",
					"panic", r,
					"stack", string(debug.Stack()))
			}
		}()

		uri := req.Params.URI
		if !strings.HasPrefix(uri, "wiki://category/") {
			return nil, mcp.ResourceNotFoundError(uri)
		}

		encodedName := strings.TrimPrefix(uri, "wiki://category/")
		name, err := url.PathUnescape(encodedName)
		if err != nil {
			return nil, fmt.Errorf("invalid category name encoding: %w", err)
		}

		if name == "" {
			return nil, fmt.Errorf("category name cannot be empty")
		}

		result, err := client.GetCategoryMembers(ctx, wiki.CategoryMembersArgs{
			Category: name,
			Limit:    100,
		})
		if err != nil {
			logger.Warn("Failed to read wiki category resource",
				"uri", uri,
				"category", name,
				"error", err,
			)
			return nil, mcp.ResourceNotFoundError(uri)
		}

		// Format as simple text list
		var content strings.Builder
		content.WriteString(fmt.Sprintf("Category: %s\n", name))
		content.WriteString(fmt.Sprintf("Pages: %d\n\n", len(result.Members)))
		for _, page := range result.Members {
			content.WriteString(fmt.Sprintf("- %s\n", page.Title))
		}
		if result.HasMore {
			content.WriteString("\n[More pages available...]")
		}

		logger.Info("Category resource accessed",
			"uri", uri,
			"category", name,
			"pages", len(result.Members),
		)

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      uri,
				MIMEType: "text/plain",
				Text:     content.String(),
			}},
		}, nil
	})
}

func ptr[T any](v T) *T {
	return &v
}
