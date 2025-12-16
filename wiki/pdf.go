package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Pre-compiled regexes for text cleaning (performance optimization)
var (
	whitespaceRegex = regexp.MustCompile(`[ \t]+`)
	lineEndingRegex = regexp.MustCompile(`\r\n|\r`)
	blankLinesRegex = regexp.MustCompile(`\n{3,}`)
)

// SearchInPDF searches for a query string in PDF content
func SearchInPDF(pdfData []byte, query string) ([]FileSearchMatch, bool, string, error) {
	if len(pdfData) == 0 {
		return nil, false, "Empty PDF data", nil
	}

	// Create a temporary file to work with pdfcpu (it works with files)
	tmpFile, err := os.CreateTemp("", "mediawiki-pdf-*.pdf")
	if err != nil {
		return nil, false, fmt.Sprintf("Failed to create temp file: %v", err), nil
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write PDF data to temp file
	if _, err := tmpFile.Write(pdfData); err != nil {
		tmpFile.Close()
		return nil, false, fmt.Sprintf("Failed to write temp file: %v", err), nil
	}
	tmpFile.Close()

	// Create output directory for extracted text
	tmpDir, err := os.MkdirTemp("", "mediawiki-pdf-extract-")
	if err != nil {
		return nil, false, fmt.Sprintf("Failed to create temp dir: %v", err), nil
	}
	defer os.RemoveAll(tmpDir)

	// Try to extract text content
	conf := model.NewDefaultConfiguration()
	err = api.ExtractContentFile(tmpPath, tmpDir, nil, conf)
	if err != nil {
		// Check if it might be a scanned/image PDF
		return nil, false, fmt.Sprintf("Failed to extract text from PDF: %v. The file may be scanned/image-based (requires OCR) or encrypted.", err), nil
	}

	// Read all extracted text files
	var allText strings.Builder
	pageCount := 0

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, false, fmt.Sprintf("Failed to read extracted content: %v", err), nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".txt") {
			pageCount++
			content, err := os.ReadFile(filepath.Join(tmpDir, entry.Name()))
			if err == nil {
				allText.WriteString(string(content))
				allText.WriteString("\n")
			}
		}
	}

	text := allText.String()
	text = cleanPDFText(text)

	if strings.TrimSpace(text) == "" {
		return nil, false, "No readable text found in PDF. The file may be scanned/image-based (requires OCR) or encrypted.", nil
	}

	// Search for query
	matches := searchInText(text, query, pageCount)

	if len(matches) == 0 {
		return []FileSearchMatch{}, true, fmt.Sprintf("No matches found for '%s' in %d pages", query, pageCount), nil
	}

	return matches, true, fmt.Sprintf("Found %d matches in PDF (%d pages)", len(matches), pageCount), nil
}

// cleanPDFText normalizes extracted PDF text
func cleanPDFText(text string) string {
	// Remove excessive whitespace
	text = whitespaceRegex.ReplaceAllString(text, " ")
	// Normalize line endings
	text = lineEndingRegex.ReplaceAllString(text, "\n")
	// Remove excessive blank lines
	text = blankLinesRegex.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// searchInText searches for query in text and returns matches with context
func searchInText(text, query string, pageCount int) []FileSearchMatch {
	var matches []FileSearchMatch

	// Case-insensitive search
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)

	lines := strings.Split(text, "\n")
	linesLower := strings.Split(lowerText, "\n")

	for lineNum, lineLower := range linesLower {
		if strings.Contains(lineLower, lowerQuery) {
			// Get context (the actual line with original case)
			context := lines[lineNum]
			if len(context) > 200 {
				// Find the match position and center context around it
				pos := strings.Index(lineLower, lowerQuery)
				start := pos - 80
				if start < 0 {
					start = 0
				}
				end := pos + len(query) + 80
				if end > len(context) {
					end = len(context)
				}
				context = "..." + strings.TrimSpace(context[start:end]) + "..."
			}

			match := FileSearchMatch{
				Line:    lineNum + 1,
				Context: context,
			}

			// Estimate page number if we have multiple pages
			// This is a rough estimate since we don't have page boundaries
			if pageCount > 1 {
				estimatedPage := (lineNum * pageCount / len(lines)) + 1
				if estimatedPage > pageCount {
					estimatedPage = pageCount
				}
				match.Page = estimatedPage
			}

			matches = append(matches, match)

			// Limit matches to prevent huge responses
			if len(matches) >= 50 {
				break
			}
		}
	}

	return matches
}
