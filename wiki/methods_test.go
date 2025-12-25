package wiki

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Private IPv4 ranges
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"loopback 127.0.0.255", "127.0.0.255", true},
		{"private 10.x", "10.0.0.1", true},
		{"private 10.255.x", "10.255.255.255", true},
		{"private 172.16.x", "172.16.0.1", true},
		{"private 172.31.x", "172.31.255.255", true},
		{"private 192.168.x", "192.168.1.1", true},
		{"link-local 169.254", "169.254.0.1", true},
		{"current network", "0.0.0.1", true},
		{"multicast", "224.0.0.1", true},
		{"broadcast", "255.255.255.255", true},

		// Public IPv4
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"public 172.15.x", "172.15.255.255", false},
		{"public 192.167.x", "192.167.1.1", false},

		// IPv6
		{"loopback ::1", "::1", true},
		{"link-local fe80::", "fe80::1", true},
		{"unique local fc00::", "fc00::1", true},
		{"multicast ff00::", "ff00::1", true},
		{"public IPv6", "2001:4860:4860::8888", false},

		// Edge cases
		{"nil IP", "", true}, // net.ParseIP returns nil for empty string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			result := isPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIsPrivateHost(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		// IP addresses
		{"private IP 10.0.0.1", "10.0.0.1", true},
		{"private IP 192.168.1.1", "192.168.1.1", true},
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"public IP 8.8.8.8", "8.8.8.8", false},

		// This relies on DNS resolution which we can't control in tests
		// So we only test IP address parsing
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := isPrivateHost(tt.hostname)
			if err != nil {
				t.Fatalf("isPrivateHost(%q) returned error: %v", tt.hostname, err)
			}
			if result != tt.expected {
				t.Errorf("isPrivateHost(%q) = %v, want %v", tt.hostname, result, tt.expected)
			}
		})
	}
}

// Test the CacheEntry struct
func TestCacheEntry(t *testing.T) {
	entry := &CacheEntry{
		Data: "test data",
		Key:  "test:key",
	}

	if entry.Data != "test data" {
		t.Errorf("CacheEntry.Data = %v, want %v", entry.Data, "test data")
	}
	if entry.Key != "test:key" {
		t.Errorf("CacheEntry.Key = %v, want %v", entry.Key, "test:key")
	}
}

// Test MaxLimit constant
func TestConstants(t *testing.T) {
	if MaxLimit != 500 {
		t.Errorf("MaxLimit = %d, want 500", MaxLimit)
	}
	if DefaultLimit != 50 {
		t.Errorf("DefaultLimit = %d, want 50", DefaultLimit)
	}
	if MaxCacheEntries != 1000 {
		t.Errorf("MaxCacheEntries = %d, want 1000", MaxCacheEntries)
	}
	if MaxConcurrentRequests != 3 {
		t.Errorf("MaxConcurrentRequests = %d, want 3", MaxConcurrentRequests)
	}
}

// Benchmark stripHTMLTags
func BenchmarkStripHTMLTags_Simple(b *testing.B) {
	input := "<p>Hello <b>World</b></p>"
	for i := 0; i < b.N; i++ {
		stripHTMLTags(input)
	}
}

func BenchmarkStripHTMLTags_Complex(b *testing.B) {
	input := `<div class="container"><p style="color:red">
		<a href="https://example.com" onclick="evil()">
			Click <b>here</b> &amp; <i>enjoy</i>
		</a>
	</p></div>`
	for i := 0; i < b.N; i++ {
		stripHTMLTags(input)
	}
}
