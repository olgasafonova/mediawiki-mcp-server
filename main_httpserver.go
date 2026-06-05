package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/mcp-servercard-go/servercard"
	"github.com/olgasafonova/mediawiki-mcp-server/converter"
	"github.com/olgasafonova/mediawiki-mcp-server/tools"
	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// runHTTPServer starts the MCP server with HTTP transport and graceful shutdown
func runHTTPServer(server *mcp.Server, logger *slog.Logger, addr, authToken, origins string, rateLimit int, trustedProxies, wikiURL string, client *wiki.Client, card *servercard.ServerCard) {
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

	// Parse trusted proxies
	var trustedProxiesList []string
	if trustedProxies != "" {
		for _, p := range strings.Split(trustedProxies, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				trustedProxiesList = append(trustedProxiesList, p)
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
		TrustedProxies: trustedProxiesList,
	}
	securedHandler := NewSecurityMiddleware(mcpHandler, logger, securityConfig)

	// Create mux for routing health checks separately from MCP
	mux := http.NewServeMux()

	// Health endpoint (no auth required - for load balancers and monitoring)
	// This is a simple liveness check that doesn't verify wiki connectivity
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"healthy","server":"%s","version":"%s"}`, ServerName, ServerVersion)
	})

	// Readiness endpoint - verifies actual wiki connectivity
	// Use short timeout to avoid blocking health checks
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		// Check if wiki URL is configured
		if wikiURL == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, `{"status":"not_ready","error":"wiki_url_not_configured"}`)
			return
		}

		// Check actual wiki connectivity with timeout
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		health := client.Ping(ctx)
		if !health.Connected {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, `{"status":"not_ready","wiki_url":"%s","error":"%s","response_time_ms":%d}`, // #nosec G705 -- health endpoint returns server config, not user input
				health.WikiURL, health.Error, health.ResponseTime.Milliseconds())
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ready","wiki_url":"%s","site_name":"%s","generator":"%s","authenticated":%t,"response_time_ms":%d}`, // #nosec G705 -- health endpoint returns server config, not user input
			health.WikiURL, health.SiteName, health.Generator, health.Authenticated, health.ResponseTime.Milliseconds())
	})

	// Prometheus metrics endpoint (no auth required - for monitoring systems)
	mux.Handle("/metrics", promhttp.Handler())

	// SEP-2127 Server Card endpoint (no auth required - for pre-connect discovery)
	card.Remotes = []servercard.Remote{{
		Type:                      "streamable-http",
		URL:                       "/",
		SupportedProtocolVersions: []string{"2025-06-18"},
	}}
	mux.Handle(servercard.WellKnownPath, servercard.Handler(card))

	// Tools discovery endpoint (no auth required - for tool introspection)
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour

		// Group tools by category
		toolsByCategory := make(map[string][]map[string]interface{})
		for _, tool := range tools.AllTools {
			toolInfo := map[string]interface{}{
				"name":        tool.Name,
				"title":       tool.Title,
				"description": tool.Description,
				"read_only":   tool.ReadOnly,
				"destructive": tool.Destructive,
				"idempotent":  tool.Idempotent,
			}
			toolsByCategory[tool.Category] = append(toolsByCategory[tool.Category], toolInfo)
		}

		response := map[string]interface{}{
			"server":     ServerName,
			"version":    ServerVersion,
			"tool_count": len(tools.AllTools),
			"categories": toolsByCategory,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode tools response", "error", err)
		}
	})

	// Circuit breaker status endpoint (no auth - for monitoring)
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		cbStats := client.CircuitBreakerStatus()
		dedupStats := client.DedupStats()

		response := map[string]interface{}{
			"server":  ServerName,
			"version": ServerVersion,
			"circuit_breaker": map[string]interface{}{
				"state":                cbStats.State,
				"consecutive_failures": cbStats.ConsecutiveFails,
				"last_failure":         cbStats.LastFailure,
			},
			"dedup": map[string]interface{}{
				"inflight_requests": dedupStats,
			},
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode status response", "error", err)
		}
	})

	// All other routes go through secured MCP handler
	mux.Handle("/", securedHandler)

	// Create HTTP server with timeouts
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
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

	// Warm cache in background (don't block startup)
	go func() {
		warmCtx, warmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer warmCancel()
		if err := client.WarmCacheWithDefaults(warmCtx); err != nil {
			logger.Warn("Cache warming failed", "error", err)
		} else {
			logger.Info("Cache warming completed")
		}
	}()

	// Security warnings
	if authToken == "" {
		logger.Warn("HTTP server running WITHOUT authentication. Set -token flag or MCP_AUTH_TOKEN env var for production use.")
	}
	if !strings.HasPrefix(addr, "127.0.0.1") && !strings.HasPrefix(addr, "localhost") {
		logger.Warn("Server binding to external interface. Ensure you're behind HTTPS proxy in production.")
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		log.Fatalf("HTTP server error: %v", err)
	case sig := <-sigChan:
		logger.Info("Shutdown signal received", "signal", sig.String())
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	logger.Info("Initiating graceful shutdown...")

	// Stop accepting new connections and wait for existing requests
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	} else {
		logger.Info("HTTP server stopped gracefully")
	}

	// Clean up resources
	if securedHandler.rateLimiter != nil {
		securedHandler.rateLimiter.Close()
		logger.Info("Rate limiter stopped")
	}

	if client != nil {
		client.Close()
		logger.Info("Wiki client closed")
	}

	logger.Info("Shutdown complete")
}

