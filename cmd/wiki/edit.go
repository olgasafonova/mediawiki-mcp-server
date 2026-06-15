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

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <title>",
		Short: "Create or edit a wiki page",
		Long: `Edit a wiki page with new content. Content can be provided via --content flag,
--file flag, or piped from stdin:

  wiki edit "Page Title" --content "= Hello ="
  wiki edit "Page Title" --file page.wiki --summary "Update from file"
  cat page.wiki | wiki edit "Page Title" --summary "Update from file"`,
		Args: cobra.ExactArgs(1),
		RunE: runEdit,
	}

	cmd.Flags().StringP("file", "f", "", "Path to file containing wikitext content")
	cmd.Flags().String("content", "", "Page content in wikitext format")
	cmd.Flags().String("summary", "", "Edit summary explaining the change")
	cmd.Flags().Bool("minor", false, "Mark as minor edit")
	cmd.Flags().Bool("bot", false, "Mark as bot edit (requires bot flag)")
	cmd.Flags().String("section", "", "Section to edit ('new' for new section, number for existing)")

	return cmd
}

func runEdit(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	title := args[0]
	content, _ := cmd.Flags().GetString("content")
	summary, _ := cmd.Flags().GetString("summary")
	minor, _ := cmd.Flags().GetBool("minor")
	bot, _ := cmd.Flags().GetBool("bot")
	section, _ := cmd.Flags().GetString("section")

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
		content, err = readStdin()
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
	}

	if content == "" {
		return fmt.Errorf("content is required (use --content or pipe from stdin)")
	}

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
