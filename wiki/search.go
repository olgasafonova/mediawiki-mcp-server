package wiki

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// htmlTagRegex is used to strip HTML tags from search snippets
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// stripHTMLTags removes HTML tags and decodes entities from a string
func stripHTMLTags(s string) string {
	// Decode HTML entities
	s = html.UnescapeString(s)
	// Remove HTML tags
	s = htmlTagRegex.ReplaceAllString(s, "")
	// Clean up whitespace
	s = strings.TrimSpace(s)
	return s
}

// Search searches for pages matching the query
func (c *Client) Search(ctx context.Context, args SearchArgs) (SearchResult, error) {
	if args.Query == "" {
		return SearchResult{}, fmt.Errorf("query is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return SearchResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "search")
	params.Set("srsearch", args.Query)
	params.Set("srlimit", strconv.Itoa(limit))
	params.Set("srprop", "snippet|size|timestamp")

	if args.Offset > 0 {
		params.Set("sroffset", strconv.Itoa(args.Offset))
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return SearchResult{}, err
	}

	query := getMap(resp["query"])
	if query == nil {
		return SearchResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	searchInfo := getMap(query["searchinfo"])
	var totalHits int
	if searchInfo != nil {
		totalHits = getInt(searchInfo["totalhits"])
	}

	searchResults := getSlice(query["search"])
	results := make([]SearchHit, 0, len(searchResults))

	for _, sr := range searchResults {
		item := getMap(sr)
		if item == nil {
			continue
		}
		title := getString(item["title"])
		hit := SearchHit{
			PageID:  getInt(item["pageid"]),
			Title:   title,
			URL:     c.pageURL(ctx, title),
			Snippet: stripHTMLTags(getString(item["snippet"])),
			Size:    getInt(item["size"]),
		}
		results = append(results, hit)
	}

	result := SearchResult{
		Query:     args.Query,
		TotalHits: totalHits,
		Results:   results,
		HasMore:   args.Offset+len(results) < totalHits,
	}

	if result.HasMore {
		result.NextOffset = args.Offset + len(results)
	}

	return result, nil
}

// SearchInPage searches for text within a specific wiki page
// compileSearchRegex compiles the search query, either as a user regex or as
// quoted literal text. It enforces a length cap on user regex input.
func compileSearchRegex(query string, useRegex bool) (*regexp.Regexp, error) {
	if useRegex {
		if len(query) > 500 {
			return nil, fmt.Errorf("regex pattern too long (max 500 characters)")
		}
		re, err := regexp.Compile("(?i)" + query)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		return re, nil
	}
	return regexp.MustCompile("(?i)" + regexp.QuoteMeta(query)), nil
}

// collectLineMatches returns all PageMatches for the regex against a single
// line, including surrounding context.
func collectLineMatches(re *regexp.Regexp, lines []string, lineNum, contextLines int) []PageMatch {
	matches := re.FindAllStringIndex(lines[lineNum], -1)
	if len(matches) == 0 {
		return nil
	}
	startLine := lineNum - contextLines
	if startLine < 0 {
		startLine = 0
	}
	endLine := lineNum + contextLines + 1
	if endLine > len(lines) {
		endLine = len(lines)
	}
	contextStr := strings.Join(lines[startLine:endLine], "\n")
	out := make([]PageMatch, 0, len(matches))
	for _, match := range matches {
		out = append(out, PageMatch{
			Line:    lineNum + 1,
			Column:  match[0] + 1,
			Text:    lines[lineNum][match[0]:match[1]],
			Context: contextStr,
		})
	}
	return out
}

func (c *Client) SearchInPage(ctx context.Context, args SearchInPageArgs) (SearchInPageResult, error) {
	if args.Title == "" {
		return SearchInPageResult{}, fmt.Errorf("title is required")
	}
	if args.Query == "" {
		return SearchInPageResult{}, fmt.Errorf("query is required")
	}

	re, err := compileSearchRegex(args.Query, args.UseRegex)
	if err != nil {
		return SearchInPageResult{}, err
	}

	// Get page content
	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return SearchInPageResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	result := SearchInPageResult{
		Title:   page.Title,
		Query:   args.Query,
		Matches: make([]PageMatch, 0),
	}

	contextLines := args.ContextLines
	if contextLines <= 0 {
		contextLines = 2
	}

	lines := strings.Split(page.Content, "\n")
	for lineNum := range lines {
		result.Matches = append(result.Matches, collectLineMatches(re, lines, lineNum, contextLines)...)
	}

	result.MatchCount = len(result.Matches)
	return result, nil
}

// SearchInFile searches for text within a wiki file (PDF, text files, etc.)
func (c *Client) SearchInFile(ctx context.Context, args SearchInFileArgs) (SearchInFileResult, error) {
	if args.Filename == "" {
		return SearchInFileResult{}, fmt.Errorf("filename is required")
	}
	if args.Query == "" {
		return SearchInFileResult{}, fmt.Errorf("query is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return SearchInFileResult{}, err
	}

	// Normalize filename to include File: prefix
	filename := args.Filename
	if !strings.HasPrefix(filename, "File:") {
		filename = "File:" + filename
	}

	// Get file info including URL
	fileURL, fileType, err := c.getFileURL(ctx, filename)
	if err != nil {
		return SearchInFileResult{}, fmt.Errorf("failed to get file info: %w", err)
	}

	// Download the file
	fileData, err := c.downloadFile(ctx, fileURL)
	if err != nil {
		return SearchInFileResult{}, fmt.Errorf("failed to download file: %w", err)
	}

	result := SearchInFileResult{
		Filename: filename,
		FileType: fileType,
		Matches:  make([]FileSearchMatch, 0),
	}

	// Handle based on file type
	switch strings.ToLower(fileType) {
	case "pdf", "application/pdf":
		matches, searchable, message, err := SearchInPDF(fileData, args.Query)
		if err != nil {
			return SearchInFileResult{}, err
		}
		result.Matches = matches
		result.MatchCount = len(matches)
		result.Searchable = searchable
		result.Message = message

	case "txt", "text", "text/plain", "md", "markdown", "csv", "json", "xml", "html":
		// Text-based files - search directly
		text := string(fileData)
		matches := searchInText(text, args.Query, 1)
		result.Matches = matches
		result.MatchCount = len(matches)
		result.Searchable = true
		if len(matches) == 0 {
			result.Message = fmt.Sprintf("No matches found for '%s'", args.Query)
		} else {
			result.Message = fmt.Sprintf("Found %d matches", len(matches))
		}

	default:
		result.Searchable = false
		result.Message = fmt.Sprintf("File type '%s' is not supported for text search. Supported types: PDF (text-based), TXT, MD, CSV, JSON, XML, HTML", fileType)
	}

	return result, nil
}

// FindSimilarPages finds pages with similar content to the given page
// scoredCandidate is the intermediate representation for similarity ranking.
// pageValueRef tracks one value extracted near the topic, with the page it came from.
