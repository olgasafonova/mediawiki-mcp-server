package wiki

import (
	"context"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Private/internal IP ranges that should be blocked for SSRF protection
var (
	privateIPBlocks []*net.IPNet
)

// safeDialer prevents DNS rebinding attacks by validating IP at connection time
// This runs AFTER DNS resolution but BEFORE the TCP connection is established
var safeDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
	Control: func(network, address string, c syscall.RawConn) error {
		// Extract IP from address (format is "ip:port")
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("invalid address format: %w", err)
		}

		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("failed to parse IP: %s", host)
		}

		if isPrivateIP(ip) {
			return fmt.Errorf("connection to private IP %s blocked (SSRF protection)", host)
		}

		return nil
	},
}

// linkCheckClient is a shared HTTP client for link checking with connection pooling
// Using a shared client improves performance by reusing TCP connections
// SECURITY: Uses safeDialer to prevent DNS rebinding attacks
var linkCheckClient = &http.Client{
	// Timeout is set per-request via context; this is a fallback max
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext:         safeDialer.DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // Link checking doesn't need compression
		ForceAttemptHTTP2:   true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Allow up to 5 redirects
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}

		// Also validate redirect targets to prevent SSRF via redirect
		lastReq := via[len(via)-1]
		if hostname := lastReq.URL.Hostname(); hostname != "" {
			isPrivate, _ := isPrivateHost(hostname)
			if isPrivate {
				return fmt.Errorf("redirect to private network blocked")
			}
		}

		return nil
	},
}

func init() {
	// Initialize private IP ranges
	// These are IPs that shouldn't be accessed via external link checking
	privateCIDRs := []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC 1918 - Private Class A
		"172.16.0.0/12",  // RFC 1918 - Private Class B
		"192.168.0.0/16", // RFC 1918 - Private Class C
		"169.254.0.0/16", // Link-local
		"0.0.0.0/8",      // Current network
		"100.64.0.0/10",  // Shared address space (CGN)
		"192.0.0.0/24",   // IETF Protocol assignments
		"192.0.2.0/24",   // TEST-NET-1
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24", // TEST-NET-3
		"224.0.0.0/4",    // Multicast
		"240.0.0.0/4",    // Reserved
		"255.255.255.255/32", // Broadcast
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local
		"ff00::/8",       // IPv6 multicast
	}

	for _, cidr := range privateCIDRs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// isPrivateIP checks if an IP address is private/internal
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true // Treat nil as private (fail-safe)
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// isPrivateHost checks if a hostname resolves to any private IP
// Returns an error if the host resolves to a private IP
func isPrivateHost(hostname string) (bool, error) {
	// First, try to parse as an IP directly
	if ip := net.ParseIP(hostname); ip != nil {
		return isPrivateIP(ip), nil
	}

	// Resolve hostname
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// DNS resolution failed - could be temporary, let the HTTP client handle it
		return false, nil
	}

	// Check all resolved IPs - if ANY is private, block it
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true, nil
		}
	}
	return false, nil
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

// GetPage retrieves page content
// Handles title normalization automatically (case, underscores, whitespace)
func (c *Client) GetPage(ctx context.Context, args GetPageArgs) (PageContent, error) {
	if args.Title == "" {
		return PageContent{}, fmt.Errorf("title is required")
	}

	// Normalize the title to handle case variations
	// MediaWiki normalizes titles internally, but we do it here for better cache hits
	// and to avoid duplicate API calls for "Module overview" vs "Module Overview"
	normalizedTitle := normalizePageTitle(args.Title)

	// Check cache with normalized title
	cacheKey := fmt.Sprintf("page_content:%s", normalizedTitle)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(PageContent), nil
	}

	format := args.Format
	if format == "" {
		format = "wikitext"
	}

	var result PageContent
	var err error

	if format == "html" {
		result, err = c.getPageHTML(ctx, normalizedTitle)
	} else {
		result, err = c.getPageWikitext(ctx, normalizedTitle)
	}

	if err != nil {
		return PageContent{}, err
	}

	// Cache the result using the canonical title from API response
	c.setCache(cacheKey, result, "page_content")

	// Also cache under the original title if different (for future lookups)
	if args.Title != normalizedTitle {
		originalCacheKey := fmt.Sprintf("page_content:%s", args.Title)
		c.setCache(originalCacheKey, result, "page_content")
	}

	return result, nil
}

func (c *Client) getPageWikitext(ctx context.Context, title string) (PageContent, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageContent{}, fmt.Errorf("authentication required: %w (configure MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD)", err)
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "revisions")
	params.Set("rvprop", "content|ids|timestamp")
	params.Set("rvslots", "main")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageContent{}, fmt.Errorf("API request failed: %w", err)
	}

	// Safely extract query object
	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'query' object. This may indicate authentication is required for reading pages")
	}

	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'pages' object")
	}

	for pageID, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if page exists
		if _, missing := page["missing"]; missing {
			// Try to suggest similar pages
			return PageContent{}, fmt.Errorf("page '%s' does not exist. Try using mediawiki_resolve_title to find the correct page name", title)
		}

		revisions, ok := page["revisions"].([]interface{})
		if !ok || len(revisions) == 0 {
			return PageContent{}, fmt.Errorf("no revisions found for page '%s'. The page may be empty or protected", title)
		}

		rev, ok := revisions[0].(map[string]interface{})
		if !ok {
			return PageContent{}, fmt.Errorf("invalid revision data for page '%s'", title)
		}

		slots, ok := rev["slots"].(map[string]interface{})
		if !ok {
			return PageContent{}, fmt.Errorf("invalid slots data for page '%s'. This may be a MediaWiki version compatibility issue", title)
		}

		main, ok := slots["main"].(map[string]interface{})
		if !ok {
			return PageContent{}, fmt.Errorf("invalid main slot data for page '%s'", title)
		}

		// MediaWiki API returns content under "*" key, not "content"
		content, ok := main["*"].(string)
		if !ok {
			// Some versions might use "content" instead
			content, ok = main["content"].(string)
			if !ok {
				return PageContent{}, fmt.Errorf("page '%s' has no content or content is not text", title)
			}
		}

		truncated := false
		if len(content) > CharacterLimit {
			content, truncated = truncateContent(content, CharacterLimit)
		}

		id, _ := strconv.Atoi(pageID)
		pageTitle, _ := page["title"].(string)
		if pageTitle == "" {
			pageTitle = title
		}

		revID := 0
		if rid, ok := rev["revid"].(float64); ok {
			revID = int(rid)
		}

		timestamp, _ := rev["timestamp"].(string)

		result := PageContent{
			Title:     pageTitle,
			PageID:    id,
			Content:   content,
			Format:    "wikitext",
			Revision:  revID,
			Timestamp: timestamp,
			Truncated: truncated,
		}

		if truncated {
			result.Message = "Content was truncated due to size limits. Consider fetching specific sections."
		}

		return result, nil
	}

	return PageContent{}, fmt.Errorf("page '%s' not found in API response", title)
}

func (c *Client) getPageHTML(ctx context.Context, title string) (PageContent, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageContent{}, fmt.Errorf("authentication required: %w (configure MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD)", err)
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", title)
	params.Set("prop", "text|revid")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageContent{}, fmt.Errorf("API request failed: %w", err)
	}

	parse, ok := resp["parse"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'parse' object. Page '%s' may not exist or authentication is required", title)
	}

	text, ok := parse["text"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'text' object for page '%s'", title)
	}

	content, ok := text["*"].(string)
	if !ok {
		return PageContent{}, fmt.Errorf("page '%s' has no HTML content", title)
	}

	// Sanitize HTML to prevent XSS
	content = sanitizeHTML(content)

	truncated := false
	if len(content) > CharacterLimit {
		content, truncated = truncateContent(content, CharacterLimit)
	}

	pageTitle, _ := parse["title"].(string)
	if pageTitle == "" {
		pageTitle = title
	}

	pageID := 0
	if pid, ok := parse["pageid"].(float64); ok {
		pageID = int(pid)
	}

	revID := 0
	if rid, ok := parse["revid"].(float64); ok {
		revID = int(rid)
	}

	result := PageContent{
		Title:     pageTitle,
		PageID:    pageID,
		Content:   content,
		Format:    "html",
		Revision:  revID,
		Truncated: truncated,
	}

	if truncated {
		result.Message = "Content was truncated due to size limits."
	}

	return result, nil
}

// getNamespacePageCount tries to get total page count for a namespace
// Returns 0 if unable to fetch statistics
func (c *Client) getNamespacePageCount(ctx context.Context, namespace int) int {
	// For main namespace (0), we can get statistics from siteinfo
	if namespace == 0 {
		params := url.Values{}
		params.Set("action", "query")
		params.Set("meta", "siteinfo")
		params.Set("siprop", "statistics")

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			return 0
		}

		query := getMap(resp["query"])
		if query == nil {
			return 0
		}
		stats := getMap(query["statistics"])
		if stats == nil {
			return 0
		}

		// "articles" gives the count of content pages in main namespace
		return getInt(stats["articles"])
	}

	// For other namespaces, we can't efficiently get totals without iterating
	return 0
}

