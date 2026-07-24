package wiki

import (
	"regexp"
	"strings"
)

// extractContextsForTerm extracts lines containing the search term
func extractContextsForTerm(content, term string, maxContexts int) []string {
	termLower := strings.ToLower(term)
	contexts := make([]string, 0)

	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(strings.ToLower(line), termLower) {
			continue
		}
		cleaned, ok := cleanContextLine(line)
		if !ok {
			continue
		}
		contexts = append(contexts, cleaned)
		if maxContexts > 0 && len(contexts) >= maxContexts {
			break
		}
	}

	return contexts
}

// cleanContextLine trims a context line and truncates it to 200 chars. ok is
// false for blank or too-short (<= 10 chars) lines, which are skipped.
func cleanContextLine(line string) (string, bool) {
	cleaned := strings.TrimSpace(line)
	if cleaned == "" || len(cleaned) <= 10 {
		return "", false
	}
	if len(cleaned) > 200 {
		cleaned = cleaned[:200] + "..."
	}
	return cleaned, true
}

// ValuePattern represents a pattern for extracting typed values
type ValuePattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// Common value patterns for inconsistency detection
var valuePatterns = []ValuePattern{
	{"timeout", regexp.MustCompile(`(?i)timeout\s*[=:]\s*(\d+)\s*(s|sec|seconds?|ms|min|minutes?)?`)},
	{"version", regexp.MustCompile(`(?i)v(?:ersion)?\s*(\d+\.\d+(?:\.\d+)?)`)},
	{"port", regexp.MustCompile(`(?i)port\s*[=:]\s*(\d+)`)},
	{"limit", regexp.MustCompile(`(?i)(?:max|limit|maximum)\s*[=:]\s*(\d+)`)},
	{"count", regexp.MustCompile(`(?i)(\d+)\s+(users?|items?|pages?|requests?|connections?)`)},
	{"size", regexp.MustCompile(`(?i)(\d+)\s*(kb|mb|gb|bytes?|b)`)},
	{"percentage", regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)},
	{"duration", regexp.MustCompile(`(?i)(\d+)\s*(seconds?|minutes?|hours?|days?|weeks?|months?|years?)`)},
}

// ExtractedValue represents a value extracted from text
type ExtractedValue struct {
	Type    string
	Value   string
	Context string
}

// extractValues finds typed values in content using patterns
func extractValues(content string) []ExtractedValue {
	values := make([]ExtractedValue, 0)
	for _, line := range strings.Split(content, "\n") {
		values = append(values, extractValuesFromLine(line)...)
	}
	return values
}

// extractValuesFromLine applies every value pattern to one line.
func extractValuesFromLine(line string) []ExtractedValue {
	var values []ExtractedValue
	trimmed := strings.TrimSpace(line)
	for _, vp := range valuePatterns {
		for _, match := range vp.Pattern.FindAllStringSubmatch(line, -1) {
			if len(match) >= 2 {
				values = append(values, ExtractedValue{
					Type:    vp.Name,
					Value:   match[0],
					Context: trimmed,
				})
			}
		}
	}
	return values
}
