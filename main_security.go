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
