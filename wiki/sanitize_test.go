package wiki

import (
	"strings"
	"testing"
)

func TestSanitizeHTML_ScriptTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple script", "<script>alert('xss')</script>"},
		{"Script with attributes", `<script type="text/javascript">alert('xss')</script>`},
		{"Script with newlines", "<script>\nalert('xss')\n</script>"},
		{"Multiple scripts", "<script>a()</script><script>b()</script>"},
		{"Uppercase SCRIPT", "<SCRIPT>alert('xss')</SCRIPT>"},
		{"Mixed case", "<ScRiPt>alert('xss')</sCrIpT>"},
		{"Script with src", `<script src="evil.js"></script>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "<script") {
				t.Errorf("Script tag not removed: %q => %q", tt.input, result)
			}
			if strings.Contains(result, "alert") {
				t.Errorf("Script content not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_StyleTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple style", "<style>body{display:none}</style>"},
		{"Style with attributes", `<style type="text/css">body{display:none}</style>`},
		{"Uppercase STYLE", "<STYLE>body{display:none}</STYLE>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "<style") {
				t.Errorf("Style tag not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_IframeTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple iframe", "<iframe src='evil.com'></iframe>"},
		{"Iframe with content", "<iframe>content</iframe>"},
		{"Uppercase IFRAME", "<IFRAME src='evil.com'></IFRAME>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "<iframe") {
				t.Errorf("Iframe tag not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_ObjectEmbedApplet(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Object tag", "<object data='malware.swf'></object>"},
		{"Embed tag", "<embed src='malware.swf'></embed>"},
		{"Applet tag", "<applet code='Evil.class'></applet>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "<object") ||
				strings.Contains(strings.ToLower(result), "<embed") ||
				strings.Contains(strings.ToLower(result), "<applet") {
				t.Errorf("Dangerous tag not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_FormTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple form", "<form action='evil.com'><input></form>"},
		{"Form with method", `<form method="post" action="steal.php">data</form>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "<form") {
				t.Errorf("Form tag not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_MetaLinkBase(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Meta redirect", `<meta http-equiv="refresh" content="0;url=evil.com">`},
		{"Link tag", `<link rel="stylesheet" href="evil.css">`},
		{"Base tag", `<base href="evil.com">`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "<meta") ||
				strings.Contains(strings.ToLower(result), "<link") ||
				strings.Contains(strings.ToLower(result), "<base") {
				t.Errorf("Dangerous tag not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_EventHandlers(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"onclick", `<div onclick="alert('xss')">click me</div>`},
		{"onerror", `<img onerror="alert('xss')" src="x">`},
		{"onload", `<body onload="alert('xss')">`},
		{"onmouseover", `<a onmouseover="alert('xss')">hover</a>`},
		{"onfocus", `<input onfocus="alert('xss')">`},
		{"onblur", `<input onblur="alert('xss')">`},
		{"Uppercase ONCLICK", `<div ONCLICK="alert('xss')">click</div>`},
		{"Mixed case OnClick", `<div OnClick="alert('xss')">click</div>`},
		{"Double quotes", `<div onclick="alert('xss')">click</div>`},
		{"Single quotes", `<div onclick='alert("xss")'>click</div>`},
		{"No quotes", `<div onclick=alert('xss')>click</div>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "onclick") ||
				strings.Contains(strings.ToLower(result), "onerror") ||
				strings.Contains(strings.ToLower(result), "onload") ||
				strings.Contains(strings.ToLower(result), "onmouseover") ||
				strings.Contains(strings.ToLower(result), "onfocus") ||
				strings.Contains(strings.ToLower(result), "onblur") {
				t.Errorf("Event handler not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_JavaScriptURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"href javascript", `<a href="javascript:alert('xss')">click</a>`},
		{"src javascript", `<img src="javascript:alert('xss')">`},
		{"action javascript", `<form action="javascript:alert('xss')">`},
		{"Uppercase JAVASCRIPT", `<a href="JAVASCRIPT:alert('xss')">click</a>`},
		{"With spaces", `<a href=" javascript:alert('xss')">click</a>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(result), "javascript:") {
				t.Errorf("JavaScript URL not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_DataURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"data URL in href", `<a href="data:text/html,<script>alert('xss')</script>">click</a>`},
		{"data URL in src", `<img src="data:image/svg+xml,<svg onload=alert('xss')>">`},
		{"Uppercase DATA", `<a href="DATA:text/html,evil">click</a>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			// The regex should remove data: URLs in href/src
			if strings.Contains(strings.ToLower(result), "data:text/html") ||
				strings.Contains(strings.ToLower(result), "data:image/svg") {
				t.Errorf("Data URL not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_StyleAttributes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Style with expression", `<div style="width:expression(alert('xss'))">content</div>`},
		{"Style with behavior", `<div style="behavior:url(evil.htc)">content</div>`},
		{"Normal style removed", `<div style="color:red">content</div>`},
		{"Uppercase STYLE", `<div STYLE="color:red">content</div>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			// Style attributes should be removed entirely
			if strings.Contains(strings.ToLower(result), "style=") {
				t.Errorf("Style attribute not removed: %q => %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeHTML_NestedMaliciousContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			"Script in div",
			"<div><script>alert('xss')</script></div>",
		},
		{
			"Multiple levels",
			"<div><p><span onclick='evil()'><script>bad()</script></span></p></div>",
		},
		{
			"Mixed dangerous content",
			`<div onclick="a()"><script>b()</script><style>c{}</style><iframe>d</iframe></div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			dangerous := []string{"<script", "<style", "<iframe", "onclick"}
			for _, d := range dangerous {
				if strings.Contains(strings.ToLower(result), d) {
					t.Errorf("Nested dangerous content not removed: %q => %q", tt.input, result)
				}
			}
		})
	}
}

func TestSanitizeHTML_PreservesSafeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"Preserves text", "<p>Hello World</p>", "Hello World"},
		{"Preserves safe tags", "<p><b>Bold</b> and <i>italic</i></p>", "Bold"},
		{"Preserves links without JS", `<a href="https://example.com">link</a>`, "link"},
		{"Preserves images without events", `<img src="image.png" alt="image">`, `src="image.png"`},
		{"Preserves tables", "<table><tr><td>cell</td></tr></table>", "cell"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHTML(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Safe content not preserved: %q => %q, expected to contain %q", tt.input, result, tt.contains)
			}
		})
	}
}

func TestSanitizeHTML_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Empty string", ""},
		{"Only whitespace", "   \n\t  "},
		{"Plain text", "Hello World"},
		{"Broken HTML", "<div><p>unclosed"},
		{"Unicode", "<script>\u0000alert('xss')</script>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := sanitizeHTML(tt.input)
			_ = result
		})
	}
}

// Benchmark sanitization performance
func BenchmarkSanitizeHTML_Simple(b *testing.B) {
	input := "<p>Hello <b>World</b></p>"
	for i := 0; i < b.N; i++ {
		sanitizeHTML(input)
	}
}

func BenchmarkSanitizeHTML_Complex(b *testing.B) {
	input := `<div onclick="evil()"><script>bad()</script><style>.x{}</style>
	<iframe src="bad.com"></iframe><a href="javascript:alert()">link</a>
	<p style="expression(alert())">text</p></div>`
	for i := 0; i < b.N; i++ {
		sanitizeHTML(input)
	}
}
