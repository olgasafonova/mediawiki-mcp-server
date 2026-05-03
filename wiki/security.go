package wiki

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// UploadAllowlistEnv names the env var that controls which source-URL hosts
// mediawiki_upload_file may fetch from when uploading to the wiki via
// uploadFromURL. Comma-separated list of hostnames; supports a leading
// "*." for subdomain wildcards (e.g. "*.example.com" matches a.example.com
// and b.c.example.com but NOT example.com itself).
//
// SECURITY (HG-3): unset or empty = deny-all (fail-closed). Mirrors the
// MIRO_SHARE_ALLOWED_DOMAINS pattern documented in
// rules/review-patterns.md.
const UploadAllowlistEnv = "MEDIAWIKI_UPLOAD_ALLOWED_DOMAINS"

// validateUploadDomain enforces the source-URL host allowlist for the
// mediawiki_upload_file tool's URL upload path. Returns nil if the host
// is allowed, or a structured *SSRFError otherwise. Fail-closed when the
// env var is unset.
//
// This pairs with validateFileURL (which blocks private-IP destinations)
// to provide two independent gates: validateFileURL refuses internal
// targets, validateUploadDomain refuses external targets that aren't on
// the operator's positive list. The wiki upload-from-URL primitive grants
// the wiki SSRF-on-behalf-of-the-bot access to any reachable URL, so
// fail-closed is the correct default for an MCP-exposed tool.
func validateUploadDomain(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &SSRFError{
			Code:    SSRFCodeInvalidURL,
			URL:     rawURL,
			Reason:  fmt.Sprintf("URL parse failed: %v", err),
			Blocked: true,
		}
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return &SSRFError{
			Code:    SSRFCodeInvalidURL,
			URL:     rawURL,
			Reason:  "missing host",
			Blocked: true,
		}
	}

	raw := strings.TrimSpace(os.Getenv(UploadAllowlistEnv))
	if raw == "" {
		return &SSRFError{
			Code: SSRFCodePrivateIP,
			URL:  rawURL,
			Reason: fmt.Sprintf(
				"upload source domain %q rejected: %s is unset or empty (fail-closed). "+
					"Set %s=example.com,*.cdn.example.com to allow specific hosts.",
				hostname, UploadAllowlistEnv, UploadAllowlistEnv),
			Blocked: true,
		}
	}

	for _, entry := range strings.Split(raw, ",") {
		allowed := strings.TrimSpace(strings.ToLower(entry))
		if allowed == "" {
			continue
		}
		if strings.HasPrefix(allowed, "*.") {
			suffix := allowed[1:] // ".example.com"
			if strings.HasSuffix(hostname, suffix) && hostname != suffix[1:] {
				return nil
			}
			continue
		}
		if hostname == allowed {
			return nil
		}
	}

	return &SSRFError{
		Code: SSRFCodePrivateIP,
		URL:  rawURL,
		Reason: fmt.Sprintf(
			"upload source domain %q is not on the %s allowlist",
			hostname, UploadAllowlistEnv),
		Blocked: true,
	}
}

// Private/internal IP ranges that should be blocked for SSRF protection
var (
	privateIPBlocks []*net.IPNet
)

// safeDialer prevents DNS rebinding attacks by validating IP at connection time
// This runs AFTER DNS resolution but BEFORE the TCP connection is established
var safeDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
	Control: func(network, address string, c syscall.RawConn) error {
		// Extract IP from address (format is "ip:port")
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("invalid address format: %w", err)
		}

		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("failed to parse IP: %s", host)
		}

		if isPrivateIP(ip) {
			return fmt.Errorf("connection to private IP %s blocked (SSRF protection)", host)
		}

		return nil
	},
}

// linkCheckClient is a shared HTTP client for link checking with connection pooling
// Using a shared client improves performance by reusing TCP connections
// SECURITY: Uses safeDialer to prevent DNS rebinding attacks
var linkCheckClient = &http.Client{
	// Timeout is set per-request via context; this is a fallback max
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext:         safeDialer.DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // Link checking doesn't need compression
		ForceAttemptHTTP2:   true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Allow up to 5 redirects
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}

		// Also validate redirect targets to prevent SSRF via redirect
		lastReq := via[len(via)-1]
		if hostname := lastReq.URL.Hostname(); hostname != "" {
			isPrivate, _ := isPrivateHost(hostname)
			if isPrivate {
				return fmt.Errorf("redirect to private network blocked")
			}
		}

		return nil
	},
}

