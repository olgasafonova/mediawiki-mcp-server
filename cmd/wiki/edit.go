package main

import (
	"fmt"
	"os"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <title>",
		Short: "Create or edit a wiki page",
		Long: `Edit a wiki page with new content. Content can be provided via --content flag,
--file flag, or piped from stdin:

  wiki edit "Page Title" --content "= Hello ="
  wiki edit "Page Title" --file page.wiki --summary "Update from file"
  cat page.wiki | wiki edit "Page Title" --summary "Update from file"

Interactive mode opens the current page content in $VISUAL (or $EDITOR) and
submits the saved buffer as the new content:

  wiki edit "Page Title" --interactive
  wiki edit "Page Title" -i --summary "Cleanup section headings"

In interactive mode the edit is skipped if the buffer is empty or unchanged.
If submission fails, the buffer is preserved at its temp-file path so nothing
is lost.

Preview a change without writing it (and without needing credentials):

  wiki edit "Page Title" --file page.wiki --dry-run
  wiki edit "Page Title" --file page.wiki --dry-run --json`,
		Args: cobra.ExactArgs(1),
		RunE: runEdit,
	}

	cmd.Flags().StringP("file", "f", "", "Path to file containing wikitext content")
	cmd.Flags().String("content", "", "Page content in wikitext format")
	cmd.Flags().String("summary", "", "Edit summary explaining the change")
	cmd.Flags().Bool("minor", false, "Mark as minor edit")
	cmd.Flags().Bool("bot", false, "Mark as bot edit (requires bot flag)")
	cmd.Flags().String("section", "", "Section to edit ('new' for new section, number for existing)")
	cmd.Flags().BoolP("interactive", "i", false, "Open the current page in $VISUAL/$EDITOR and submit the saved buffer")
	cmd.Flags().Bool("dry-run", false, "Resolve the content and print what would be written, then exit without editing")

	return cmd
}

// editRequest bundles the parameters shared by the edit submission, dry-run,
// and CAPTCHA-retry helpers, so they don't have to thread every field
// through long argument lists.
type editRequest struct {
	title         string
	content       string
	summary       string
	section       string
	minor         bool
	bot           bool
	baseTimestamp string
}

func (r editRequest) toArgs() wiki.EditPageArgs {
	return wiki.EditPageArgs{
		Title:         r.title,
		Content:       r.content,
		Summary:       r.summary,
		Minor:         r.minor,
		Bot:           r.bot,
		Section:       r.section,
		BaseTimestamp: r.baseTimestamp,
	}
}

func editRequestFromFlags(cmd *cobra.Command, title string) editRequest {
	content, _ := cmd.Flags().GetString("content")
	summary, _ := cmd.Flags().GetString("summary")
	minor, _ := cmd.Flags().GetBool("minor")
	bot, _ := cmd.Flags().GetBool("bot")
	section, _ := cmd.Flags().GetString("section")
	return editRequest{
		title:   title,
		content: content,
		summary: summary,
		section: section,
		minor:   minor,
		bot:     bot,
	}
}

func runEdit(cmd *cobra.Command, args []string) error {
	req := editRequestFromFlags(cmd, args[0])
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	interactive, _ := cmd.Flags().GetBool("interactive")

	// Interactive mode takes precedence over every other content source: it
	// fetches the current page content, opens an editor, and submits whatever
	// the user saves.
	if interactive {
		if err := checkInteractiveConflicts(cmd, req); err != nil {
			return err
		}
		return runInteractiveEdit(cmd, req, dryRun)
	}

	content, err := resolveEditContent(cmd, req)
	if err != nil {
		return err
	}
	req.content = content

	// --dry-run resolves the content and reports what would be written
	// without contacting the wiki. It deliberately skips client creation so
	// an agent can preview a change without valid credentials — this is the
	// safeguard the primary write previously lacked.
	if dryRun {
		return emitEditDryRun(cmd, req)
	}

	return submitEdit(cmd, req)
}

// resolveEditContent returns the page content from --content, --file, or
// piped stdin, in that order of precedence. An empty result from all three
// sources is a usage error.
func resolveEditContent(cmd *cobra.Command, req editRequest) (string, error) {
	if req.content != "" {
		return req.content, nil
	}

	// --content is empty: try --file
	content, err := contentFromFileFlag(cmd)
	if err != nil {
		return "", err
	}
	if content != "" {
		return content, nil
	}

	// Still empty: try reading from stdin
	content, err = readStdin()
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	if content == "" {
		return "", usageErr(fmt.Errorf("content is required (use --content, --file, or pipe from stdin)"))
	}
	return content, nil
}

// contentFromFileFlag reads the file named by --file, or returns empty when
// the flag is unset.
func contentFromFileFlag(cmd *cobra.Command) (string, error) {
	filePath, _ := cmd.Flags().GetString("file")
	if filePath == "" {
		return "", nil
	}
	fileBytes, err := os.ReadFile(filePath) // #nosec G304 -- path supplied via CLI flag by the invoking user
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(fileBytes), nil
}

// submitEdit performs the non-interactive edit, including the CAPTCHA retry
// loop and result reporting.
func submitEdit(cmd *cobra.Command, req editRequest) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.EditPage(cmd.Context(), req.toArgs())
	if err != nil {
		return fmt.Errorf("edit failed: %w", err)
	}

	session := editSession{client: client, req: req}
	result = session.retryLoop(cmd, result)

	if isJSON(cmd) {
		return printJSON(result)
	}
	printEditOutcome(result)
	return nil
}

// printEditOutcome reports the edit result in human-readable form.
func printEditOutcome(result wiki.EditResult) {
	if !result.Success {
		fmt.Printf("Failed to edit %s: %s\n", result.Title, result.Message)
		return
	}
	printEditSuccess(result)
}

// printEditSuccess reports a successful edit: created vs edited, plus the
// page URL when the wiki returned one.
func printEditSuccess(result wiki.EditResult) {
	if result.NewPage {
		fmt.Printf("Created %s (rev: %d)\n", result.Title, result.RevisionID)
	} else {
		fmt.Printf("Edited %s (rev: %d)\n", result.Title, result.RevisionID)
	}
	if result.PageURL != "" {
		fmt.Printf("URL: %s\n", result.PageURL)
	}
}

// emitEditDryRun reports what `wiki edit` would write, without performing
// the edit. JSON under --json, otherwise a short human-readable block. The
// content itself is summarized by byte count rather than echoed, so a large
// page doesn't flood the output.
func emitEditDryRun(cmd *cobra.Command, req editRequest) error {
	if isJSON(cmd) {
		return printJSON(map[string]any{
			"dry_run":       true,
			"action":        "edit",
			"title":         req.title,
			"summary":       req.summary,
			"section":       req.section,
			"minor":         req.minor,
			"bot":           req.bot,
			"content_bytes": len(req.content),
		})
	}
	fmt.Printf("DRY RUN — no change made.\n")
	fmt.Printf("  would edit: %s\n", req.title)
	fmt.Printf("  summary:    %s\n", req.summary)
	if req.section != "" {
		fmt.Printf("  section:    %s\n", req.section)
	}
	fmt.Printf("  content:    %d bytes\n", len(req.content))
	fmt.Printf("  minor=%t bot=%t\n", req.minor, req.bot)
	return nil
}
