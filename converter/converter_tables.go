package converter

import (
	"regexp"
	"strings"
)

// Table-recognition patterns: a pipe-bounded row and a header separator row.
var (
	tablePipeRegex      = regexp.MustCompile(`^\|.*\|$`)
	tableSeparatorRegex = regexp.MustCompile(`^\|[\s\-:|]+\|$`)
)

// isTableStart reports whether line opens a Markdown table (pipe-bounded row).
func isTableStart(line string) bool {
	return strings.Contains(line, "|") && tablePipeRegex.MatchString(line)
}

// isTableSeparatorRow reports whether line is a header separator like |---|---|.
func isTableSeparatorRow(line string) bool {
	return tableSeparatorRegex.MatchString(line) && strings.Contains(line, "-")
}

// splitTableCells extracts the middle cells from a pipe-bounded line.
func splitTableCells(line string) []string {
	cells := strings.Split(line, "|")
	if len(cells) <= 2 {
		return nil
	}
	return cells[1 : len(cells)-1]
}

// cellsAreAllDashes reports whether every cell is a separator (only - and :).
func cellsAreAllDashes(cells []string) bool {
	for _, cell := range cells {
		if strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(cell, "-", ""), ":", "")) != "" {
			return false
		}
	}
	return true
}

// emitTableHeader appends a wikitable opening + header cells from the markdown
// header line and returns the index just past any separator row.
func emitTableHeader(line string, lines []string, i int, result *[]string) int {
	*result = append(*result, `{| class="wikitable"`)
	cells := strings.Split(line, "|")
	cells = cells[1 : len(cells)-1]
	*result = append(*result, "|-")
	for _, cell := range cells {
		*result = append(*result, "! "+strings.TrimSpace(cell))
	}
	i++
	if i < len(lines) {
		nextLine := strings.TrimSpace(lines[i])
		if strings.Contains(nextLine, "|") && strings.Contains(nextLine, "-") {
			i++
		}
	}
	return i
}

// emitTableBodyRow appends a non-header row's cells, skipping separator rows.
func emitTableBodyRow(line string, result *[]string) {
	if isTableSeparatorRow(line) {
		return
	}
	cells := splitTableCells(line)
	if cells == nil || cellsAreAllDashes(cells) {
		return
	}
	*result = append(*result, "|-")
	for _, cell := range cells {
		*result = append(*result, "| "+strings.TrimSpace(cell))
	}
}

// convertTables converts Markdown tables to MediaWiki format
func convertTables(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	inTable := false

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		switch {
		case !inTable && isTableStart(line):
			inTable = true
			i = emitTableHeader(line, lines, i, &result)
		case inTable && strings.Contains(line, "|"):
			emitTableBodyRow(line, &result)
			i++
		default:
			if inTable {
				result = append(result, "|}")
				inTable = false
			}
			result = append(result, lines[i])
			i++
		}
	}

	if inTable {
		result = append(result, "|}")
	}

	return strings.Join(result, "\n")
}
