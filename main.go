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

// serverInstructions is the tool-selection guide handed to MCP clients on
// connect. Kept as a top-level constant so main() stays small.
const serverInstructions = `MediaWiki MCP Server - Tool Selection Guide

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
Read operations work without authentication.`

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

// cliFlags holds the parsed command-line flags.
type cliFlags struct {
	httpAddr       string
	bearerToken    string
	allowedOrigins string
	rateLimit      int
	trustedProxies string
}

// parseFlags parses the command-line flags into a cliFlags value.
func parseFlags() cliFlags {
	httpAddr := flag.String("http", "", "HTTP address to listen on (e.g., :8080 or 127.0.0.1:8080). If empty, uses stdio transport.")
	bearerToken := flag.String("token", "", "Bearer token for HTTP authentication. Can also use MCP_AUTH_TOKEN env var.")
	allowedOrigins := flag.String("origins", "", "Comma-separated allowed origins for CORS (e.g., 'https://chat.openai.com,https://n8n.example.com'). Empty allows all.")
	rateLimit := flag.Int("rate-limit", 60, "Maximum requests per minute per IP (0 = unlimited)")
	trustedProxies := flag.String("trusted-proxies", "", "Comma-separated trusted proxy IPs/CIDRs (e.g., '10.0.0.0/8,192.168.1.1'). Required to trust X-Forwarded-For header.")
	flag.Parse()
	return cliFlags{
		httpAddr:       *httpAddr,
		bearerToken:    *bearerToken,
		allowedOrigins: *allowedOrigins,
		rateLimit:      *rateLimit,
		trustedProxies: *trustedProxies,
	}
}

// newLogger configures logging to stderr (stdout is reserved for the MCP
// protocol in stdio mode).
func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// setupTracing initializes OpenTelemetry tracing and returns a shutdown
// function (no-op when tracing is disabled).
func setupTracing(logger *slog.Logger) func() {
	tracingConfig := tracing.DefaultConfig()
	tracingConfig.ServiceVersion = ServerVersion
	shutdownTracing, err := tracing.Setup(context.Background(), tracingConfig)
	if err != nil {
		logger.Warn("Failed to initialize tracing", "error", err)
		return func() {}
	}
	if !tracingConfig.Enabled {
		return func() {}
	}
	logger.Info("OpenTelemetry tracing enabled",
		"endpoint", tracingConfig.OTLPEndpoint,
		"service", tracingConfig.ServiceName)
	return func() { _ = shutdownTracing(context.Background()) }
}

// loadConfigAndClient loads configuration, builds the wiki client, and wires up
// client-level audit logging when configured.
func loadConfigAndClient(logger *slog.Logger) (*wiki.Config, *wiki.Client) {
	// Uses LoadConfigOrUnconfigured so the server starts even without
	// MEDIAWIKI_URL, allowing MCP registries (Glama, Smithery) to inspect tool
	// definitions. Tool calls return a clear error if the wiki URL is unset.
	config, err := wiki.LoadConfigOrUnconfigured()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	if !config.IsConfigured() {
		logger.Warn("MEDIAWIKI_URL not set. Server will start in inspection mode: tools are listed but calls will fail until configured.")
	}

	client := wiki.NewClient(config, logger)
	if auditLogPath := os.Getenv("MEDIAWIKI_AUDIT_LOG"); auditLogPath != "" {
		auditLogger, err := wiki.NewFileAuditLogger(auditLogPath, logger)
		if err != nil {
			logger.Warn("Failed to create audit logger", "path", auditLogPath, "error", err)
		} else {
			client.SetAuditLogger(auditLogger)
			logger.Info("Audit logging enabled", "path", auditLogPath)
		}
	}
	return config, client
}

// resolveAuthToken prefers the -token flag, falling back to MCP_AUTH_TOKEN.
func resolveAuthToken(flagToken string) string {
	if flagToken != "" {
		return flagToken
	}
	return os.Getenv("MCP_AUTH_TOKEN")
}

// newMCPServer constructs the MCP server with the tool-selection instructions.
func newMCPServer(logger *slog.Logger) *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Logger: logger,
		// Suppress pre-initialize notifications/tools/list_changed from go-sdk.
		// Without this, AddTool triggers a notification before the client
		// completes the initialize handshake, causing intermittent failures.
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		Instructions: serverInstructions,
	})
}

// registerToolsAndResources registers all wiki tools, the converter tool, and
// the wiki resources. It returns a cleanup function for any audit logger.
func registerToolsAndResources(server *mcp.Server, client *wiki.Client, logger *slog.Logger) func() {
	registry := tools.NewHandlerRegistry(client, logger)
	cleanup := func() {}

	// Handler-level audit logging covers all tool calls, not just writes.
	if auditLogPath := os.Getenv("MEDIAWIKI_AUDIT_LOG"); auditLogPath != "" {
		toolAuditLogger, err := tools.NewFileToolAuditLogger(auditLogPath, logger)
		if err != nil {
			logger.Warn("Failed to create tool audit logger", "path", auditLogPath, "error", err)
		} else {
			registry.WithAuditLogger(toolAuditLogger)
			cleanup = func() { toolAuditLogger.Close() }
			logger.Info("Tool audit logging enabled", "path", auditLogPath)
		}
	}

	registry.RegisterAll(server)
	registerConverterTool(server, logger)
	registerResources(server, client, logger)
	return cleanup
}

// buildServerCard builds the SEP-2127 Server Card for HTTP discovery.
func buildServerCard() *servercard.ServerCard {
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
	return serverCard
}

// runStdioServer runs the server over stdio with graceful shutdown handling.
func runStdioServer(server *mcp.Server, client *wiki.Client, config *wiki.Config, logger *slog.Logger) {
	logger.Info("Starting MediaWiki MCP Server (stdio mode)",
		"name", ServerName,
		"version", ServerVersion,
		"wiki_url", config.BaseURL,
	)

	if config.IsConfigured() {
		warmCacheAsync(client, logger)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := <-sigChan
		logger.Info("Shutdown signal received", "signal", sig.String())
		cancel()
	}()

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && err != context.Canceled {
		log.Fatalf("Server error: %v", err)
	}

	client.Close()
	logger.Info("Shutdown complete")
}

func main() {
	flags := parseFlags()
	logger := newLogger()

	shutdownTracing := setupTracing(logger)
	defer shutdownTracing()

	config, client := loadConfigAndClient(logger)
	authToken := resolveAuthToken(flags.bearerToken)

	server := newMCPServer(logger)
	cleanupAudit := registerToolsAndResources(server, client, logger)
	defer cleanupAudit()

	serverCard := buildServerCard()

	if flags.httpAddr != "" {
		// HTTP transport mode (for ChatGPT, n8n, and remote clients)
		runHTTPServer(httpServerConfig{
			Server:         server,
			Logger:         logger,
			Addr:           flags.httpAddr,
			AuthToken:      authToken,
			Origins:        flags.allowedOrigins,
			RateLimit:      flags.rateLimit,
			TrustedProxies: flags.trustedProxies,
			WikiURL:        config.BaseURL,
			Client:         client,
			Card:           serverCard,
		})
		return
	}

	runStdioServer(server, client, config, logger)
}

func ptr[T any](v T) *T {
	return &v
}
