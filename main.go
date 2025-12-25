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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/mediawiki-mcp-server/converter"
	"github.com/olgasafonova/mediawiki-mcp-server/tools"
	"github.com/olgasafonova/mediawiki-mcp-server/tracing"
	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	ServerVersion = "1.23.0" // Resilience features and monitoring endpoints
)

// =============================================================================
// Markdown Converter Types (not a wiki.Client method)
// =============================================================================

// ConvertMarkdownArgs holds parameters for the mediawiki_convert_markdown tool
type ConvertMarkdownArgs struct {
	// Markdown is the source text to convert (required)
	Markdown string `json:"markdown" jsonschema:"required" jsonschema_description:"The Markdown text to convert to MediaWiki markup"`

	// Theme selects the color scheme: "tieto", "neutral" (default), or "dark"
	Theme string `json:"theme,omitempty" jsonschema_description:"Color theme: 'tieto' (brand colors), 'neutral' (no styling, default), or 'dark' (dark mode)"`

	// AddCSS includes CSS styling block in output for branded appearance
	AddCSS *bool `json:"add_css,omitempty" jsonschema_description:"Include CSS styling block for branded appearance"`

	// ReverseChangelog reorders changelog entries newest-first
	ReverseChangelog *bool `json:"reverse_changelog,omitempty" jsonschema_description:"Reorder changelog entries with newest first"`

	// PrettifyChecks replaces plain checkmarks (✓) with emoji (✅)
	PrettifyChecks *bool `json:"prettify_checks,omitempty" jsonschema_description:"Replace plain checkmarks with emoji ✅"`
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

// RateLimiter implements a simple token bucket rate limiter per IP
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientLimiter
	rate     int           // requests per interval
	interval time.Duration // interval duration
	cleanup  time.Duration // cleanup old entries
	stopCh   chan struct{} // graceful shutdown signal
	stopOnce sync.Once     // ensure single shutdown
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
		stopCh:   make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// Close gracefully shuts down the rate limiter cleanup loop
func (rl *RateLimiter) Close() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
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
	refill := int(elapsed/rl.interval) * rl.rate
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
	maxBodySize    int64
	trustedProxies []*net.IPNet // CIDR ranges of trusted proxies
}

// SecurityConfig holds configuration for the security middleware
type SecurityConfig struct {
	BearerToken    string   // Required token for authentication (empty = no auth)
	AllowedOrigins []string // Allowed Origin headers (empty = allow all)
	RateLimit      int      // Requests per minute per IP (0 = unlimited)
	MaxBodySize    int64    // Maximum request body size in bytes (0 = default 2MB)
	TrustedProxies []string // CIDR ranges of trusted proxies (e.g., "10.0.0.0/8", "192.168.1.1/32")
}

// Default and maximum body size limits
const (
	DefaultMaxBodySize = 2 * 1024 * 1024  // 2MB - generous for MCP requests
	MaxAllowedBodySize = 10 * 1024 * 1024 // 10MB - absolute maximum
)

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

	// Set body size limit with sensible defaults
	maxBody := config.MaxBodySize
	if maxBody <= 0 {
		maxBody = DefaultMaxBodySize
	} else if maxBody > MaxAllowedBodySize {
		maxBody = MaxAllowedBodySize
	}

	// Parse trusted proxy CIDR ranges
	var trustedProxies []*net.IPNet
	for _, cidr := range config.TrustedProxies {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		// If no CIDR suffix, assume /32 for IPv4 or /128 for IPv6
		if !strings.Contains(cidr, "/") {
			if strings.Contains(cidr, ":") {
				cidr += "/128"
			} else {
				cidr += "/32"
			}
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			logger.Warn("Invalid trusted proxy CIDR, skipping",
				"cidr", cidr,
				"error", err,
			)
			continue
		}
		trustedProxies = append(trustedProxies, ipNet)
	}

	return &SecurityMiddleware{
		handler:        handler,
		logger:         logger,
		bearerToken:    config.BearerToken,
		allowedOrigins: origins,
		rateLimiter:    rl,
		maxBodySize:    maxBody,
		trustedProxies: trustedProxies,
	}
}

