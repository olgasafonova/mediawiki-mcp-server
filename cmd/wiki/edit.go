package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

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

func runEdit(cmd *cobra.Command, args []string) error {
	title := args[0]
	content, _ := cmd.Flags().GetString("content")
	summary, _ := cmd.Flags().GetString("summary")
	minor, _ := cmd.Flags().GetBool("minor")
	bot, _ := cmd.Flags().GetBool("bot")
	section, _ := cmd.Flags().GetString("section")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	interactive, _ := cmd.Flags().GetBool("interactive")

	// Interactive mode takes precedence over every other content source: it
	// fetches the current page content, opens an editor, and submits whatever
	// the user saves. Conflicts with --content/--file/stdin are caught here
	// rather than inside the helper so the user gets one error covering all
	// input-source problems. --json is incompatible because the editor is an
	// inherently human-driven flow; advertising "ignored" behavior would
	// silently change the contract under --json.
	if interactive {
		if isJSON(cmd) {
			return usageErr(fmt.Errorf("--interactive cannot be combined with --json (the editor is interactive by definition)"))
		}
		if content != "" {
			return usageErr(fmt.Errorf("--interactive cannot be combined with --content"))
		}
		if filePath, _ := cmd.Flags().GetString("file"); filePath != "" {
			return usageErr(fmt.Errorf("--interactive cannot be combined with --file"))
		}
		// readStdin reports "no piped data" as empty+nil rather than an error,
		// so a stdin leak from a parent shell would silently win over -i.
		// Refuse explicitly: if stdin has data, the user almost certainly
		// meant to pipe it.
		stdinContent, err := readStdin()
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if stdinContent != "" {
			return usageErr(fmt.Errorf("--interactive cannot be combined with piped stdin"))
		}
		if section != "" {
			return usageErr(fmt.Errorf("--interactive cannot be combined with --section (interactive mode edits the full page)"))
		}
		return runInteractiveEdit(cmd, title, summary, minor, bot, section, dryRun)
	}

	// If --content is empty, try --file
	if content == "" {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath != "" {
			fileBytes, err := os.ReadFile(filePath) // #nosec G304 -- path supplied via CLI flag by the invoking user
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			content = string(fileBytes)
		}
	}

	// If still empty, try reading from stdin
	if content == "" {
		var err error
		content, err = readStdin()
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
	}

	if content == "" {
		return usageErr(fmt.Errorf("content is required (use --content, --file, or pipe from stdin)"))
	}

	// --dry-run resolves the content and reports what would be written
	// without contacting the wiki. It deliberately skips client creation so
	// an agent can preview a change without valid credentials — this is the
	// safeguard the primary write previously lacked.
	if dryRun {
		return emitEditDryRun(cmd, title, summary, section, content, minor, bot)
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.EditPage(cmd.Context(), wiki.EditPageArgs{
		Title:   title,
		Content: content,
		Summary: summary,
		Minor:   minor,
		Bot:     bot,
		Section: section,
	})
	if err != nil {
		return fmt.Errorf("edit failed: %w", err)
	}

	// CAPTCHA retry loop
	for !result.Success && result.CaptchaType != "" {
		if isJSON(cmd) {
			break
		}
		result = promptAndRetryCaptcha(cmd, client, title, content, summary, minor, bot, section, result)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	// Human-readable output
	if !result.Success {
		fmt.Printf("Failed to edit %s: %s\n", result.Title, result.Message)
	} else if result.NewPage {
		fmt.Printf("Created %s (rev: %d)\n", result.Title, result.RevisionID)
	} else {
		fmt.Printf("Edited %s (rev: %d)\n", result.Title, result.RevisionID)
	}
	if result.Success && result.PageURL != "" {
		fmt.Printf("URL: %s\n", result.PageURL)
	}

	return nil
}

// emitEditDryRun reports what `wiki edit` would write, without performing
// the edit. JSON under --json, otherwise a short human-readable block. The
// content itself is summarized by byte count rather than echoed, so a large
// page doesn't flood the output.
func emitEditDryRun(cmd *cobra.Command, title, summary, section, content string, minor, bot bool) error {
	if isJSON(cmd) {
		return printJSON(map[string]any{
			"dry_run":       true,
			"action":        "edit",
			"title":         title,
			"summary":       summary,
			"section":       section,
			"minor":         minor,
			"bot":           bot,
			"content_bytes": len(content),
		})
	}
	fmt.Printf("DRY RUN — no change made.\n")
	fmt.Printf("  would edit: %s\n", title)
	fmt.Printf("  summary:    %s\n", summary)
	if section != "" {
		fmt.Printf("  section:    %s\n", section)
	}
	fmt.Printf("  content:    %d bytes\n", len(content))
	fmt.Printf("  minor=%t bot=%t\n", minor, bot)
	return nil
}

// promptAndRetryCaptcha prompts the user for a CAPTCHA answer and retries
// the edit. Tries /dev/tty first, then os.Stdin if it's a terminal. Prints
// a hint to stderr when no interactive prompt is available.
func promptAndRetryCaptcha(cmd *cobra.Command, client *wiki.Client, title, content, summary string, minor, bot bool, section string, original wiki.EditResult) wiki.EditResult {
	question := original.CaptchaQuestion
	if question == "" {
		question = fmt.Sprintf("CAPTCHA type: %s", original.CaptchaType)
	}

	var in io.Reader
	var out io.Writer
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		defer tty.Close()
		in = tty
		out = tty
	} else if info, statErr := os.Stdin.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice != 0 {
		in = os.Stdin
		out = os.Stderr
	}

	if in != nil {
		br := bufio.NewReader(in)
		for {
			fmt.Fprintf(out, "CAPTCHA required: %s\nAnswer: ", question)
			line, readErr := br.ReadString('\n')
			answer := strings.TrimSpace(line)
			if readErr != nil && answer == "" {
				break
			}
			if answer != "" {
				return retryEditWithCaptcha(cmd.Context(), client, title, content, summary, minor, bot, section, original, answer)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "CAPTCHA required but no interactive input available (pass --json to see CAPTCHA details)\n")
	original.CaptchaType = ""
	return original
}

func retryEditWithCaptcha(ctx context.Context, client *wiki.Client, title, content, summary string, minor, bot bool, section string, original wiki.EditResult, answer string) wiki.EditResult {
	result, err := client.EditPage(ctx, wiki.EditPageArgs{
		Title:       title,
		Content:     content,
		Summary:     summary,
		Minor:       minor,
		Bot:         bot,
		Section:     section,
		CaptchaID:   original.CaptchaID,
		CaptchaWord: answer,
	})
	if err != nil {
		return wiki.EditResult{
			Success: false,
			Title:   title,
			Message: fmt.Sprintf("Edit failed on CAPTCHA retry: %v", err),
		}
	}

	return result
}

// runInteractiveEdit fetches the current page content, opens it in
// $VISUAL/$EDITOR for editing, and submits the saved buffer as the new
// content. The flow mirrors the shell idiom
// `wiki page $P > tmp && $EDITOR tmp && wiki edit $P < tmp`, but without
// leaving temp files lying around or silently re-injecting the `wiki page`
// header into the saved content.
//
// Behavior:
//   - resolves $VISUAL, then $EDITOR; errors out if neither is set
//   - fetches current page content; empty buffer for new pages
//   - writes content to a 0600 temp file with a wikitext-friendly suffix
//   - opens the editor with /dev/tty attached when available, otherwise
//     fails with a clear "no interactive terminal" message
//   - shows a unified diff of the changes before submitting
//   - skips the edit if the saved buffer is empty or byte-identical to the
//     original content
//   - prompts for confirmation before submitting; declines keep the buffer
//   - with --dry-run, shows the diff and skips submission
//   - on submit failure, keeps the temp file and prints its path so the
//     user can recover the buffer
func runInteractiveEdit(cmd *cobra.Command, title, summary string, minor, bot bool, section string, dryRun bool) error {
	editor, err := resolveEditor()
	if err != nil {
		return usageErr(err)
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	original, err := fetchInteractivePage(cmd.Context(), client, title)
	if err != nil {
		return fmt.Errorf("failed to fetch page: %w", err)
	}

	tmpFile, originalBytes, err := writeInteractiveBuffer(title, original)
	if err != nil {
		return fmt.Errorf("failed to prepare edit buffer: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Editing %q in %s (buffer: %s)\n", title, editor, tmpFile)
	if dryRun {
		fmt.Fprintf(os.Stderr, "Dry run: save and quit to preview changes. Empty the buffer or leave it unchanged to cancel.\n")
	} else {
		fmt.Fprintf(os.Stderr, "Save and quit to submit. Empty the buffer or leave it unchanged to cancel.\n")
	}

	if err := runEditor(editor, tmpFile); err != nil {
		// Editor failure is not an edit failure: keep the buffer so the
		// user can retry manually.
		return fmt.Errorf("editor exited with error: %w (buffer kept at %s)", err, tmpFile)
	}

	newContent, err := os.ReadFile(tmpFile) // #nosec G304 -- tmpFile is our own path created minutes ago in this process
	if err != nil {
		return fmt.Errorf("failed to read edited buffer: %w (buffer kept at %s)", err, tmpFile)
	}

	if bytes.Equal(newContent, originalBytes) {
		_ = os.Remove(tmpFile) //nolint:errcheck // best-effort cleanup of unused buffer
		fmt.Fprintln(os.Stderr, "Buffer unchanged — edit skipped.")
		return nil
	}
	if strings.TrimSpace(string(newContent)) == "" {
		_ = os.Remove(tmpFile) //nolint:errcheck // best-effort cleanup of unused buffer
		fmt.Fprintln(os.Stderr, "Buffer is empty — edit skipped.")
		return nil
	}

	// Always show the diff so the user can review what will change.
	showInteractiveDiff(title, original, newContent, tmpFile)

	// Prompt for confirmation. In dry-run mode the prompt says so, making
	// it clear that confirming still won't submit.
	prompt := "Submit this edit?"
	if dryRun {
		prompt = "Submit this edit (dry run)?"
	}
	if !promptConfirm(prompt) {
		fmt.Fprintln(os.Stderr, "Edit cancelled — buffer kept.")
		return nil
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "\nDRY RUN — no change made. Buffer kept at: %s\n", tmpFile)
		fmt.Fprintf(os.Stderr, "To submit: wiki edit %q --file %s\n", title, tmpFile)
		return nil
	}

	result, err := client.EditPage(cmd.Context(), wiki.EditPageArgs{
		Title:   title,
		Content: string(newContent),
		Summary: summary,
		Minor:   minor,
		Bot:     bot,
		Section: section,
	})
	if err != nil {
		return fmt.Errorf("edit failed: %w (buffer kept at %s)", err, tmpFile)
	}

	// CAPTCHA retry loop mirrors the non-interactive flow so an interactive
	// edit doesn't lose the saved buffer if the wiki requires CAPTCHA.
	// An empty answer or repeated wrong answers break the loop — Ctrl-C
	// is the other exit.
	const maxCAPTCHARetries = 3
	for attempt := 0; !result.Success && result.CaptchaType != "" && attempt < maxCAPTCHARetries; attempt++ {
		fmt.Fprintf(os.Stderr, "CAPTCHA required: %s\n", result.CaptchaQuestion)
		answer, ok := promptInteractiveAnswer()
		if !ok || answer == "" {
			break
		}
		result = retryEditWithCaptcha(cmd.Context(), client, title, string(newContent), summary, minor, bot, section, result, answer)
	}

	if isJSON(cmd) {
		// Defensive: runEdit refuses --json + --interactive, but keep the
		// guard so the helper is safe in isolation.
		return printJSON(result)
	}

	if !result.Success {
		// Don't delete the buffer on failure — the user may want to retry
		// manually or attach it to a bug report.
		fmt.Fprintf(os.Stderr, "Failed to edit %s: %s\n", result.Title, result.Message)
		fmt.Fprintf(os.Stderr, "Buffer kept at %s\n", tmpFile)
		return fmt.Errorf("edit failed: %s", result.Message)
	}

	// Edit succeeded — buffer can be cleaned up safely.
	_ = os.Remove(tmpFile) //nolint:errcheck // best-effort cleanup after successful edit

	if result.NewPage {
		fmt.Printf("Created %s (rev: %d)\n", result.Title, result.RevisionID)
	} else {
		fmt.Printf("Edited %s (rev: %d)\n", result.Title, result.RevisionID)
	}
	if result.PageURL != "" {
		fmt.Printf("URL: %s\n", result.PageURL)
	}
	return nil
}

// resolveEditor returns the editor command to invoke for interactive edits.
// $VISUAL wins over $EDITOR per the long-standing Unix convention; both must
// resolve to an executable in $PATH.
func resolveEditor() (string, error) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if candidate := strings.TrimSpace(os.Getenv(env)); candidate != "" {
			if _, lookErr := exec.LookPath(candidate); lookErr == nil {
				return candidate, nil
			}
		}
	}
	return "", errors.New("no editor configured: set $VISUAL or $EDITOR to an executable in $PATH")
}

// showInteractiveDiff displays a unified diff between the original and new
// content for interactive edits. It writes the original to a temp file,
// runs diff, and cleans up.
func showInteractiveDiff(title string, original []byte, newContent []byte, tmpFile string) {
	origFile := tmpFile + ".orig"
	if err := os.WriteFile(origFile, original, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create temp file for diff: %v\n", err)
		fmt.Fprintf(os.Stderr, "New content is at: %s\n", tmpFile)
		return
	}
	defer os.Remove(origFile) //nolint:errcheck // best-effort cleanup

	diffCmd := exec.Command("diff", "-u", "--label", "original", "--label", "edited", origFile, tmpFile)
	diffCmd.Stdout = os.Stdout
	diffCmd.Stderr = os.Stderr
	if err := diffCmd.Run(); err != nil {
		// diff exits with status 1 when files differ — that's not an error here
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() > 1 {
			fmt.Fprintf(os.Stderr, "Diff failed: %v\n", err)
		}
	}
}

// fetchInteractivePage retrieves the current content of title. A "page does
// not exist" response is treated as success with empty content so the user
// can author a new page from scratch.
func fetchInteractivePage(ctx context.Context, client *wiki.Client, title string) (string, error) {
	page, err := client.GetPage(ctx, wiki.GetPageArgs{Title: title, Format: "wikitext"})
	if err == nil {
		return page.Content, nil
	}
	if strings.Contains(err.Error(), "does not exist") {
		return "", nil
	}
	return "", err
}

// writeInteractiveBuffer stages the page content in a temp file and returns
// the path along with the exact bytes we wrote, so runInteractiveEdit can
// later compare against the saved buffer to detect "no-op" edits. The file
// is 0600 because it may briefly contain unreviewed content from the wiki.
func writeInteractiveBuffer(title, content string) (path string, original []byte, err error) {
	// Slug the title for the temp file name. We deliberately keep this
	// permissive — the path is only used as a hint to the editor and is
	// created in os.TempDir(), not anywhere user-visible.
	slug := slugifyForTmpFile(title)
	if slug == "" {
		slug = "untitled"
	}
	f, err := os.CreateTemp("", "wiki-edit-"+slug+"-*.wiki")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	// Tight permissions: the file contains page content that may be
	// sensitive on private wikis.
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name()) //nolint:errcheck // best-effort cleanup of failed-to-secure buffer
		return "", nil, fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name()) //nolint:errcheck // best-effort cleanup of failed-to-write buffer
		return "", nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name()) //nolint:errcheck // best-effort cleanup of failed-to-close buffer
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}
	return f.Name(), []byte(content), nil
}

