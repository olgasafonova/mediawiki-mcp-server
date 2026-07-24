package main

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

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
	BearerToken    string   // #nosec G117 -- config field name, not a hardcoded secret
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

// isLoopbackBind reports whether addr binds only to the loopback interface.
// A bare ":8080" or "0.0.0.0:8080" binds every interface and is NOT loopback.
// A hostname that isn't "localhost" and doesn't parse as an IP is treated as
// non-loopback (the safe default).
func isLoopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port present; treat the whole value as the host.
		host = addr
	}
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// enforceBindSecurity fails closed at startup. A server bound to a non-loopback
// interface without an auth token would expose both the MCP surface and the
// internal aux endpoints to unauthenticated callers, so that combination is
// refused. Loopback binds keep the prior warn-only behavior for local dev.
func enforceBindSecurity(logger *slog.Logger, authToken, addr string) error {
	loopback := isLoopbackBind(addr)
	if authToken == "" {
		if !loopback {
			return fmt.Errorf("refusing to start: HTTP server bound to non-loopback address %q without an auth token; set -token or MCP_AUTH_TOKEN, or bind to 127.0.0.1 for local use", addr)
		}
		logger.Warn("HTTP server running WITHOUT authentication on a loopback bind. Set -token flag or MCP_AUTH_TOKEN env var for production use.")
	}
	if !loopback {
		logger.Warn("Server binding to external interface. Ensure you're behind an HTTPS proxy in production.")
	}
	return nil
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
		maxBodySize:    clampMaxBodySize(config.MaxBodySize),
		trustedProxies: parseTrustedProxies(logger, config.TrustedProxies),
	}
}

// clampMaxBodySize applies sensible defaults and an absolute ceiling.
func clampMaxBodySize(maxBody int64) int64 {
	if maxBody <= 0 {
		return DefaultMaxBodySize
	}
	if maxBody > MaxAllowedBodySize {
		return MaxAllowedBodySize
	}
	return maxBody
}

// normalizeCIDR appends a single-host suffix (/32 or /128) when none is given.
func normalizeCIDR(cidr string) string {
	if strings.Contains(cidr, "/") {
		return cidr
	}
	if strings.Contains(cidr, ":") {
		return cidr + "/128"
	}
	return cidr + "/32"
}

// parseTrustedProxies parses CIDR ranges, skipping blanks and invalid entries.
func parseTrustedProxies(logger *slog.Logger, cidrs []string) []*net.IPNet {
	var trustedProxies []*net.IPNet
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(normalizeCIDR(cidr))
		if err != nil {
			logger.Warn("Invalid trusted proxy CIDR, skipping",
				"cidr", cidr,
				"error", err,
			)
			continue
		}
		trustedProxies = append(trustedProxies, ipNet)
	}
	return trustedProxies
}

// secureRequest bundles the per-request state shared by the middleware checks.
type secureRequest struct {
	w        http.ResponseWriter
	r        *http.Request
	clientIP string
}

func (s *SecurityMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Client IP is needed for logging and rate limiting
	req := &secureRequest{w: w, r: r, clientIP: s.getClientIP(r)}

	if !s.enforceBodyLimit(req) {
		return
	}
	if !s.enforceRateLimit(req) {
		return
	}
	origin := r.Header.Get("Origin")
	if !s.enforceOrigin(req) {
		return
	}
	if !s.enforceBearerToken(req) {
		return
	}

	// Set security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")

	// Handle CORS preflight
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
		"client_ip", req.clientIP,
		"origin", origin,
	)

	// Pass to the underlying handler
	s.handler.ServeHTTP(w, r)
}

// enforceBodyLimit rejects oversized payloads (DoS prevention) and wraps the
// body reader so the limit holds even when Content-Length is missing or wrong.
// It reports whether the request may proceed.
func (s *SecurityMiddleware) enforceBodyLimit(req *secureRequest) bool {
	if req.r.Body == nil {
		return true
	}
	if req.r.ContentLength > s.maxBodySize {
		s.logger.Warn("Request body too large",
			"client_ip", req.clientIP,
			"content_length", req.r.ContentLength,
			"max_size", s.maxBodySize,
		)
		http.Error(req.w, fmt.Sprintf("Request body too large (max %d bytes)", s.maxBodySize), http.StatusRequestEntityTooLarge)
		return false
	}
	req.r.Body = http.MaxBytesReader(req.w, req.r.Body, s.maxBodySize)
	return true
}

// enforceRateLimit reports whether the request is within the per-IP rate limit.
func (s *SecurityMiddleware) enforceRateLimit(req *secureRequest) bool {
	if s.rateLimiter == nil || s.rateLimiter.Allow(req.clientIP) {
		return true
	}
	s.logger.Warn("Rate limit exceeded",
		"client_ip", req.clientIP,
		"path", req.r.URL.Path,
	)
	http.Error(req.w, "Rate limit exceeded", http.StatusTooManyRequests)
	return false
}

// enforceOrigin validates the Origin header against the allow-list
// (protects against DNS rebinding attacks). It reports whether the
// request may proceed.
func (s *SecurityMiddleware) enforceOrigin(req *secureRequest) bool {
	origin := req.r.Header.Get("Origin")
	if origin == "" || len(s.allowedOrigins) == 0 {
		return true
	}
	if s.allowedOrigins[origin] || s.allowedOrigins["*"] {
		return true
	}
	s.logger.Warn("Origin not allowed",
		"origin", origin,
		"client_ip", req.clientIP,
	)
	http.Error(req.w, "Origin not allowed", http.StatusForbidden)
	return false
}

// enforceBearerToken authenticates the request when a bearer token is
// configured. It reports whether the request may proceed.
func (s *SecurityMiddleware) enforceBearerToken(req *secureRequest) bool {
	if s.bearerToken == "" {
		return true
	}
	auth := req.r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		s.logger.Warn("Missing Bearer token",
			"client_ip", req.clientIP,
			"path", req.r.URL.Path,
		)
		req.w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(req.w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.bearerToken)) != 1 {
		s.logger.Warn("Invalid Bearer token",
			"client_ip", req.clientIP,
			"path", req.r.URL.Path,
		)
		http.Error(req.w, "Invalid token", http.StatusUnauthorized)
		return false
	}
	return true
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

	// Prefer X-Forwarded-For, then X-Real-IP (some proxies use this instead)
	if ip := s.clientIPFromXFF(r.Header); ip != "" {
		return ip
	}
	if ip := s.clientIPFromXRealIP(r.Header); ip != "" {
		return ip
	}

	// Fall back to remote address
	return remoteIP
}

// clientIPFromXFF walks X-Forwarded-For backward and returns the rightmost
// untrusted IP (the real client). Format: X-Forwarded-For: client, proxy1, proxy2.
// Returns "" when the header yields no untrusted IP.
func (s *SecurityMiddleware) clientIPFromXFF(h http.Header) string {
	xff := h.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}
	ips := strings.Split(xff, ",")
	for i := len(ips) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(ips[i])
		if ip != "" && !s.isTrustedProxy(ip) {
			return ip
		}
	}
	return ""
}

// clientIPFromXRealIP returns the X-Real-IP value when it is a non-empty,
// untrusted address; otherwise "".
func (s *SecurityMiddleware) clientIPFromXRealIP(h http.Header) string {
	xri := strings.TrimSpace(h.Get("X-Real-IP"))
	if xri != "" && !s.isTrustedProxy(xri) {
		return xri
	}
	return ""
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