// registerConverterTool registers the Markdown converter (not a wiki.Client method)
func registerConverterTool(server *mcp.Server, logger *slog.Logger) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "mediawiki_convert_markdown",
		Description: `Convert Markdown text to MediaWiki markup. Use this tool when you need to transform Markdown-formatted content into wiki-compatible format before creating or editing wiki pages.

WHEN TO USE:
- User provides Markdown content to add to the wiki
- Converting documentation from GitHub/GitLab to wiki format
- Transforming README files for wiki publishing
- Preparing release notes written in Markdown

THEMES:
- "tieto": Tieto brand colors (Hero Blue #021e57 headings, yellow code highlights)
- "neutral": Clean output without custom colors (default)
- "dark": Dark mode optimized colors

OPTIONS:
- add_css: Include CSS styling block for branded appearance
- reverse_changelog: Reorder changelog entries newest-first
- prettify_checks: Replace plain checkmarks with emoji

EXAMPLE:
Input: "# Hello\n**bold** and *italic*\n- item 1\n- item 2"
Output: "= Hello =\n'''bold''' and ''italic''\n* item 1\n* item 2"`,
		Annotations: &mcp.ToolAnnotations{
			Title:          "Convert Markdown to MediaWiki",
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  ptr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ConvertMarkdownArgs) (*mcp.CallToolResult, ConvertMarkdownResult, error) {
		defer recoverPanic(logger, "convert_markdown")

		// Build config from args
		config := converter.DefaultConfig()
		if args.Theme != "" {
			config.Theme = args.Theme
		}
		if args.AddCSS != nil {
			config.AddCSS = *args.AddCSS
		}
		if args.ReverseChangelog != nil {
			config.ReverseChangelog = *args.ReverseChangelog
		}
		if args.PrettifyChecks != nil {
			config.PrettifyChecks = *args.PrettifyChecks
		}

		// Perform conversion
		wikitext := converter.Convert(args.Markdown, config)

		// Get available themes for info
		themes := converter.ListThemes()

		result := ConvertMarkdownResult{
			Wikitext:        wikitext,
			InputLength:     len(args.Markdown),
			OutputLength:    len(wikitext),
			ThemeUsed:       config.Theme,
			AvailableThemes: themes,
		}

		logger.Info("Tool executed",
			"tool", "mediawiki_convert_markdown",
			"theme", config.Theme,
			"input_chars", len(args.Markdown),
			"output_chars", len(wikitext),
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
