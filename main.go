// MediaWiki MCP Server - A Model Context Protocol server for MediaWiki wikis
// Provides tools for searching, reading, and editing MediaWiki content
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"runtime/debug"

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
	ServerVersion = "1.3.0" // Added content quality tools: translations, broken internal links, orphaned pages, backlinks
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
		Instructions: `MediaWiki MCP Server provides tools for interacting with MediaWiki wikis.

Available tools:
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

Configure via environment variables:
- MEDIAWIKI_URL: Wiki API URL (e.g., https://wiki.example.com/api.php)
- MEDIAWIKI_USERNAME: Bot username (for editing)
- MEDIAWIKI_PASSWORD: Bot password (for editing)`,
	})

	// Register all tools
	registerTools(server, client, logger)

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
		Description: "Search for pages in the wiki by text. Returns matching page titles and snippets.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Search Wiki",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.SearchArgs) (*mcp.CallToolResult, wiki.SearchResult, error) {
		defer recoverPanic(logger, "search")
		result, err := client.Search(ctx, args)
		if err != nil {
			return nil, wiki.SearchResult{}, fmt.Errorf("search failed: %w", err)
		}
		return nil, result, nil
	})

	// Get page content
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_page",
		Description: "Get the content of a wiki page. Can return wikitext source or parsed HTML.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Page Content",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetPageArgs) (*mcp.CallToolResult, wiki.PageContent, error) {
		defer recoverPanic(logger, "get_page")
		result, err := client.GetPage(ctx, args)
		if err != nil {
			return nil, wiki.PageContent{}, fmt.Errorf("failed to get page: %w", err)
		}
		return nil, result, nil
	})

	// List pages
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_list_pages",
		Description: "List all pages in the wiki with pagination. Use 'continue_from' for pagination.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "List Pages",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ListPagesArgs) (*mcp.CallToolResult, wiki.ListPagesResult, error) {
		defer recoverPanic(logger, "list_pages")
		result, err := client.ListPages(ctx, args)
		if err != nil {
			return nil, wiki.ListPagesResult{}, fmt.Errorf("failed to list pages: %w", err)
		}
		return nil, result, nil
	})

	// List categories
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_list_categories",
		Description: "List all categories in the wiki with pagination.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "List Categories",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ListCategoriesArgs) (*mcp.CallToolResult, wiki.ListCategoriesResult, error) {
		defer recoverPanic(logger, "list_categories")
		result, err := client.ListCategories(ctx, args)
		if err != nil {
			return nil, wiki.ListCategoriesResult{}, fmt.Errorf("failed to list categories: %w", err)
		}
		return nil, result, nil
	})

	// Get category members
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_category_members",
		Description: "Get all pages that belong to a specific category.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Category Members",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CategoryMembersArgs) (*mcp.CallToolResult, wiki.CategoryMembersResult, error) {
		defer recoverPanic(logger, "get_category_members")
		result, err := client.GetCategoryMembers(ctx, args)
		if err != nil {
			return nil, wiki.CategoryMembersResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		return nil, result, nil
	})

	// Get page info
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_page_info",
		Description: "Get metadata about a page including last edit, size, and protection status.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Page Info",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.PageInfoArgs) (*mcp.CallToolResult, wiki.PageInfo, error) {
		defer recoverPanic(logger, "get_page_info")
		result, err := client.GetPageInfo(ctx, args)
		if err != nil {
			return nil, wiki.PageInfo{}, fmt.Errorf("failed to get page info: %w", err)
		}
		return nil, result, nil
	})

	// Edit page (requires authentication)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_edit_page",
		Description: "Create or edit a wiki page. Requires bot password authentication. Set MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD environment variables.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Edit Page",
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
			IdempotentHint:  false,
			OpenWorldHint:   ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.EditPageArgs) (*mcp.CallToolResult, wiki.EditResult, error) {
		defer recoverPanic(logger, "edit_page")
		result, err := client.EditPage(ctx, args)
		if err != nil {
			return nil, wiki.EditResult{}, fmt.Errorf("failed to edit page: %w", err)
		}
		return nil, result, nil
	})

	// Get recent changes
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_recent_changes",
		Description: "Get recent changes to the wiki. Useful for monitoring activity.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Recent Changes",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.RecentChangesArgs) (*mcp.CallToolResult, wiki.RecentChangesResult, error) {
		defer recoverPanic(logger, "get_recent_changes")
		result, err := client.GetRecentChanges(ctx, args)
		if err != nil {
			return nil, wiki.RecentChangesResult{}, fmt.Errorf("failed to get recent changes: %w", err)
		}
		return nil, result, nil
	})

	// Parse wikitext
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_parse",
		Description: "Parse wikitext and return rendered HTML. Useful for previewing content before saving.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Parse Wikitext",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ParseArgs) (*mcp.CallToolResult, wiki.ParseResult, error) {
		defer recoverPanic(logger, "parse")
		result, err := client.Parse(ctx, args)
		if err != nil {
			return nil, wiki.ParseResult{}, fmt.Errorf("failed to parse wikitext: %w", err)
		}
		return nil, result, nil
	})

	// Get wiki info
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_wiki_info",
		Description: "Get information about the wiki including name, version, and statistics.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Wiki Info",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.WikiInfoArgs) (*mcp.CallToolResult, wiki.WikiInfo, error) {
		defer recoverPanic(logger, "get_wiki_info")
		result, err := client.GetWikiInfo(ctx, args)
		if err != nil {
			return nil, wiki.WikiInfo{}, fmt.Errorf("failed to get wiki info: %w", err)
		}
		return nil, result, nil
	})

	// Get external links from a page
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_external_links",
		Description: "Get all external links (URLs) from a wiki page. Useful for finding outbound links and checking for broken links.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get External Links",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetExternalLinksArgs) (*mcp.CallToolResult, wiki.ExternalLinksResult, error) {
		defer recoverPanic(logger, "get_external_links")
		result, err := client.GetExternalLinks(ctx, args)
		if err != nil {
			return nil, wiki.ExternalLinksResult{}, fmt.Errorf("failed to get external links: %w", err)
		}
		return nil, result, nil
	})

	// Get external links from multiple pages
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_external_links_batch",
		Description: "Get external links from multiple wiki pages in one call (max 10 pages). Returns links for each page separately.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get External Links (Batch)",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetExternalLinksBatchArgs) (*mcp.CallToolResult, wiki.ExternalLinksBatchResult, error) {
		defer recoverPanic(logger, "get_external_links_batch")
		result, err := client.GetExternalLinksBatch(ctx, args)
		if err != nil {
			return nil, wiki.ExternalLinksBatchResult{}, fmt.Errorf("failed to get external links batch: %w", err)
		}
		return nil, result, nil
	})

	// Check if URLs are broken
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_check_links",
		Description: "Check if URLs are accessible (broken link detection). Checks up to 20 URLs and reports their status. Use this after getting external links to find broken ones.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Check Links",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CheckLinksArgs) (*mcp.CallToolResult, wiki.CheckLinksResult, error) {
		defer recoverPanic(logger, "check_links")
		result, err := client.CheckLinks(ctx, args)
		if err != nil {
			return nil, wiki.CheckLinksResult{}, fmt.Errorf("failed to check links: %w", err)
		}
		return nil, result, nil
	})

	// Check terminology consistency
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_check_terminology",
		Description: "Check wiki pages for terminology inconsistencies. Uses a glossary wiki page (default: 'Brand Terminology Glossary') with a table of incorrect/correct terms. Specify pages directly or scan an entire category.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Check Terminology",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CheckTerminologyArgs) (*mcp.CallToolResult, wiki.CheckTerminologyResult, error) {
		defer recoverPanic(logger, "check_terminology")
		result, err := client.CheckTerminology(ctx, args)
		if err != nil {
			return nil, wiki.CheckTerminologyResult{}, fmt.Errorf("failed to check terminology: %w", err)
		}
		return nil, result, nil
	})

	// Check translations (find missing localized pages)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_check_translations",
		Description: "Find pages missing in specific languages. Check if base pages have translations in all required languages. Supports different naming patterns: subpages (Page/lang), suffixes (Page (lang)), or prefixes (lang:Page).",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Check Translations",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.CheckTranslationsArgs) (*mcp.CallToolResult, wiki.CheckTranslationsResult, error) {
		defer recoverPanic(logger, "check_translations")
		result, err := client.CheckTranslations(ctx, args)
		if err != nil {
			return nil, wiki.CheckTranslationsResult{}, fmt.Errorf("failed to check translations: %w", err)
		}
		return nil, result, nil
	})

	// Find broken internal links
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_find_broken_internal_links",
		Description: "Find internal wiki links that point to non-existent pages. Scans page content for [[links]] and verifies each target exists. Returns broken links with line numbers and context.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Find Broken Internal Links",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.FindBrokenInternalLinksArgs) (*mcp.CallToolResult, wiki.FindBrokenInternalLinksResult, error) {
		defer recoverPanic(logger, "find_broken_internal_links")
		result, err := client.FindBrokenInternalLinks(ctx, args)
		if err != nil {
			return nil, wiki.FindBrokenInternalLinksResult{}, fmt.Errorf("failed to find broken internal links: %w", err)
		}
		return nil, result, nil
	})

	// Find orphaned pages
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_find_orphaned_pages",
		Description: "Find pages with no incoming links from other pages. These 'lonely pages' may be hard to discover through normal wiki navigation.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Find Orphaned Pages",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.FindOrphanedPagesArgs) (*mcp.CallToolResult, wiki.FindOrphanedPagesResult, error) {
		defer recoverPanic(logger, "find_orphaned_pages")
		result, err := client.FindOrphanedPages(ctx, args)
		if err != nil {
			return nil, wiki.FindOrphanedPagesResult{}, fmt.Errorf("failed to find orphaned pages: %w", err)
		}
		return nil, result, nil
	})

	// Get backlinks ("What links here")
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_get_backlinks",
		Description: "Get pages that link to a specific page ('What links here'). Useful for understanding page relationships and impact of changes.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Backlinks",
			ReadOnlyHint: true,
			OpenWorldHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.GetBacklinksArgs) (*mcp.CallToolResult, wiki.GetBacklinksResult, error) {
		defer recoverPanic(logger, "get_backlinks")
		result, err := client.GetBacklinks(ctx, args)
		if err != nil {
			return nil, wiki.GetBacklinksResult{}, fmt.Errorf("failed to get backlinks: %w", err)
		}
		return nil, result, nil
	})
}

func ptr[T any](v T) *T {
	return &v
}
