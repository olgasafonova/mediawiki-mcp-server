package converter

import "testing"

// TestGetThemeReturnsConfiguredTheme verifies GetTheme resolves each built-in
// theme by name and that the returned Theme carries the configured data, not
// just a matching Name. It also exercises the default-to-neutral fallback for
// unknown and empty names.
func TestGetThemeReturnsConfiguredTheme(t *testing.T) {
	tests := []struct {
		name         string
		lookup       string
		wantName     string
		wantHeadings int // number of heading-level color entries expected
	}{
		{name: "tieto resolves", lookup: "tieto", wantName: "tieto", wantHeadings: 6},
		{name: "neutral resolves", lookup: "neutral", wantName: "neutral", wantHeadings: 0},
		{name: "dark resolves", lookup: "dark", wantName: "dark", wantHeadings: 6},
		{name: "unknown falls back to neutral", lookup: "does-not-exist", wantName: "neutral", wantHeadings: 0},
		{name: "empty falls back to neutral", lookup: "", wantName: "neutral", wantHeadings: 0},
		{name: "case-sensitive miss falls back", lookup: "Tieto", wantName: "neutral", wantHeadings: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTheme(tt.lookup)
			if got.Name != tt.wantName {
				t.Errorf("GetTheme(%q).Name = %q, want %q", tt.lookup, got.Name, tt.wantName)
			}
			if len(got.Headings) != tt.wantHeadings {
				t.Errorf("GetTheme(%q) headings count = %d, want %d", tt.lookup, len(got.Headings), tt.wantHeadings)
			}
		})
	}
}

// TestGetThemeCalloutCompleteness verifies every built-in theme defines the
// full set of callout types the converter relies on, each with a non-empty
// emoji, label, and colors. A missing callout would silently degrade output.
func TestGetThemeCalloutCompleteness(t *testing.T) {
	requiredCallouts := []string{
		"note", "info", "tip", "warning", "caution", "important", "success",
	}

	for _, themeName := range []string{"tieto", "neutral", "dark"} {
		theme := GetTheme(themeName)
		for _, callout := range requiredCallouts {
			style, ok := theme.Callouts[callout]
			if !ok {
				t.Errorf("theme %q missing callout %q", themeName, callout)
				continue
			}
			if style.Emoji == "" {
				t.Errorf("theme %q callout %q has empty emoji", themeName, callout)
			}
			if style.Label == "" {
				t.Errorf("theme %q callout %q has empty label", themeName, callout)
			}
			if style.BorderColor == "" || style.BgColor == "" || style.TextColor == "" {
				t.Errorf("theme %q callout %q has an empty color field", themeName, callout)
			}
		}
	}
}

// TestGetThemeInlineAndCodeBlockStyles verifies the inline-code and code-block
// styling fields the renderer reads are populated for every built-in theme.
func TestGetThemeInlineAndCodeBlockStyles(t *testing.T) {
	for _, themeName := range []string{"tieto", "neutral", "dark"} {
		theme := GetTheme(themeName)
		if theme.InlineCode.BackgroundColor == "" || theme.InlineCode.TextColor == "" {
			t.Errorf("theme %q inline code style missing colors", themeName)
		}
		if theme.InlineCode.FontFamily == "" {
			t.Errorf("theme %q inline code style missing font family", themeName)
		}
		if theme.CodeBlock.BackgroundColor == "" || theme.CodeBlock.FontFamily == "" {
			t.Errorf("theme %q code block style missing fields", themeName)
		}
	}
}

// TestListThemesMatchesAvailable verifies ListThemes reports exactly the
// registered built-in themes, each with name and description carried through
// from the underlying Theme.
func TestListThemesMatchesAvailable(t *testing.T) {
	infos := ListThemes()

	if len(infos) != len(AvailableThemes) {
		t.Fatalf("ListThemes returned %d entries, want %d", len(infos), len(AvailableThemes))
	}

	seen := make(map[string]string, len(infos))
	for _, info := range infos {
		if info.Name == "" {
			t.Error("ListThemes returned an entry with empty name")
		}
		if info.Description == "" {
			t.Errorf("ListThemes entry %q has empty description", info.Name)
		}
		seen[info.Name] = info.Description
	}

	for name, theme := range AvailableThemes {
		desc, ok := seen[name]
		if !ok {
			t.Errorf("ListThemes omitted registered theme %q", name)
			continue
		}
		if desc != theme.Description {
			t.Errorf("ListThemes description for %q = %q, want %q", name, desc, theme.Description)
		}
	}
}
