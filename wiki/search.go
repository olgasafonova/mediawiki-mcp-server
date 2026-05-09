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

	// Validate regex upfront before fetching the page
	var re *regexp.Regexp
	if args.UseRegex {
		if len(args.Query) > 500 {
			return SearchInPageResult{}, fmt.Errorf("regex pattern too long (max 500 characters)")
		}
		var err error
		re, err = regexp.Compile("(?i)" + args.Query)
		if err != nil {
			return SearchInPageResult{}, fmt.Errorf("invalid regex: %w", err)
		}
	} else {
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(args.Query))
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
// scoredCandidate is the intermediate representation for similarity ranking.
type scoredCandidate struct {
	title     string
	score     float64
	terms     []string
	isLinked  bool
	linksBack bool
}

// collectSimilarityCandidates returns candidate page titles for the similarity
// search, drawn either from a category or from a full-text search query.
// Excludes the source page itself.
func (c *Client) collectSimilarityCandidates(ctx context.Context, source string, category, searchQuery string) []string {
	var candidates []string
	if category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{Category: category, Limit: 100})
		if err != nil {
			return nil
		}
		for _, member := range catResult.Members {
			if member.Title != source {
				candidates = append(candidates, member.Title)
			}
		}
		return candidates
	}
	searchResult, err := c.Search(ctx, SearchArgs{Query: searchQuery, Limit: 50})
	if err != nil {
		return nil
	}
	for _, hit := range searchResult.Results {
		if hit.Title != source {
			candidates = append(candidates, hit.Title)
		}
	}
	return candidates
}

// loadOutgoingLinkSet returns the set of titles that the given page links to.
// On error, returns an empty set so callers can use it as a "no links" sentinel.
func (c *Client) loadOutgoingLinkSet(ctx context.Context, page string, limit int) map[string]bool {
	set := make(map[string]bool)
	links, err := c.getPageLinks(ctx, page, limit)
	if err != nil {
		return set
	}
	for _, link := range links {
		set[link.Title] = true
	}
	return set
}

// candidateLinksBackTo reports whether the candidate page contains a link to
// the source title.
func (c *Client) candidateLinksBackTo(ctx context.Context, candidate, source string) bool {
	candLinks, err := c.getPageLinks(ctx, candidate, 500)
	if err != nil {
		return false
	}
	for _, link := range candLinks {
		if link.Title == source {
			return true
		}
	}
	return false
}

// scoreSimilarityCandidate computes the similarity score and link metadata for
// one candidate page. Returns ok=false if the candidate fails to fetch or
// scores below the threshold.
func (c *Client) scoreSimilarityCandidate(ctx context.Context, candidate, source string, sourceTerms []string, sourceLinks map[string]bool, minScore float64) (scoredCandidate, bool) {
	candContent, err := c.GetPage(ctx, GetPageArgs{Title: candidate})
	if err != nil {
		return scoredCandidate{}, false
	}
	candTerms := extractKeyTerms(candContent.Content)
	similarity := calculateJaccardSimilarity(sourceTerms, candTerms)
	if similarity < minScore {
		return scoredCandidate{}, false
	}
	return scoredCandidate{
		title:     candidate,
		score:     similarity,
		terms:     findCommonTerms(sourceTerms, candTerms, 10),
		isLinked:  sourceLinks[candidate],
		linksBack: c.candidateLinksBackTo(ctx, candidate, source),
	}, true
}

// similarityRecommendation returns advice for the user given the link state
// between the source and candidate pages.
func similarityRecommendation(score float64, isLinked, linksBack bool, source, candidate string) string {
	switch {
	case score > 0.6 && !isLinked && !linksBack:
		return "Possible duplicate - high similarity but no links between pages"
	case !isLinked && !linksBack:
		return "Consider cross-linking - related content but no links"
	case isLinked && !linksBack:
		return fmt.Sprintf("Add backlink from '%s' to '%s'", candidate, source)
	case !isLinked && linksBack:
		return fmt.Sprintf("Add link from '%s' to '%s'", source, candidate)
	default:
		return "Already cross-linked"
	}
}

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
	source := normalizePageTitle(args.Page)

	sourceContent, err := c.GetPage(ctx, GetPageArgs{Title: source})
	if err != nil {
		return FindSimilarPagesResult{}, fmt.Errorf("failed to get source page: %w", err)
	}
	sourceTerms := extractKeyTerms(sourceContent.Content)
	if len(sourceTerms) == 0 {
		return FindSimilarPagesResult{
			SourcePage:   source,
			SimilarPages: []SimilarPage{},
			Message:      "Source page has no significant terms for comparison",
		}, nil
	}

	searchQuery := strings.Join(extractTopTerms(sourceContent.Content, 5), " ")
	candidates := c.collectSimilarityCandidates(ctx, source, args.Category, searchQuery)
	if len(candidates) == 0 {
		return FindSimilarPagesResult{
			SourcePage:   source,
			SimilarPages: []SimilarPage{},
			Message:      "No candidate pages found for comparison",
		}, nil
	}

	sourceLinks := c.loadOutgoingLinkSet(ctx, source, 500)

	scored := make([]scoredCandidate, 0)
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		if sc, ok := c.scoreSimilarityCandidate(ctx, candidate, source, sourceTerms, sourceLinks, minScore); ok {
			scored = append(scored, sc)
		}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	similarPages := make([]SimilarPage, 0, limit)
	for i, sp := range scored {
		if i >= limit {
			break
		}
		similarPages = append(similarPages, SimilarPage{
			Title:           sp.title,
			SimilarityScore: sp.score,
			CommonTerms:     sp.terms,
			IsLinked:        sp.isLinked,
			LinksBack:       sp.linksBack,
			Recommendation:  similarityRecommendation(sp.score, sp.isLinked, sp.linksBack, source, sp.title),
		})
	}

	return FindSimilarPagesResult{
		SourcePage:    source,
		SimilarPages:  similarPages,
		TotalCompared: len(candidates),
	}, nil
}