// ListPages lists pages in the wiki
func (c *Client) ListPages(ctx context.Context, args ListPagesArgs) (ListPagesResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ListPagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "allpages")
	params.Set("aplimit", strconv.Itoa(limit))

	if args.Prefix != "" {
		params.Set("apprefix", args.Prefix)
	}

	if args.Namespace >= 0 {
		params.Set("apnamespace", strconv.Itoa(args.Namespace))
	}

	if args.ContinueFrom != "" {
		params.Set("apcontinue", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ListPagesResult{}, err
	}

	query := getMap(resp["query"])
	if query == nil {
		return ListPagesResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	allpages := getSlice(query["allpages"])
	pages := make([]PageSummary, 0, len(allpages))
	for _, p := range allpages {
		page := getMap(p)
		if page == nil {
			continue
		}
		pages = append(pages, PageSummary{
			PageID: getInt(page["pageid"]),
			Title:  getString(page["title"]),
		})
	}

	result := ListPagesResult{
		Pages:         pages,
		ReturnedCount: len(pages),
		TotalCount:    len(pages), // Deprecated: kept for backwards compatibility
	}

	// Check for continuation
	if cont := getMap(resp["continue"]); cont != nil {
		if apcontinue := getString(cont["apcontinue"]); apcontinue != "" {
			result.HasMore = true
			result.ContinueFrom = apcontinue
		}
	}

	// Try to get namespace statistics for total estimate (only when no prefix filter)
	if args.Prefix == "" && args.Namespace >= 0 {
		if estimate := c.getNamespacePageCount(ctx, args.Namespace); estimate > 0 {
			result.TotalEstimate = estimate
		}
	}

	return result, nil
}

// ListCategories lists categories in the wiki
func (c *Client) ListCategories(ctx context.Context, args ListCategoriesArgs) (ListCategoriesResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ListCategoriesResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "allcategories")
	params.Set("aclimit", strconv.Itoa(limit))
	params.Set("acprop", "size")

	if args.Prefix != "" {
		params.Set("acprefix", args.Prefix)
	}

	if args.ContinueFrom != "" {
		params.Set("accontinue", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ListCategoriesResult{}, err
	}

	query := getMap(resp["query"])
	if query == nil {
		return ListCategoriesResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	allcats := getSlice(query["allcategories"])
	categories := make([]CategoryInfo, 0, len(allcats))
	for _, cat := range allcats {
		catMap := getMap(cat)
		if catMap == nil {
			continue
		}
		categories = append(categories, CategoryInfo{
			Title:   getString(catMap["*"]),
			Members: getInt(catMap["size"]),
		})
	}

	result := ListCategoriesResult{
		Categories: categories,
	}

	// Check for continuation
	if cont := getMap(resp["continue"]); cont != nil {
		if accontinue := getString(cont["accontinue"]); accontinue != "" {
			result.HasMore = true
			result.ContinueFrom = accontinue
		}
	}

	return result, nil
}

// GetCategoryMembers gets pages in a category
func (c *Client) GetCategoryMembers(ctx context.Context, args CategoryMembersArgs) (CategoryMembersResult, error) {
	if args.Category == "" {
		return CategoryMembersResult{}, fmt.Errorf("category is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CategoryMembersResult{}, err
	}

	category := normalizeCategoryName(args.Category)
	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "categorymembers")
	params.Set("cmtitle", category)
	params.Set("cmlimit", strconv.Itoa(limit))

	if args.Type != "" {
		params.Set("cmtype", args.Type)
	}

	if args.ContinueFrom != "" {
		params.Set("cmcontinue", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return CategoryMembersResult{}, err
	}

	query := getMap(resp["query"])
	if query == nil {
		return CategoryMembersResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	members := getSlice(query["categorymembers"])
	pages := make([]PageSummary, 0, len(members))
	for _, m := range members {
		member := getMap(m)
		if member == nil {
			continue
		}
		pages = append(pages, PageSummary{
			PageID: getInt(member["pageid"]),
			Title:  getString(member["title"]),
		})
	}

	result := CategoryMembersResult{
		Category: category,
		Members:  pages,
	}

	// Check for continuation
	if cont := getMap(resp["continue"]); cont != nil {
		if cmcontinue := getString(cont["cmcontinue"]); cmcontinue != "" {
			result.HasMore = true
			result.ContinueFrom = cmcontinue
		}
	}

	return result, nil
}

// GetPageInfo gets metadata about a page
// Handles title normalization automatically
func (c *Client) GetPageInfo(ctx context.Context, args PageInfoArgs) (PageInfo, error) {
	if args.Title == "" {
		return PageInfo{}, fmt.Errorf("title is required")
	}

	// Normalize the title for consistent lookups
	normalizedTitle := normalizePageTitle(args.Title)

	// Check cache first
	cacheKey := fmt.Sprintf("page_info:%s", normalizedTitle)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(PageInfo), nil
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageInfo{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", normalizedTitle)
	params.Set("prop", "info|categories|links")
	params.Set("inprop", "protection|url")
	params.Set("cllimit", "50")
	params.Set("pllimit", "max")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageInfo{}, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	for _, pageData := range pages {
		page := pageData.(map[string]interface{})

		// Check if page exists
		if _, missing := page["missing"]; missing {
			return PageInfo{
				Title:  args.Title,
				Exists: false,
			}, nil
		}

		info := PageInfo{
			Title:        page["title"].(string),
			PageID:       int(page["pageid"].(float64)),
			Namespace:    int(page["ns"].(float64)),
			ContentModel: getString(page["contentmodel"]),
			PageLanguage: getString(page["pagelanguage"]),
			Length:       int(page["length"].(float64)),
			Touched:      getString(page["touched"]),
			LastRevision: int(page["lastrevid"].(float64)),
			Exists:       true,
		}

		// Categories
		if cats, ok := page["categories"].([]interface{}); ok {
			for _, cat := range cats {
				c := cat.(map[string]interface{})
				info.Categories = append(info.Categories, c["title"].(string))
			}
		}

		// Links count
		if links, ok := page["links"].([]interface{}); ok {
			info.Links = len(links)
		}

		// Redirect
		if _, isRedirect := page["redirect"]; isRedirect {
			info.Redirect = true
		}

		// Protection
		if protection, ok := page["protection"].([]interface{}); ok {
			for _, p := range protection {
				prot := p.(map[string]interface{})
				info.Protection = append(info.Protection, fmt.Sprintf("%s: %s", prot["type"], prot["level"]))
			}
		}

		// Cache the result
		c.setCache(cacheKey, info, "page_info")

		return info, nil
	}

	return PageInfo{}, fmt.Errorf("page '%s' not found", normalizedTitle)
}

// EditPage creates or edits a page
func (c *Client) EditPage(ctx context.Context, args EditPageArgs) (EditResult, error) {
	if args.Title == "" {
		return EditResult{}, &ValidationError{
			Field:   "title",
			Message: "page title is required",
			Suggestion: `Provide a title for the page you want to edit.

Example:
  Title: "My Page"
  Title: "Category:My Category"
  Title: "User:Username/Subpage"`,
		}
	}
	if args.Content == "" {
		return EditResult{}, &ValidationError{
			Field:   "content",
			Message: "page content is required",
			Suggestion: `Provide the wikitext content for the page.

Example:
  Content: "== Section ==\nThis is the page content."

If you want to clear a page, use a single space or redirect instead.`,
		}
	}

	// Validate content size
	if err := ValidateContentSize(args.Content, args.Title, MaxEditSize); err != nil {
		return EditResult{}, err
	}

	// Validate content for dangerous patterns
	if err := ValidateWikitextContent(args.Content, args.Title); err != nil {
		return EditResult{}, err
	}

	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return EditResult{}, fmt.Errorf("authentication failed: %w", err)
	}

	params := url.Values{}
	params.Set("action", "edit")
	params.Set("title", args.Title)
	params.Set("text", args.Content)
	params.Set("token", token)

	if args.Summary != "" {
		params.Set("summary", args.Summary)
	}

	if args.Minor {
		params.Set("minor", "1")
	}

	if args.Bot {
		params.Set("bot", "1")
	}

	if args.Section != "" {
		params.Set("section", args.Section)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return EditResult{}, err
	}

	edit := resp["edit"].(map[string]interface{})
	result := getString(edit["result"])

	if result != "Success" {
		return EditResult{
			Success: false,
			Title:   args.Title,
			Message: fmt.Sprintf("Edit failed: %s", result),
		}, nil
	}

	editResult := EditResult{
		Success:    true,
		Title:      getString(edit["title"]),
		PageID:     int(edit["pageid"].(float64)),
		RevisionID: int(edit["newrevid"].(float64)),
		NewPage:    edit["new"] != nil,
		Message:    "Page edited successfully",
	}

	if editResult.NewPage {
		editResult.Message = "Page created successfully"
	}

	return editResult, nil
}

// GetRecentChanges gets recent changes
func (c *Client) GetRecentChanges(ctx context.Context, args RecentChangesArgs) (RecentChangesResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return RecentChangesResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "recentchanges")
	params.Set("rclimit", strconv.Itoa(limit))
	params.Set("rcprop", "title|ids|sizes|flags|user|timestamp|comment")

	if args.Namespace >= 0 {
		params.Set("rcnamespace", strconv.Itoa(args.Namespace))
	}

	if args.Type != "" {
		params.Set("rctype", args.Type)
	}

	if args.ContinueFrom != "" {
		params.Set("rccontinue", args.ContinueFrom)
	}

	if args.Start != "" {
		params.Set("rcstart", args.Start)
	}

	if args.End != "" {
		params.Set("rcend", args.End)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return RecentChangesResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	rcList := query["recentchanges"].([]interface{})

	changes := make([]RecentChange, 0, len(rcList))
	for _, rc := range rcList {
		change := rc.(map[string]interface{})

		ts, _ := time.Parse(time.RFC3339, change["timestamp"].(string))

		changes = append(changes, RecentChange{
			Type:       getString(change["type"]),
			Title:      getString(change["title"]),
			PageID:     getInt(change["pageid"]),
			RevisionID: getInt(change["revid"]),
			User:       getString(change["user"]),
			Timestamp:  ts,
			Comment:    getString(change["comment"]),
			SizeDiff:   getInt(change["newlen"]) - getInt(change["oldlen"]),
			New:        change["new"] != nil,
			Minor:      change["minor"] != nil,
			Bot:        change["bot"] != nil,
		})
	}

	result := RecentChangesResult{}

	// Check for continuation
	if cont, ok := resp["continue"].(map[string]interface{}); ok {
		if rccontinue, ok := cont["rccontinue"].(string); ok {
			result.HasMore = true
			result.ContinueFrom = rccontinue
		}
	}

	// Handle aggregation if requested
	if args.AggregateBy != "" {
		aggregated := aggregateChanges(changes, args.AggregateBy)
		if aggregated != nil {
			result.Aggregated = aggregated
			return result, nil
		}
		// Invalid aggregate_by value, fall through to return raw changes
	}

	result.Changes = changes
	return result, nil
}

// Parse parses wikitext and returns HTML
func (c *Client) Parse(ctx context.Context, args ParseArgs) (ParseResult, error) {
	if args.Wikitext == "" {
		return ParseResult{}, fmt.Errorf("wikitext is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ParseResult{}, err
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("text", args.Wikitext)
	params.Set("contentmodel", "wikitext")
	params.Set("prop", "text|categories|links")

	if args.Title != "" {
		params.Set("title", args.Title)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ParseResult{}, err
	}

	parse := resp["parse"].(map[string]interface{})
	text := parse["text"].(map[string]interface{})
	htmlContent := text["*"].(string)

	// Sanitize HTML to prevent XSS
	htmlContent = sanitizeHTML(htmlContent)

	truncated := false
	if len(htmlContent) > CharacterLimit {
		htmlContent, truncated = truncateContent(htmlContent, CharacterLimit)
	}

	result := ParseResult{
		HTML:      htmlContent,
		Truncated: truncated,
	}

	// Categories
	if cats, ok := parse["categories"].([]interface{}); ok {
		for _, cat := range cats {
			c := cat.(map[string]interface{})
			result.Categories = append(result.Categories, c["*"].(string))
		}
	}

	// Links
	if links, ok := parse["links"].([]interface{}); ok {
		for _, link := range links {
			l := link.(map[string]interface{})
			result.Links = append(result.Links, l["*"].(string))
		}
	}

	if truncated {
		result.Message = "Content was truncated due to size limits."
	}

	return result, nil
}

// GetWikiInfo gets information about the wiki
func (c *Client) GetWikiInfo(ctx context.Context, args WikiInfoArgs) (WikiInfo, error) {
	// Check cache first
	cacheKey := "wiki_info"
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(WikiInfo), nil
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return WikiInfo{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "siteinfo")
	params.Set("siprop", "general|statistics")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return WikiInfo{}, err
	}

	query := resp["query"].(map[string]interface{})
	general := query["general"].(map[string]interface{})

	info := WikiInfo{
		SiteName:    getString(general["sitename"]),
		MainPage:    getString(general["mainpage"]),
		Base:        getString(general["base"]),
		Generator:   getString(general["generator"]),
		PHPVersion:  getString(general["phpversion"]),
		Language:    getString(general["lang"]),
		ArticlePath: getString(general["articlepath"]),
		Server:      getString(general["server"]),
		Timezone:    getString(general["timezone"]),
		WriteAPI:    general["writeapi"] != nil,
	}

	// Statistics
	if stats, ok := query["statistics"].(map[string]interface{}); ok {
		info.Statistics = &WikiStats{
			Pages:       getInt(stats["pages"]),
			Articles:    getInt(stats["articles"]),
			Edits:       getInt(stats["edits"]),
			Images:      getInt(stats["images"]),
			Users:       getInt(stats["users"]),
			ActiveUsers: getInt(stats["activeusers"]),
			Admins:      getInt(stats["admins"]),
		}
	}

	// Cache the result
	c.setCache(cacheKey, info, "wiki_info")

	return info, nil
}

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	// Decode HTML entities
	s = html.UnescapeString(s)
	// Remove HTML tags
	s = htmlTagRegex.ReplaceAllString(s, "")
	// Clean up whitespace
	s = strings.TrimSpace(s)
	return s
}

// GetExternalLinks retrieves external links from a wiki page
func (c *Client) GetExternalLinks(ctx context.Context, args GetExternalLinksArgs) (ExternalLinksResult, error) {
	if args.Title == "" {
		return ExternalLinksResult{}, fmt.Errorf("title is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ExternalLinksResult{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", args.Title)
	params.Set("prop", "extlinks")
	params.Set("ellimit", "500")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ExternalLinksResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return ExternalLinksResult{}, fmt.Errorf("unexpected response format")
	}

	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return ExternalLinksResult{}, fmt.Errorf("no pages in response")
	}

	links := make([]ExternalLink, 0) // Initialize as empty slice, not nil, to avoid JSON null
	var pageTitle string

	for _, pageData := range pages {
		page := pageData.(map[string]interface{})

		// Check if page exists
		if _, missing := page["missing"]; missing {
			return ExternalLinksResult{}, fmt.Errorf("page '%s' does not exist", args.Title)
		}

		pageTitle = page["title"].(string)

		if extlinks, ok := page["extlinks"].([]interface{}); ok {
			for _, el := range extlinks {
				link := el.(map[string]interface{})
				linkURL := getString(link["*"])
				if linkURL == "" {
					linkURL = getString(link["url"])
				}
				if linkURL != "" {
					protocol := ""
					if u, err := url.Parse(linkURL); err == nil {
						protocol = u.Scheme
					}
					links = append(links, ExternalLink{
						URL:      linkURL,
						Protocol: protocol,
					})
				}
			}
		}
		break
	}

	return ExternalLinksResult{
		Title: pageTitle,
		Links: links,
		Count: len(links),
	}, nil
}

// GetExternalLinksBatch retrieves external links from multiple wiki pages
// Uses a worker pool pattern to limit concurrent API requests
func (c *Client) GetExternalLinksBatch(ctx context.Context, args GetExternalLinksBatchArgs) (ExternalLinksBatchResult, error) {
	if len(args.Titles) == 0 {
		return ExternalLinksBatchResult{}, fmt.Errorf("at least one title is required")
	}

	// Limit batch size to prevent overwhelming the API
	maxBatch := 10
	if len(args.Titles) > maxBatch {
		args.Titles = args.Titles[:maxBatch]
	}

	// Worker pool configuration
	numWorkers := 4 // Limit concurrent API requests
	if len(args.Titles) < numWorkers {
		numWorkers = len(args.Titles)
	}

	// Job and result types
	type job struct {
		index int
		title string
	}
	type pageResult struct {
		index int
		data  PageExternalLinks
	}

	jobs := make(chan job, len(args.Titles))
	results := make(chan pageResult, len(args.Titles))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Check context cancellation
				select {
				case <-ctx.Done():
					results <- pageResult{
						index: j.index,
						data: PageExternalLinks{
							Title: j.title,
							Links: make([]ExternalLink, 0),
							Error: "request cancelled",
						},
					}
					continue
				default:
				}

				pageLinks, err := c.GetExternalLinks(ctx, GetExternalLinksArgs{Title: j.title})
				if err != nil {
					results <- pageResult{
						index: j.index,
						data: PageExternalLinks{
							Title: j.title,
							Links: make([]ExternalLink, 0),
							Error: err.Error(),
						},
					}
					continue
				}

				results <- pageResult{
					index: j.index,
					data: PageExternalLinks{
						Title: pageLinks.Title,
						Links: pageLinks.Links,
						Count: pageLinks.Count,
					},
				}
			}
		}()
	}

	// Send jobs to workers
	for i, title := range args.Titles {
		jobs <- job{index: i, title: title}
	}
	close(jobs)

	// Close results channel when all workers complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results maintaining order
	pageResults := make([]PageExternalLinks, len(args.Titles))
	totalLinks := 0

	for pr := range results {
		pageResults[pr.index] = pr.data
		totalLinks += pr.data.Count
	}

	return ExternalLinksBatchResult{
		Pages:      pageResults,
		TotalLinks: totalLinks,
	}, nil
}

