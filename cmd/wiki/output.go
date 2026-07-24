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
	if maxLen <= 3 {
		// Not enough room for an ellipsis; hard-cut, guarding against a
		// negative slice bound when maxLen < 3.
		if maxLen < 0 {
			return ""
		}
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// idTitleTable describes a paginated list of pages identified by ID and Title.
// The caller composes Header + EmptyMsg so the wording stays specific to each
// command.
type idTitleTable struct {
	Header       string
	EmptyMsg     string
	Noun         string
	Items        []wiki.PageSummary
	HasMore      bool
	ContinueFrom string
}

// printIDTitleTable prints a paginated list of pages identified by ID and Title.
// Shared by `wiki list pages` and `wiki list members` since both emit the same
// shape (header, table of PageSummary, optional continuation hint).
func printIDTitleTable(t idTitleTable) {
	if len(t.Items) == 0 {
		fmt.Println(t.EmptyMsg)
		return
	}

	fmt.Println(t.Header)
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "ID\tTITLE\n")
	for _, item := range t.Items {
		fmt.Fprintf(tw, "%d\t%s\n", item.PageID, item.Title)
	}
	_ = tw.Flush()

	if t.HasMore {
		fmt.Printf("\nMore %s available. Use --continue %q to see next page.\n", t.Noun, t.ContinueFrom)
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
