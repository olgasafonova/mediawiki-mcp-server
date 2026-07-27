package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

// editSession pairs a wiki client with the request being submitted so the
// CAPTCHA-retry helpers share state instead of long argument lists. Both
// `wiki edit` and `wiki publish` submit through it.
type editSession struct {
	client *wiki.Client
	req    editRequest
}

// retryLoop keeps prompting for CAPTCHA answers until the edit succeeds,
// the CAPTCHA requirement clears, or JSON mode makes prompting impossible.
func (s editSession) retryLoop(cmd *cobra.Command, result wiki.EditResult) wiki.EditResult {
	for !result.Success && result.CaptchaType != "" {
		if isJSON(cmd) {
			break
		}
		result = s.promptAndRetryCaptcha(cmd, result)
	}
	return result
}

// promptAndRetryCaptcha prompts the user for a CAPTCHA answer and retries
// the edit. Tries /dev/tty first, then os.Stdin if it's a terminal. Prints
// a hint to stderr when no interactive prompt is available.
func (s editSession) promptAndRetryCaptcha(cmd *cobra.Command, original wiki.EditResult) wiki.EditResult {
	question := original.CaptchaQuestion
	if question == "" {
		question = fmt.Sprintf("CAPTCHA type: %s", original.CaptchaType)
	}

	in, out, cleanup := captchaPromptIO()
	defer cleanup()

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
				// Non-interactive edits don't fetch the page first, so there
				// is no base timestamp to assert against.
				return s.retryEditWithCaptcha(cmd.Context(), original, answer)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "CAPTCHA required but no interactive input available (pass --json to see CAPTCHA details)\n")
	original.CaptchaType = ""
	return original
}

// captchaPromptIO resolves where CAPTCHA prompts read from and write to:
// /dev/tty when available, otherwise os.Stdin if it is a terminal. A nil
// reader means no interactive prompt is possible. The returned cleanup
// closes /dev/tty when it was opened.
func captchaPromptIO() (io.Reader, io.Writer, func()) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		return tty, tty, func() { _ = tty.Close() }
	}
	if info, statErr := os.Stdin.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice != 0 {
		return os.Stdin, os.Stderr, func() {}
	}
	return nil, nil, func() {}
}

// retryEditWithCaptcha re-submits the edit with the CAPTCHA answer attached.
func (s editSession) retryEditWithCaptcha(ctx context.Context, original wiki.EditResult, answer string) wiki.EditResult {
	args := s.req.toArgs()
	args.CaptchaID = original.CaptchaID
	args.CaptchaWord = answer

	result, err := s.client.EditPage(ctx, args)
	if err != nil {
		return wiki.EditResult{
			Success: false,
			Title:   s.req.title,
			Message: fmt.Sprintf("Edit failed on CAPTCHA retry: %v", err),
		}
	}

	return result
}
