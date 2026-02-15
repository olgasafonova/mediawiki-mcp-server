package wiki

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// Pre-compiled regexes for text cleaning (performance optimization)
var (
	whitespaceRegex = regexp.MustCompile(`[ \t]+`)
	lineEndingRegex = regexp.MustCompile(`\r\n|\r`)
	blankLinesRegex = regexp.MustCompile(`\n{3,}`)
)

// isPdfToTextAvailable checks if pdftotext command is available
func isPdfToTextAvailable() bool {
	_, err := exec.LookPath("pdftotext")
	return err == nil
}

// SearchInPDF searches for a query string in PDF content using external pdftotext
func SearchInPDF(pdfData []byte, query string) ([]FileSearchMatch, bool, string, error) {
	if len(pdfData) == 0 {
		return nil, false, "Empty PDF data", nil
	}

	// Check if pdftotext is available
	if !isPdfToTextAvailable() {
		installHint := getInstallHint()
		return nil, false, fmt.Sprintf("PDF search requires 'pdftotext' (poppler-utils). %s", installHint), nil
	}

	// Create a temporary file for the PDF
	tmpPDF, err := os.CreateTemp("", "mediawiki-pdf-*.pdf")
	if err != nil {
		return nil, false, fmt.Sprintf("Failed to create temp file: %v", err), nil
	}
	tmpPDFPath := tmpPDF.Name()
	defer func() { _ = os.Remove(tmpPDFPath) }()

	// Write PDF data to temp file
	if _, err := tmpPDF.Write(pdfData); err != nil {
		_ = tmpPDF.Close() // Best effort cleanup on error path
		return nil, false, fmt.Sprintf("Failed to write temp file: %v", err), nil
	}
	if err := tmpPDF.Close(); err != nil {
		return nil, false, fmt.Sprintf("Failed to close temp file: %v", err), nil
	}

	// Create temp file for text output
	tmpTXT, err := os.CreateTemp("", "mediawiki-pdf-*.txt")
	if err != nil {
		return nil, false, fmt.Sprintf("Failed to create temp text file: %v", err), nil
	}
	tmpTXTPath := tmpTXT.Name()
	if err := tmpTXT.Close(); err != nil {
		return nil, false, fmt.Sprintf("Failed to close temp text file: %v", err), nil
	}
	defer func() { _ = os.Remove(tmpTXTPath) }()

	// Run pdftotext
	// -layout preserves the original layout
	// -enc UTF-8 ensures proper encoding
	// #nosec G204 G702 -- paths are from os.CreateTemp, not user input
	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", tmpPDFPath, tmpTXTPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "Incorrect password") || strings.Contains(errMsg, "encrypted") {
			return nil, false, "PDF is password-protected or encrypted", nil
		}
		return nil, false, fmt.Sprintf("Failed to extract text from PDF: %v. The file may be corrupted or in an unsupported format.", err), nil
	}

	// Read extracted text
	// #nosec G304 G703 -- path is from os.CreateTemp, not user input
	textBytes, err := os.ReadFile(tmpTXTPath)
	if err != nil {
		return nil, false, fmt.Sprintf("Failed to read extracted text: %v", err), nil
	}

	text := string(textBytes)
	text = cleanPDFText(text)

	if strings.TrimSpace(text) == "" {
		return nil, false, "No readable text found in PDF. The file may be scanned/image-based (requires OCR) or empty.", nil
	}

	// Estimate page count from form feeds or content structure
	pageCount := strings.Count(text, "\f") + 1
	text = strings.ReplaceAll(text, "\f", "\n\n") // Replace form feeds with double newlines

	// Search for query
	matches := searchInText(text, query, pageCount)

	if len(matches) == 0 {
		return []FileSearchMatch{}, true, fmt.Sprintf("No matches found for '%s' in %d pages", query, pageCount), nil
	}

	return matches, true, fmt.Sprintf("Found %d matches in PDF (%d pages)", len(matches), pageCount), nil
}

// getInstallHint returns platform-specific installation instructions
func getInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "Install with: brew install poppler"
	case "linux":
		return "Install with: apt install poppler-utils (Debian/Ubuntu) or yum install poppler-utils (RHEL/CentOS)"
	case "windows":
		return "Install with: choco install poppler (or download from https://github.com/oschwartz10612/poppler-windows/releases)"
	default:
		return "Install poppler-utils for your platform"
	}
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
			if pageCount > 1 && len(lines) > 0 {
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
