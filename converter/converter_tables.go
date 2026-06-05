package converter

import (
	"regexp"
	"strings"
)

// convertTables converts Markdown tables to MediaWiki format
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

// emitTableBodyRow appends a non-header row's cells, skipping pure separator rows.
func emitTableBodyRow(line string, result *[]string) {
	cells := splitTableCells(line)
	if cells == nil || cellsAreAllDashes(cells) {
		return
	}
	*result = append(*result, "|-")
	for _, cell := range cells {
		*result = append(*result, "| "+strings.TrimSpace(cell))
	}
}

func convertTables(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	inTable := false

	pipeRegex := regexp.MustCompile(`^\|.*\|$`)
	separatorRegex := regexp.MustCompile(`^\|[\s\-:|]+\|$`)

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		if !inTable && strings.Contains(line, "|") && pipeRegex.MatchString(line) {
			inTable = true
			i = emitTableHeader(line, lines, i, &result)
			continue
		}
		if inTable && strings.Contains(line, "|") {
			if !(separatorRegex.MatchString(line) && strings.Contains(line, "-")) {
				emitTableBodyRow(line, &result)
			}
			i++
			continue
		}
		if inTable && !strings.Contains(line, "|") {
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
