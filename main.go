// MediaWiki MCP Server - A Model Context Protocol server for MediaWiki wikis
// Provides tools for searching, reading, and editing MediaWiki content
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/mcp-servercard-go/servercard"
	"github.com/olgasafonova/mediawiki-mcp-server/converter"
	"github.com/olgasafonova/mediawiki-mcp-server/tools"
	"github.com/olgasafonova/mediawiki-mcp-server/tracing"
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
	ServerVersion = "1.28.1"
)

// =============================================================================
// Markdown Converter Types (not a wiki.Client method)
// =============================================================================

// ConvertMarkdownArgs holds parameters for the mediawiki_convert_markdown tool
type ConvertMarkdownArgs struct {
	// Markdown is the source text to convert (required)
	Markdown string `json:"markdown" jsonschema:"The Markdown text to convert to MediaWiki markup"`

	// Theme selects the color scheme: "tieto", "neutral" (default), or "dark"
	Theme string `json:"theme,omitempty" jsonschema:"Color theme: 'tieto' (brand colors), 'neutral' (no styling, default), or 'dark' (dark mode)"`

	// AddCSS includes CSS styling block in output for branded appearance
	AddCSS *bool `json:"add_css,omitempty" jsonschema:"Include CSS styling block for branded appearance"`

	// ReverseChangelog reorders changelog entries newest-first
	ReverseChangelog *bool `json:"reverse_changelog,omitempty" jsonschema:"Reorder changelog entries with newest first"`

	// PrettifyChecks replaces plain checkmarks (✓) with emoji (✅)
	PrettifyChecks *bool `json:"prettify_checks,omitempty" jsonschema:"Replace plain checkmarks with emoji ✅"`
}

// ConvertMarkdownResult contains the conversion output
type ConvertMarkdownResult struct {
	// Wikitext is the converted MediaWiki markup
	Wikitext string `json:"wikitext"`

	// InputLength is the character count of input Markdown
	InputLength int `json:"input_length"`

	// OutputLength is the character count of output wikitext
	OutputLength int `json:"output_length"`

	// ThemeUsed indicates which theme was applied
	ThemeUsed string `json:"theme_used"`

	// AvailableThemes lists all supported themes
	AvailableThemes []converter.ThemeInfo `json:"available_themes"`
}

// =============================================================================
// Security Middleware for HTTP Transport
// =============================================================================