// CheckLinks checks if URLs are accessible (broken link detection)
func (c *Client) CheckLinks(ctx context.Context, args CheckLinksArgs) (CheckLinksResult, error) {
	if len(args.URLs) == 0 {
		return CheckLinksResult{}, fmt.Errorf("at least one URL is required")
	}

	// Limit URLs to prevent abuse
	maxURLs := 20
	if len(args.URLs) > maxURLs {
		args.URLs = args.URLs[:maxURLs]
	}

	// Set timeout (default 10s, max 30s)
	timeout := 10
	if args.Timeout > 0 && args.Timeout <= 30 {
		timeout = args.Timeout
	}
	requestTimeout := time.Duration(timeout) * time.Second

	result := CheckLinksResult{
		Results:    make([]LinkCheckResult, 0, len(args.URLs)),
		TotalLinks: len(args.URLs),
	}

	// Use a semaphore to limit concurrent checks
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, linkURL := range args.URLs {
		wg.Add(1)
		go func(checkURL string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			linkResult := LinkCheckResult{URL: checkURL}

			// Validate URL format
			parsedURL, err := url.Parse(checkURL)
			if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
				linkResult.Status = "invalid_url"
				linkResult.Error = "Invalid URL format"
				linkResult.Broken = true
			} else if hostname := parsedURL.Hostname(); hostname != "" {
				// SSRF protection: block private/internal IPs
				isPrivate, _ := isPrivateHost(hostname)
				if isPrivate {
					linkResult.Status = "blocked"
					linkResult.Error = "URLs pointing to private/internal networks are not allowed"
					linkResult.Broken = true
				} else {
					// Create per-request context with timeout
					reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)

					// Make HEAD request first (faster)
					req, _ := http.NewRequestWithContext(reqCtx, "HEAD", checkURL, nil)
					req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")

					resp, err := linkCheckClient.Do(req)
					if err != nil {
						// Try GET if HEAD fails (some servers don't support HEAD)
						req, _ = http.NewRequestWithContext(reqCtx, "GET", checkURL, nil)
						req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")
						resp, err = linkCheckClient.Do(req)
					}

					cancel() // Release context resources

					if err != nil {
						linkResult.Status = "error"
						linkResult.Error = err.Error()
						linkResult.Broken = true
					} else {
						_ = resp.Body.Close() // Error ignored; we only need the status
						linkResult.StatusCode = resp.StatusCode
						linkResult.Status = resp.Status

						// Consider 4xx and 5xx as broken (except 403 which might be access denied)
						if resp.StatusCode >= 400 {
							linkResult.Broken = true
						}
					}
				}
			}

			mu.Lock()
			result.Results = append(result.Results, linkResult)
			if linkResult.Broken {
				result.BrokenCount++
			} else {
				result.ValidCount++
			}
			mu.Unlock()
		}(linkURL)
	}

	wg.Wait()
	return result, nil
}

// CheckTerminology checks pages for terminology inconsistencies based on a wiki glossary
func (c *Client) CheckTerminology(ctx context.Context, args CheckTerminologyArgs) (CheckTerminologyResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTerminologyResult{}, err
	}

	// Default glossary page
	glossaryPage := args.GlossaryPage
	if glossaryPage == "" {
		glossaryPage = "Brand Terminology Glossary"
	}

	// Load glossary from wiki
	glossary, err := c.loadGlossary(ctx, glossaryPage)
	if err != nil {
		return CheckTerminologyResult{}, fmt.Errorf("failed to load glossary from '%s': %w", glossaryPage, err)
	}

	if len(glossary) == 0 {
		return CheckTerminologyResult{}, fmt.Errorf("no terms found in glossary page '%s'", glossaryPage)
	}

	// Get pages to check
	var pagesToCheck []string
	limit := normalizeLimit(args.Limit, 10, 50)

	if len(args.Pages) > 0 {
		pagesToCheck = args.Pages
		if len(pagesToCheck) > limit {
			pagesToCheck = pagesToCheck[:limit]
		}
	} else if args.Category != "" {
		// Get pages from category
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return CheckTerminologyResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			pagesToCheck = append(pagesToCheck, p.Title)
		}
	} else {
		return CheckTerminologyResult{}, fmt.Errorf("either 'pages' or 'category' must be specified")
	}

	result := CheckTerminologyResult{
		GlossaryPage: glossaryPage,
		TermsLoaded:  len(glossary),
		Pages:        make([]PageTerminologyResult, 0, len(pagesToCheck)),
	}

	// Determine if code blocks should be excluded (default: true)
	excludeCode := true
	if args.ExcludeCodeBlocks != nil {
		excludeCode = *args.ExcludeCodeBlocks
	}

	// Check each page
	for _, pageTitle := range pagesToCheck {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		pageResult := c.checkPageTerminology(ctx, pageTitle, glossary, excludeCode)
		result.Pages = append(result.Pages, pageResult)
		result.IssuesFound += pageResult.IssueCount
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// loadGlossary parses a wiki table to extract glossary terms
func (c *Client) loadGlossary(ctx context.Context, glossaryPage string) ([]GlossaryTerm, error) {
	page, err := c.GetPage(ctx, GetPageArgs{Title: glossaryPage, Format: "wikitext"})
	if err != nil {
		return nil, err
	}

	return parseWikiTableGlossary(page.Content), nil
}

// parseWikiTableGlossary extracts terms from wikitable format
func parseWikiTableGlossary(content string) []GlossaryTerm {
	var terms []GlossaryTerm

	// Match wiki tables with class containing "mcp-glossary" or any wikitable
	// Format: {| class="wikitable..." ... |}
	tableRegex := regexp.MustCompile(`(?s)\{\|[^\n]*class="[^"]*(?:mcp-glossary|wikitable)[^"]*"[^\n]*\n(.*?)\|\}`)
	tables := tableRegex.FindAllStringSubmatch(content, -1)

	for _, table := range tables {
		if len(table) < 2 {
			continue
		}

		tableContent := table[1]

		// Split into rows (|-) and process each
		rows := strings.Split(tableContent, "|-")
		for _, row := range rows {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, "!") {
				// Skip empty rows and header rows
				continue
			}

			// Parse cells (|| separator or | at line start)
			cells := parseTableRow(row)
			if len(cells) >= 2 {
				term := GlossaryTerm{
					Incorrect: strings.TrimSpace(cells[0]),
					Correct:   strings.TrimSpace(cells[1]),
				}

				// Skip if incorrect is empty or equals correct
				if term.Incorrect == "" || term.Incorrect == term.Correct {
					continue
				}

				if len(cells) >= 3 {
					term.Pattern = strings.TrimSpace(cells[2])
				}
				if len(cells) >= 4 {
					term.Notes = strings.TrimSpace(cells[3])
				}
				terms = append(terms, term)
			}
		}
	}

	return terms
}

