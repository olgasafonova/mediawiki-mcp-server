package wiki

// ========== List Users Types ==========

// ListUsersArgs contains parameters for listing wiki users.
type ListUsersArgs struct {
	BaseArgs
	Group        string `json:"group,omitempty" jsonschema:"Filter by user group: 'sysop' (admins), 'bureaucrat', 'bot', or empty for all users"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum users to return (default 50, max 500)"`
	ContinueFrom string `json:"continue_from,omitempty" jsonschema:"Continue token for pagination"`
	ActiveOnly   bool   `json:"active_only,omitempty" jsonschema:"Only return users active in the last 30 days"`
}

// ListUsersResult contains a paginated list of wiki users.
type ListUsersResult struct {
	Users        []UserInfo `json:"users"`
	TotalCount   int        `json:"total_count"`
	HasMore      bool       `json:"has_more"`
	ContinueFrom string     `json:"continue_from,omitempty"`
	Group        string     `json:"group,omitempty"`
}

// UserInfo describes a wiki user account.
type UserInfo struct {
	UserID       int      `json:"user_id"`
	Name         string   `json:"name"`
	Groups       []string `json:"groups,omitempty"`
	EditCount    int      `json:"edit_count,omitempty"`
	Registration string   `json:"registration,omitempty"`
}

// ========== Get Sections Types ==========

// GetSectionsArgs contains parameters for retrieving page section structure.
type GetSectionsArgs struct {
	BaseArgs
	Title   string `json:"title" jsonschema:"Page title to get sections from"`
	Section int    `json:"section,omitempty" jsonschema:"Specific section number to retrieve content for (0 = intro, 1+ = sections). Omit to list all sections."`
	Format  string `json:"format,omitempty" jsonschema:"Output format for section content: 'wikitext' (default) or 'html'"`
}

// GetSectionsResult contains section headings and optional section content.
type GetSectionsResult struct {
	Title          string        `json:"title"`
	PageID         int           `json:"page_id"`
	Sections       []SectionInfo `json:"sections,omitempty"`
	SectionContent string        `json:"section_content,omitempty"`
	SectionTitle   string        `json:"section_title,omitempty"`
	Format         string        `json:"format,omitempty"`
	Message        string        `json:"message,omitempty"`
}

// SectionInfo describes a single section heading in a page.
type SectionInfo struct {
	Index   int    `json:"index"`
	Level   int    `json:"level"`
	Title   string `json:"title"`
	Anchor  string `json:"anchor"`
	LineNum int    `json:"line_number,omitempty"`
}

// ========== Related Pages Types ==========

// GetRelatedArgs contains parameters for finding related pages.
type GetRelatedArgs struct {
	BaseArgs
	Title  string `json:"title" jsonschema:"Page title to find related pages for"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum related pages to return (default 20, max 50)"`
	Method string `json:"method,omitempty" jsonschema:"Method to find related: 'categories' (default), 'links', 'backlinks', or 'all'"`
}

// GetRelatedResult contains pages related to the source page.
type GetRelatedResult struct {
	Title        string        `json:"title"`
	RelatedPages []RelatedPage `json:"related_pages"`
	Count        int           `json:"count"`
	Method       string        `json:"method"`
	Categories   []string      `json:"categories_used,omitempty"`
}

// RelatedPage represents a page related by category, link, or backlink.
type RelatedPage struct {
	Title      string   `json:"title"`
	PageID     int      `json:"page_id"`
	Relation   string   `json:"relation"`
	Categories []string `json:"shared_categories,omitempty"`
	Score      int      `json:"relevance_score,omitempty"`
}

// ========== Upload File Types ==========

// UploadFileArgs contains parameters for uploading a file to the wiki.
//
// Three mutually-exclusive content sources:
//   - FileURL: wiki fetches the URL itself (subject to host allowlist + SSRF guards).
//   - FileData: caller supplies the bytes directly. Used by the `wiki` CLI,
//     which reads the local file on the user's behalf. JSON-hidden so MCP
//     callers cannot smuggle arbitrary bytes through the tool surface.
//   - FilePath: rejected at the client layer — the MCP server doesn't read
//     local files. Kept for backwards compatibility / explicit error messaging.
type UploadFileArgs struct {
	BaseWriteArgs
	Filename       string `json:"filename" jsonschema:"Target filename on the wiki (e.g., 'Example.png')"`
	FilePath       string `json:"file_path,omitempty" jsonschema:"Local file path to upload (rejected via MCP; CLI use FileData)"`
	FileURL        string `json:"file_url,omitempty" jsonschema:"URL to fetch and upload (alternative to file_path)"`
	FileData       []byte `json:"-"` // CLI-only: bytes read locally by the caller. Not exposed via MCP.
	Text           string `json:"text,omitempty" jsonschema:"File description page content (wikitext)"`
	Comment        string `json:"comment,omitempty" jsonschema:"Upload comment for the log"`
	IgnoreWarnings bool   `json:"ignore_warnings,omitempty" jsonschema:"Ignore duplicate/overwrite warnings"`
}

// UploadFileResult contains the result of a file upload operation.
type UploadFileResult struct {
	Success  bool     `json:"success"`
	Filename string   `json:"filename"`
	PageID   int      `json:"page_id,omitempty"`
	URL      string   `json:"url,omitempty"`
	Size     int      `json:"size,omitempty"`
	Message  string   `json:"message"`
	Warnings []string `json:"warnings,omitempty"`
}

// ========== Get Images Types ==========

// GetImagesArgs contains parameters for retrieving images used on a page.
type GetImagesArgs struct {
	BaseArgs
	Title string `json:"title" jsonschema:"Page title to get images from"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum images to return (default 50, max 500)"`
}

// GetImagesResult contains images and files embedded in a page.
type GetImagesResult struct {
	Title  string      `json:"title"`
	Images []ImageInfo `json:"images"`
	Count  int         `json:"count"`
}

// ImageInfo describes an image or file used on a page.
type ImageInfo struct {
	Title    string `json:"title"`
	URL      string `json:"url,omitempty"`
	ThumbURL string `json:"thumb_url,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Size     int    `json:"size,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// ========== File Search Types ==========

// SearchInFileArgs contains parameters for searching within uploaded files.
type SearchInFileArgs struct {
	BaseArgs
	Filename string `json:"filename" jsonschema:"File page name (e.g., 'File:Report.pdf' or just 'Report.pdf')"`
	Query    string `json:"query" jsonschema:"Text to search for in the file"`
}

// SearchInFileResult contains text matches found in an uploaded file.
type SearchInFileResult struct {
	Filename   string            `json:"filename"`
	FileType   string            `json:"file_type"`
	Matches    []FileSearchMatch `json:"matches"`
	MatchCount int               `json:"match_count"`
	Searchable bool              `json:"searchable"`
	Message    string            `json:"message,omitempty"`
}

// FileSearchMatch represents a text match within an uploaded file.
type FileSearchMatch struct {
	Page    int    `json:"page,omitempty"`
	Line    int    `json:"line,omitempty"`
	Context string `json:"context"`
}
