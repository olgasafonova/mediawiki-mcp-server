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
	regexp.MustCompile(`\[\[Category:[^\]]+\]\]`),        // Category links
	regexp.MustCompile(`\[\[[^\]|]+\|([^\]]+)\]\]`),      // Links with display text
	regexp.MustCompile(`\[\[([^\]]+)\]\]`),               // Simple links
	regexp.MustCompile(`\{\{[^}]+\}\}`),                  // Templates
	regexp.MustCompile(`<ref[^>]*>.*?</ref>`),            // References
	regexp.MustCompile(`<ref[^/]*/?>`),                   // Self-closing refs
	regexp.MustCompile(`<[^>]+>`),                        // All HTML tags
	regexp.MustCompile(`'''([^']+)'''`),                  // Bold
	regexp.MustCompile(`''([^']+)''`),                    // Italic
	regexp.MustCompile(`={2,}([^=]+)={2,}`),              // Section headers
	regexp.MustCompile(`\|[^|}\n]+`),                     // Table cells
	regexp.MustCompile(`\{\|[^}]*\|\}`),                  // Tables
	regexp.MustCompile(`^\*+\s*`),                        // List items
	regexp.MustCompile(`^#+\s*`),                         // Numbered lists
	regexp.MustCompile(`https?://[^\s\]]+`),              // URLs
	regexp.MustCompile(`\[https?://[^\s\]]+ ([^\]]+)\]`), // External links with text
	regexp.MustCompile(`\[https?://[^\]]+\]`),            // External links
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

// isSignificantTerm reports whether a token should count toward term
// frequency: at least 3 chars, not a stopword, and not purely numeric.
func isSignificantTerm(word string) bool {
	return len(word) >= 3 && !stopwords[word] && !isNumeric(word)
}

// tokenizePlainText lowercases content, strips wiki markup, and splits into
// word/digit tokens.
func tokenizePlainText(content string) []string {
	plainText := strings.ToLower(removeWikiMarkup(content))
	return strings.FieldsFunc(plainText, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// extractTopTerms gets the most frequent significant terms
func extractTopTerms(content string, limit int) []string {
	freq := make(map[string]int)
	for _, word := range tokenizePlainText(content) {
		if isSignificantTerm(word) {
			freq[word]++
		}
	}

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

	result := make([]string, 0, limit)
	for i := 0; i < len(ranked) && i < limit; i++ {
		result = append(result, ranked[i].term)
	}

	return result
}
