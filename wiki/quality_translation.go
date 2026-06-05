package wiki

import (
	"context"
	"fmt"
)

var validTranslationPatterns = map[string]struct{}{
	"subpage": {},
	"suffix":  {},
	"prefix":  {},
}

// validateTranslationPattern resolves the default and validates the pattern name.
func validateTranslationPattern(pattern string) (string, error) {
	if pattern == "" {
		pattern = "subpage"
	}
	if _, ok := validTranslationPatterns[pattern]; !ok {
		return "", fmt.Errorf("invalid pattern: %s (use 'subpage', 'suffix', or 'prefix')", pattern)
	}
	return pattern, nil
}

// buildTranslationTitle composes the localized page title for a base page,
// language, and naming pattern.
func buildTranslationTitle(basePage, lang, pattern string) string {
	switch pattern {
	case "suffix":
		return fmt.Sprintf("%s (%s)", basePage, lang)
	case "prefix":
		return fmt.Sprintf("%s:%s", lang, basePage)
	default: // "subpage"
		return fmt.Sprintf("%s/%s", basePage, lang)
	}
}

// checkBasePageTranslations checks one base page across all requested languages
// and returns the per-page result plus the count of missing translations.
func (c *Client) checkBasePageTranslations(ctx context.Context, basePage string, languages []string, pattern string) (PageTranslationResult, int) {
	pageResult := PageTranslationResult{
		BasePage:     basePage,
		Translations: make(map[string]TranslationStatus),
		Complete:     true,
	}
	missing := 0

	for _, lang := range languages {
		langPage := buildTranslationTitle(basePage, lang, pattern)
		status := TranslationStatus{PageTitle: langPage}

		info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: langPage})
		if err == nil && info.Exists {
			status.Exists = true
			status.PageID = info.PageID
			status.Length = info.Length
		} else {
			pageResult.MissingLangs = append(pageResult.MissingLangs, lang)
			pageResult.Complete = false
			missing++
		}

		pageResult.Translations[lang] = status
	}

	return pageResult, missing
}

// CheckTranslations checks if pages exist in all specified languages
func (c *Client) CheckTranslations(ctx context.Context, args CheckTranslationsArgs) (CheckTranslationsResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTranslationsResult{}, err
	}

	if len(args.Languages) == 0 {
		return CheckTranslationsResult{}, fmt.Errorf("at least one language is required")
	}

	pattern, err := validateTranslationPattern(args.Pattern)
	if err != nil {
		return CheckTranslationsResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 100)
	basePages, err := c.collectPagesFromArgs(ctx, args.BasePages, args.Category, limit, "base_pages")
	if err != nil {
		return CheckTranslationsResult{}, err
	}

	result := CheckTranslationsResult{
		LanguagesChecked: args.Languages,
		Pattern:          pattern,
		Pages:            make([]PageTranslationResult, 0, len(basePages)),
	}

	for _, basePage := range basePages {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		pageResult, missing := c.checkBasePageTranslations(ctx, basePage, args.Languages, pattern)
		result.MissingCount += missing
		result.Pages = append(result.Pages, pageResult)
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}
