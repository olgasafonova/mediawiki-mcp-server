package converter

import (
	"fmt"
	"regexp"
	"strings"
)

// Config holds conversion configuration options
type Config struct {
	Theme            string // Theme name: "tieto", "neutral", "dark"
	AddCSS           bool   // Include CSS styling block in output
	ReverseChangelog bool   // Reverse changelog entries (newest first)
	PrettifyChecks   bool   // Replace ✓ with ✅
}

// DefaultConfig returns sensible defaults for conversion
func DefaultConfig() Config {
	return Config{
		Theme:            "neutral",
		AddCSS:           false,
		ReverseChangelog: true,
		PrettifyChecks:   true,
	}
}

// Convert transforms Markdown text to MediaWiki markup
func Convert(markdown string, config Config) string {
	theme := GetTheme(config.Theme)
	text := markdown

	// Add CSS styling header if requested
	if config.AddCSS {
		text = generateCSS(theme) + "\n\n" + text
	}

	// Process in order (code first to protect special chars)
	text = convertCode(text, theme)
	text = convertBoldItalic(text)
	text = convertHeaders(text, theme)
	text = convertLinks(text)
	text = convertCallouts(text, theme)
	text = convertLists(text)
	text = convertTables(text)
	text = convertHorizontalRules(text)

	// Post-processing
	if config.ReverseChangelog {
		text = reverseChangelogOrder(text)
	}
	if config.PrettifyChecks {
		text = prettifyCheckmarks(text)
	}

	return text
}

