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

// convertCallouts converts markdown callouts to MediaWiki styled boxes
func convertCallouts(text string, theme Theme) string {
	for calloutType, style := range theme.Callouts {
		// Multi-line callout: > [!TYPE]\n> content
		pattern := fmt.Sprintf(`(?im)^>\s*\[!%s\]\s*\n?((?:>.*\n?)+)`, calloutType)
		calloutRegex := regexp.MustCompile(pattern)

		text = calloutRegex.ReplaceAllStringFunc(text, func(match string) string {
			contentRegex := regexp.MustCompile(`(?im)^>\s*\[!` + calloutType + `\]\s*\n?((?:>.*\n?)+)`)
			submatch := contentRegex.FindStringSubmatch(match)
			if len(submatch) < 2 {
				return match
			}

			// Clean content lines
			contentLines := strings.Split(submatch[1], "\n")
			var cleanLines []string
			for _, line := range contentLines {
				cleaned := regexp.MustCompile(`^>\s?`).ReplaceAllString(line, "")
				if strings.TrimSpace(cleaned) != "" || len(cleanLines) > 0 {
					cleanLines = append(cleanLines, cleaned)
				}
			}
			content := strings.TrimSpace(strings.Join(cleanLines, "<br/>"))

			return fmt.Sprintf(`{| class="wikitable" style="border-left:4px solid %s; background-color:%s; width:100%%;"
| <div style="padding:0.5em;">
<strong style="color:%s;">%s %s:</strong><br/>%s
</div>
|}`, style.BorderColor, style.BgColor, style.TextColor, style.Emoji, style.Label, content)
		})

		// Single-line callout: > [!TYPE] content
		singleLinePattern := fmt.Sprintf(`(?im)^>\s*\[!%s\]\s+(.+)$`, calloutType)
		singleLineRegex := regexp.MustCompile(singleLinePattern)
		text = singleLineRegex.ReplaceAllStringFunc(text, func(match string) string {
			submatch := singleLineRegex.FindStringSubmatch(match)
			if len(submatch) < 2 {
				return match
			}
			content := strings.TrimSpace(submatch[1])

			return fmt.Sprintf(`{| class="wikitable" style="border-left:4px solid %s; background-color:%s; width:100%%;"
| <div style="padding:0.5em;">
<strong style="color:%s;">%s %s:</strong> %s
</div>
|}`, style.BorderColor, style.BgColor, style.TextColor, style.Emoji, style.Label, content)
		})
	}

	return text
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

// listItem tracks list type at each indent level
type listItem struct {
	indentLevel int
	listType    string // "*" or "#"
}

// convertLists converts Markdown lists to MediaWiki format
func convertLists(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))

	unorderedRegex := regexp.MustCompile(`^(\s*)[-\*]\s+(.*)$`)
	orderedRegex := regexp.MustCompile(`^(\s*)\d+\.\s+(.*)$`)

	var listStack []listItem

	for _, line := range lines {
		if matches := unorderedRegex.FindStringSubmatch(line); matches != nil {
			indent := len(matches[1])
			content := matches[2]
			currentLevel := indent / 2

			prefix := buildListPrefix(listStack, currentLevel, "*")
			line = prefix + " " + content
			listStack = updateListStack(listStack, currentLevel, "*")

		} else if matches := orderedRegex.FindStringSubmatch(line); matches != nil {
			indent := len(matches[1])
			content := matches[2]
			currentLevel := indent / 2

			prefix := buildListPrefix(listStack, currentLevel, "#")
			line = prefix + " " + content
			listStack = updateListStack(listStack, currentLevel, "#")

		} else {
			listStack = nil
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func buildListPrefix(stack []listItem, currentLevel int, currentType string) string {
	prefix := ""
	for i := 0; i <= currentLevel && i < len(stack); i++ {
		prefix += stack[i].listType
	}
	if currentLevel >= len(stack) {
		prefix += currentType
	}
	return prefix
}

func updateListStack(stack []listItem, currentLevel int, currentType string) []listItem {
	if currentLevel < len(stack) {
		stack = stack[:currentLevel]
	}
	if currentLevel < len(stack) {
		stack[currentLevel].listType = currentType
	} else {
		stack = append(stack, listItem{
			indentLevel: currentLevel,
			listType:    currentType,
		})
	}
	return stack
}

// convertTables converts Markdown tables to MediaWiki format
func convertTables(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	inTable := false

	pipeRegex := regexp.MustCompile(`^\|.*\|$`)
	separatorRegex := regexp.MustCompile(`^\|[\s\-:|]+\|$`)

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		if strings.Contains(line, "|") && !inTable {
			if pipeRegex.MatchString(line) {
				result = append(result, `{| class="wikitable"`)
				inTable = true

				// Header row
				cells := strings.Split(line, "|")
				cells = cells[1 : len(cells)-1]
				result = append(result, "|-")
				for _, cell := range cells {
					result = append(result, "! "+strings.TrimSpace(cell))
				}

				// Skip separator line
				i++
				if i < len(lines) {
					nextLine := strings.TrimSpace(lines[i])
					if strings.Contains(nextLine, "|") && strings.Contains(nextLine, "-") {
						i++
					}
				}
				continue
			}
		} else if inTable && strings.Contains(line, "|") {
			if separatorRegex.MatchString(line) && strings.Contains(line, "-") {
				i++
				continue
			}

			cells := strings.Split(line, "|")
			if len(cells) > 2 {
				cells = cells[1 : len(cells)-1]

				// Skip separator rows
				allDashes := true
				for _, cell := range cells {
					if strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(cell, "-", ""), ":", "")) != "" {
						allDashes = false
						break
					}
				}
				if allDashes {
					i++
					continue
				}

				result = append(result, "|-")
				for _, cell := range cells {
					result = append(result, "| "+strings.TrimSpace(cell))
				}
			}
			i++
			continue
		} else if inTable && !strings.Contains(line, "|") {
			result = append(result, "|}")
			inTable = false
		}

		result = append(result, lines[i])
		i++
	}

	if inTable {
		result = append(result, "|}")
	}

	return strings.Join(result, "\n")
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

	var versions []string
	for i, match := range versionMatches {
		start := match[0]
		var end int
		if i < len(versionMatches)-1 {
			end = versionMatches[i+1][0]
		} else {
			end = changelogEndIdx
		}
		if start < end {
			versions = append(versions, remainingText[start:end])
		}
	}

	// Reverse
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}

	afterChangelog := remainingText[changelogEndIdx:]
	newChangelog := changelogHeader + "\n\n" + strings.Join(versions, "")

	return beforeChangelog + newChangelog + afterChangelog
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