func main() {
	// Parse command-line flags
	httpAddr := flag.String("http", "", "HTTP address to listen on (e.g., :8080 or 127.0.0.1:8080). If empty, uses stdio transport.")
	bearerToken := flag.String("token", "", "Bearer token for HTTP authentication. Can also use MCP_AUTH_TOKEN env var.")
	allowedOrigins := flag.String("origins", "", "Comma-separated allowed origins for CORS (e.g., 'https://chat.openai.com,https://n8n.example.com'). Empty allows all.")
	rateLimit := flag.Int("rate-limit", 60, "Maximum requests per minute per IP (0 = unlimited)")
	trustedProxies := flag.String("trusted-proxies", "", "Comma-separated trusted proxy IPs/CIDRs (e.g., '10.0.0.0/8,192.168.1.1'). Required to trust X-Forwarded-For header.")
	flag.Parse()

	// Configure logging to stderr (stdout is used for MCP protocol in stdio mode)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Initialize OpenTelemetry tracing
	tracingConfig := tracing.DefaultConfig()
	tracingConfig.ServiceVersion = ServerVersion
	shutdownTracing, err := tracing.Setup(context.Background(), tracingConfig)
	if err != nil {
		logger.Warn("Failed to initialize tracing", "error", err)
	} else if tracingConfig.Enabled {
		defer func() { _ = shutdownTracing(context.Background()) }()
		logger.Info("OpenTelemetry tracing enabled",
			"endpoint", tracingConfig.OTLPEndpoint,
			"service", tracingConfig.ServiceName)
	}

	// Load configuration from environment.
	// Uses LoadConfigOrUnconfigured so the server starts even without MEDIAWIKI_URL,
	// allowing MCP registries (Glama, Smithery) to inspect tool definitions.
	// Tool calls will return a clear error if the wiki URL is not configured.
	config, err := wiki.LoadConfigOrUnconfigured()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	if !config.IsConfigured() {
		logger.Warn("MEDIAWIKI_URL not set. Server will start in inspection mode: tools are listed but calls will fail until configured.")
	}

	// Create MediaWiki client
	client := wiki.NewClient(config, logger)

	// Configure audit logging if MEDIAWIKI_AUDIT_LOG is set
	if auditLogPath := os.Getenv("MEDIAWIKI_AUDIT_LOG"); auditLogPath != "" {
		auditLogger, err := wiki.NewFileAuditLogger(auditLogPath, logger)
		if err != nil {
			logger.Warn("Failed to create audit logger", "path", auditLogPath, "error", err)
		} else {
			client.SetAuditLogger(auditLogger)
			logger.Info("Audit logging enabled", "path", auditLogPath)
		}
	}

	// Get bearer token from flag or environment
	authToken := *bearerToken
	if authToken == "" {
		authToken = os.Getenv("MCP_AUTH_TOKEN")
	}

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Logger: logger,
		// Suppress pre-initialize notifications/tools/list_changed from go-sdk.
		// Without this, AddTool triggers a notification before the client completes
		// the initialize handshake, causing intermittent connection failures.
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		Instructions: `MediaWiki MCP Server - Tool Selection Guide

` + WikiEditingGuidelines + `

` + MCP2025BestPractices + `

## DECISION TREE: Choosing the Right Tool

### User wants to EDIT something:

1. "Strike out/cross out [name]" or "mark [text] as deleted"
   -> USE: mediawiki_apply_formatting (format="strikethrough")

2. "Make [text] bold/italic/underlined"
   -> USE: mediawiki_apply_formatting (format="bold"/"italic"/"underline")

3. "Replace [X] with [Y]" or "Change [old] to [new]" on ONE page
   -> USE: mediawiki_find_replace (use preview=true first)

4. "Update [term] across all pages" or "Fix [brand name] everywhere"
   -> USE: mediawiki_bulk_replace (specify pages=[] or category="...")

5. "Add new content" or "Create a page" or complex multi-section edits
   -> USE: mediawiki_edit_page (last resort for simple edits!)

### User wants to FIND something:

1. "Find [text] on the wiki" (don't know which page)
   -> USE: mediawiki_search

2. "Find [text] on [specific page]" (know the page)
   -> USE: mediawiki_search_in_page

3. "What's on page [title]?" or "Show me [page]"
   -> USE: mediawiki_get_page (try exact title first)

4. Page not found? Title might be wrong?
   -> USE: mediawiki_resolve_title (handles case sensitivity, typos)

### User asks about HISTORY/CHANGES:

1. "Who edited [page]?" or "Show edit history"
   -> USE: mediawiki_get_revisions

2. "What changed?" or "Show me the diff"
   -> USE: mediawiki_compare_revisions

3. "What did [user] edit?"
   -> USE: mediawiki_get_user_contributions

4. "What's new on the wiki?"
   -> USE: mediawiki_get_recent_changes

### User asks about QUALITY/LINKS:

1. "Run a wiki health check" or "Audit the wiki" or "Check wiki quality"
   -> USE: mediawiki_audit (runs all checks in parallel, returns health score 0-100)

2. "Check for broken links"
   -> External URLs: mediawiki_check_links
   -> Internal wiki links: mediawiki_find_broken_internal_links

3. "Check terminology/brand consistency"
   -> USE: mediawiki_check_terminology

4. "Find orphaned/unlinked pages"
   -> USE: mediawiki_find_orphaned_pages

5. "What links to [page]?"
   -> USE: mediawiki_get_backlinks

### User wants to CONVERT content:

1. "Convert this Markdown to wiki format" or "Transform README for wiki"
   -> USE: mediawiki_convert_markdown

2. "Add this Markdown content to the wiki" (two-step process)
   -> FIRST: mediawiki_convert_markdown (to get wikitext)
   -> THEN: mediawiki_edit_page (to save the converted content)

3. "Convert with Tieto branding" or "Use brand colors"
   -> USE: mediawiki_convert_markdown (theme="tieto", add_css=true)

## COMMON MISTAKES TO AVOID

X DON'T use mediawiki_edit_page for simple text changes
   -> Instead use mediawiki_find_replace or mediawiki_apply_formatting

X DON'T guess page titles - they are case-sensitive
   -> If page not found, use mediawiki_resolve_title

X DON'T edit without preview for destructive changes
   -> Always use preview=true first for find_replace and bulk_replace

X DON'T fetch entire page just to search within it
   -> Use mediawiki_search_in_page instead

## EXAMPLE USER REQUESTS -> TOOL MAPPING

| User Says | Use This Tool |
|-----------|---------------|
| "Strike out John Smith - he left" | mediawiki_apply_formatting |
| "Replace Public 360 with Public 360°" | mediawiki_find_replace |
| "Update our brand name on all docs" | mediawiki_bulk_replace |
| "What does the API page say?" | mediawiki_get_page |
| "Is Module Overview or Module overview?" | mediawiki_resolve_title |
| "Find all mentions of deprecated" | mediawiki_search |
| "Who changed the release notes?" | mediawiki_get_revisions |
| "Convert this README to wiki format" | mediawiki_convert_markdown |
| "Add release notes (in Markdown) to wiki" | mediawiki_convert_markdown -> mediawiki_edit_page |

## RESOURCES (Direct Context Access)

- wiki://page/{title} - Get page content directly
- wiki://category/{name} - List category members

## AUTHENTICATION

Editing requires MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD environment variables.
Read operations work without authentication.`,
	})

	// Register all wiki tools using the registry
	registry := tools.NewHandlerRegistry(client, logger)

	// Configure handler-level audit logging (covers all tool calls, not just writes)
	if auditLogPath := os.Getenv("MEDIAWIKI_AUDIT_LOG"); auditLogPath != "" {
		toolAuditLogger, err := tools.NewFileToolAuditLogger(auditLogPath, logger)
		if err != nil {
			logger.Warn("Failed to create tool audit logger", "path", auditLogPath, "error", err)
		} else {
			registry.WithAuditLogger(toolAuditLogger)
			defer toolAuditLogger.Close()
			logger.Info("Tool audit logging enabled", "path", auditLogPath)
		}
	}

	registry.RegisterAll(server)

	// Register the Markdown converter tool (not a wiki.Client method)
	registerConverterTool(server, logger)

	// Register resources for direct wiki page access
	registerResources(server, client, logger)

	// Build SEP-2127 Server Card for HTTP discovery
	cardOpts := servercard.Options{
		Name:        "io.github.olgasafonova/mediawiki-mcp-server",
		Version:     ServerVersion,
		Description: "MCP server for MediaWiki wikis. Search, read, edit, and analyze wiki content with 40+ tools.",
		Title:       "MediaWiki MCP Server",
		WebsiteURL:  "https://github.com/olgasafonova/mediawiki-mcp-server",
		Repository: &servercard.Repository{
			URL:    "https://github.com/olgasafonova/mediawiki-mcp-server",
			Source: "github",
		},
		Provider: &servercard.Provider{
			Name: "Olga Safonova",
			URL:  "https://github.com/olgasafonova",
		},
	}
	serverCard, err := servercard.Build(cardOpts)
	if err != nil {
		log.Fatalf("Server card error: %v", err)
	}

	ctx := context.Background()

	// Choose transport based on flags
	if *httpAddr != "" {
		// HTTP transport mode (for ChatGPT, n8n, and remote clients)
		runHTTPServer(server, logger, *httpAddr, authToken, *allowedOrigins, *rateLimit, *trustedProxies, config.BaseURL, client, serverCard)
	} else {
		// stdio transport mode (default, for Claude Desktop, Cursor, etc.)
		logger.Info("Starting MediaWiki MCP Server (stdio mode)",
			"name", ServerName,
			"version", ServerVersion,
			"wiki_url", config.BaseURL,
		)

		// Warm cache in background (don't block startup); skip if unconfigured
		if config.IsConfigured() {
			go func() {
				warmCtx, warmCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer warmCancel()
				if err := client.WarmCacheWithDefaults(warmCtx); err != nil {
					logger.Warn("Cache warming failed", "error", err)
				} else {
					logger.Info("Cache warming completed")
				}
			}()
		}

		// Set up signal handling for graceful shutdown in stdio mode
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// Create a cancellable context for the server
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Handle shutdown signal in background
		go func() {
			sig := <-sigChan
			logger.Info("Shutdown signal received", "signal", sig.String())
			cancel()
		}()

		// Run the server
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && err != context.Canceled {
			log.Fatalf("Server error: %v", err)
		}

		// Clean up resources
		client.Close()
		logger.Info("Shutdown complete")
	}
}

func ptr[T any](v T) *T {
	return &v
}
