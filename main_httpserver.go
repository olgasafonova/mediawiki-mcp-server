package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

// httpServerConfig groups the configuration needed to run the HTTP transport.
// Grouping these into one struct keeps runHTTPServer's signature small.
type httpServerConfig struct {
	Server         *mcp.Server
	Logger         *slog.Logger
	Addr           string
	AuthToken      string
	Origins        string
	RateLimit      int
	TrustedProxies string
	WikiURL        string
	Client         *wiki.Client
	Card           *servercard.ServerCard
}

// splitCSV parses a comma-separated value into a trimmed, non-empty list.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// runHTTPServer starts the MCP server with HTTP transport and graceful shutdown
func runHTTPServer(cfg httpServerConfig) {
	allowedOriginsList := splitCSV(cfg.Origins)

	securedHandler := newSecuredMCPHandler(cfg, allowedOriginsList)
	mux := buildHTTPMux(cfg, securedHandler)
	httpServer := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logStartup(cfg, allowedOriginsList)
	warmCacheAsync(cfg.Client, cfg.Logger)
	logSecurityWarnings(cfg.Logger, cfg.AuthToken, cfg.Addr)

	serveUntilSignal(httpServer, cfg.Logger)
	shutdownServer(httpServer, securedHandler, cfg.Client, cfg.Logger)
}

// newSecuredMCPHandler builds the Streamable HTTP MCP handler wrapped in the
// security middleware.
func newSecuredMCPHandler(cfg httpServerConfig, allowedOrigins []string) *SecurityMiddleware {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return cfg.Server
	}, nil)

	securityConfig := SecurityConfig{
		BearerToken:    cfg.AuthToken,
		AllowedOrigins: allowedOrigins,
		RateLimit:      cfg.RateLimit,
		TrustedProxies: splitCSV(cfg.TrustedProxies),
	}
	return NewSecurityMiddleware(mcpHandler, cfg.Logger, securityConfig)
}

// buildHTTPMux wires the health, readiness, metrics, server-card, tools, status,
// and MCP routes onto a fresh mux.
func buildHTTPMux(cfg httpServerConfig, securedHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleReady(cfg.WikiURL, cfg.Client))
	mux.Handle("/metrics", promhttp.Handler())

	cfg.Card.Remotes = []servercard.Remote{{
		Type:                      "streamable-http",
		URL:                       "/",
		SupportedProtocolVersions: []string{"2025-06-18"},
	}}
	mux.Handle(servercard.WellKnownPath, servercard.Handler(cfg.Card))

	mux.HandleFunc("/tools", handleTools(cfg.Logger))
	mux.HandleFunc("/status", handleStatus(cfg.Client, cfg.Logger))
	mux.Handle("/", securedHandler)
	return mux
}

// handleHealth is a simple liveness check that does not verify wiki connectivity
// (no auth required - for load balancers and monitoring).
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"healthy","server":"%s","version":"%s"}`, ServerName, ServerVersion)
}

// handleReady returns a readiness handler that verifies actual wiki
// connectivity with a short timeout.
func handleReady(wikiURL string, client *wiki.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		if wikiURL == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, `{"status":"not_ready","error":"wiki_url_not_configured"}`)
			return
		}

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
	}
}

// handleTools returns a handler that lists available tools grouped by category
// (no auth required - for tool introspection).
func handleTools(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour

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
	}
}

// handleStatus returns a handler exposing circuit-breaker and dedup status
// (no auth - for monitoring).
func handleStatus(client *wiki.Client, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
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
	}
}

// logStartup emits the server startup info line.
func logStartup(cfg httpServerConfig, allowedOrigins []string) {
	cfg.Logger.Info("Starting MediaWiki MCP Server (HTTP mode)",
		"name", ServerName,
		"version", ServerVersion,
		"address", cfg.Addr,
		"wiki_url", cfg.WikiURL,
		"auth_enabled", cfg.AuthToken != "",
		"rate_limit", cfg.RateLimit,
		"allowed_origins", allowedOrigins,
	)
}

// warmCacheAsync warms the client cache in the background without blocking
// startup.
func warmCacheAsync(client *wiki.Client, logger *slog.Logger) {
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

// logSecurityWarnings warns about insecure configurations at startup.
func logSecurityWarnings(logger *slog.Logger, authToken, addr string) {
	if authToken == "" {
		logger.Warn("HTTP server running WITHOUT authentication. Set -token flag or MCP_AUTH_TOKEN env var for production use.")
	}
	if !strings.HasPrefix(addr, "127.0.0.1") && !strings.HasPrefix(addr, "localhost") {
		logger.Warn("Server binding to external interface. Ensure you're behind HTTPS proxy in production.")
	}
}

// serveUntilSignal starts the HTTP server and blocks until a shutdown signal or
// a fatal server error arrives.
func serveUntilSignal(httpServer *http.Server, logger *slog.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	serverErrors := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	select {
	case err := <-serverErrors:
		log.Fatalf("HTTP server error: %v", err)
	case sig := <-sigChan:
		logger.Info("Shutdown signal received", "signal", sig.String())
	}
}

// shutdownServer performs the graceful shutdown sequence and resource cleanup.
func shutdownServer(httpServer *http.Server, securedHandler *SecurityMiddleware, client *wiki.Client, logger *slog.Logger) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	logger.Info("Initiating graceful shutdown...")

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	} else {
		logger.Info("HTTP server stopped gracefully")
	}

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
