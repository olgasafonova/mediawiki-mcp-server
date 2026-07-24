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

// validateManageCategoriesArgs checks the required inputs for ManageCategories.
func validateManageCategoriesArgs(args ManageCategoriesArgs) error {
	if args.Title == "" {
		return fmt.Errorf("title is required")
	}
	if len(args.Add) == 0 && len(args.Remove) == 0 {
		return fmt.Errorf("at least one category to add or remove is required")
	}
	return nil
}

func (c *Client) ManageCategories(ctx context.Context, args ManageCategoriesArgs) (ManageCategoriesResult, error) {
	if err := validateManageCategoriesArgs(args); err != nil {
		return ManageCategoriesResult{}, err
	}

	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return ManageCategoriesResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	preview := args.PreviewEnabled()
	result := ManageCategoriesResult{
		Title:   page.Title,
		Preview: preview,
	}

	newContent := applyCategoryChanges(page, args, &result)

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

	edit := EditPageArgs{
		Title:   page.Title,
		Content: newContent,
		Summary: categoryEditSummary(args, result),
		Minor:   true,
	}
	if err := c.commitCategoryChanges(ctx, edit, page.Revision, &result); err != nil {
		return ManageCategoriesResult{}, err
	}
	return result, nil
}

// applyCategoryChanges rewrites the page content per the requested add/remove
// lists, records the per-category outcome on the result, and returns the
// rewritten wikitext.
func applyCategoryChanges(page PageContent, args ManageCategoriesArgs, result *ManageCategoriesResult) string {
	existing := parseExistingCategories(page.Content)

	newContent, removed, notFound := removeCategoriesFromContent(page.Content, args.Remove, existing)
	result.Removed = removed
	result.NotFound = notFound

	newContent, added, alreadyPresent := addCategoriesToContent(newContent, args.Add, existing)
	result.Added = added
	result.AlreadyPresent = alreadyPresent
	result.CurrentCategories = keysOf(existing)

	return newContent
}

// categoryEditSummary returns the user-provided summary, or a generated one
// describing the applied changes.
func categoryEditSummary(args ManageCategoriesArgs, result ManageCategoriesResult) string {
	if args.Summary != "" {
		return args.Summary
	}
	return buildCategoryEditSummary(result.Added, result.Removed)
}

// commitCategoryChanges saves the rewritten content and fills in the edit
// outcome and revision info on the result.
func (c *Client) commitCategoryChanges(ctx context.Context, edit EditPageArgs, oldRevision int, result *ManageCategoriesResult) error {
	editResult, err := c.EditPage(ctx, edit)
	if err != nil {
		return fmt.Errorf("failed to save changes: %w", err)
	}
	result.Success = editResult.Success
	result.RevisionID = editResult.RevisionID
	result.Message = fmt.Sprintf("Added %d, removed %d categories", len(result.Added), len(result.Removed))
	result.Revision, result.Undo = c.buildEditRevisionInfo(edit.Title, oldRevision, editResult.RevisionID)
	return nil
}

// keysOf returns the keys of a string-bool map as a slice.
func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
