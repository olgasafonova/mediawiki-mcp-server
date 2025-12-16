// MediaWiki MCP Server - A Model Context Protocol server for MediaWiki wikis
// Provides tools for searching, reading, and editing MediaWiki content
package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

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
	ServerVersion = "1.9.0" // Added: HTTP transport support for ChatGPT and n8n
)

// =============================================================================
// Security Middleware for HTTP Transport
// =============================================================================

// RateLimiter implements a simple token bucket rate limiter per IP
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientLimiter
	rate     int           // requests per interval
	interval time.Duration // interval duration
	cleanup  time.Duration // cleanup old entries
}

type clientLimiter struct {
	tokens    int
	lastCheck time.Time
}

// NewRateLimiter creates a rate limiter with specified rate per interval
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients:  make(map[string]*clientLimiter),
		rate:     rate,
		interval: interval,
		cleanup:  interval * 10,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, client := range rl.clients {
			if now.Sub(client.lastCheck) > rl.cleanup {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks if a request from the given IP should be allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	client, exists := rl.clients[ip]
	if !exists {
		rl.clients[ip] = &clientLimiter{
			tokens:    rl.rate - 1,
			lastCheck: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(client.lastCheck)
	refill := int(elapsed / rl.interval) * rl.rate
	client.tokens = min(client.tokens+refill, rl.rate)
	client.lastCheck = now

	if client.tokens > 0 {
		client.tokens--
		return true
	}
	return false
}

// SecurityMiddleware wraps an HTTP handler with security checks
type SecurityMiddleware struct {
	handler        http.Handler
	logger         *slog.Logger
	bearerToken    string
	allowedOrigins map[string]bool
	rateLimiter    *RateLimiter
}

// SecurityConfig holds configuration for the security middleware
type SecurityConfig struct {
	BearerToken    string   // Required token for authentication (empty = no auth)
	AllowedOrigins []string // Allowed Origin headers (empty = allow all)
	RateLimit      int      // Requests per minute per IP (0 = unlimited)
}

// NewSecurityMiddleware creates a new security middleware
func NewSecurityMiddleware(handler http.Handler, logger *slog.Logger, config SecurityConfig) *SecurityMiddleware {
	origins := make(map[string]bool)
	for _, o := range config.AllowedOrigins {
		origins[o] = true
	}

	var rl *RateLimiter
	if config.RateLimit > 0 {
		rl = NewRateLimiter(config.RateLimit, time.Minute)
	}

	return &SecurityMiddleware{
		handler:        handler,
		logger:         logger,
		bearerToken:    config.BearerToken,
		allowedOrigins: origins,
		rateLimiter:    rl,
	}
}

func (s *SecurityMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get client IP for logging and rate limiting
	clientIP := getClientIP(r)

	// 1. Rate limiting
	if s.rateLimiter != nil && !s.rateLimiter.Allow(clientIP) {
		s.logger.Warn("Rate limit exceeded",
			"client_ip", clientIP,
			"path", r.URL.Path,
		)
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// 2. Origin validation (protect against DNS rebinding attacks)
	origin := r.Header.Get("Origin")
	if origin != "" && len(s.allowedOrigins) > 0 {
		if !s.allowedOrigins[origin] && !s.allowedOrigins["*"] {
			s.logger.Warn("Origin not allowed",
				"origin", origin,
				"client_ip", clientIP,
			)
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}
	}

	// 3. Bearer token authentication
	if s.bearerToken != "" {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			s.logger.Warn("Missing Bearer token",
				"client_ip", clientIP,
				"path", r.URL.Path,
			)
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.bearerToken)) != 1 {
			s.logger.Warn("Invalid Bearer token",
				"client_ip", clientIP,
				"path", r.URL.Path,
			)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
	}

	// 4. Set security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")

	// 5. Handle CORS preflight
	if r.Method == http.MethodOptions {
		setCORSHeaders(w, r, s.allowedOrigins)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Set CORS headers for actual requests
	setCORSHeaders(w, r, s.allowedOrigins)

	// Log the request
	s.logger.Info("HTTP request",
		"method", r.Method,
		"path", r.URL.Path,
		"client_ip", clientIP,
		"origin", origin,
	)

	// Pass to the underlying handler
	s.handler.ServeHTTP(w, r)
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request, allowedOrigins map[string]bool) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	// If specific origins are configured, check them
	if len(allowedOrigins) > 0 {
		if allowedOrigins["*"] {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
	} else {
		// No restrictions configured, allow all
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (common with proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}

func main() {
	// Parse command-line flags
	httpAddr := flag.String("http", "", "HTTP address to listen on (e.g., :8080 or 127.0.0.1:8080). If empty, uses stdio transport.")
	bearerToken := flag.String("token", "", "Bearer token for HTTP authentication. Can also use MCP_AUTH_TOKEN env var.")
	allowedOrigins := flag.String("origins", "", "Comma-separated allowed origins for CORS (e.g., 'https://chat.openai.com,https://n8n.example.com'). Empty allows all.")
	rateLimit := flag.Int("rate-limit", 60, "Maximum requests per minute per IP (0 = unlimited)")
	flag.Parse()

	// Configure logging to stderr (stdout is used for MCP protocol in stdio mode)
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
		Instructions: `MediaWiki MCP Server - Tool Selection Guide

` + WikiEditingGuidelines + `

` + MCP2025BestPractices + `

## DECISION TREE: Choosing the Right Tool

### User wants to EDIT something:

1. "Strike out/cross out [name]" or "mark [text] as deleted"
   → USE: mediawiki_apply_formatting (format="strikethrough")

2. "Make [text] bold/italic/underlined"
   → USE: mediawiki_apply_formatting (format="bold"/"italic"/"underline")

3. "Replace [X] with [Y]" or "Change [old] to [new]" on ONE page
   → USE: mediawiki_find_replace (use preview=true first)

4. "Update [term] across all pages" or "Fix [brand name] everywhere"
   → USE: mediawiki_bulk_replace (specify pages=[] or category="...")

5. "Add new content" or "Create a page" or complex multi-section edits
   → USE: mediawiki_edit_page (last resort for simple edits!)

### User wants to FIND something:

1. "Find [text] on the wiki" (don't know which page)
   → USE: mediawiki_search

2. "Find [text] on [specific page]" (know the page)
   → USE: mediawiki_search_in_page

3. "What's on page [title]?" or "Show me [page]"
   → USE: mediawiki_get_page (try exact title first)

4. Page not found? Title might be wrong?
   → USE: mediawiki_resolve_title (handles case sensitivity, typos)

### User asks about HISTORY/CHANGES:

1. "Who edited [page]?" or "Show edit history"
   → USE: mediawiki_get_revisions

2. "What changed?" or "Show me the diff"
   → USE: mediawiki_compare_revisions

3. "What did [user] edit?"
   → USE: mediawiki_get_user_contributions

4. "What's new on the wiki?"
   → USE: mediawiki_get_recent_changes

### User asks about QUALITY/LINKS:

1. "Check for broken links"
   → External URLs: mediawiki_check_links
   → Internal wiki links: mediawiki_find_broken_internal_links

2. "Check terminology/brand consistency"
   → USE: mediawiki_check_terminology

3. "Find orphaned/unlinked pages"
   → USE: mediawiki_find_orphaned_pages

4. "What links to [page]?"
   → USE: mediawiki_get_backlinks

## COMMON MISTAKES TO AVOID

❌ DON'T use mediawiki_edit_page for simple text changes
   → Instead use mediawiki_find_replace or mediawiki_apply_formatting

❌ DON'T guess page titles - they are case-sensitive
   → If page not found, use mediawiki_resolve_title

❌ DON'T edit without preview for destructive changes
   → Always use preview=true first for find_replace and bulk_replace

❌ DON'T fetch entire page just to search within it
   → Use mediawiki_search_in_page instead

## EXAMPLE USER REQUESTS → TOOL MAPPING

| User Says | Use This Tool |
|-----------|---------------|
| "Strike out John Smith - he left" | mediawiki_apply_formatting |
| "Replace Public 360 with Public 360°" | mediawiki_find_replace |
| "Update our brand name on all docs" | mediawiki_bulk_replace |
| "What does the API page say?" | mediawiki_get_page |
| "Is Module Overview or Module overview?" | mediawiki_resolve_title |
| "Find all mentions of deprecated" | mediawiki_search |
| "Who changed the release notes?" | mediawiki_get_revisions |

## RESOURCES (Direct Context Access)

- wiki://page/{title} - Get page content directly
- wiki://category/{name} - List category members

## AUTHENTICATION

Editing requires MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD environment variables.
Read operations work without authentication.`,
	})

	// Register all tools
	registerTools(server, client, logger)

	// Register resources for direct wiki page access
	registerResources(server, client, logger)

	ctx := context.Background()

	// Choose transport based on flags
	if *httpAddr != "" {
		// HTTP transport mode (for ChatGPT, n8n, and remote clients)
		runHTTPServer(server, logger, *httpAddr, authToken, *allowedOrigins, *rateLimit, config.BaseURL)
	} else {
		// stdio transport mode (default, for Claude Desktop, Cursor, etc.)
		logger.Info("Starting MediaWiki MCP Server (stdio mode)",
			"name", ServerName,
			"version", ServerVersion,
			"wiki_url", config.BaseURL,
		)

		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}

// runHTTPServer starts the MCP server with HTTP transport
func runHTTPServer(server *mcp.Server, logger *slog.Logger, addr, authToken, origins string, rateLimit int, wikiURL string) {
	// Parse allowed origins
	var allowedOriginsList []string
	if origins != "" {
		for _, o := range strings.Split(origins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOriginsList = append(allowedOriginsList, o)
			}
		}
	}

	// Create the Streamable HTTP handler
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)

	// Wrap with security middleware
	securityConfig := SecurityConfig{
		BearerToken:    authToken,
		AllowedOrigins: allowedOriginsList,
		RateLimit:      rateLimit,
	}
	securedHandler := NewSecurityMiddleware(mcpHandler, logger, securityConfig)

	// Create HTTP server with timeouts
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      securedHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Log startup info
	logger.Info("Starting MediaWiki MCP Server (HTTP mode)",
		"name", ServerName,
		"version", ServerVersion,
		"address", addr,
		"wiki_url", wikiURL,
		"auth_enabled", authToken != "",
		"rate_limit", rateLimit,
		"allowed_origins", allowedOriginsList,
	)

	// Security warnings
	if authToken == "" {
		logger.Warn("⚠️  HTTP server running WITHOUT authentication. Set -token flag or MCP_AUTH_TOKEN env var for production use.")
	}
	if !strings.HasPrefix(addr, "127.0.0.1") && !strings.HasPrefix(addr, "localhost") {
		logger.Warn("⚠️  Server binding to external interface. Ensure you're behind HTTPS proxy in production.")
	}

	// Start the server
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func registerTools(server *mcp.Server, client *wiki.Client, logger *slog.Logger) {
	// Search tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_search",
		Description: "Search ACROSS the entire wiki. Use when user doesn't know which page contains info, e.g., 'find pages about API' or 'where is X documented?'. For searching within a specific known page, use mediawiki_search_in_page instead.",
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
		Description: "Create new pages or rewrite entire page content. WARNING: For simple edits (changing text, formatting), use mediawiki_find_replace or mediawiki_apply_formatting instead. This tool overwrites entire page content unless 'section' is specified.",
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

	// ========== Simple Edit Tools ==========

	// Find and replace text in a page
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_find_replace",
		Description: "PREFERRED for simple text changes. Replace specific text in a page without fetching/rewriting the whole page. Examples: fix typos, update names, correct terminology. Always use preview=true first to verify matches.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Find and Replace",
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
			IdempotentHint:  false,
			OpenWorldHint:   ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.FindReplaceArgs) (*mcp.CallToolResult, wiki.FindReplaceResult, error) {
		defer recoverPanic(logger, "find_replace")
		result, err := client.FindReplace(ctx, args)
		if err != nil {
			return nil, wiki.FindReplaceResult{}, fmt.Errorf("find/replace failed: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_find_replace",
			"title", args.Title,
			"matches", result.MatchCount,
			"replaced", result.ReplaceCount,
			"preview", args.Preview,
		)
		return nil, result, nil
	})

	// Apply formatting to text
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_apply_formatting",
		Description: "BEST for formatting requests. Apply strikethrough/bold/italic/underline/code to specific text. Use when user says 'strike out', 'cross out', 'make bold', 'italicize'. Example: 'strike out John Smith' → format='strikethrough', text='John Smith'.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Apply Formatting",
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
			IdempotentHint:  false,
			OpenWorldHint:   ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ApplyFormattingArgs) (*mcp.CallToolResult, wiki.ApplyFormattingResult, error) {
		defer recoverPanic(logger, "apply_formatting")
		result, err := client.ApplyFormatting(ctx, args)
		if err != nil {
			return nil, wiki.ApplyFormattingResult{}, fmt.Errorf("apply formatting failed: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_apply_formatting",
			"title", args.Title,
			"format", args.Format,
			"matches", result.MatchCount,
			"formatted", result.FormatCount,
		)
		return nil, result, nil
	})

	// Bulk find and replace across multiple pages
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_bulk_replace",
		Description: "Update text across MULTIPLE pages at once. Use when user says 'update everywhere', 'fix on all pages', 'change brand name across docs'. Specify pages=[] list OR category='CategoryName'. ALWAYS preview=true first!",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Bulk Replace",
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
			IdempotentHint:  false,
			OpenWorldHint:   ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.BulkReplaceArgs) (*mcp.CallToolResult, wiki.BulkReplaceResult, error) {
		defer recoverPanic(logger, "bulk_replace")
		result, err := client.BulkReplace(ctx, args)
		if err != nil {
			return nil, wiki.BulkReplaceResult{}, fmt.Errorf("bulk replace failed: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_bulk_replace",
			"pages_processed", result.PagesProcessed,
			"pages_modified", result.PagesModified,
			"total_changes", result.TotalChanges,
			"preview", args.Preview,
		)
		return nil, result, nil
	})

	// Search within a specific page
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_search_in_page",
		Description: "Search WITHIN a known page (not across wiki). Use when user says 'find X on page Y' or 'does page Y mention X'. More efficient than get_page + manual search. Also use before find_replace to preview matches.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Search in Page",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.SearchInPageArgs) (*mcp.CallToolResult, wiki.SearchInPageResult, error) {
		defer recoverPanic(logger, "search_in_page")
		result, err := client.SearchInPage(ctx, args)
		if err != nil {
			return nil, wiki.SearchInPageResult{}, fmt.Errorf("search in page failed: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_search_in_page",
			"title", args.Title,
			"query", args.Query,
			"matches", result.MatchCount,
		)
		return nil, result, nil
	})

	// Resolve page title with fuzzy matching
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_resolve_title",
		Description: "RECOVERY tool when page not found. Wiki titles are case-sensitive! If 'Module overview' fails, this finds 'Module Overview'. Also handles typos and partial matches. Use BEFORE retrying get_page with a guessed title.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "Resolve Title",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ResolveTitleArgs) (*mcp.CallToolResult, wiki.ResolveTitleResult, error) {
		defer recoverPanic(logger, "resolve_title")
		result, err := client.ResolveTitle(ctx, args)
		if err != nil {
			return nil, wiki.ResolveTitleResult{}, fmt.Errorf("resolve title failed: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_resolve_title",
			"input_title", args.Title,
			"exact_match", result.ExactMatch,
			"resolved_title", result.ResolvedTitle,
			"suggestions", len(result.Suggestions),
		)
		return nil, result, nil
	})

	// List users by group
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mediawiki_list_users",
		Description: "List wiki users, optionally filtered by group. Use group='sysop' for admins, 'bureaucrat' for bureaucrats, 'bot' for bots. Returns user names, groups, edit counts, and registration dates.",
		Annotations: &mcp.ToolAnnotations{
			Title:          "List Users",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.ListUsersArgs) (*mcp.CallToolResult, wiki.ListUsersResult, error) {
		defer recoverPanic(logger, "list_users")
		result, err := client.ListUsers(ctx, args)
		if err != nil {
			return nil, wiki.ListUsersResult{}, fmt.Errorf("list users failed: %w", err)
		}
		logger.Info("Tool executed",
			"tool", "mediawiki_list_users",
			"group", args.Group,
			"users_returned", len(result.Users),
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