// parseTableRow extracts cells from a wiki table row
func parseTableRow(row string) []string {
	var cells []string
	lines := strings.Split(row, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		// Remove leading | if present
		line = strings.TrimPrefix(line, "|")

		// Split by || for multiple cells on one line
		parts := strings.Split(line, "||")
		for _, part := range parts {
			cell := strings.TrimSpace(part)
			if cell != "" {
				cells = append(cells, cell)
			}
		}
	}

	return cells
}

// checkPageTerminology checks a single page against the glossary
func (c *Client) checkPageTerminology(ctx context.Context, title string, glossary []GlossaryTerm, excludeCode bool) PageTerminologyResult {
	result := PageTerminologyResult{
		Title:  title,
		Issues: make([]TerminologyIssue, 0),
	}

	page, err := c.GetPage(ctx, GetPageArgs{Title: title, Format: "wikitext"})
	if err != nil {
		result.Error = err.Error()
		return result
	}

	content := page.Content

	// Strip code blocks to avoid false positives on code paths like SI.Data
	if excludeCode {
		content = stripCodeBlocksForTerminology(content)
	}

	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		for _, term := range glossary {
			// Use regex pattern if specified, otherwise literal match
			var re *regexp.Regexp
			var err error

			if term.Pattern != "" {
				re, err = regexp.Compile("(?i)" + term.Pattern)
			} else {
				// Escape special regex characters and do case-insensitive match
				escaped := regexp.QuoteMeta(term.Incorrect)
				re, err = regexp.Compile("(?i)" + escaped)
			}

			if err != nil {
				continue
			}

			matches := re.FindAllStringIndex(line, -1)
			for _, match := range matches {
				// Extract the actual matched text
				matchedText := line[match[0]:match[1]]

				// Skip if the matched text is actually the correct form
				if strings.EqualFold(matchedText, term.Correct) {
					continue
				}

				// Get context (surrounding text)
				context := extractContext(line, match[0], match[1], 40)

				result.Issues = append(result.Issues, TerminologyIssue{
					Incorrect: matchedText,
					Correct:   term.Correct,
					Line:      lineNum + 1,
					Context:   context,
					Notes:     term.Notes,
				})
			}
		}
	}

	result.IssueCount = len(result.Issues)
	return result
}

// extractContext extracts surrounding text for context
func extractContext(line string, start, end, contextLen int) string {
	// Calculate bounds
	ctxStart := start - contextLen
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEnd := end + contextLen
	if ctxEnd > len(line) {
		ctxEnd = len(line)
	}

	context := line[ctxStart:ctxEnd]

	// Add ellipsis if truncated
	if ctxStart > 0 {
		context = "..." + context
	}
	if ctxEnd < len(line) {
		context = context + "..."
	}

	return context
}

// stripCodeBlocksForTerminology removes code block content while preserving line structure
// This prevents false positives on code paths like SI.Data, namespace.Class, etc.
func stripCodeBlocksForTerminology(content string) string {
	// Replace content inside code tags with spaces to preserve line numbers
	// Handles: <syntaxhighlight>, <source>, <pre>, <code>
	codeTagPatterns := []string{
		`(?is)<syntaxhighlight[^>]*>(.*?)</syntaxhighlight>`,
		`(?is)<source[^>]*>(.*?)</source>`,
		`(?is)<pre[^>]*>(.*?)</pre>`,
		`(?is)<code[^>]*>(.*?)</code>`,
	}

	for _, pattern := range codeTagPatterns {
		re := regexp.MustCompile(pattern)
		content = re.ReplaceAllStringFunc(content, func(match string) string {
			// Replace the entire match with spaces, preserving newlines
			var result strings.Builder
			for _, ch := range match {
				if ch == '\n' {
					result.WriteRune('\n')
				} else {
					result.WriteRune(' ')
				}
			}
			return result.String()
		})
	}

	return content
}

// CheckTranslations checks if pages exist in all specified languages
func (c *Client) CheckTranslations(ctx context.Context, args CheckTranslationsArgs) (CheckTranslationsResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTranslationsResult{}, err
	}

	if len(args.Languages) == 0 {
		return CheckTranslationsResult{}, fmt.Errorf("at least one language is required")
	}

	// Default pattern
	pattern := args.Pattern
	if pattern == "" {
		pattern = "subpage"
	}
	if pattern != "subpage" && pattern != "suffix" && pattern != "prefix" {
		return CheckTranslationsResult{}, fmt.Errorf("invalid pattern: %s (use 'subpage', 'suffix', or 'prefix')", pattern)
	}

	// Get base pages to check
	var basePages []string
	limit := normalizeLimit(args.Limit, 20, 100)

	if len(args.BasePages) > 0 {
		basePages = args.BasePages
		if len(basePages) > limit {
			basePages = basePages[:limit]
		}
	} else if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return CheckTranslationsResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			basePages = append(basePages, p.Title)
		}
	} else {
		return CheckTranslationsResult{}, fmt.Errorf("either 'base_pages' or 'category' must be specified")
	}

	result := CheckTranslationsResult{
		LanguagesChecked: args.Languages,
		Pattern:          pattern,
		Pages:            make([]PageTranslationResult, 0, len(basePages)),
	}

	// Check each base page for all languages
	for _, basePage := range basePages {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		pageResult := PageTranslationResult{
			BasePage:     basePage,
			Translations: make(map[string]TranslationStatus),
			Complete:     true,
		}

		for _, lang := range args.Languages {
			// Build page title based on pattern
			var langPage string
			switch pattern {
			case "subpage":
				langPage = fmt.Sprintf("%s/%s", basePage, lang)
			case "suffix":
				langPage = fmt.Sprintf("%s (%s)", basePage, lang)
			case "prefix":
				langPage = fmt.Sprintf("%s:%s", lang, basePage)
			}

			// Check if page exists
			info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: langPage})
			status := TranslationStatus{
				PageTitle: langPage,
			}

			if err == nil && info.Exists {
				status.Exists = true
				status.PageID = info.PageID
				status.Length = info.Length
			} else {
				status.Exists = false
				pageResult.MissingLangs = append(pageResult.MissingLangs, lang)
				pageResult.Complete = false
				result.MissingCount++
			}

			pageResult.Translations[lang] = status
		}

		result.Pages = append(result.Pages, pageResult)
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// FindBrokenInternalLinks finds internal wiki links that point to non-existent pages
// Optimized to use batch page existence checks instead of individual API calls
func (c *Client) FindBrokenInternalLinks(ctx context.Context, args FindBrokenInternalLinksArgs) (FindBrokenInternalLinksResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindBrokenInternalLinksResult{}, err
	}

	// Get pages to check
	var pagesToCheck []string
	limit := normalizeLimit(args.Limit, 20, 100)

	if len(args.Pages) > 0 {
		pagesToCheck = args.Pages
		if len(pagesToCheck) > limit {
			pagesToCheck = pagesToCheck[:limit]
		}
	} else if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return FindBrokenInternalLinksResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			pagesToCheck = append(pagesToCheck, p.Title)
		}
	} else {
		return FindBrokenInternalLinksResult{}, fmt.Errorf("either 'pages' or 'category' must be specified")
	}

	result := FindBrokenInternalLinksResult{
		Pages: make([]PageBrokenLinksResult, 0, len(pagesToCheck)),
	}

	// Regex to match internal wiki links: [[Target]] or [[Target|Display]]
	linkRegex := regexp.MustCompile(`\[\[([^\]|#]+)(?:[|#][^\]]*)?]]`)

	// First pass: collect all unique link targets from all pages
	type linkLocation struct {
		pageTitle string
		target    string
		line      int
		context   string
	}
	var allLinkLocations []linkLocation
	allTargets := make(map[string]bool)
	pageContents := make(map[string]string)

	for _, pageTitle := range pagesToCheck {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		page, err := c.GetPage(ctx, GetPageArgs{Title: pageTitle, Format: "wikitext"})
		if err != nil {
			result.Pages = append(result.Pages, PageBrokenLinksResult{
				Title:       pageTitle,
				BrokenLinks: make([]BrokenLink, 0),
				Error:       err.Error(),
			})
			continue
		}
		pageContents[pageTitle] = page.Content

		lines := strings.Split(page.Content, "\n")
		for lineNum, line := range lines {
			matches := linkRegex.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) < 2 {
					continue
				}

				target := strings.TrimSpace(match[1])

				// Skip special prefixes (categories, files, interwiki)
				lowerTarget := strings.ToLower(target)
				if strings.HasPrefix(lowerTarget, "category:") ||
					strings.HasPrefix(lowerTarget, "file:") ||
					strings.HasPrefix(lowerTarget, "image:") ||
					strings.HasPrefix(lowerTarget, ":") ||
					strings.HasPrefix(lowerTarget, "http") {
					continue
				}

				allTargets[target] = true
				allLinkLocations = append(allLinkLocations, linkLocation{
					pageTitle: pageTitle,
					target:    target,
					line:      lineNum + 1,
					context:   extractContext(line, strings.Index(line, match[0]), strings.Index(line, match[0])+len(match[0]), 30),
				})
			}
		}
	}

	// Second pass: batch check all targets at once
	targetList := make([]string, 0, len(allTargets))
	for target := range allTargets {
		targetList = append(targetList, target)
	}

	existenceMap, err := c.checkPagesExist(ctx, targetList)
	if err != nil {
		return FindBrokenInternalLinksResult{}, fmt.Errorf("failed to check page existence: %w", err)
	}

	// Third pass: build results using the existence map
	pageResults := make(map[string]*PageBrokenLinksResult)
	for _, pageTitle := range pagesToCheck {
		if _, hasContent := pageContents[pageTitle]; hasContent {
			pageResults[pageTitle] = &PageBrokenLinksResult{
				Title:       pageTitle,
				BrokenLinks: make([]BrokenLink, 0),
			}
		}
	}

	seenLinks := make(map[string]map[string]bool) // page -> target -> seen
	for _, loc := range allLinkLocations {
		if pageResults[loc.pageTitle] == nil {
			continue
		}

		// Deduplicate links within the same page
		if seenLinks[loc.pageTitle] == nil {
			seenLinks[loc.pageTitle] = make(map[string]bool)
		}
		if seenLinks[loc.pageTitle][loc.target] {
			continue
		}
		seenLinks[loc.pageTitle][loc.target] = true

		// Check if the target exists
		if exists, ok := existenceMap[loc.target]; !ok || !exists {
			pageResults[loc.pageTitle].BrokenLinks = append(pageResults[loc.pageTitle].BrokenLinks, BrokenLink{
				Target:  loc.target,
				Line:    loc.line,
				Context: loc.context,
			})
		}
	}

	// Finalize results
	for _, pageTitle := range pagesToCheck {
		if pr, ok := pageResults[pageTitle]; ok {
			pr.BrokenCount = len(pr.BrokenLinks)
			result.BrokenCount += pr.BrokenCount
			result.Pages = append(result.Pages, *pr)
		}
	}

	// Add pages that had errors (already added above)
	for _, pr := range result.Pages {
		if pr.Error != "" {
			continue // Already counted
		}
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// FindOrphanedPages finds pages that have no incoming links from other pages
func (c *Client) FindOrphanedPages(ctx context.Context, args FindOrphanedPagesArgs) (FindOrphanedPagesResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindOrphanedPagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, 200)

	// Use the API's lonelypages query (pages with no links to them)
	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "querypage")
	params.Set("qppage", "Lonelypages")
	params.Set("qplimit", strconv.Itoa(limit))

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return FindOrphanedPagesResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return FindOrphanedPagesResult{}, fmt.Errorf("unexpected response format")
	}

	querypage, ok := query["querypage"].(map[string]interface{})
	if !ok {
		return FindOrphanedPagesResult{}, fmt.Errorf("querypage not found in response")
	}

	results, ok := querypage["results"].([]interface{})
	if !ok {
		return FindOrphanedPagesResult{}, fmt.Errorf("results not found in querypage")
	}

	// Collect filtered titles
	var filteredTitles []string
	for _, r := range results {
		page := r.(map[string]interface{})

		// Filter by namespace if specified
		ns := getInt(page["ns"])
		if args.Namespace >= 0 && ns != args.Namespace {
			continue
		}

		title := getString(page["title"])

		// Filter by prefix if specified
		if args.Prefix != "" && !strings.HasPrefix(title, args.Prefix) {
			continue
		}

		filteredTitles = append(filteredTitles, title)
	}

	// Fetch actual page info (pageid, length, touched) for the filtered pages
	orphaned := make([]OrphanedPage, 0, len(filteredTitles))
	if len(filteredTitles) > 0 {
		// Batch fetch page info (up to 50 at a time)
		for i := 0; i < len(filteredTitles); i += 50 {
			end := i + 50
			if end > len(filteredTitles) {
				end = len(filteredTitles)
			}
			batch := filteredTitles[i:end]

			infoParams := url.Values{}
			infoParams.Set("action", "query")
			infoParams.Set("titles", strings.Join(batch, "|"))
			infoParams.Set("prop", "info")

			infoResp, err := c.apiRequest(ctx, infoParams)
			if err != nil {
				// Fall back to basic info if fetch fails
				for _, title := range batch {
					orphaned = append(orphaned, OrphanedPage{Title: title})
				}
				continue
			}

			infoQuery, _ := infoResp["query"].(map[string]interface{})
			pages, _ := infoQuery["pages"].(map[string]interface{})

			for _, pageData := range pages {
				p := pageData.(map[string]interface{})
				orphaned = append(orphaned, OrphanedPage{
					Title:      getString(p["title"]),
					PageID:     getInt(p["pageid"]),
					Length:     getInt(p["length"]),
					LastEdited: getString(p["touched"]),
				})
			}
		}
	}

	return FindOrphanedPagesResult{
		OrphanedPages: orphaned,
		TotalChecked:  len(results),
		OrphanedCount: len(orphaned),
	}, nil
}

