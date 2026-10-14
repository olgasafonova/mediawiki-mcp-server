package wiki

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// previewEdit computes what an edit would change without touching the write
// API: no CSRF token is requested, no audit entry is written, nothing is
// saved. The current content comes from the read path so the caller can weigh
// the planned content against the live revision before applying it.
func (c *Client) previewEdit(ctx context.Context, args EditPageArgs) (EditResult, error) {
	target, err := c.previewTargetFor(ctx, args)
	if err != nil {
		return EditResult{}, err
	}

	result := EditResult{
		Success: true,
		Title:   args.Title,
		Preview: true,
		NewPage: !target.exists,
		Changes: diffLines(target.content, args.Content),
	}
	if target.title != "" {
		result.Title = target.title
	}
	result.Message = previewEditMessage(result, args.Section)
	return result, nil
}

// previewTarget describes the current state of the content an edit would
// replace.
type previewTarget struct {
	title   string // wiki-normalized title; empty when the page does not exist
	content string // current wikitext of the whole page or the targeted section
	exists  bool
}

// previewTargetFor fetches what the edit would overwrite. A missing page is
// not an error: it means the edit would create the page, which the preview
// must report instead of failing. Section edits diff against the targeted
// section only — diffing a section against the whole page would flag every
// untouched line as changed.
func (c *Client) previewTargetFor(ctx context.Context, args EditPageArgs) (previewTarget, error) {
	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		if isPageMissingError(err) {
			return previewTarget{}, nil
		}
		return previewTarget{}, fmt.Errorf("failed to get page: %w", err)
	}

	target := previewTarget{title: page.Title, content: page.Content, exists: true}
	switch args.Section {
	case "":
		return target, nil
	case "new":
		// A new section is appended, replacing nothing.
		target.content = ""
		return target, nil
	}

	sectionNum, convErr := strconv.Atoi(args.Section)
	if convErr != nil {
		return previewTarget{}, fmt.Errorf("invalid section %q: must be 'new' or a section number", args.Section)
	}
	section, err := c.getSectionContent(ctx, normalizePageTitle(args.Title), sectionNum, "wikitext")
	if err != nil {
		return previewTarget{}, fmt.Errorf("failed to get section %s: %w", args.Section, err)
	}
	target.content = section.SectionContent
	return target, nil
}

// isPageMissingError reports whether err is the read path's missing-page
// error (getPageWikitext phrases it "page 'X' does not exist"), as opposed to
// a transport or auth failure. Such an edit would create the page, which the
// preview reports instead of failing. String probing follows the existing
// badtoken check in write.go.
func isPageMissingError(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}

// previewEditMessage summarizes a dry run so an agent can decide whether to
// re-run with preview=false.
func previewEditMessage(result EditResult, section string) string {
	if result.NewPage {
		return fmt.Sprintf("Preview: would create %s with %d line(s) of content. Nothing saved — set preview=false to apply.", result.Title, len(result.Changes))
	}
	if len(result.Changes) == 0 {
		return fmt.Sprintf("Preview: content is identical to the current revision of %s — nothing would change. Nothing saved.", result.Title)
	}
	target := result.Title
	if section != "" {
		target = fmt.Sprintf("section %s of %s", section, result.Title)
	}
	return fmt.Sprintf("Preview: %d line(s) would change in %s. Nothing saved — set preview=false to apply.", len(result.Changes), target)
}

// maxDiffCells bounds the longest-common-subsequence table used by diffLines.
// Past this size the quadratic table (4 bytes per cell) would exhaust hundreds
// of megabytes on a large rewrite, so the diff falls back to positional
// pairing of the changed region.
const maxDiffCells = 4 << 20

