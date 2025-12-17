package converter

import (
	"strings"
	"testing"
)

func TestConvertHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		theme    string
		contains string
	}{
		{
			name:     "H1 with tieto theme",
			input:    "# Hello World",
			theme:    "tieto",
			contains: `=<span style="color:#021e57;">Hello World</span>=`,
		},
		{
			name:     "H2 with tieto theme",
			input:    "## Section Title",
			theme:    "tieto",
			contains: `==<span style="color:#021e57;">Section Title</span>==`,
		},
		{
			name:     "H1 with neutral theme (no color)",
			input:    "# Hello World",
			theme:    "neutral",
			contains: `=Hello World=`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{Theme: tt.theme}
			result := Convert(tt.input, config)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestConvertBoldItalic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Bold with asterisks",
			input:    "This is **bold** text",
			expected: "This is '''bold''' text",
		},
		{
			name:     "Bold with underscores",
			input:    "This is __bold__ text",
			expected: "This is '''bold''' text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertBoldItalic(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConvertLinks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "External link",
			input:    "[Click here](https://example.com)",
			expected: "[https://example.com Click here]",
		},
		{
			name:     "External link with https",
			input:    "[Google](https://www.google.com)",
			expected: "[https://www.google.com Google]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertLinks(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConvertCode(t *testing.T) {
	theme := ThemeNeutral

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Inline code",
			input:    "Use the `print` function",
			contains: "<code",
		},
		{
			name:     "Code block with language",
			input:    "```python\nprint('hello')\n```",
			contains: `<syntaxhighlight lang="python"`,
		},
		{
			name:     "Code block auto-detect JSON",
			input:    "```\n{\"key\": \"value\"}\n```",
			contains: `<syntaxhighlight lang="json"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertCode(tt.input, theme)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestConvertLists(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Unordered list",
			input:    "- Item 1\n- Item 2",
			contains: "* Item 1",
		},
		{
			name:     "Ordered list",
			input:    "1. First\n2. Second",
			contains: "# First",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertLists(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestConvertTables(t *testing.T) {
	input := `| Header 1 | Header 2 |
| -------- | -------- |
| Cell 1   | Cell 2   |`

	result := convertTables(input)

	if !strings.Contains(result, `{| class="wikitable"`) {
		t.Error("Expected MediaWiki table opening")
	}
	if !strings.Contains(result, "! Header 1") {
		t.Error("Expected header row")
	}
	if !strings.Contains(result, "| Cell 1") {
		t.Error("Expected data row")
	}
	if !strings.Contains(result, "|}") {
		t.Error("Expected table closing")
	}
}

func TestConvertCallouts(t *testing.T) {
	theme := ThemeTieto

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Note callout",
			input:    "> [!NOTE]\n> This is a note",
			contains: "📝 Note:",
		},
		{
			name:     "Warning callout",
			input:    "> [!WARNING]\n> Be careful!",
			contains: "⚠️ Warning:",
		},
		{
			name:     "Single line callout",
			input:    "> [!TIP] Quick tip here",
			contains: "💡 Tip:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertCallouts(tt.input, theme)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestConvertHorizontalRules(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"---", "----"},
		{"***", "----"},
		{"___", "----"},
		{"-----", "----"},
	}

	for _, tt := range tests {
		result := convertHorizontalRules(tt.input)
		if result != tt.expected {
			t.Errorf("Expected %q, got %q", tt.expected, result)
		}
	}
}

func TestPrettifyCheckmarks(t *testing.T) {
	input := "Task complete ✓"
	expected := "Task complete ✅"
	result := prettifyCheckmarks(input)
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestGetTheme(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"tieto", "tieto"},
		{"neutral", "neutral"},
		{"dark", "dark"},
		{"nonexistent", "neutral"}, // Should default to neutral
	}

	for _, tt := range tests {
		theme := GetTheme(tt.name)
		if theme.Name != tt.expected {
			t.Errorf("GetTheme(%q) = %q, want %q", tt.name, theme.Name, tt.expected)
		}
	}
}

func TestListThemes(t *testing.T) {
	themes := ListThemes()
	if len(themes) < 3 {
		t.Errorf("Expected at least 3 themes, got %d", len(themes))
	}

	// Check that all themes have names and descriptions
	for _, theme := range themes {
		if theme.Name == "" {
			t.Error("Theme has empty name")
		}
		if theme.Description == "" {
			t.Errorf("Theme %q has empty description", theme.Name)
		}
	}
}

func TestFullConversion(t *testing.T) {
	input := `# Welcome

This is **bold** and *italic* text.

## Features

- Feature 1
- Feature 2

| Name | Value |
|------|-------|
| A    | 1     |

` + "```json\n{\"key\": \"value\"}\n```"

	config := Config{
		Theme:            "tieto",
		AddCSS:           false,
		ReverseChangelog: true,
		PrettifyChecks:   true,
	}

	result := Convert(input, config)

	// Verify key conversions happened
	checks := []string{
		"=<span style=\"color:#021e57;\">Welcome</span>=", // Header with color
		"'''bold'''",           // Bold
		`{| class="wikitable"`, // Table
		"<syntaxhighlight",     // Code block
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("Expected result to contain %q", check)
		}
	}
}

func TestConversionWithCSS(t *testing.T) {
	input := "# Hello"
	config := Config{
		Theme:  "tieto",
		AddCSS: true,
	}

	result := Convert(input, config)

	if !strings.Contains(result, "<style>") {
		t.Error("Expected CSS style block when AddCSS is true")
	}
	if !strings.Contains(result, "display:none") {
		t.Error("Expected hidden div wrapper for CSS")
	}
}

func BenchmarkConvert(b *testing.B) {
	input := strings.Repeat(`# Heading

This is **bold** and *italic* text with `+"`inline code`"+`.

- List item 1
- List item 2

| Col 1 | Col 2 |
|-------|-------|
| A     | B     |

`, 10)

	config := DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Convert(input, config)
	}
}