// GetBacklinks returns pages that link to the specified page ("What links here")
func (c *Client) GetBacklinks(ctx context.Context, args GetBacklinksArgs) (GetBacklinksResult, error) {
	if args.Title == "" {
		return GetBacklinksResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetBacklinksResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "backlinks")
	params.Set("bltitle", args.Title)
	params.Set("bllimit", strconv.Itoa(limit))

	if args.Namespace >= 0 {
		params.Set("blnamespace", strconv.Itoa(args.Namespace))
	}

	if !args.Redirect {
		params.Set("blfilterredir", "nonredirects")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetBacklinksResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return GetBacklinksResult{}, fmt.Errorf("unexpected response format")
	}

	backlinks, ok := query["backlinks"].([]interface{})
	if !ok {
		return GetBacklinksResult{Title: args.Title, Backlinks: make([]BacklinkInfo, 0)}, nil
	}

	result := GetBacklinksResult{
		Title:     args.Title,
		Backlinks: make([]BacklinkInfo, 0, len(backlinks)),
	}

	for _, bl := range backlinks {
		link := bl.(map[string]interface{})
		info := BacklinkInfo{
			PageID:    getInt(link["pageid"]),
			Title:     getString(link["title"]),
			Namespace: getInt(link["ns"]),
		}
		if _, isRedirect := link["redirect"]; isRedirect {
			info.IsRedirect = true
		}
		result.Backlinks = append(result.Backlinks, info)
	}

	result.Count = len(result.Backlinks)

	// Check for continuation
	if _, ok := resp["continue"]; ok {
		result.HasMore = true
	}

	return result, nil
}

// GetRevisions returns the revision history for a page
func (c *Client) GetRevisions(ctx context.Context, args GetRevisionsArgs) (GetRevisionsResult, error) {
	if args.Title == "" {
		return GetRevisionsResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetRevisionsResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 100)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", args.Title)
	params.Set("prop", "revisions")
	params.Set("rvprop", "ids|timestamp|user|size|comment|flags")
	params.Set("rvlimit", strconv.Itoa(limit))

	if args.Start != "" {
		params.Set("rvstart", args.Start)
	}
	if args.End != "" {
		params.Set("rvend", args.End)
	}
	if args.User != "" {
		params.Set("rvuser", args.User)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetRevisionsResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return GetRevisionsResult{}, fmt.Errorf("unexpected response format")
	}

	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return GetRevisionsResult{}, fmt.Errorf("pages not found in response")
	}

	result := GetRevisionsResult{
		Title:     args.Title,
		Revisions: make([]RevisionInfo, 0),
	}

	for pageIDStr, pageData := range pages {
		pageID, _ := strconv.Atoi(pageIDStr)
		if pageID < 0 {
			return GetRevisionsResult{}, fmt.Errorf("page '%s' not found", args.Title)
		}

		page := pageData.(map[string]interface{})
		result.PageID = pageID
		result.Title = getString(page["title"])

		revisions, ok := page["revisions"].([]interface{})
		if !ok {
			return result, nil
		}

		var prevSize int
		for i, rev := range revisions {
			r := rev.(map[string]interface{})
			info := RevisionInfo{
				RevID:     getInt(r["revid"]),
				ParentID:  getInt(r["parentid"]),
				User:      getString(r["user"]),
				Timestamp: getString(r["timestamp"]),
				Size:      getInt(r["size"]),
				Comment:   getString(r["comment"]),
			}

			if _, isMinor := r["minor"]; isMinor {
				info.Minor = true
			}

			// Calculate size diff
			if i == 0 {
				prevSize = info.Size
			} else {
				info.SizeDiff = info.Size - prevSize
				prevSize = info.Size
			}

			result.Revisions = append(result.Revisions, info)
		}

		break // Only process first page
	}

	result.Count = len(result.Revisions)

	// Check for continuation
	if _, ok := resp["continue"]; ok {
		result.HasMore = true
	}

	return result, nil
}

