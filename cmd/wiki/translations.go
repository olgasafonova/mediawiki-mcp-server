package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newTranslationsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "translations",
		Short: "Find missing language translations for a set of pages",
		Long: `Check which language versions exist (or don't) for a set of base pages.

Specify base pages via --base (comma-separated) or --category. --languages is
always required and accepts a comma-separated list of language codes.

  wiki translations --base "Getting Started,FAQ" --languages "en,no,sv"
  wiki translations --category "Documentation" --languages "en,de" --pattern suffix
  wiki translations --base Home --languages "en,no" --json`,
		Args: cobra.NoArgs,
		RunE: runTranslations,
	}

	cmd.Flags().String("base", "", "Comma-separated base page names")
	cmd.Flags().String("category", "", "Category to get base pages from (alternative to --base)")
	cmd.Flags().String("languages", "", "Comma-separated language codes (required, e.g. 'en,no,sv')")
	cmd.Flags().String("pattern", "subpage", "Language page pattern: 'subpage' (Page/lang), 'suffix' (Page (lang)), or 'prefix' (lang:Page)")
	cmd.Flags().IntP("limit", "n", 20, "Max base pages to check (max 100)")

	return cmd
}

func runTranslations(cmd *cobra.Command, _ []string) error {
	baseStr, _ := cmd.Flags().GetString("base")
	category, _ := cmd.Flags().GetString("category")
	langsStr, _ := cmd.Flags().GetString("languages")
	pattern, _ := cmd.Flags().GetString("pattern")
	limit, _ := cmd.Flags().GetInt("limit")

	base := splitCSV(baseStr)
	languages := splitCSV(langsStr)

	if len(languages) == 0 {
		return usageErr(fmt.Errorf("--languages is required (e.g. --languages 'en,no,sv')"))
	}
	if len(base) == 0 && category == "" {
		return usageErr(fmt.Errorf("either --base or --category is required"))
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.CheckTranslations(context.Background(), wiki.CheckTranslationsArgs{
		BasePages: base,
		Category:  category,
		Languages: languages,
		Pattern:   pattern,
		Limit:     limit,
	})
	if err != nil {
		return fmt.Errorf("translations failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Checked %d page(s) in [%s] using pattern=%s\n",
		result.PagesChecked, strings.Join(result.LanguagesChecked, ", "), result.Pattern)
	fmt.Printf("Missing translations: %d\n\n", result.MissingCount)

	tw := table()
	header := []string{"PAGE", "COMPLETE"}
	header = append(header, result.LanguagesChecked...)
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	for _, p := range result.Pages {
		row := []string{p.BasePage}
		if p.Complete {
			row = append(row, "yes")
		} else {
			row = append(row, "no")
		}
		for _, lang := range result.LanguagesChecked {
			if st, ok := p.Translations[lang]; ok && st.Exists {
				row = append(row, "ok")
			} else {
				row = append(row, "-")
			}
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()

	return nil
}
