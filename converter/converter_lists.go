package converter

import (
	"regexp"
	"strings"
)

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