// CompareTopic compares how a topic is described across multiple pages
// pageValueRef tracks one value extracted near the topic, with the page it came from.
type pageValueRef struct {
	page  string
	value string
}

// findPagesForTopic returns candidate page titles for topic comparison, drawn
// from a category if specified, otherwise from a full-text search.
func (c *Client) findPagesForTopic(ctx context.Context, topic, category string, searchLimit int) ([]string, error) {
	var pageTitles []string
	if category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{Category: category, Limit: 100})
		if err != nil {
			return pageTitles, nil //nolint:nilerr // category lookup failure falls through to "no candidates"
		}
		for _, member := range catResult.Members {
			pageTitles = append(pageTitles, member.Title)
		}
		return pageTitles, nil
	}
	searchResult, err := c.Search(ctx, SearchArgs{Query: topic, Limit: searchLimit})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	for _, hit := range searchResult.Results {
		pageTitles = append(pageTitles, hit.Title)
	}
	return pageTitles, nil
}

// extractTopicValues collects values from the page that appear near a context
// where the topic is mentioned. Caller-supplied contexts are the windows in
// which co-occurrence is checked.
func extractTopicValues(content, topic string, contexts []string, page string) map[string][]pageValueRef {
	out := make(map[string][]pageValueRef)
	topicLower := strings.ToLower(topic)
	for _, v := range extractValues(content) {
		valueCtxLower := strings.ToLower(v.Context)
		for _, ctxStr := range contexts {
			if strings.Contains(strings.ToLower(ctxStr), valueCtxLower) ||
				strings.Contains(valueCtxLower, topicLower) {
				out[v.Type] = append(out[v.Type], pageValueRef{page: page, value: v.Value})
				break
			}
		}
	}
	return out
}

// analyzeTopicOnPage fetches the page, checks for the topic, and returns the
// mention summary plus any near-topic values. Returns ok=false when the page
// either fails to fetch or doesn't mention the topic.
func (c *Client) analyzeTopicOnPage(ctx context.Context, title, topic string) (TopicMention, map[string][]pageValueRef, bool) {
	page, err := c.GetPage(ctx, GetPageArgs{Title: title})
	if err != nil {
		return TopicMention{}, nil, false
	}
	contentLower := strings.ToLower(page.Content)
	topicLower := strings.ToLower(topic)
	if !strings.Contains(contentLower, topicLower) {
		return TopicMention{}, nil, false
	}
	contexts := extractContextsForTerm(page.Content, topic, 3)
	info, _ := c.GetPageInfo(ctx, PageInfoArgs{Title: title})
	mention := TopicMention{
		PageTitle:  title,
		Mentions:   strings.Count(contentLower, topicLower),
		Contexts:   contexts,
		LastEdited: info.Touched,
	}
	return mention, extractTopicValues(page.Content, topic, contexts, title), true
}

// detectValueInconsistencies pairs up cross-page values of the same type and
// reports any that disagree once normalized.
func detectValueInconsistencies(allValues map[string][]pageValueRef) []Inconsistency {
	inconsistencies := make([]Inconsistency, 0)
	for valueType, pageValues := range allValues {
		if len(pageValues) < 2 {
			continue
		}
		for i := 0; i < len(pageValues)-1; i++ {
			for j := i + 1; j < len(pageValues); j++ {
				if pageValues[i].page == pageValues[j].page {
					continue
				}
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
	return inconsistencies
}

func (c *Client) CompareTopic(ctx context.Context, args CompareTopicArgs) (CompareTopicResult, error) {
	if args.Topic == "" {
		return CompareTopicResult{}, fmt.Errorf("topic is required")
	}
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CompareTopicResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 50)
	pageTitles, err := c.findPagesForTopic(ctx, args.Topic, args.Category, limit*2)
	if err != nil {
		return CompareTopicResult{}, err
	}
	if len(pageTitles) == 0 {
		return CompareTopicResult{
			Topic:        args.Topic,
			PageMentions: []TopicMention{},
			Summary:      fmt.Sprintf("No pages found mentioning '%s'", args.Topic),
		}, nil
	}

	mentions := make([]TopicMention, 0)
	allValues := make(map[string][]pageValueRef)
	for _, title := range pageTitles {
		if len(mentions) >= limit || ctx.Err() != nil {
			break
		}
		mention, values, ok := c.analyzeTopicOnPage(ctx, title, args.Topic)
		if !ok {
			continue
		}
		mentions = append(mentions, mention)
		for valueType, refs := range values {
			allValues[valueType] = append(allValues[valueType], refs...)
		}
	}

	inconsistencies := detectValueInconsistencies(allValues)
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
