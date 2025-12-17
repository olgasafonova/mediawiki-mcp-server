package wiki

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// English stopwords to filter out during term extraction
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "must": true, "shall": true,
	"this": true, "that": true, "these": true, "those": true,
	"and": true, "or": true, "but": true, "if": true, "then": true,
	"for": true, "with": true, "from": true, "to": true, "of": true,
	"in": true, "on": true, "at": true, "by": true, "as": true,
	"it": true, "its": true, "you": true, "your": true, "we": true,
	"our": true, "they": true, "their": true, "he": true, "she": true,
	"his": true, "her": true, "who": true, "what": true, "which": true,
	"when": true, "where": true, "how": true, "why": true,
	"can": true, "not": true, "all": true, "each": true, "every": true,
	"both": true, "few": true, "more": true, "most": true, "other": true,
	"some": true, "such": true, "than": true, "too": true, "very": true,
	"just": true, "also": true, "only": true, "own": true, "same": true,
	"so": true, "into": true, "over": true, "after": true, "before": true,
	"between": true, "under": true, "again": true, "further": true,
	"here": true, "there": true, "once": true, "during": true,
	"about": true, "through": true, "above": true, "below": true,
	"any": true, "no": true, "nor": true, "because": true, "until": true,
	"while": true, "out": true, "up": true, "down": true, "off": true,
	"now": true, "well": true, "back": true, "get": true, "got": true,
	"see": true, "use": true, "used": true, "using": true,
}

// Pre-compiled regex for whitespace normalization (performance optimization)
// Uses \s+ to match all whitespace including newlines for wiki markup cleanup
var multiWhitespaceRegex = regexp.MustCompile(`\s+`)

// Wiki markup patterns to remove
var wikiMarkupPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\[\[Category:[^\]]+\]\]`),                    // Category links
	regexp.MustCompile(`\[\[[^\]|]+\|([^\]]+)\]\]`),                  // Links with display text
	regexp.MustCompile(`\[\[([^\]]+)\]\]`),                           // Simple links
	regexp.MustCompile(`\{\{[^}]+\}\}`),                              // Templates
	regexp.MustCompile(`<ref[^>]*>.*?</ref>`),                        // References
	regexp.MustCompile(`<ref[^/]*/?>`),                               // Self-closing refs
	regexp.MustCompile(`<[^>]+>`),                                    // All HTML tags
	regexp.MustCompile(`'''([^']+)'''`),                              // Bold
	regexp.MustCompile(`''([^']+)''`),                                // Italic
	regexp.MustCompile(`={2,}([^=]+)={2,}`),                          // Section headers
	regexp.MustCompile(`\|[^|}\n]+`),                                 // Table cells
	regexp.MustCompile(`\{\|[^}]*\|\}`),                              // Tables
	regexp.MustCompile(`^\*+\s*`),                                    // List items
	regexp.MustCompile(`^#+\s*`),                                     // Numbered lists
	regexp.MustCompile(`https?://[^\s\]]+`),                          // URLs
	regexp.MustCompile(`\[https?://[^\s\]]+ ([^\]]+)\]`),             // External links with text
	regexp.MustCompile(`\[https?://[^\]]+\]`),                        // External links
}

// removeWikiMarkup strips wiki markup from content, leaving plain text
func removeWikiMarkup(content string) string {
	result := content

	// Apply all patterns
	for _, pattern := range wikiMarkupPatterns {
		result = pattern.ReplaceAllString(result, " $1 ")
	}

	// Remove multiple spaces (using pre-compiled regex)
	result = multiWhitespaceRegex.ReplaceAllString(result, " ")

	return strings.TrimSpace(result)
}

// extractKeyTerms extracts significant terms from content
func extractKeyTerms(content string) []string {
	// Remove wiki markup first
	plainText := removeWikiMarkup(content)

	// Lowercase
	plainText = strings.ToLower(plainText)

	// Tokenize: split on non-letter characters
	words := strings.FieldsFunc(plainText, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	// Filter and dedupe
	termSet := make(map[string]bool)
	for _, word := range words {
		// Skip short words
		if len(word) < 3 {
			continue
		}
		// Skip stopwords
		if stopwords[word] {
			continue
		}
		// Skip pure numbers
		if isNumeric(word) {
			continue
		}
		termSet[word] = true
	}

	// Convert to slice
	terms := make([]string, 0, len(termSet))
	for term := range termSet {
		terms = append(terms, term)
	}

	return terms
}

// isNumeric checks if a string is purely numeric
func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// calculateJaccardSimilarity calculates Jaccard similarity between two term sets
func calculateJaccardSimilarity(termsA, termsB []string) float64 {
	if len(termsA) == 0 && len(termsB) == 0 {
		return 0
	}

	setA := make(map[string]bool)
	for _, term := range termsA {
		setA[term] = true
	}

	setB := make(map[string]bool)
	for _, term := range termsB {
		setB[term] = true
	}

	// Calculate intersection
	intersection := 0
	for term := range setA {
		if setB[term] {
			intersection++
		}
	}

	// Calculate union
	union := len(setA)
	for term := range setB {
		if !setA[term] {
			union++
		}
	}

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// findCommonTerms returns terms present in both slices
func findCommonTerms(termsA, termsB []string, limit int) []string {
	setA := make(map[string]bool)
	for _, term := range termsA {
		setA[term] = true
	}

	common := make([]string, 0)
	seen := make(map[string]bool)
	for _, term := range termsB {
		if setA[term] && !seen[term] {
			common = append(common, term)
			seen[term] = true
		}
	}

	// Sort by length (longer = more significant)
	sort.Slice(common, func(i, j int) bool {
		return len(common[i]) > len(common[j])
	})

	if limit > 0 && len(common) > limit {
		return common[:limit]
	}
	return common
}

// extractContextsForTerm extracts lines containing the search term
func extractContextsForTerm(content, term string, maxContexts int) []string {
	lines := strings.Split(content, "\n")
	termLower := strings.ToLower(term)
	contexts := make([]string, 0)

	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), termLower) {
			// Clean up the line
			cleaned := strings.TrimSpace(line)
			if cleaned != "" && len(cleaned) > 10 {
				// Truncate long lines
				if len(cleaned) > 200 {
					cleaned = cleaned[:200] + "..."
				}
				contexts = append(contexts, cleaned)
				if maxContexts > 0 && len(contexts) >= maxContexts {
					break
				}
			}
		}
	}

	return contexts
}

// extractTopTerms gets the most frequent significant terms
func extractTopTerms(content string, limit int) []string {
	// Remove wiki markup
	plainText := removeWikiMarkup(content)
	plainText = strings.ToLower(plainText)

	// Tokenize
	words := strings.FieldsFunc(plainText, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	// Count frequencies
	freq := make(map[string]int)
	for _, word := range words {
		if len(word) < 3 || stopwords[word] || isNumeric(word) {
			continue
		}
		freq[word]++
	}

	// Sort by frequency
	type termFreq struct {
		term  string
		count int
	}
	ranked := make([]termFreq, 0, len(freq))
	for term, count := range freq {
		ranked = append(ranked, termFreq{term, count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].count > ranked[j].count
	})

	// Take top N
	result := make([]string, 0, limit)
	for i := 0; i < len(ranked) && i < limit; i++ {
		result = append(result, ranked[i].term)
	}

	return result
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

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		for _, vp := range valuePatterns {
			matches := vp.Pattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					fullMatch := match[0]
					values = append(values, ExtractedValue{
						Type:    vp.Name,
						Value:   fullMatch,
						Context: strings.TrimSpace(line),
					})
				}
			}
		}
	}

	return values
}
