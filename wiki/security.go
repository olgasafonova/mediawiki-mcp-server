package wiki

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

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