// diffLines returns the per-line changes that turn oldText into newText,
// reusing the find/replace TextChange shape: Before alone is a removed line,
// After alone is an added line, and a modified line appears as its removal
// followed by its addition (only the over-budget positional fallback pairs
// them into a single entry with both fields set). Line is the
// 1-based position in the planned content for added and modified lines, and
// in the current content for removed lines. Unchanged lines are omitted so
// the caller sees only what the edit would actually touch.
func diffLines(oldText, newText string) []TextChange {
	oldLines, newLines := splitLines(oldText), splitLines(newText)

	// Typical edits touch a few lines inside a large page. Trimming the
	// common prefix and suffix first shrinks the quadratic core to the
	// edited region, so the cell budget below is almost never reached.
	prefix := commonPrefixLen(oldLines, newLines)
	suffix := commonSuffixLen(oldLines[prefix:], newLines[prefix:])
	oldCore := oldLines[prefix : len(oldLines)-suffix]
	newCore := newLines[prefix : len(newLines)-suffix]
	if len(oldCore) == 0 && len(newCore) == 0 {
		return nil
	}
	if len(oldCore)*len(newCore) > maxDiffCells {
		return positionalChanges(oldCore, newCore, prefix)
	}
	return lcsChanges(oldCore, newCore, prefix)
}

// splitLines splits text into lines for diffing. A trailing newline does not
// produce a phantom final empty line, so "line" and "line\n" diff as equal —
// MediaWiki stores content with a trailing newline and flagging it as a
// change would be noise.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// commonPrefixLen returns the length of the identical leading run of a and b.
func commonPrefixLen(a, b []string) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// commonSuffixLen returns the length of the identical trailing run of a and b.
func commonSuffixLen(a, b []string) int {
	n := min(len(a), len(b))
	for k := 1; k <= n; k++ {
		if a[len(a)-k] != b[len(b)-k] {
			return k - 1
		}
	}
	return n
}

// lcsChanges diffs two line slices with a longest-common-subsequence table,
// emitting one TextChange per removed, added, or modified line.
func lcsChanges(oldLines, newLines []string, offset int) []TextChange {
	n, m := len(oldLines), len(newLines)
	table := make([][]int32, n+1)
	for i := range table {
		table[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case oldLines[i] == newLines[j]:
				table[i][j] = table[i+1][j+1] + 1
			case table[i+1][j] >= table[i][j+1]:
				table[i][j] = table[i+1][j]
			default:
				table[i][j] = table[i][j+1]
			}
		}
	}

	var changes []TextChange
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case oldLines[i] == newLines[j]:
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			// Removed line: keep its position in the current content.
			changes = append(changes, TextChange{Line: offset + i + 1, Before: oldLines[i]})
			i++
		default:
			// Added line: report its position in the planned content.
			changes = append(changes, TextChange{Line: offset + j + 1, After: newLines[j]})
			j++
		}
	}
	for ; i < n; i++ {
		changes = append(changes, TextChange{Line: offset + i + 1, Before: oldLines[i]})
	}
	for ; j < m; j++ {
		changes = append(changes, TextChange{Line: offset + j + 1, After: newLines[j]})
	}
	return changes
}

// positionalChanges is the over-budget fallback for diffLines: it pairs the
// old and new lines of the changed region by position instead of by content.
// It is only reached when the LCS table would exceed maxDiffCells — a
// wholesale rewrite of a very large page, where nearly every line differs
// anyway.
func positionalChanges(oldLines, newLines []string, offset int) []TextChange {
	changes := make([]TextChange, 0, max(len(oldLines), len(newLines)))
	for k := 0; k < len(oldLines) || k < len(newLines); k++ {
		var change TextChange
		switch {
		case k < len(oldLines):
			change = TextChange{Line: offset + k + 1, Before: oldLines[k]}
			if k < len(newLines) {
				change.After = newLines[k]
			}
		default:
			change = TextChange{Line: offset + k + 1, After: newLines[k]}
		}
		if change.Before == change.After {
			continue
		}
		changes = append(changes, change)
	}
	return changes
}
