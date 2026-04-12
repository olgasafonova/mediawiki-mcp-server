package wiki

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// GetRecentChanges retrieves recent changes from the wiki
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

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return RecentChangesResult{}, fmt.Errorf("unexpected API response: missing 'query' object")
	}
	rcList, ok := query["recentchanges"].([]interface{})
	if !ok {
		return RecentChangesResult{}, fmt.Errorf("unexpected API response: missing 'recentchanges' list")
	}

	changes := make([]RecentChange, 0, len(rcList))
	for _, rc := range rcList {
		change, ok := rc.(map[string]interface{})
		if !ok {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, getString(change["timestamp"]))

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

// GetRevisions retrieves the revision history of a page
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

		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		result.PageID = pageID
		result.Title = getString(page["title"])

		revisions, ok := page["revisions"].([]interface{})
		if !ok {
			return result, nil
		}

		var prevSize int
		for i, rev := range revisions {
			r, ok := rev.(map[string]interface{})
			if !ok {
				continue
			}
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
		contrib, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
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

// aggregateChanges groups recent changes by the specified field
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