func init() {
	// Initialize private IP ranges
	// These are IPs that shouldn't be accessed via external link checking
	privateCIDRs := []string{
		"127.0.0.0/8",        // IPv4 loopback
		"10.0.0.0/8",         // RFC 1918 - Private Class A
		"172.16.0.0/12",      // RFC 1918 - Private Class B
		"192.168.0.0/16",     // RFC 1918 - Private Class C
		"169.254.0.0/16",     // Link-local
		"0.0.0.0/8",          // Current network
		"100.64.0.0/10",      // Shared address space (CGN)
		"192.0.0.0/24",       // IETF Protocol assignments
		"192.0.2.0/24",       // TEST-NET-1
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"224.0.0.0/4",        // Multicast
		"240.0.0.0/4",        // Reserved
		"255.255.255.255/32", // Broadcast
		"::1/128",            // IPv6 loopback
		"fe80::/10",          // IPv6 link-local
		"fc00::/7",           // IPv6 unique local
		"ff00::/8",           // IPv6 multicast
	}

	for _, cidr := range privateCIDRs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// isPrivateIP checks if an IP address is private/internal
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true // Treat nil as private (fail-safe)
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// downloadClient is a shared HTTP client for file downloads with SSRF protections.
// Mirrors linkCheckClient: safeDialer for DNS-rebinding protection plus a
// CheckRedirect that re-validates redirect targets so a public host can't
// 302 the download path into a private network (e.g. cloud metadata).
var downloadClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		DialContext:         safeDialer.DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Allow up to 5 redirects (matches linkCheckClient).
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}

		// Re-validate the redirect target. Without this, a public-IP server
		// could return 302 Location: http://169.254.169.254/ and the Go
		// http.Client would happily follow it.
		if hostname := req.URL.Hostname(); hostname != "" {
			isPrivate, _ := isPrivateHost(hostname)
			if isPrivate {
				return &SSRFError{
					Code:    SSRFCodeRedirect,
					URL:     req.URL.String(),
					Reason:  "redirect to private network blocked",
					Blocked: true,
				}
			}
		}

		return nil
	},
}

// validateFileURL checks whether a download URL is safe to fetch.
// Used by Client.downloadFile to enforce SSRF protections at the call site,
// in addition to the safeDialer / CheckRedirect guards on downloadClient
// (defense in depth: the dialer protects against DNS rebinding, this catches
// obviously bad URLs early with a structured SSRFError).
//
// Allowed: http(s) URLs whose host (or all DNS-resolved IPs) are public.
// Blocked: private/internal IPs, link-local, loopback, multicast, malformed
// URLs, non-http(s) schemes, and DNS-resolution failures (fail-closed).
func validateFileURL(rawURL string) error {
	if rawURL == "" {
		return &SSRFError{
			Code:    SSRFCodeInvalidURL,
			URL:     rawURL,
			Reason:  "empty URL",
			Blocked: true,
		}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &SSRFError{
			Code:    SSRFCodeInvalidURL,
			URL:     rawURL,
			Reason:  fmt.Sprintf("URL parse failed: %v", err),
			Blocked: true,
		}
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &SSRFError{
			Code:    SSRFCodeInvalidURL,
			URL:     rawURL,
			Reason:  fmt.Sprintf("unsupported scheme %q (only http/https allowed)", parsed.Scheme),
			Blocked: true,
		}
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return &SSRFError{
			Code:    SSRFCodeInvalidURL,
			URL:     rawURL,
			Reason:  "missing host",
			Blocked: true,
		}
	}

	isPrivate, ssrfErr := isPrivateHost(hostname)
	if isPrivate {
		if ssrfErr != nil {
			// DNS error - return the structured error (fail-closed)
			return ssrfErr
		}
		return &SSRFError{
			Code:    SSRFCodePrivateIP,
			URL:     rawURL,
			Reason:  fmt.Sprintf("host %q resolves to a private/internal network", hostname),
			Blocked: true,
		}
	}

	return nil
}

// isPrivateHost checks if a hostname resolves to any private IP
// Returns (true, nil) if private, (false, nil) if public, (true, error) if DNS fails
// SECURITY: Fails closed - DNS errors are treated as potentially private (blocked)
func isPrivateHost(hostname string) (bool, error) {
	// First, try to parse as an IP directly
	if ip := net.ParseIP(hostname); ip != nil {
		return isPrivateIP(ip), nil
	}

	// Resolve hostname with timeout
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// SECURITY: Fail closed - DNS errors could hide SSRF attempts
		// An attacker could use DNS that times out initially then resolves to private IP
		return true, &SSRFError{
			Code:    SSRFCodeDNSError,
			URL:     hostname,
			Reason:  fmt.Sprintf("DNS resolution failed: %v", err),
			Blocked: true,
		}
	}

	// Check for empty response (shouldn't happen, but fail closed)
	if len(ips) == 0 {
		return true, &SSRFError{
			Code:    SSRFCodeDNSError,
			URL:     hostname,
			Reason:  "DNS returned no IP addresses",
			Blocked: true,
		}
	}

	// Check all resolved IPs - if ANY is private, block it
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true, nil
		}
	}
	return false, nil
}
