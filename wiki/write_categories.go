package wiki

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var categoryTagRegex = regexp.MustCompile(`\[\[Category:([^\]|]+)(?:\|[^\]]*)?\]\]`)

// parseExistingCategories returns the set of category names already present in
// the wikitext content.
func parseExistingCategories(content string) map[string]bool {
	existing := make(map[string]bool)
	for _, m := range categoryTagRegex.FindAllStringSubmatch(content, -1) {
		existing[strings.TrimSpace(m[1])] = true
	}
	return existing
}

// removeCategoriesFromContent removes the listed categories from the content
// and returns the rewritten content plus the per-category outcome (removed,
// not-found). The existing-set is updated in place.
func removeCategoriesFromContent(content string, toRemove []string, existing map[string]bool) (newContent string, removed, notFound []string) {
	newContent = content
	for _, cat := range toRemove {
		cat = strings.TrimSpace(cat)
		if !existing[cat] {
			notFound = append(notFound, cat)
			continue
		}
		removeRegex := regexp.MustCompile(`\n?\[\[Category:` + regexp.QuoteMeta(cat) + `(?:\|[^\]]*)?\]\]\n?`)
		newContent = removeRegex.ReplaceAllString(newContent, "\n")
		removed = append(removed, cat)
		delete(existing, cat)
	}
	return newContent, removed, notFound
}

// addCategoriesToContent appends category tags missing from the existing-set.
// Categories already present are reported via alreadyPresent.
func addCategoriesToContent(content string, toAdd []string, existing map[string]bool) (newContent string, added, alreadyPresent []string) {
	newContent = content
	for _, cat := range toAdd {
		cat = strings.TrimSpace(cat)
		if existing[cat] {
			alreadyPresent = append(alreadyPresent, cat)
			continue
		}
		newContent = strings.TrimRight(newContent, "\n") + "\n[[Category:" + cat + "]]\n"
		added = append(added, cat)
		existing[cat] = true
	}
	return newContent, added, alreadyPresent
}

// buildCategoryEditSummary composes the default edit summary for category changes.
func buildCategoryEditSummary(added, removed []string) string {
	parts := make([]string, 0, 2)
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("Added categories: %s", strings.Join(added, ", ")))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("Removed categories: %s", strings.Join(removed, ", ")))
	}
	return strings.Join(parts, ". ")
}

func (c *Client) ManageCategories(ctx context.Context, args ManageCategoriesArgs) (ManageCategoriesResult, error) {
	if args.Title == "" {
		return ManageCategoriesResult{}, fmt.Errorf("title is required")
	}
	if len(args.Add) == 0 && len(args.Remove) == 0 {
		return ManageCategoriesResult{}, fmt.Errorf("at least one category to add or remove is required")
	}

	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return ManageCategoriesResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	preview := args.PreviewEnabled()
	existing := parseExistingCategories(page.Content)
	result := ManageCategoriesResult{
		Title:             page.Title,
		Preview:           preview,
		CurrentCategories: keysOf(existing),
	}

	newContent, removed, notFound := removeCategoriesFromContent(page.Content, args.Remove, existing)
	result.Removed = removed
	result.NotFound = notFound

	newContent, added, alreadyPresent := addCategoriesToContent(newContent, args.Add, existing)
	result.Added = added
	result.AlreadyPresent = alreadyPresent
	result.CurrentCategories = keysOf(existing)

	if len(result.Added) == 0 && len(result.Removed) == 0 {
		result.Success = true
		result.Message = "No changes needed"
		return result, nil
	}
	if preview {
		result.Success = true
		result.Message = fmt.Sprintf("Preview: would add %d and remove %d categories", len(result.Added), len(result.Removed))
		return result, nil
	}

	summary := args.Summary
	if summary == "" {
		summary = buildCategoryEditSummary(result.Added, result.Removed)
	}
	oldRevision := page.Revision
	editResult, err := c.EditPage(ctx, EditPageArgs{
		Title:   page.Title,
		Content: newContent,
		Summary: summary,
		Minor:   true,
	})
	if err != nil {
		return ManageCategoriesResult{}, fmt.Errorf("failed to save changes: %w", err)
	}
	result.Success = editResult.Success
	result.RevisionID = editResult.RevisionID
	result.Message = fmt.Sprintf("Added %d, removed %d categories", len(result.Added), len(result.Removed))
	result.Revision, result.Undo = c.buildEditRevisionInfo(page.Title, oldRevision, editResult.RevisionID)
	return result, nil
}

// keysOf returns the keys of a string-bool map as a slice.
func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