// slugifyForTmpFile produces a filename-safe approximation of a wiki page
// title. We do not try to round-trip the title — only to give the user
// something recognizable in their `ls /tmp` output.
func slugifyForTmpFile(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// runEditor launches editor with tmpFile as its argument. The child inherits
// /dev/tty when available so it can read user input even if the CLI was
// invoked with stdin/stdout piped. If /dev/tty cannot be opened we fail
// rather than fall back silently, because the editor would otherwise hang
// waiting on the parent's stdio.
func runEditor(editor, tmpFile string) error {
	tty := openTTYForChild()
	if tty == nil {
		return fmt.Errorf("no interactive terminal available; run from a real terminal to use --interactive")
	}
	defer tty.Close() //nolint:errcheck // tty close on exit, error non-actionable

	cmd := exec.Command(editor, tmpFile) // #nosec G204 -- editor is user-configured via $VISUAL/$EDITOR; this is the canonical "launch the user's editor" use of exec.Command
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	return cmd.Run()
}

// promptInteractiveAnswer reads one line of user input for CAPTCHA prompts
// during an interactive edit, preferring /dev/tty so piped stdin doesn't
// capture the answer.
func promptInteractiveAnswer() (string, bool) {
	tty := openTTYForChild()
	if tty == nil {
		return "", false
	}
	defer tty.Close() //nolint:errcheck // tty close on exit, error non-actionable

	reader := bufio.NewReader(tty)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimSpace(line), true
}

// promptConfirm displays a yes/no question on /dev/tty and returns true if
// the user answers with y or Y. Any other answer (including EOF or error)
// is treated as "no".
func promptConfirm(question string) bool {
	tty := openTTYForChild()
	if tty == nil {
		return false
	}
	defer tty.Close() //nolint:errcheck // tty close on exit, error non-actionable

	fmt.Fprintf(tty, "%s [y/N] ", question)
	reader := bufio.NewReader(tty)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}

// readStdin reads all available data from stdin if it's not a terminal.
func readStdin() (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}

	// No piped data
	if info.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large pages
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return sb.String(), nil
}
