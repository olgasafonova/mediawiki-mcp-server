package wiki

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
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
		hit := SearchHit{
			PageID:  getInt(item["pageid"]),
			Title:   getString(item["title"]),
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
func (c *Client) SearchInPage(ctx context.Context, args SearchInPageArgs) (SearchInPageResult, error) {
	if args.Title == "" {
		return SearchInPageResult{}, fmt.Errorf("title is required")
	}
	if args.Query == "" {
		return SearchInPageResult{}, fmt.Errorf("query is required")
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

	// Build regex
	var re *regexp.Regexp
	if args.UseRegex {
		re, err = regexp.Compile("(?i)" + args.Query)
		if err != nil {
			return SearchInPageResult{}, fmt.Errorf("invalid regex: %w", err)
		}
	} else {
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(args.Query))
	}

	contextLines := args.ContextLines
	if contextLines <= 0 {
		contextLines = 2
	}

	lines := strings.Split(page.Content, "\n")

	for lineNum, line := range lines {
		matches := re.FindAllStringIndex(line, -1)
		for _, match := range matches {
			// Build context from surrounding lines
			startLine := lineNum - contextLines
			if startLine < 0 {
				startLine = 0
			}
			endLine := lineNum + contextLines + 1
			if endLine > len(lines) {
				endLine = len(lines)
			}

			contextStr := strings.Join(lines[startLine:endLine], "\n")

			result.Matches = append(result.Matches, PageMatch{
				Line:    lineNum + 1,
				Column:  match[0] + 1,
				Text:    line[match[0]:match[1]],
				Context: contextStr,
			})
		}
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
func (c *Client) FindSimilarPages(ctx context.Context, args FindSimilarPagesArgs) (FindSimilarPagesResult, error) {
	if args.Page == "" {
		return FindSimilarPagesResult{}, fmt.Errorf("page is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindSimilarPagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 10, 50)
	minScore := args.MinScore
	if minScore <= 0 {
		minScore = 0.1
	}

	normalizedTitle := normalizePageTitle(args.Page)

	// 1. Get source page content
	sourceContent, err := c.GetPage(ctx, GetPageArgs{Title: normalizedTitle})
	if err != nil {
		return FindSimilarPagesResult{}, fmt.Errorf("failed to get source page: %w", err)
	}

	// 2. Extract key terms from source
	sourceTerms := extractKeyTerms(sourceContent.Content)
	if len(sourceTerms) == 0 {
		return FindSimilarPagesResult{
			SourcePage:   normalizedTitle,
			SimilarPages: []SimilarPage{},
			Message:      "Source page has no significant terms for comparison",
		}, nil
	}

	// 3. Get top terms for search query
	topTerms := extractTopTerms(sourceContent.Content, 5)
	searchQuery := strings.Join(topTerms, " ")

	// 4. Get candidate pages
	var candidatePages []string
	if args.Category != "" {
		// Search within category
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    100,
		})
		if err == nil {
			for _, member := range catResult.Members {
				if member.Title != normalizedTitle {
					candidatePages = append(candidatePages, member.Title)
				}
			}
		}
	} else {
		// Search wiki for similar pages
		searchResult, err := c.Search(ctx, SearchArgs{
			Query: searchQuery,
			Limit: 50,
		})
		if err == nil {
			for _, hit := range searchResult.Results {
				if hit.Title != normalizedTitle {
					candidatePages = append(candidatePages, hit.Title)
				}
			}
		}
	}

	if len(candidatePages) == 0 {
		return FindSimilarPagesResult{
			SourcePage:   normalizedTitle,
			SimilarPages: []SimilarPage{},
			Message:      "No candidate pages found for comparison",
		}, nil
	}

	// 5. Get source page links (for link status checking)
	sourceLinks := make(map[string]bool)
	linksInfo, err := c.getPageLinks(ctx, normalizedTitle, 500)
	if err == nil {
		for _, link := range linksInfo {
			sourceLinks[link.Title] = true
		}
	}

	// 6. Compare each candidate
	type scoredPage struct {
		title     string
		score     float64
		terms     []string
		isLinked  bool
		linksBack bool
	}

	scored := make([]scoredPage, 0)

	for _, candidateTitle := range candidatePages {
		// Check for context cancellation
		if ctx.Err() != nil {
			break
		}

		// Get candidate content
		candContent, err := c.GetPage(ctx, GetPageArgs{Title: candidateTitle})
		if err != nil {
			continue
		}

		// Extract terms and calculate similarity
		candTerms := extractKeyTerms(candContent.Content)
		similarity := calculateJaccardSimilarity(sourceTerms, candTerms)

		if similarity >= minScore {
			commonTerms := findCommonTerms(sourceTerms, candTerms, 10)

			// Check if source links to candidate
			isLinked := sourceLinks[candidateTitle]

			// Check if candidate links back
			linksBack := false
			candLinks, err := c.getPageLinks(ctx, candidateTitle, 500)
			if err == nil {
				for _, link := range candLinks {
					if link.Title == normalizedTitle {
						linksBack = true
						break
					}
				}
			}

			scored = append(scored, scoredPage{
				title:     candidateTitle,
				score:     similarity,
				terms:     commonTerms,
				isLinked:  isLinked,
				linksBack: linksBack,
			})
		}
	}

	// 7. Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 8. Build result
	similarPages := make([]SimilarPage, 0, limit)
	for i, sp := range scored {
		if i >= limit {
			break
		}

		// Generate recommendation
		var recommendation string
		if sp.score > 0.6 && !sp.isLinked && !sp.linksBack {
			recommendation = "Possible duplicate - high similarity but no links between pages"
		} else if !sp.isLinked && !sp.linksBack {
			recommendation = "Consider cross-linking - related content but no links"
		} else if sp.isLinked && !sp.linksBack {
			recommendation = fmt.Sprintf("Add backlink from '%s' to '%s'", sp.title, normalizedTitle)
		} else if !sp.isLinked && sp.linksBack {
			recommendation = fmt.Sprintf("Add link from '%s' to '%s'", normalizedTitle, sp.title)
		} else {
			recommendation = "Already cross-linked"
		}

		similarPages = append(similarPages, SimilarPage{
			Title:           sp.title,
			SimilarityScore: sp.score,
			CommonTerms:     sp.terms,
			IsLinked:        sp.isLinked,
			LinksBack:       sp.linksBack,
			Recommendation:  recommendation,
		})
	}

	return FindSimilarPagesResult{
		SourcePage:    normalizedTitle,
		SimilarPages:  similarPages,
		TotalCompared: len(candidatePages),
	}, nil
}

// CompareTopic compares how a topic is described across multiple pages
func (c *Client) CompareTopic(ctx context.Context, args CompareTopicArgs) (CompareTopicResult, error) {
	if args.Topic == "" {
		return CompareTopicResult{}, fmt.Errorf("topic is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CompareTopicResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 50)

	// 1. Find pages mentioning the topic
	var pageTitles []string
	if args.Category != "" {
		// Search within category
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    100,
		})
		if err == nil {
			for _, member := range catResult.Members {
				pageTitles = append(pageTitles, member.Title)
			}
		}
	} else {
		// Search wiki
		searchResult, err := c.Search(ctx, SearchArgs{
			Query: args.Topic,
			Limit: limit * 2, // Get extra in case some don't have the term
		})
		if err != nil {
			return CompareTopicResult{}, fmt.Errorf("search failed: %w", err)
		}
		for _, hit := range searchResult.Results {
			pageTitles = append(pageTitles, hit.Title)
		}
	}

	if len(pageTitles) == 0 {
		return CompareTopicResult{
			Topic:        args.Topic,
			PageMentions: []TopicMention{},
			Summary:      fmt.Sprintf("No pages found mentioning '%s'", args.Topic),
		}, nil
	}

	// 2. Analyze each page
	mentions := make([]TopicMention, 0)
	allValues := make(map[string][]struct {
		page  string
		value string
	})

	for _, title := range pageTitles {
		if len(mentions) >= limit {
			break
		}

		// Check for context cancellation
		if ctx.Err() != nil {
			break
		}

		// Get page content
		pageContent, err := c.GetPage(ctx, GetPageArgs{Title: title})
		if err != nil {
			continue
		}

		// Check if topic actually appears in content
		topicLower := strings.ToLower(args.Topic)
		contentLower := strings.ToLower(pageContent.Content)
		if !strings.Contains(contentLower, topicLower) {
			continue
		}

		// Count mentions
		mentionCount := strings.Count(contentLower, topicLower)

		// Extract contexts
		contexts := extractContextsForTerm(pageContent.Content, args.Topic, 3)

		// Get page info for last edited
		pageInfo, _ := c.GetPageInfo(ctx, PageInfoArgs{Title: title})

		mentions = append(mentions, TopicMention{
			PageTitle:  title,
			Mentions:   mentionCount,
			Contexts:   contexts,
			LastEdited: pageInfo.Touched,
		})

		// Extract values for inconsistency detection
		values := extractValues(pageContent.Content)
		for _, v := range values {
			// Only track values that appear near the topic
			for _, ctxStr := range contexts {
				if strings.Contains(strings.ToLower(ctxStr), strings.ToLower(v.Context)) ||
					strings.Contains(strings.ToLower(v.Context), topicLower) {
					allValues[v.Type] = append(allValues[v.Type], struct {
						page  string
						value string
					}{title, v.Value})
					break
				}
			}
		}
	}

	// 3. Detect inconsistencies
	inconsistencies := make([]Inconsistency, 0)
	for valueType, pageValues := range allValues {
		if len(pageValues) < 2 {
			continue
		}

		// Compare values between pages
		for i := 0; i < len(pageValues)-1; i++ {
			for j := i + 1; j < len(pageValues); j++ {
				// Skip same page comparisons
				if pageValues[i].page == pageValues[j].page {
					continue
				}

				// Normalize values for comparison
				v1 := normalizeValue(pageValues[i].value)
				v2 := normalizeValue(pageValues[j].value)

				if v1 != v2 && v1 != "" && v2 != "" {
					inconsistencies = append(inconsistencies, Inconsistency{
						Type:        valueType,
						Description: fmt.Sprintf("%s values differ", valueType),
						PageA:       pageValues[i].page,
						PageB:       pageValues[j].page,
						ValueA:      pageValues[i].value,
						ValueB:      pageValues[j].value,
					})
				}
			}
		}
	}

	// 4. Generate summary
	summary := fmt.Sprintf("Found %d pages mentioning '%s'", len(mentions), args.Topic)
	if len(inconsistencies) > 0 {
		summary += fmt.Sprintf(". Detected %d potential inconsistencies", len(inconsistencies))
	}

	return CompareTopicResult{
		Topic:           args.Topic,
		PagesFound:      len(mentions),
		PageMentions:    mentions,
		Inconsistencies: inconsistencies,
		Summary:         summary,
	}, nil
}

// normalizeValue extracts the core numeric value for comparison
func normalizeValue(value string) string {
	// Extract just numbers for comparison
	re := regexp.MustCompile(`\d+(?:\.\d+)?`)
	matches := re.FindAllString(value, -1)
	if len(matches) > 0 {
		return matches[0]
	}
	return strings.TrimSpace(strings.ToLower(value))
}