// convertHeaders converts Markdown headers to MediaWiki format with theme colors
func convertHeaders(text string, theme Theme) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))

	headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

	for _, line := range lines {
		matches := headerRegex.FindStringSubmatch(line)
		if matches != nil {
			level := len(matches[1])
			content := matches[2]
			equals := strings.Repeat("=", level)

			// Apply color if theme defines it
			if color, ok := theme.Headings[level]; ok && color != "" {
				result = append(result, fmt.Sprintf(`%s<span style="color:%s;">%s</span>%s`, equals, color, content, equals))
			} else {
				// No color styling
				result = append(result, fmt.Sprintf(`%s%s%s`, equals, content, equals))
			}
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// convertBoldItalic converts bold and italic formatting
func convertBoldItalic(text string) string {
	// Protect code blocks from processing
	codeBlockRegex := regexp.MustCompile(`(?s)<syntaxhighlight[^>]*>.*?</syntaxhighlight>`)
	codeBlocks := codeBlockRegex.FindAllString(text, -1)
	for i, block := range codeBlocks {
		placeholder := fmt.Sprintf("XYZCODEBLOCKREPLACEMENTXYZ%dXYZ", i)
		text = strings.Replace(text, block, placeholder, 1)
	}

	// Protect inline code tags
	inlineCodeRegex := regexp.MustCompile(`<code[^>]*>.*?</code>`)
	inlineCodes := inlineCodeRegex.FindAllString(text, -1)
	for i, code := range inlineCodes {
		placeholder := fmt.Sprintf("XYZINLINECODEREPLACEMENTXYZ%dXYZ", i)
		text = strings.Replace(text, code, placeholder, 1)
	}

	// Obsidian highlights: ==text== -> <mark>text</mark>
	highlightRegex := regexp.MustCompile(`==([^=\n]+)==`)
	text = highlightRegex.ReplaceAllString(text, `<mark style="background-color:#f5ff56">$1</mark>`)

	// Bold: **text** or __text__ -> '''text'''
	boldRegex1 := regexp.MustCompile(`\*\*(.+?)\*\*`)
	text = boldRegex1.ReplaceAllString(text, `'''$1'''`)
	boldRegex2 := regexp.MustCompile(`__(.+?)__`)
	text = boldRegex2.ReplaceAllString(text, `'''$1'''`)

	// Italic: *text* or _text_ -> ''text''
	// Capture boundary characters to preserve spacing around formatting
	italicRegex1 := regexp.MustCompile(`(^|[^\*])\*([^\*\n]+?)\*([^\*]|$)`)
	text = italicRegex1.ReplaceAllString(text, `$1''$2''$3`)
	italicRegex2 := regexp.MustCompile(`(^|[^_])_([^_\n]+?)_([^_]|$)`)
	text = italicRegex2.ReplaceAllString(text, `$1''$2''$3`)

	// Restore inline code
	for i, code := range inlineCodes {
		placeholder := fmt.Sprintf("XYZINLINECODEREPLACEMENTXYZ%dXYZ", i)
		text = strings.Replace(text, placeholder, code, 1)
	}

	// Restore code blocks
	for i, block := range codeBlocks {
		placeholder := fmt.Sprintf("XYZCODEBLOCKREPLACEMENTXYZ%dXYZ", i)
		text = strings.Replace(text, placeholder, block, 1)
	}

	return text
}

// convertLinks converts Markdown links to MediaWiki format
func convertLinks(text string) string {
	// External links: [text](url) -> [url text]
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\)]+)\)`)
	text = linkRegex.ReplaceAllString(text, `[$2 $1]`)

	// Internal wiki links: [[Page]] or [[Page|Text]] stay as-is (MediaWiki native)
	return text
}

// calloutForm describes one callout syntax variant: how to match it (the
// pattern gets the callout type spliced in), how to clean the captured
// content, and what separates the label from the content in the output.
type calloutForm struct {
	pattern string
	clean   func(string) string
	sep     string
}

// calloutForms lists the supported callout syntaxes, in match order:
// multi-line ("> [!TYPE]\n> content") before single-line ("> [!TYPE] content").
var calloutForms = []calloutForm{
	{pattern: `(?im)^>\s*\[!%s\]\s*\n?((?:>.*\n?)+)`, clean: cleanCalloutContent, sep: "<br/>"},
	{pattern: `(?im)^>\s*\[!%s\]\s+(.+)$`, clean: strings.TrimSpace, sep: " "},
}

// convertCallouts converts markdown callouts to MediaWiki styled boxes
func convertCallouts(text string, theme Theme) string {
	for calloutType, style := range theme.Callouts {
		for _, form := range calloutForms {
			text = convertCalloutForm(text, calloutType, form, style)
		}
	}

	return text
}

// renderCalloutBox formats a styled wikitable callout box. sep separates the
// label from the content ("<br/>" for multi-line callouts, " " for single-line).
func renderCalloutBox(style CalloutStyle, sep, content string) string {
	return fmt.Sprintf(`{| class="wikitable" style="border-left:4px solid %s; background-color:%s; width:100%%;"
| <div style="padding:0.5em;">
<strong style="color:%s;">%s %s:</strong>%s%s
</div>
|}`, style.BorderColor, style.BgColor, style.TextColor, style.Emoji, style.Label, sep, content)
}

// cleanCalloutContent strips the leading "> " quote markers and joins the
// remaining lines with <br/>, dropping leading blank lines.
func cleanCalloutContent(raw string) string {
	quoteMarker := regexp.MustCompile(`^>\s?`)
	var cleanLines []string
	for _, line := range strings.Split(raw, "\n") {
		cleaned := quoteMarker.ReplaceAllString(line, "")
		if strings.TrimSpace(cleaned) != "" || len(cleanLines) > 0 {
			cleanLines = append(cleanLines, cleaned)
		}
	}
	return strings.TrimSpace(strings.Join(cleanLines, "<br/>"))
}

// convertCalloutForm converts every occurrence of one callout syntax variant
// for one callout type into a styled box.
func convertCalloutForm(text, calloutType string, form calloutForm, style CalloutStyle) string {
	calloutRegex := regexp.MustCompile(fmt.Sprintf(form.pattern, calloutType))

	return calloutRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := calloutRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		return renderCalloutBox(style, form.sep, form.clean(submatch[1]))
	})
}

// convertCode converts code formatting
func convertCode(text string, theme Theme) string {
	// Fenced code blocks first
	codeBlockRegex := regexp.MustCompile("(?s)```(\\w+)?\\n(.*?)```")
	text = codeBlockRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := codeBlockRegex.FindStringSubmatch(match)
		lang := strings.TrimSpace(submatch[1])
		code := strings.TrimSpace(submatch[2])

		// Auto-detect language if not specified
		if lang == "" {
			lang = detectLanguage(code)
		}

		return fmt.Sprintf("<syntaxhighlight lang=\"%s\" line>\n%s\n</syntaxhighlight>", lang, code)
	})

	// Inline code: `code` -> <code style="...">code</code>
	style := theme.InlineCode
	inlineCodeRegex := regexp.MustCompile("`([^`\n]+)`")
	replacement := fmt.Sprintf(`<code style="background-color:%s;color:%s;padding:%s;border-radius:%s;font-family:%s;">$1</code>`,
		style.BackgroundColor, style.TextColor, style.Padding, style.BorderRadius, style.FontFamily)
	text = inlineCodeRegex.ReplaceAllString(text, replacement)

	return text
}

// detectLanguage attempts to auto-detect code language
func detectLanguage(code string) string {
	codeStripped := strings.TrimSpace(code)
	if strings.HasPrefix(codeStripped, "{") || strings.HasPrefix(codeStripped, "[") {
		return "json"
	} else if strings.HasPrefix(codeStripped, "<") {
		return "xml"
	} else if strings.Contains(strings.ToUpper(code), "SELECT") || strings.Contains(strings.ToUpper(code), "FROM") {
		return "sql"
	}
	return "text"
}

// convertHorizontalRules converts Markdown hr to MediaWiki
func convertHorizontalRules(text string) string {
	hrRegex := regexp.MustCompile(`(?m)^[\s]*[-*_]{3,}[\s]*$`)
	return hrRegex.ReplaceAllString(text, "----")
}

// reverseChangelogOrder reverses version sections in changelogs
func reverseChangelogOrder(text string) string {
	changelogHeaderRegex := regexp.MustCompile(`(?s)(===<span[^>]*>.*?Changelog.*?</span>===)`)
	headerMatch := changelogHeaderRegex.FindStringIndex(text)

	if headerMatch == nil {
		return text
	}

	beforeChangelog := text[:headerMatch[0]]
	changelogHeader := text[headerMatch[0]:headerMatch[1]]

	remainingText := text[headerMatch[1]:]

	versionHeaderRegex := regexp.MustCompile(`====<span[^>]*>Version[^<]*</span>====`)
	versionMatches := versionHeaderRegex.FindAllStringIndex(remainingText, -1)

	if len(versionMatches) == 0 {
		return text
	}

	nextSectionRegex := regexp.MustCompile(`(?m)^={1,3}<span[^>]*>[^<]*</span>={1,3}$`)
	changelogEndIdx := len(remainingText)
	nextSection := nextSectionRegex.FindStringIndex(remainingText)
	if nextSection != nil {
		changelogEndIdx = nextSection[0]
	}

	versions := sliceVersionSections(remainingText, versionMatches, changelogEndIdx)
	reverseStringsInPlace(versions)

	afterChangelog := remainingText[changelogEndIdx:]
	newChangelog := changelogHeader + "\n\n" + strings.Join(versions, "")

	return beforeChangelog + newChangelog + afterChangelog
}

// sliceVersionSections cuts the changelog body into one string per version
// section, using the match positions as boundaries.
func sliceVersionSections(remainingText string, versionMatches [][]int, changelogEndIdx int) []string {
	var versions []string
	for i, match := range versionMatches {
		start := match[0]
		end := changelogEndIdx
		if i < len(versionMatches)-1 {
			end = versionMatches[i+1][0]
		}
		if start < end {
			versions = append(versions, remainingText[start:end])
		}
	}
	return versions
}

// reverseStringsInPlace reverses the order of the elements in s.
func reverseStringsInPlace(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// prettifyCheckmarks replaces plain checkmarks with emoji
func prettifyCheckmarks(text string) string {
	return strings.ReplaceAll(text, "✓", "✅")
}

// generateCSS creates a CSS block for wiki styling
func generateCSS(theme Theme) string {
	style := theme.CodeBlock
	inline := theme.InlineCode

	var headingCSS strings.Builder
	for level := 1; level <= 6; level++ {
		if color, ok := theme.Headings[level]; ok && color != "" {
			headingCSS.WriteString(fmt.Sprintf(`
.mw-parser-output h%d,
h%d {
    color: %s !important;
    font-weight: 600 !important;
}
`, level, level, color))
		}
	}

	return fmt.Sprintf(`<div style="display:none;">
<!-- Theme: %s - %s -->
<style>
/* Code block container */
.mw-highlight {
    background-color: %s !important;
    border: 1px solid %s !important;
    border-left: 3px solid %s !important;
    padding: 1em !important;
    border-radius: 4px;
    font-family: %s !important;
    font-size: 0.95em !important;
    line-height: 1.5 !important;
}

/* Inline code styling */
code {
    background-color: %s !important;
    color: %s !important;
    padding: %s !important;
    border-radius: %s !important;
    font-family: %s !important;
}
%s
</style>
</div>`, theme.Name, theme.Description,
		style.BackgroundColor, style.BorderColor, style.BorderLeftColor, style.FontFamily,
		inline.BackgroundColor, inline.TextColor, inline.Padding, inline.BorderRadius, inline.FontFamily,
		headingCSS.String())
}
