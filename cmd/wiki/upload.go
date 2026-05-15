package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload <filename>",
		Short: "Upload a file to the wiki from a local path or URL",
		Long: `Upload an image or document to the wiki.

The first argument is the target filename on the wiki (e.g. "Logo.png").
The source is supplied via --file (local path) or --url (remote URL).

  wiki upload "Logo.png" --file ./logo.png --text "Company logo"
  wiki upload "Diagram.svg" --url https://example.com/d.svg --comment "Initial upload"
  wiki upload "Old.png" --file new.png --force  # overwrite existing`,
		Args: cobra.ExactArgs(1),
		RunE: runUpload,
	}

	cmd.Flags().String("file", "", "Local file path to upload")
	cmd.Flags().String("url", "", "URL to fetch and upload (alternative to --file)")
	cmd.Flags().String("text", "", "File description page content (wikitext)")
	cmd.Flags().String("comment", "", "Upload comment shown in the upload log")
	cmd.Flags().Bool("force", false, "Ignore duplicate / overwrite warnings")

	return cmd
}

func runUpload(cmd *cobra.Command, args []string) error {
	filename := args[0]
	filePath, _ := cmd.Flags().GetString("file")
	fileURL, _ := cmd.Flags().GetString("url")
	text, _ := cmd.Flags().GetString("text")
	comment, _ := cmd.Flags().GetString("comment")
	force, _ := cmd.Flags().GetBool("force")

	if filePath == "" && fileURL == "" {
		return usageErr(fmt.Errorf("--file or --url is required"))
	}
	if filePath != "" && fileURL != "" {
		return usageErr(fmt.Errorf("--file and --url are mutually exclusive"))
	}

	// Read the file locally on the user's behalf and hand the bytes to the
	// wiki client via FileData. The client deliberately refuses to read
	// arbitrary local files itself (MCP-safety), so the CLI is the right
	// layer to do filesystem I/O.
	var fileData []byte
	if filePath != "" {
		abs, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", filePath, err)
		}
		fileData, err = os.ReadFile(abs) // #nosec G304 -- path supplied via CLI flag by the invoking user
		if err != nil {
			return fmt.Errorf("read %s: %w", abs, err)
		}
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.UploadFile(context.Background(), wiki.UploadFileArgs{
		Filename:       filename,
		FileData:       fileData,
		FileURL:        fileURL,
		Text:           text,
		Comment:        comment,
		IgnoreWarnings: force,
	})
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if result.Success {
		fmt.Printf("Uploaded %s", result.Filename)
		if result.URL != "" {
			fmt.Printf(" -> %s", result.URL)
		}
		fmt.Println()
	} else {
		fmt.Printf("Upload of %s did not complete: %s\n", result.Filename, result.Message)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}
	return nil
}
