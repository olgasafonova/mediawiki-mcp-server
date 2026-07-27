package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

// checkInteractiveConflicts rejects flag combinations that conflict with
// --interactive. Conflicts with --content/--file/stdin are caught here
// rather than inside the interactive helper so the user gets one error
// covering all input-source problems. --json is incompatible because the
// editor is an inherently human-driven flow; advertising "ignored" behavior
// would silently change the contract under --json.
func checkInteractiveConflicts(cmd *cobra.Command, req editRequest) error {
	if isJSON(cmd) {
		return usageErr(fmt.Errorf("--interactive cannot be combined with --json (the editor is interactive by definition)"))
	}
	if req.content != "" {
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
	if req.section != "" {
		return usageErr(fmt.Errorf("--interactive cannot be combined with --section (interactive mode edits the full page)"))
	}
	return nil
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
func runInteractiveEdit(cmd *cobra.Command, req editRequest, dryRun bool) error {
	editor, err := resolveEditor()
	if err != nil {
		return usageErr(err)
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := newInteractiveEditSession(cmd, client, req, dryRun)
	if err != nil {
		return err
	}
	session.editor = editor

	newContent, submit, err := session.collectBuffer()
	if err != nil || !submit {
		return err
	}
	return session.finish(newContent)
}

// interactiveEditSession carries the state shared by the phases of an
// interactive edit: the wiki client, the pending request, and the temp
// buffer staged on disk.
type interactiveEditSession struct {
	cmd      *cobra.Command
	client   *wiki.Client
	req      editRequest
	editor   string
	tmpFile  string
	original []byte
	dryRun   bool
}

// newInteractiveEditSession fetches the current page content and stages it
// in the temp buffer the editor will open.
func newInteractiveEditSession(cmd *cobra.Command, client *wiki.Client, req editRequest, dryRun bool) (*interactiveEditSession, error) {
	original, baseTimestamp, err := fetchInteractivePage(cmd.Context(), client, req.title)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	req.baseTimestamp = baseTimestamp

	tmpFile, originalBytes, err := writeInteractiveBuffer(req.title, original)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare edit buffer: %w", err)
	}

	return &interactiveEditSession{
		cmd:      cmd,
		client:   client,
		req:      req,
		tmpFile:  tmpFile,
		original: originalBytes,
		dryRun:   dryRun,
	}, nil
}

// announce tells the user which editor is opening and how to cancel.
func (s *interactiveEditSession) announce() {
	fmt.Fprintf(os.Stderr, "Editing %q in %s (buffer: %s)\n", s.req.title, s.editor, s.tmpFile)
	if s.dryRun {
		fmt.Fprintf(os.Stderr, "Dry run: save and quit to preview changes. Empty the buffer or leave it unchanged to cancel.\n")
	} else {
		fmt.Fprintf(os.Stderr, "Save and quit to submit. Empty the buffer or leave it unchanged to cancel.\n")
	}
}

// collectBuffer opens the editor on the temp buffer and returns the saved
// content. submit=false means the edit was skipped (unchanged or empty
// buffer) and the reason was already reported to the user.
func (s *interactiveEditSession) collectBuffer() (newContent []byte, submit bool, err error) {
	s.announce()

	if err := s.runEditor(); err != nil {
		// Editor failure is not an edit failure: keep the buffer so the
		// user can retry manually.
		return nil, false, fmt.Errorf("editor exited with error: %w (buffer kept at %s)", err, s.tmpFile)
	}

	newContent, err = os.ReadFile(s.tmpFile) // #nosec G304 -- tmpFile is our own path created minutes ago in this process
	if err != nil {
		return nil, false, fmt.Errorf("failed to read edited buffer: %w (buffer kept at %s)", err, s.tmpFile)
	}

	if bytes.Equal(newContent, s.original) {
		_ = os.Remove(s.tmpFile) //nolint:errcheck // best-effort cleanup of unused buffer
		fmt.Fprintln(os.Stderr, "Buffer unchanged — edit skipped.")
		return nil, false, nil
	}
	if strings.TrimSpace(string(newContent)) == "" {
		_ = os.Remove(s.tmpFile) //nolint:errcheck // best-effort cleanup of unused buffer
		fmt.Fprintln(os.Stderr, "Buffer is empty — edit skipped.")
		return nil, false, nil
	}
	return newContent, true, nil
}

// finish reviews the change with the user and, unless this is a dry run,
// submits it.
func (s *interactiveEditSession) finish(newContent []byte) error {
	// Always show the diff so the user can review what will change.
	s.showDiff(newContent)

	// Prompt for confirmation. In dry-run mode the prompt says so, making
	// it clear that confirming still won't submit.
	prompt := "Submit this edit?"
	if s.dryRun {
		prompt = "Submit this edit (dry run)?"
	}
	if !promptConfirm(prompt) {
		fmt.Fprintln(os.Stderr, "Edit canceled — buffer kept.")
		return nil
	}

	if s.dryRun {
		fmt.Fprintf(os.Stderr, "\nDRY RUN — no change made. Buffer kept at: %s\n", s.tmpFile)
		fmt.Fprintf(os.Stderr, "To submit: wiki edit %q --file %s\n", s.req.title, s.tmpFile)
		return nil
	}

	return s.submit(newContent)
}

// submit performs the interactive edit against the wiki, handling edit
// conflicts, CAPTCHA retries, and result reporting.
func (s *interactiveEditSession) submit(newContent []byte) error {
	s.req.content = string(newContent)

	// BaseTimestamp makes the wiki reject the submit with 'editconflict'
	// if someone else edited the page while the editor was open, instead
	// of silently overwriting their change. Interactive sessions last
	// minutes, not milliseconds, so this window is real.
	result, err := s.client.EditPage(s.cmd.Context(), s.req.toArgs())
	if err != nil {
		return s.submitError(err)
	}

	result = s.retryCaptchaLoop(result)
	return s.report(result)
}

// submitError translates a failed submit into a user-facing error, with
// special handling for edit conflicts. The buffer is always kept.
func (s *interactiveEditSession) submitError(err error) error {
	if strings.Contains(err.Error(), "editconflict") {
		fmt.Fprintf(os.Stderr, "Edit conflict: %q was edited by someone else while your editor was open.\n", s.req.title)
		fmt.Fprintf(os.Stderr, "Your changes are kept at: %s\n", s.tmpFile)
		fmt.Fprintf(os.Stderr, "Re-run 'wiki edit %q -i' to fetch the latest version and reapply them.\n", s.req.title)
		return fmt.Errorf("edit conflict on %q (buffer kept at %s)", s.req.title, s.tmpFile)
	}
	return fmt.Errorf("edit failed: %w (buffer kept at %s)", err, s.tmpFile)
}

// retryCaptchaLoop mirrors the non-interactive CAPTCHA flow so an
// interactive edit doesn't lose the saved buffer if the wiki requires
// CAPTCHA. An empty answer or repeated wrong answers break the loop —
// Ctrl-C is the other exit.
func (s *interactiveEditSession) retryCaptchaLoop(result wiki.EditResult) wiki.EditResult {
	const maxCAPTCHARetries = 3
	session := editSession{client: s.client, req: s.req}
	for attempt := 0; !result.Success && result.CaptchaType != "" && attempt < maxCAPTCHARetries; attempt++ {
		fmt.Fprintf(os.Stderr, "CAPTCHA required: %s\n", result.CaptchaQuestion)
		answer, ok := promptInteractiveAnswer()
		if !ok || answer == "" {
			break
		}
		result = session.retryEditWithCaptcha(s.cmd.Context(), result, answer)
	}
	return result
}

// report prints the outcome of an interactive edit and cleans up the temp
// buffer on success.
func (s *interactiveEditSession) report(result wiki.EditResult) error {
	if isJSON(s.cmd) {
		// Defensive: runEdit refuses --json + --interactive, but keep the
		// guard so the helper is safe in isolation.
		return printJSON(result)
	}

	if !result.Success {
		// Don't delete the buffer on failure — the user may want to retry
		// manually or attach it to a bug report.
		fmt.Fprintf(os.Stderr, "Failed to edit %s: %s\n", result.Title, result.Message)
		fmt.Fprintf(os.Stderr, "Buffer kept at %s\n", s.tmpFile)
		return fmt.Errorf("edit failed: %s", result.Message)
	}

	// Edit succeeded — buffer can be cleaned up safely.
	_ = os.Remove(s.tmpFile) //nolint:errcheck // best-effort cleanup after successful edit

	printEditSuccess(result)
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

// showDiff displays a unified diff between the original and new content for
// interactive edits. It writes the original to a temp file, runs diff, and
// cleans up.
func (s *interactiveEditSession) showDiff(newContent []byte) {
	origFile := s.tmpFile + ".orig"
	if err := os.WriteFile(origFile, s.original, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create temp file for diff: %v\n", err)
		fmt.Fprintf(os.Stderr, "New content is at: %s\n", s.tmpFile)
		return
	}
	defer os.Remove(origFile) //nolint:errcheck // best-effort cleanup

	// #nosec G204 -- both paths are self-created temp files, not user input
	diffCmd := exec.Command("diff", "-u", "--label", "original", "--label", "edited", origFile, s.tmpFile) //nolint:gosec // G204: self-created temp files

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
func fetchInteractivePage(ctx context.Context, client *wiki.Client, title string) (content, timestamp string, err error) {
	page, err := client.GetPage(ctx, wiki.GetPageArgs{Title: title, Format: "wikitext"})
	if err == nil {
		return page.Content, page.Timestamp, nil
	}
	if strings.Contains(err.Error(), "does not exist") {
		return "", "", nil
	}
	return "", "", err
}

// writeInteractiveBuffer stages the page content in a temp file and returns
// the path along with the exact bytes we wrote, so the interactive flow can
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
	out := strings.Trim(strings.Map(slugRune, title), "_")
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// slugRune maps a title rune to its temp-file-name replacement: ASCII
// letters, digits, and a few separator characters pass through, everything
// else becomes an underscore.
func slugRune(r rune) rune {
	isAlnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	if isAlnum || strings.ContainsRune("-_.", r) {
		return r
	}
	return '_'
}

// runEditor launches the resolved editor on the temp buffer. The child
// inherits /dev/tty when available so it can read user input even if the CLI
// was invoked with stdin/stdout piped. If /dev/tty cannot be opened we fail
// rather than fall back silently, because the editor would otherwise hang
// waiting on the parent's stdio.
func (s *interactiveEditSession) runEditor() error {
	tty := openTTYForChild()
	if tty == nil {
		return fmt.Errorf("no interactive terminal available; run from a real terminal to use --interactive")
	}
	defer tty.Close() //nolint:errcheck // tty close on exit, error non-actionable

	cmd := exec.Command(s.editor, s.tmpFile) // #nosec G204 -- editor is user-configured via $VISUAL/$EDITOR; this is the canonical "launch the user's editor" use of exec.Command
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
