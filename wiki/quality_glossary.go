package wiki

import (
	"context"
	"regexp"
	"strings"
)

// loadGlossary parses a wiki table to extract glossary terms
func (c *Client) loadGlossary(ctx context.Context, glossaryPage string) ([]GlossaryTerm, error) {
	page, err := c.GetPage(ctx, GetPageArgs{Title: glossaryPage, Format: "wikitext"})
	if err != nil {
		return nil, err
	}

	return parseWikiTableGlossary(page.Content), nil
}

// glossaryTableRegex matches wikitable blocks tagged with mcp-glossary or wikitable.
var glossaryTableRegex = regexp.MustCompile(`(?s)\{\|[^\n]*class="[^"]*(?:mcp-glossary|wikitable)[^"]*"[^\n]*\n(.*?)\|\}`)

// glossaryTermFromCells converts parsed table cells into a GlossaryTerm.
// Returns ok=false for rows that should be skipped (too few cells, empty, or
// where the "incorrect" form already matches the "correct" form).
func glossaryTermFromCells(cells []string) (GlossaryTerm, bool) {
	if len(cells) < 2 {
		return GlossaryTerm{}, false
	}
	term := GlossaryTerm{
		Incorrect: strings.TrimSpace(cells[0]),
		Correct:   strings.TrimSpace(cells[1]),
	}
	if term.Incorrect == "" || term.Incorrect == term.Correct {
		return GlossaryTerm{}, false
	}
	if len(cells) >= 3 {
		term.Pattern = strings.TrimSpace(cells[2])
	}
	if len(cells) >= 4 {
		term.Notes = strings.TrimSpace(cells[3])
	}
	return term, true
}

// parseWikiTableGlossary extracts terms from wikitable format
func parseWikiTableGlossary(content string) []GlossaryTerm {
	var terms []GlossaryTerm
	for _, table := range glossaryTableRegex.FindAllStringSubmatch(content, -1) {
		if len(table) < 2 {
			continue
		}
		terms = append(terms, parseGlossaryTableRows(table[1])...)
	}
	return terms
}

// parseGlossaryTableRows parses the rows of a single glossary table body into
// glossary terms.
func parseGlossaryTableRows(tableBody string) []GlossaryTerm {
	var terms []GlossaryTerm
	for _, row := range strings.Split(tableBody, "|-") {
		row = strings.TrimSpace(row)
		if row == "" || strings.HasPrefix(row, "!") {
			continue
		}
		if term, ok := glossaryTermFromCells(parseTableRow(row)); ok {
			terms = append(terms, term)
		}
	}
	return terms
}

// parseTableRow extracts cells from a wiki table row
func parseTableRow(row string) []string {
	var cells []string
	for _, line := range strings.Split(row, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		cells = append(cells, parseRowLineCells(line)...)
	}
	return cells
}

// parseRowLineCells splits one table-row line into trimmed, non-empty cells.
func parseRowLineCells(line string) []string {
	var cells []string
	// Remove leading | if present, then split by || for multiple cells.
	for _, part := range strings.Split(strings.TrimPrefix(line, "|"), "||") {
		if cell := strings.TrimSpace(part); cell != "" {
			cells = append(cells, cell)
		}
	}
	return cells
}