// CompareRevisions compares two revisions and returns the diff
func (c *Client) CompareRevisions(ctx context.Context, args CompareRevisionsArgs) (CompareRevisionsResult, error) {
	if args.FromRev == 0 && args.FromTitle == "" {
		return CompareRevisionsResult{}, fmt.Errorf("either from_rev or from_title is required")
	}
	if args.ToRev == 0 && args.ToTitle == "" {
		return CompareRevisionsResult{}, fmt.Errorf("either to_rev or to_title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CompareRevisionsResult{}, err
	}

	params := url.Values{}
	params.Set("action", "compare")

	if args.FromRev > 0 {
		params.Set("fromrev", strconv.Itoa(args.FromRev))
	} else {
		params.Set("fromtitle", args.FromTitle)
	}

	if args.ToRev > 0 {
		params.Set("torev", strconv.Itoa(args.ToRev))
	} else {
		params.Set("totitle", args.ToTitle)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return CompareRevisionsResult{}, err
	}

	compare, ok := resp["compare"].(map[string]interface{})
	if !ok {
		return CompareRevisionsResult{}, fmt.Errorf("compare not found in response")
	}

	result := CompareRevisionsResult{
		FromTitle:     getString(compare["fromtitle"]),
		FromRevID:     getInt(compare["fromrevid"]),
		ToTitle:       getString(compare["totitle"]),
		ToRevID:       getInt(compare["torevid"]),
		Diff:          getString(compare["*"]),
		FromUser:      getString(compare["fromuser"]),
		ToUser:        getString(compare["touser"]),
		FromTimestamp: getString(compare["fromtimestamp"]),
		ToTimestamp:   getString(compare["totimestamp"]),
	}

	// Clean up the diff HTML for readability
	if result.Diff != "" {
		result.Diff = sanitizeHTML(result.Diff)
	}

	return result, nil
}

// GetUserContributions returns the contributions (edits) made by a user
func (c *Client) GetUserContributions(ctx context.Context, args GetUserContributionsArgs) (GetUserContributionsResult, error) {
	if args.User == "" {
		return GetUserContributionsResult{}, fmt.Errorf("user is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetUserContributionsResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "usercontribs")
	params.Set("ucuser", args.User)
	params.Set("ucprop", "ids|title|timestamp|comment|size|sizediff|flags")
	params.Set("uclimit", strconv.Itoa(limit))

	if args.Namespace >= 0 {
		params.Set("ucnamespace", strconv.Itoa(args.Namespace))
	}
	if args.Start != "" {
		params.Set("ucstart", args.Start)
	}
	if args.End != "" {
		params.Set("ucend", args.End)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetUserContributionsResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return GetUserContributionsResult{}, fmt.Errorf("unexpected response format")
	}

	contribs, ok := query["usercontribs"].([]interface{})
	if !ok {
		return GetUserContributionsResult{User: args.User, Contributions: make([]UserContribution, 0)}, nil
	}

	result := GetUserContributionsResult{
		User:          args.User,
		Contributions: make([]UserContribution, 0, len(contribs)),
	}

	for _, c := range contribs {
		contrib := c.(map[string]interface{})
		uc := UserContribution{
			PageID:    getInt(contrib["pageid"]),
			Title:     getString(contrib["title"]),
			Namespace: getInt(contrib["ns"]),
			RevID:     getInt(contrib["revid"]),
			ParentID:  getInt(contrib["parentid"]),
			Timestamp: getString(contrib["timestamp"]),
			Comment:   getString(contrib["comment"]),
			Size:      getInt(contrib["size"]),
			SizeDiff:  getInt(contrib["sizediff"]),
		}

		if _, isMinor := contrib["minor"]; isMinor {
			uc.Minor = true
		}
		if _, isNew := contrib["new"]; isNew {
			uc.New = true
		}

		result.Contributions = append(result.Contributions, uc)
	}

	result.Count = len(result.Contributions)

	// Check for continuation
	if _, ok := resp["continue"]; ok {
		result.HasMore = true
	}

	return result, nil
}

// ========== Simple Edit Tools ==========

// FindReplace finds and replaces text in a wiki page
func (c *Client) FindReplace(ctx context.Context, args FindReplaceArgs) (FindReplaceResult, error) {
	if args.Title == "" {
		return FindReplaceResult{}, fmt.Errorf("title is required")
	}
	if args.Find == "" {
		return FindReplaceResult{}, fmt.Errorf("find text is required")
	}

	// Get current page content
	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return FindReplaceResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	result := FindReplaceResult{
		Title:   page.Title,
		Preview: args.Preview,
	}

	// Build regex pattern
	var re *regexp.Regexp
	if args.UseRegex {
		re, err = regexp.Compile(args.Find)
		if err != nil {
			return FindReplaceResult{}, fmt.Errorf("invalid regex pattern: %w", err)
		}
	} else {
		re = regexp.MustCompile(regexp.QuoteMeta(args.Find))
	}

	// Find all matches with line context
	lines := strings.Split(page.Content, "\n")
	var changes []TextChange
	newLines := make([]string, len(lines))
	copy(newLines, lines)

	for lineNum, line := range lines {
		matches := re.FindAllStringIndex(line, -1)
		if len(matches) == 0 {
			continue
		}

		result.MatchCount += len(matches)

		// Apply replacements
		if args.All {
			newLine := re.ReplaceAllString(line, args.Replace)
			if newLine != line {
				changes = append(changes, TextChange{
					Line:    lineNum + 1,
					Before:  line,
					After:   newLine,
					Context: extractContext(line, matches[0][0], matches[0][1], 40),
				})
				newLines[lineNum] = newLine
				result.ReplaceCount += len(matches)
			}
		} else if result.ReplaceCount == 0 {
			// Replace first occurrence only
			newLine := re.ReplaceAllStringFunc(line, func(match string) string {
				if result.ReplaceCount == 0 {
					result.ReplaceCount++
					return args.Replace
				}
				return match
			})
			if newLine != line {
				changes = append(changes, TextChange{
					Line:    lineNum + 1,
					Before:  line,
					After:   newLine,
					Context: extractContext(line, matches[0][0], matches[0][1], 40),
				})
				newLines[lineNum] = newLine
			}
		}
	}

	result.Changes = changes

	if result.MatchCount == 0 {
		result.Message = fmt.Sprintf("No matches found for '%s'", args.Find)
		return result, nil
	}

	if args.Preview {
		result.Success = true
		result.Message = fmt.Sprintf("Preview: %d matches found, %d would be replaced", result.MatchCount, result.ReplaceCount)
		return result, nil
	}

	// Capture old revision before edit
	oldRevision := page.Revision

	// Apply the edit
	newContent := strings.Join(newLines, "\n")
	summary := args.Summary
	if summary == "" {
		summary = fmt.Sprintf("Replaced '%s' with '%s'", truncateString(args.Find, 30), truncateString(args.Replace, 30))
	}

	editResult, err := c.EditPage(ctx, EditPageArgs{
		Title:   page.Title,
		Content: newContent,
		Summary: summary,
		Minor:   args.Minor,
	})
	if err != nil {
		return FindReplaceResult{}, fmt.Errorf("failed to save changes: %w", err)
	}

	result.Success = editResult.Success
	result.RevisionID = editResult.RevisionID
	result.Message = fmt.Sprintf("Replaced %d occurrence(s)", result.ReplaceCount)

	// Add revision info and undo instructions
	result.Revision, result.Undo = c.buildEditRevisionInfo(page.Title, oldRevision, editResult.RevisionID)

	return result, nil
}

// ApplyFormatting applies formatting to text in a wiki page
func (c *Client) ApplyFormatting(ctx context.Context, args ApplyFormattingArgs) (ApplyFormattingResult, error) {
	if args.Title == "" {
		return ApplyFormattingResult{}, fmt.Errorf("title is required")
	}
	if args.Text == "" {
		return ApplyFormattingResult{}, fmt.Errorf("text is required")
	}
	if args.Format == "" {
		return ApplyFormattingResult{}, fmt.Errorf("format is required")
	}

	// Map format to wikitext markup
	formatMap := map[string][2]string{
		"strikethrough": {"<s>", "</s>"},
		"strike":        {"<s>", "</s>"},
		"bold":          {"'''", "'''"},
		"italic":        {"''", "''"},
		"underline":     {"<u>", "</u>"},
		"code":          {"<code>", "</code>"},
		"nowiki":        {"<nowiki>", "</nowiki>"},
	}

	markup, ok := formatMap[strings.ToLower(args.Format)]
	if !ok {
		return ApplyFormattingResult{}, fmt.Errorf("unknown format: %s (use: strikethrough, bold, italic, underline, code, nowiki)", args.Format)
	}

	// Use FindReplace to apply formatting
	replacement := markup[0] + args.Text + markup[1]

	findArgs := FindReplaceArgs{
		Title:   args.Title,
		Find:    args.Text,
		Replace: replacement,
		All:     args.All,
		Preview: args.Preview,
		Minor:   true,
	}

	if args.Summary != "" {
		findArgs.Summary = args.Summary
	} else {
		findArgs.Summary = fmt.Sprintf("Applied %s formatting to '%s'", args.Format, truncateString(args.Text, 30))
	}

	frResult, err := c.FindReplace(ctx, findArgs)
	if err != nil {
		return ApplyFormattingResult{}, err
	}

	return ApplyFormattingResult{
		Success:     frResult.Success,
		Title:       frResult.Title,
		Format:      args.Format,
		MatchCount:  frResult.MatchCount,
		FormatCount: frResult.ReplaceCount,
		Preview:     args.Preview,
		Changes:     frResult.Changes,
		RevisionID:  frResult.RevisionID,
		Revision:    frResult.Revision,
		Undo:        frResult.Undo,
		Message:     frResult.Message,
	}, nil
}

// BulkReplace performs find/replace across multiple pages
func (c *Client) BulkReplace(ctx context.Context, args BulkReplaceArgs) (BulkReplaceResult, error) {
	if args.Find == "" {
		return BulkReplaceResult{}, fmt.Errorf("find text is required")
	}

	// Get pages to process
	var pagesToProcess []string
	limit := normalizeLimit(args.Limit, 10, 50)

	if len(args.Pages) > 0 {
		pagesToProcess = args.Pages
		if len(pagesToProcess) > limit {
			pagesToProcess = pagesToProcess[:limit]
		}
	} else if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return BulkReplaceResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			pagesToProcess = append(pagesToProcess, p.Title)
		}
	} else {
		return BulkReplaceResult{}, fmt.Errorf("either 'pages' or 'category' must be specified")
	}

	result := BulkReplaceResult{
		Preview: args.Preview,
		Results: make([]PageReplaceResult, 0, len(pagesToProcess)),
	}

	summary := args.Summary
	if summary == "" {
		summary = fmt.Sprintf("Bulk replace: '%s' → '%s'", truncateString(args.Find, 20), truncateString(args.Replace, 20))
	}

	for _, pageTitle := range pagesToProcess {
		pageResult := PageReplaceResult{Title: pageTitle}

		frResult, err := c.FindReplace(ctx, FindReplaceArgs{
			Title:    pageTitle,
			Find:     args.Find,
			Replace:  args.Replace,
			UseRegex: args.UseRegex,
			All:      true,
			Preview:  args.Preview,
			Summary:  summary,
		})

		if err != nil {
			pageResult.Error = err.Error()
		} else {
			pageResult.MatchCount = frResult.MatchCount
			pageResult.ReplaceCount = frResult.ReplaceCount
			pageResult.RevisionID = frResult.RevisionID
			pageResult.Revision = frResult.Revision
			pageResult.Undo = frResult.Undo
			if args.Preview {
				pageResult.Changes = frResult.Changes
			}

			if frResult.ReplaceCount > 0 {
				result.PagesModified++
				result.TotalChanges += frResult.ReplaceCount
			}
		}

		result.Results = append(result.Results, pageResult)
	}

	result.PagesProcessed = len(result.Results)

	if args.Preview {
		result.Message = fmt.Sprintf("Preview: %d pages would be modified with %d total changes", result.PagesModified, result.TotalChanges)
	} else {
		result.Message = fmt.Sprintf("Modified %d pages with %d total changes", result.PagesModified, result.TotalChanges)
	}

	return result, nil
}

// SearchInPage searches for text within a specific page
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

// ResolveTitle tries to find the correct page title with fuzzy matching
func (c *Client) ResolveTitle(ctx context.Context, args ResolveTitleArgs) (ResolveTitleResult, error) {
	if args.Title == "" {
		return ResolveTitleResult{}, fmt.Errorf("title is required")
	}

	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	result := ResolveTitleResult{
		Suggestions: make([]TitleSuggestion, 0),
	}

	// First try exact match with normalization
	normalizedTitle := normalizePageTitle(args.Title)
	info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: normalizedTitle})
	if err == nil && info.Exists {
		result.ExactMatch = true
		result.ResolvedTitle = info.Title
		result.PageID = info.PageID
		result.Message = "Exact match found"
		return result, nil
	}

	// Try case-insensitive search
	searchResult, err := c.Search(ctx, SearchArgs{
		Query: args.Title,
		Limit: maxResults * 2, // Get more to filter
	})
	if err != nil {
		return ResolveTitleResult{}, fmt.Errorf("search failed: %w", err)
	}

	// Calculate similarity and rank results
	titleLower := strings.ToLower(args.Title)
	for _, hit := range searchResult.Results {
		hitLower := strings.ToLower(hit.Title)

		// Calculate simple similarity score
		similarity := calculateSimilarity(titleLower, hitLower)

		result.Suggestions = append(result.Suggestions, TitleSuggestion{
			Title:      hit.Title,
			PageID:     hit.PageID,
			Similarity: similarity,
		})
	}

	// Sort by similarity (descending)
	for i := 0; i < len(result.Suggestions)-1; i++ {
		for j := i + 1; j < len(result.Suggestions); j++ {
			if result.Suggestions[j].Similarity > result.Suggestions[i].Similarity {
				result.Suggestions[i], result.Suggestions[j] = result.Suggestions[j], result.Suggestions[i]
			}
		}
	}

	// Limit results
	if len(result.Suggestions) > maxResults {
		result.Suggestions = result.Suggestions[:maxResults]
	}

	if len(result.Suggestions) > 0 && result.Suggestions[0].Similarity > 0.8 {
		result.ResolvedTitle = result.Suggestions[0].Title
		result.PageID = result.Suggestions[0].PageID
		result.Message = fmt.Sprintf("Did you mean '%s'?", result.Suggestions[0].Title)
	} else if len(result.Suggestions) > 0 {
		result.Message = fmt.Sprintf("Page '%s' not found. Similar pages found.", args.Title)
	} else {
		result.Message = fmt.Sprintf("Page '%s' not found. No similar pages.", args.Title)
	}

	return result, nil
}

// Helper function to truncate string for display
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// buildEditRevisionInfo creates revision info and undo instructions for an edit
func (c *Client) buildEditRevisionInfo(title string, oldRevision, newRevision int) (*EditRevisionInfo, *UndoInfo) {
	if oldRevision == 0 || newRevision == 0 {
		return nil, nil
	}

	// Derive wiki base URL from API URL (replace api.php with index.php)
	wikiBaseURL := strings.TrimSuffix(c.config.BaseURL, "api.php") + "index.php"

	// Build diff URL
	diffURL := fmt.Sprintf("%s?diff=%d&oldid=%d", wikiBaseURL, newRevision, oldRevision)

	// Build undo URL
	encodedTitle := url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
	undoURL := fmt.Sprintf("%s?title=%s&action=edit&undoafter=%d&undo=%d", wikiBaseURL, encodedTitle, oldRevision, newRevision)

	// Build undo instruction
	undoInstruction := fmt.Sprintf("To undo: use wiki URL or revert to revision %d", oldRevision)

	return &EditRevisionInfo{
			OldRevision: int64(oldRevision),
			NewRevision: int64(newRevision),
			DiffURL:     diffURL,
		}, &UndoInfo{
			Instruction: undoInstruction,
			WikiURL:     undoURL,
		}
}

