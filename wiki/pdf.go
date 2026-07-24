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

	text, failMsg := extractPDFText(pdfData)
	if failMsg != "" {
		return nil, false, failMsg, nil
	}

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

// pdfTempFiles holds the temp PDF input and text output paths for pdftotext.
type pdfTempFiles struct {
	pdfPath string
	txtPath string
}

// cleanup removes both temp files (best effort).
func (f pdfTempFiles) cleanup() {
	_ = os.Remove(f.pdfPath)
	_ = os.Remove(f.txtPath)
}

// extractPDFText writes the PDF to a temp file, runs pdftotext, and returns
// the cleaned text. A non-empty failMsg describes a user-facing failure.
func extractPDFText(pdfData []byte) (text, failMsg string) {
	files, failMsg := createPDFTempFiles(pdfData)
	if failMsg != "" {
		return "", failMsg
	}
	defer files.cleanup()

	if failMsg := files.runPdfToText(); failMsg != "" {
		return "", failMsg
	}

	return files.readText()
}

// createPDFTempFiles prepares the temp input/output file pair for pdftotext.
func createPDFTempFiles(pdfData []byte) (pdfTempFiles, string) {
	pdfPath, failMsg := writeTempPDF(pdfData)
	if failMsg != "" {
		return pdfTempFiles{}, failMsg
	}
	txtPath, failMsg := createTempTxt()
	if failMsg != "" {
		_ = os.Remove(pdfPath)
		return pdfTempFiles{}, failMsg
	}
	return pdfTempFiles{pdfPath: pdfPath, txtPath: txtPath}, ""
}

// readText reads and normalizes the extracted text output.
func (f pdfTempFiles) readText() (text, failMsg string) {
	// #nosec G304 G703 -- path is from os.CreateTemp, not user input
	textBytes, err := os.ReadFile(f.txtPath)
	if err != nil {
		return "", fmt.Sprintf("Failed to read extracted text: %v", err)
	}
	return cleanPDFText(string(textBytes)), ""
}

// writeTempPDF writes the PDF bytes to a temp file and returns its path.
func writeTempPDF(pdfData []byte) (path, failMsg string) {
	tmpPDF, err := os.CreateTemp("", "mediawiki-pdf-*.pdf")
	if err != nil {
		return "", fmt.Sprintf("Failed to create temp file: %v", err)
	}
	path = tmpPDF.Name()

	if _, err := tmpPDF.Write(pdfData); err != nil {
		_ = tmpPDF.Close() // Best effort cleanup on error path
		_ = os.Remove(path)
		return "", fmt.Sprintf("Failed to write temp file: %v", err)
	}
	if err := tmpPDF.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Sprintf("Failed to close temp file: %v", err)
	}
	return path, ""
}

// createTempTxt creates an empty temp file for the pdftotext output.
func createTempTxt() (path, failMsg string) {
	tmpTXT, err := os.CreateTemp("", "mediawiki-pdf-*.txt")
	if err != nil {
		return "", fmt.Sprintf("Failed to create temp text file: %v", err)
	}
	path = tmpTXT.Name()
	if err := tmpTXT.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Sprintf("Failed to close temp text file: %v", err)
	}
	return path, ""
}

// runPdfToText invokes pdftotext and maps failures to user-facing messages.
func (f pdfTempFiles) runPdfToText() (failMsg string) {
	// -layout preserves the original layout
	// -enc UTF-8 ensures proper encoding
	// #nosec G204 G702 -- paths are from os.CreateTemp, not user input
	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", f.pdfPath, f.txtPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "Incorrect password") || strings.Contains(errMsg, "encrypted") {
			return "PDF is password-protected or encrypted"
		}
		return fmt.Sprintf("Failed to extract text from PDF: %v. The file may be corrupted or in an unsupported format.", err)
	}
	return ""
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

// searchQuery is a case-insensitive search term.
type searchQuery struct {
	raw   string
	lower string
}

// docLine is one line of extracted text in original and case-folded form.
type docLine struct {
	raw   string
	lower string
}

// context returns the display context for a matched line, centered on the
// match when the line is long.
func (q searchQuery) context(line docLine) string {
	if len(line.raw) <= 200 {
		return line.raw
	}
	pos := strings.Index(line.lower, q.lower)
	start := max(pos-80, 0)
	end := min(pos+len(q.raw)+80, len(line.raw))
	return "..." + strings.TrimSpace(line.raw[start:end]) + "..."
}

// pdfTextDoc is extracted PDF text split into lines, with page structure.
type pdfTextDoc struct {
	lines     []docLine
	pageCount int
}

// newPDFTextDoc splits text into original-case and case-folded lines.
func newPDFTextDoc(text string, pageCount int) pdfTextDoc {
	raw := strings.Split(text, "\n")
	lower := strings.Split(strings.ToLower(text), "\n")
	lines := make([]docLine, len(raw))
	for i := range raw {
		lines[i] = docLine{raw: raw[i], lower: lower[i]}
	}
	return pdfTextDoc{lines: lines, pageCount: pageCount}
}

// estimatePage estimates which page a line falls on. Returns 0 (unset) for
// single-page documents.
func (d pdfTextDoc) estimatePage(lineNum int) int {
	if d.pageCount <= 1 || len(d.lines) == 0 {
		return 0
	}
	return min((lineNum*d.pageCount/len(d.lines))+1, d.pageCount)
}

// search returns up to 50 matching lines with context and page estimates.
func (d pdfTextDoc) search(query searchQuery) []FileSearchMatch {
	var matches []FileSearchMatch
	for lineNum, line := range d.lines {
		if !strings.Contains(line.lower, query.lower) {
			continue
		}
		matches = append(matches, FileSearchMatch{
			Line:    lineNum + 1,
			Context: query.context(line),
			Page:    d.estimatePage(lineNum),
		})
		// Limit matches to prevent huge responses
		if len(matches) >= 50 {
			break
		}
	}
	return matches
}

// searchInText searches for query in text and returns matches with context
func searchInText(text, query string, pageCount int) []FileSearchMatch {
	q := searchQuery{raw: query, lower: strings.ToLower(query)}
	return newPDFTextDoc(text, pageCount).search(q)
}
