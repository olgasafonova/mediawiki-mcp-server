package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

// printJSON marshals v to JSON and prints to stdout.
func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// table creates a tabwriter for aligned column output.
func table() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// truncate shortens s to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// printIDTitleTable prints a paginated list of pages identified by ID and Title.
// Shared by `wiki list pages` and `wiki list members` since both emit the same
// shape (header, table of PageSummary, optional continuation hint). The caller
// composes header + emptyMsg so the wording stays specific to each command.
func printIDTitleTable(header, emptyMsg, noun string, items []wiki.PageSummary, hasMore bool, continueFrom string) {
	if len(items) == 0 {
		fmt.Println(emptyMsg)
		return
	}

	fmt.Println(header)
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "ID\tTITLE\n")
	for _, item := range items {
		fmt.Fprintf(tw, "%d\t%s\n", item.PageID, item.Title)
	}
	_ = tw.Flush()

	if hasMore {
		fmt.Printf("\nMore %s available. Use --continue %q to see next page.\n", noun, continueFrom)
	}
}

// stripHTML removes basic HTML tags from MediaWiki snippets.
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}
	return result.String()
}