// Helper function to calculate string similarity (Jaccard-like)
func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// Split into words
	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	// Count common words
	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}

	common := 0
	for _, w := range words2 {
		if set1[w] {
			common++
		}
	}

	// Jaccard similarity
	union := len(words1) + len(words2) - common
	if union == 0 {
		return 0.0
	}

	return float64(common) / float64(union)
}

// checkPagesExist checks if multiple pages exist using MediaWiki's multi-value API
// Returns a map of page title -> exists (bool)
// This is much more efficient than calling GetPageInfo for each page individually
func (c *Client) checkPagesExist(ctx context.Context, titles []string) (map[string]bool, error) {
	if len(titles) == 0 {
		return make(map[string]bool), nil
	}

	result := make(map[string]bool, len(titles))

	// MediaWiki API supports up to 50 titles per request
	const maxTitlesPerRequest = 50

	for i := 0; i < len(titles); i += maxTitlesPerRequest {
		end := i + maxTitlesPerRequest
		if end > len(titles) {
			end = len(titles)
		}
		batch := titles[i:end]

		// Join titles with pipe separator for MediaWiki API
		params := url.Values{}
		params.Set("action", "query")
		params.Set("titles", strings.Join(batch, "|"))

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			return nil, err
		}

		query, ok := resp["query"].(map[string]interface{})
		if !ok {
			continue
		}

		pages, ok := query["pages"].(map[string]interface{})
		if !ok {
			continue
		}

		// Build a normalized title map (MediaWiki may normalize titles)
		normalized := make(map[string]string)
		if normList, ok := query["normalized"].([]interface{}); ok {
			for _, n := range normList {
				norm := n.(map[string]interface{})
				from := getString(norm["from"])
				to := getString(norm["to"])
				normalized[to] = from
			}
		}

		// Check each page in the response
		for _, pageData := range pages {
			page := pageData.(map[string]interface{})
			title := getString(page["title"])

			// Check if page exists (missing key indicates non-existence)
			_, missing := page["missing"]
			exists := !missing

			result[title] = exists

			// Also map the original (unnormalized) title if applicable
			if originalTitle, ok := normalized[title]; ok {
				result[originalTitle] = exists
			}
		}
	}

	// Set any titles we didn't get a response for as non-existent
	for _, title := range titles {
		if _, ok := result[title]; !ok {
			result[title] = false
		}
	}

	return result, nil
}

// ListUsers lists wiki users, optionally filtered by group
func (c *Client) ListUsers(ctx context.Context, args ListUsersArgs) (ListUsersResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ListUsersResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "allusers")
	params.Set("aulimit", strconv.Itoa(limit))
	params.Set("auprop", "groups|editcount|registration")

	if args.Group != "" {
		params.Set("augroup", args.Group)
	}

	if args.ActiveOnly {
		params.Set("auactiveusers", "1")
	}

	if args.ContinueFrom != "" {
		params.Set("aufrom", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ListUsersResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return ListUsersResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	allusers, ok := query["allusers"].([]interface{})
	if !ok {
		return ListUsersResult{}, fmt.Errorf("unexpected response format: missing allusers")
	}

	users := make([]UserInfo, 0, len(allusers))
	for _, u := range allusers {
		user := u.(map[string]interface{})

		userInfo := UserInfo{
			UserID:       getInt(user["userid"]),
			Name:         getString(user["name"]),
			EditCount:    getInt(user["editcount"]),
			Registration: getString(user["registration"]),
		}

		// Extract groups
		if groups, ok := user["groups"].([]interface{}); ok {
			for _, g := range groups {
				if groupName, ok := g.(string); ok {
					userInfo.Groups = append(userInfo.Groups, groupName)
				}
			}
		}

		users = append(users, userInfo)
	}

	result := ListUsersResult{
		Users:      users,
		TotalCount: len(users),
		Group:      args.Group,
	}

	// Check for continuation
	if cont, ok := resp["continue"].(map[string]interface{}); ok {
		if aufrom, ok := cont["aufrom"].(string); ok {
			result.HasMore = true
			result.ContinueFrom = aufrom
		}
	}

	return result, nil
}

// GetSections retrieves section structure and optionally section content from a page
func (c *Client) GetSections(ctx context.Context, args GetSectionsArgs) (GetSectionsResult, error) {
	if args.Title == "" {
		return GetSectionsResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetSectionsResult{}, err
	}

	normalizedTitle := normalizePageTitle(args.Title)

	// If section number is specified, retrieve that section's content
	if args.Section > 0 || (args.Section == 0 && args.Format != "") {
		return c.getSectionContent(ctx, normalizedTitle, args.Section, args.Format)
	}

	// Otherwise, list all sections
	cacheKey := fmt.Sprintf("sections:%s", normalizedTitle)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(GetSectionsResult), nil
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", normalizedTitle)
	params.Set("prop", "sections")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetSectionsResult{}, err
	}

	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return GetSectionsResult{}, fmt.Errorf("%s", errInfo["info"])
	}

	parse := resp["parse"].(map[string]interface{})
	pageID := int(parse["pageid"].(float64))
	title := parse["title"].(string)

	sectionsRaw := parse["sections"].([]interface{})
	sections := make([]SectionInfo, 0, len(sectionsRaw))

	for _, s := range sectionsRaw {
		sec := s.(map[string]interface{})
		index, _ := strconv.Atoi(sec["index"].(string))
		level, _ := strconv.Atoi(sec["level"].(string))
		lineNum := 0
		if line, ok := sec["line"].(float64); ok {
			lineNum = int(line)
		}

		sections = append(sections, SectionInfo{
			Index:   index,
			Level:   level,
			Title:   stripHTMLTags(sec["line"].(string)),
			Anchor:  sec["anchor"].(string),
			LineNum: lineNum,
		})
	}

	result := GetSectionsResult{
		Title:    title,
		PageID:   pageID,
		Sections: sections,
		Message:  fmt.Sprintf("Found %d sections. Use section parameter to get specific section content.", len(sections)),
	}

	c.setCache(cacheKey, result, "page_content")
	return result, nil
}

// getSectionContent retrieves the content of a specific section
func (c *Client) getSectionContent(ctx context.Context, title string, section int, format string) (GetSectionsResult, error) {
	if format == "" {
		format = "wikitext"
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", title)
	params.Set("section", strconv.Itoa(section))

	if format == "html" {
		params.Set("prop", "text|displaytitle")
	} else {
		params.Set("prop", "wikitext|displaytitle")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetSectionsResult{}, err
	}

	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return GetSectionsResult{}, fmt.Errorf("%s", errInfo["info"])
	}

	parse := resp["parse"].(map[string]interface{})
	pageID := int(parse["pageid"].(float64))
	pageTitle := parse["title"].(string)

	var content string
	var sectionTitle string

	if format == "html" {
		if text, ok := parse["text"].(map[string]interface{}); ok {
			content = text["*"].(string)
		}
	} else {
		if wikitext, ok := parse["wikitext"].(map[string]interface{}); ok {
			content = wikitext["*"].(string)
		}
	}

	// Extract section title from content if it starts with ==
	if format == "wikitext" && strings.HasPrefix(strings.TrimSpace(content), "==") {
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) > 0 {
			sectionTitle = strings.Trim(lines[0], "= \t")
		}
	}

	return GetSectionsResult{
		Title:          pageTitle,
		PageID:         pageID,
		SectionContent: content,
		SectionTitle:   sectionTitle,
		Format:         format,
	}, nil
}

// GetRelated finds pages related to the given page
func (c *Client) GetRelated(ctx context.Context, args GetRelatedArgs) (GetRelatedResult, error) {
	if args.Title == "" {
		return GetRelatedResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetRelatedResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 50)
	method := args.Method
	if method == "" {
		method = "categories"
	}

	normalizedTitle := normalizePageTitle(args.Title)
	result := GetRelatedResult{
		Title:  normalizedTitle,
		Method: method,
	}

	relatedMap := make(map[string]*RelatedPage)

	// Get categories for the source page
	if method == "categories" || method == "all" {
		cats, err := c.getPageCategories(ctx, normalizedTitle)
		if err == nil {
			result.Categories = cats

			// Get pages from each category
			for _, cat := range cats {
				if len(relatedMap) >= limit {
					break
				}
				members, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
					Category: cat,
					Limit:    limit,
					Type:     "page",
				})
				if err == nil {
					for _, m := range members.Members {
						if m.Title == normalizedTitle {
							continue
						}
						if existing, ok := relatedMap[m.Title]; ok {
							existing.Categories = append(existing.Categories, cat)
							existing.Score++
						} else {
							relatedMap[m.Title] = &RelatedPage{
								Title:      m.Title,
								PageID:     m.PageID,
								Relation:   "same_category",
								Categories: []string{cat},
								Score:      1,
							}
						}
					}
				}
			}
		}
	}

	// Get pages linked from this page
	if method == "links" || method == "all" {
		links, err := c.getPageLinks(ctx, normalizedTitle, limit)
		if err == nil {
			for _, link := range links {
				if existing, ok := relatedMap[link.Title]; ok {
					existing.Relation = "linked_and_categorized"
					existing.Score += 2
				} else {
					relatedMap[link.Title] = &RelatedPage{
						Title:    link.Title,
						PageID:   link.PageID,
						Relation: "linked_from",
						Score:    2,
					}
				}
			}
		}
	}

	// Get pages that link to this page
	if method == "backlinks" || method == "all" {
		backlinks, err := c.GetBacklinks(ctx, GetBacklinksArgs{
			Title: normalizedTitle,
			Limit: limit,
		})
		if err == nil {
			for _, bl := range backlinks.Backlinks {
				if existing, ok := relatedMap[bl.Title]; ok {
					existing.Relation = "bidirectional_link"
					existing.Score += 3
				} else {
					relatedMap[bl.Title] = &RelatedPage{
						Title:    bl.Title,
						PageID:   bl.PageID,
						Relation: "links_to",
						Score:    1,
					}
				}
			}
		}
	}

	// Convert map to slice and sort by score
	related := make([]RelatedPage, 0, len(relatedMap))
	for _, rp := range relatedMap {
		related = append(related, *rp)
	}

	// Sort by score descending
	for i := 0; i < len(related)-1; i++ {
		for j := i + 1; j < len(related); j++ {
			if related[j].Score > related[i].Score {
				related[i], related[j] = related[j], related[i]
			}
		}
	}

	// Limit results
	if len(related) > limit {
		related = related[:limit]
	}

	result.RelatedPages = related
	result.Count = len(related)

	return result, nil
}

// getPageCategories gets categories for a page
func (c *Client) getPageCategories(ctx context.Context, title string) ([]string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "categories")
	params.Set("cllimit", "50")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	var categories []string
	for _, p := range pages {
		page := p.(map[string]interface{})
		if cats, ok := page["categories"].([]interface{}); ok {
			for _, cat := range cats {
				c := cat.(map[string]interface{})
				catTitle := c["title"].(string)
				// Remove "Category:" prefix
				catTitle = strings.TrimPrefix(catTitle, "Category:")
				categories = append(categories, catTitle)
			}
		}
	}

	return categories, nil
}

