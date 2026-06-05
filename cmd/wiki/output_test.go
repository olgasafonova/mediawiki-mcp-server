package main

import (
	"strings"
	"testing"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

func TestPrintIDTitleTableNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("printIDTitleTable panicked: %v", r)
		}
	}()
	// Empty path.
	printIDTitleTable("Header", "none found", "pages", nil, false, "")
	// Populated path with continuation hint.
	items := []wiki.PageSummary{
		{PageID: 1, Title: "Alpha"},
		{PageID: 2, Title: "Beta"},
	}
	printIDTitleTable("Header", "none", "pages", items, true, "cont-token")
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"equal to max", "hello", 5, "hello"},
		{"longer truncated with ellipsis", "hello world", 8, "hello..."},
		{"newlines replaced with spaces", "a\nb\nc", 10, "a b c"},
		{"newline replaced then truncated", "abcdef\nghij", 8, "abcde..."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.in, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.maxLen, got, tc.want)
			}
		})
	}
}

func TestTruncateLengthBound(t *testing.T) {
	// Truncated output must never exceed maxLen.
	in := strings.Repeat("x", 100)
	for _, maxLen := range []int{4, 10, 50} {
		got := truncate(in, maxLen)
		if len(got) > maxLen {
			t.Errorf("truncate produced %d chars, want <= %d", len(got), maxLen)
		}
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text unchanged", "hello world", "hello world"},
		{"single tag removed", "<b>bold</b>", "bold"},
		{"search snippet", `<span class="searchmatch">budget</span> report`, "budget report"},
		{"nested tags", "<div><p>text</p></div>", "text"},
		{"empty string", "", ""},
		{"only tags", "<br/>", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripHTML(tc.in)
			if got != tc.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