func (s *SecurityMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get client IP for logging and rate limiting
	clientIP := s.getClientIP(r)

	// 1. Request body size limit (prevents DoS via large payloads)
	if r.Body != nil && r.ContentLength > s.maxBodySize {
		s.logger.Warn("Request body too large",
			"client_ip", clientIP,
			"content_length", r.ContentLength,
			"max_size", s.maxBodySize,
		)
		http.Error(w, fmt.Sprintf("Request body too large (max %d bytes)", s.maxBodySize), http.StatusRequestEntityTooLarge)
		return
	}
	// Wrap body reader to enforce limit even when Content-Length is missing/wrong
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBodySize)
	}

	// 2. Rate limiting
	if s.rateLimiter != nil && !s.rateLimiter.Allow(clientIP) {
		s.logger.Warn("Rate limit exceeded",
			"client_ip", clientIP,
			"path", r.URL.Path,
		)
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// 3. Origin validation (protect against DNS rebinding attacks)
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

	// 4. Bearer token authentication
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

	// 5. Set security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")

	// 6. Handle CORS preflight
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

// getClientIP extracts the real client IP, accounting for trusted proxies.
// When trusted proxies are configured, it walks backward through X-Forwarded-For
// to find the rightmost IP that isn't from a trusted proxy.
// This prevents IP spoofing attacks while supporting legitimate proxy chains.
func (s *SecurityMiddleware) getClientIP(r *http.Request) string {
	// Get the direct connection IP (strip port if present)
	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = host
	}

	// If no trusted proxies configured, don't trust any forwarding headers
	// This is the secure default - only trust headers when explicitly configured
	if len(s.trustedProxies) == 0 {
		return remoteIP
	}

	// Check if the direct connection is from a trusted proxy
	if !s.isTrustedProxy(remoteIP) {
		return remoteIP
	}

	// Process X-Forwarded-For header (rightmost untrusted IP is the client)
	// Format: X-Forwarded-For: client, proxy1, proxy2
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		// Walk backward to find the rightmost untrusted IP
		for i := len(ips) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(ips[i])
			if ip == "" {
				continue
			}
			if !s.isTrustedProxy(ip) {
				return ip
			}
		}
	}

	// Check X-Real-IP header (some proxies use this instead)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		xri = strings.TrimSpace(xri)
		if xri != "" && !s.isTrustedProxy(xri) {
			return xri
		}
	}

	// Fall back to remote address
	return remoteIP
}

// isTrustedProxy checks if an IP is within any trusted proxy CIDR range
func (s *SecurityMiddleware) isTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, network := range s.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

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

	// Load configuration from environment
	config, err := wiki.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
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
	registry.RegisterAll(server)

	// Register the Markdown converter tool (not a wiki.Client method)
	registerConverterTool(server, logger)

	// Register resources for direct wiki page access
	registerResources(server, client, logger)

	ctx := context.Background()

	// Choose transport based on flags
	if *httpAddr != "" {
		// HTTP transport mode (for ChatGPT, n8n, and remote clients)
		runHTTPServer(server, logger, *httpAddr, authToken, *allowedOrigins, *rateLimit, *trustedProxies, config.BaseURL, client)
	} else {
		// stdio transport mode (default, for Claude Desktop, Cursor, etc.)
		logger.Info("Starting MediaWiki MCP Server (stdio mode)",
			"name", ServerName,
			"version", ServerVersion,
			"wiki_url", config.BaseURL,
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

// runHTTPServer starts the MCP server with HTTP transport and graceful shutdown
func runHTTPServer(server *mcp.Server, logger *slog.Logger, addr, authToken, origins string, rateLimit int, trustedProxies, wikiURL string, client *wiki.Client) {
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
			_, _ = fmt.Fprintf(w, `{"status":"not_ready","wiki_url":"%s","error":"%s","response_time_ms":%d}`,
				health.WikiURL, health.Error, health.ResponseTime.Milliseconds())
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ready","wiki_url":"%s","site_name":"%s","generator":"%s","authenticated":%t,"response_time_ms":%d}`,
			health.WikiURL, health.SiteName, health.Generator, health.Authenticated, health.ResponseTime.Milliseconds())
	})

	// Prometheus metrics endpoint (no auth required - for monitoring systems)
	mux.Handle("/metrics", promhttp.Handler())

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
				"state":               cbStats.State,
				"consecutive_failures": cbStats.ConsecutiveFails,
				"last_failure":        cbStats.LastFailure,
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

func ptr[T any](v T) *T {
	return &v
}