// getPageLinks gets outgoing links from a page
func (c *Client) getPageLinks(ctx context.Context, title string, limit int) ([]PageSummary, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "links")
	params.Set("pllimit", strconv.Itoa(limit))
	params.Set("plnamespace", "0") // Main namespace only

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	var links []PageSummary
	for _, p := range pages {
		page := p.(map[string]interface{})
		if linksList, ok := page["links"].([]interface{}); ok {
			for _, l := range linksList {
				link := l.(map[string]interface{})
				links = append(links, PageSummary{
					Title: link["title"].(string),
				})
			}
		}
	}

	return links, nil
}

// GetImages retrieves images/files used on a page
func (c *Client) GetImages(ctx context.Context, args GetImagesArgs) (GetImagesResult, error) {
	if args.Title == "" {
		return GetImagesResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetImagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)
	normalizedTitle := normalizePageTitle(args.Title)

	// First get list of images on the page
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", normalizedTitle)
	params.Set("prop", "images")
	params.Set("imlimit", strconv.Itoa(limit))

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetImagesResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	var imageTitles []string
	for _, p := range pages {
		page := p.(map[string]interface{})
		if images, ok := page["images"].([]interface{}); ok {
			for _, img := range images {
				i := img.(map[string]interface{})
				imageTitles = append(imageTitles, i["title"].(string))
			}
		}
	}

	if len(imageTitles) == 0 {
		return GetImagesResult{
			Title:  normalizedTitle,
			Images: []ImageInfo{},
			Count:  0,
		}, nil
	}

	// Get image info (URLs, dimensions) for each image
	images, err := c.getImageInfo(ctx, imageTitles)
	if err != nil {
		// Return basic info without URLs if imageinfo fails
		basicImages := make([]ImageInfo, 0, len(imageTitles))
		for _, t := range imageTitles {
			basicImages = append(basicImages, ImageInfo{Title: t})
		}
		return GetImagesResult{
			Title:  normalizedTitle,
			Images: basicImages,
			Count:  len(basicImages),
		}, nil
	}

	return GetImagesResult{
		Title:  normalizedTitle,
		Images: images,
		Count:  len(images),
	}, nil
}

// getImageInfo retrieves detailed info for images
func (c *Client) getImageInfo(ctx context.Context, titles []string) ([]ImageInfo, error) {
	if len(titles) == 0 {
		return nil, nil
	}

	// Batch in groups of 50
	batchSize := 50
	var allImages []ImageInfo

	for i := 0; i < len(titles); i += batchSize {
		end := i + batchSize
		if end > len(titles) {
			end = len(titles)
		}
		batch := titles[i:end]

		params := url.Values{}
		params.Set("action", "query")
		params.Set("titles", strings.Join(batch, "|"))
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|size|mime")
		params.Set("iiurlwidth", "300") // Get thumbnail URL

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			continue
		}

		query := resp["query"].(map[string]interface{})
		pages := query["pages"].(map[string]interface{})

		for _, p := range pages {
			page := p.(map[string]interface{})
			title := getString(page["title"])

			imgInfo := ImageInfo{Title: title}

			if imageinfo, ok := page["imageinfo"].([]interface{}); ok && len(imageinfo) > 0 {
				info := imageinfo[0].(map[string]interface{})
				imgInfo.URL = getString(info["url"])
				imgInfo.ThumbURL = getString(info["thumburl"])
				imgInfo.Width = getInt(info["width"])
				imgInfo.Height = getInt(info["height"])
				imgInfo.Size = getInt(info["size"])
				imgInfo.MimeType = getString(info["mime"])
			}

			allImages = append(allImages, imgInfo)
		}
	}

	return allImages, nil
}

// UploadFile uploads a file to the wiki
func (c *Client) UploadFile(ctx context.Context, args UploadFileArgs) (UploadFileResult, error) {
	if args.Filename == "" {
		return UploadFileResult{}, fmt.Errorf("filename is required")
	}
	if args.FilePath == "" && args.FileURL == "" {
		return UploadFileResult{}, fmt.Errorf("either file_path or file_url is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return UploadFileResult{}, fmt.Errorf("authentication required for uploads: %w", err)
	}

	// Get CSRF token
	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return UploadFileResult{}, fmt.Errorf("failed to get edit token: %w", err)
	}

	// If URL provided, use URL upload
	if args.FileURL != "" {
		return c.uploadFromURL(ctx, args, token)
	}

	// For local file upload, we need multipart form
	return c.uploadFromFile(ctx, args, token)
}

// uploadFromURL uploads a file from a URL
func (c *Client) uploadFromURL(ctx context.Context, args UploadFileArgs, token string) (UploadFileResult, error) {
	params := url.Values{}
	params.Set("action", "upload")
	params.Set("filename", args.Filename)
	params.Set("url", args.FileURL)
	params.Set("token", token)

	if args.Text != "" {
		params.Set("text", args.Text)
	}
	if args.Comment != "" {
		params.Set("comment", args.Comment)
	}
	if args.IgnoreWarnings {
		params.Set("ignorewarnings", "1")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return UploadFileResult{}, err
	}

	return c.parseUploadResponse(resp, args.Filename)
}

// uploadFromFile uploads a local file using multipart form
func (c *Client) uploadFromFile(ctx context.Context, args UploadFileArgs, token string) (UploadFileResult, error) {
	// Read file
	fileData, err := c.readLocalFile(args.FilePath)
	if err != nil {
		return UploadFileResult{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Create multipart request
	boundary := "----WikiUploadBoundary" + strconv.FormatInt(time.Now().UnixNano(), 36)

	var body strings.Builder
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"action\"\r\n\r\n")
	body.WriteString("upload\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"format\"\r\n\r\n")
	body.WriteString("json\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"filename\"\r\n\r\n")
	body.WriteString(args.Filename + "\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"token\"\r\n\r\n")
	body.WriteString(token + "\r\n")

	if args.Text != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"text\"\r\n\r\n")
		body.WriteString(args.Text + "\r\n")
	}

	if args.Comment != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"comment\"\r\n\r\n")
		body.WriteString(args.Comment + "\r\n")
	}

	if args.IgnoreWarnings {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"ignorewarnings\"\r\n\r\n")
		body.WriteString("1\r\n")
	}

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", args.Filename))
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.Write(fileData)
	body.WriteString("\r\n")
	body.WriteString("--" + boundary + "--\r\n")

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, strings.NewReader(body.String()))
	if err != nil {
		return UploadFileResult{}, err
	}

	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	// Use HTTP client to make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return UploadFileResult{}, err
	}
	defer resp.Body.Close()

	// Parse JSON response
	var result map[string]interface{}
	if err := c.parseJSONResponse(resp, &result); err != nil {
		return UploadFileResult{}, err
	}

	return c.parseUploadResponse(result, args.Filename)
}

// readLocalFile reads a file from the local filesystem
func (c *Client) readLocalFile(path string) ([]byte, error) {
	// This is a placeholder - in a real implementation you'd read the file
	// For security, MCP servers typically don't have direct filesystem access
	// Instead, the file content should be passed directly or via URL
	return nil, fmt.Errorf("local file upload not supported - use file_url instead")
}

// parseUploadResponse parses the upload API response
func (c *Client) parseUploadResponse(resp map[string]interface{}, filename string) (UploadFileResult, error) {
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return UploadFileResult{
			Success:  false,
			Filename: filename,
			Message:  fmt.Sprintf("Upload failed: %s", errInfo["info"]),
		}, nil
	}

	upload, ok := resp["upload"].(map[string]interface{})
	if !ok {
		return UploadFileResult{
			Success:  false,
			Filename: filename,
			Message:  "Unexpected response format",
		}, nil
	}

	result := UploadFileResult{
		Filename: filename,
	}

	status := getString(upload["result"])
	switch status {
	case "Success":
		result.Success = true
		result.Message = "File uploaded successfully"
		if imageinfo, ok := upload["imageinfo"].(map[string]interface{}); ok {
			result.URL = getString(imageinfo["url"])
			result.Size = getInt(imageinfo["size"])
		}
	case "Warning":
		result.Success = false
		result.Message = "Upload has warnings - set ignore_warnings=true to proceed"
		if warnings, ok := upload["warnings"].(map[string]interface{}); ok {
			for k, v := range warnings {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", k, v))
			}
		}
	default:
		result.Success = false
		result.Message = fmt.Sprintf("Upload status: %s", status)
	}

	return result, nil
}

// parseJSONResponse parses a JSON response from http.Response
func (c *Client) parseJSONResponse(resp *http.Response, target interface{}) error {
	// Import would be needed: "encoding/json"
	// For now, return error indicating this needs implementation
	return fmt.Errorf("JSON parsing not implemented for multipart upload")
}

// SearchInFile searches for text within a wiki file (PDF, text, etc.)
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

// getFileURL retrieves the download URL and type for a wiki file
func (c *Client) getFileURL(ctx context.Context, filename string) (string, string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", filename)
	params.Set("prop", "imageinfo")
	params.Set("iiprop", "url|mime|size")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return "", "", err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("unexpected response format")
	}

	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("no pages in response")
	}

	for _, pageData := range pages {
		page := pageData.(map[string]interface{})

		// Check if file exists
		if _, missing := page["missing"]; missing {
			return "", "", fmt.Errorf("file '%s' does not exist", filename)
		}

		imageinfo, ok := page["imageinfo"].([]interface{})
		if !ok || len(imageinfo) == 0 {
			return "", "", fmt.Errorf("no file info available for '%s'", filename)
		}

		info := imageinfo[0].(map[string]interface{})
		fileURL := getString(info["url"])
		mimeType := getString(info["mime"])

		if fileURL == "" {
			return "", "", fmt.Errorf("no download URL for '%s'", filename)
		}

		// Extract file type from mime or filename
		fileType := mimeType
		if mimeType == "application/pdf" {
			fileType = "pdf"
		} else if strings.HasPrefix(mimeType, "text/") {
			fileType = strings.TrimPrefix(mimeType, "text/")
		}

		return fileURL, fileType, nil
	}

	return "", "", fmt.Errorf("file '%s' not found", filename)
}

// downloadFile downloads a file from the given URL
func (c *Client) downloadFile(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Limit download size to 50MB
	const maxSize = 50 * 1024 * 1024
	limitedReader := &io.LimitedReader{R: resp.Body, N: maxSize}

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file data: %w", err)
	}

	if limitedReader.N <= 0 {
		return nil, fmt.Errorf("file exceeds maximum size of 50MB")
	}

	return data, nil
}

// aggregateChanges groups recent changes by the specified field and returns counts
func aggregateChanges(changes []RecentChange, by string) *AggregatedChanges {
	counts := make(map[string]int)

	for _, change := range changes {
		var key string
		switch by {
		case "user":
			key = change.User
		case "page":
			key = change.Title
		case "type":
			key = change.Type
		default:
			return nil // Invalid aggregation type
		}
		counts[key]++
	}

	// Convert map to sorted slice (by count descending)
	items := make([]AggregateCount, 0, len(counts))
	for key, count := range counts {
		items = append(items, AggregateCount{Key: key, Count: count})
	}

	// Sort by count descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})

	return &AggregatedChanges{
		By:           by,
		TotalChanges: len(changes),
		Items:        items,
	}
}

// FindSimilarPages finds pages similar to the given page based on content similarity
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
		select {
		case <-ctx.Done():
			break
		default:
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
		select {
		case <-ctx.Done():
			break
		default:
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
