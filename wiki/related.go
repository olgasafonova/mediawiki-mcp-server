package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// addCategoryRelations populates relatedMap from category memberships for the
// source title. Returns the list of source-page categories for the result.
func (c *Client) addCategoryRelations(ctx context.Context, normalizedTitle string, limit int, relatedMap map[string]*RelatedPage) []string {
	cats, err := c.getPageCategories(ctx, normalizedTitle)
	if err != nil {
		return nil
	}
	for _, cat := range cats {
		if len(relatedMap) >= limit {
			break
		}
		c.addCategoryMembers(ctx, cat, normalizedTitle, limit, relatedMap)
	}
	return cats
}

// addCategoryMembers adds members of one category to relatedMap, skipping the source page.
func (c *Client) addCategoryMembers(ctx context.Context, cat, normalizedTitle string, limit int, relatedMap map[string]*RelatedPage) {
	members, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
		Category: cat,
		Limit:    limit,
		Type:     "page",
	})
	if err != nil {
		return
	}
	for _, m := range members.Members {
		if m.Title == normalizedTitle {
			continue
		}
		if existing, ok := relatedMap[m.Title]; ok {
			existing.Categories = append(existing.Categories, cat)
			existing.Score++
			continue
		}
		relatedMap[m.Title] = &RelatedPage{
			Title:      m.Title,
			PageID:     m.PageID,
			Relation:   "same_category",
			Categories: []string{cat},
			Score:      1,
		}
	}
}

// addLinkRelations populates relatedMap with pages linked from the source page.
func (c *Client) addLinkRelations(ctx context.Context, normalizedTitle string, limit int, relatedMap map[string]*RelatedPage) {
	links, err := c.getPageLinks(ctx, normalizedTitle, limit)
	if err != nil {
		return
	}
	for _, link := range links {
		if existing, ok := relatedMap[link.Title]; ok {
			existing.Relation = "linked_and_categorized"
			existing.Score += 2
			continue
		}
		relatedMap[link.Title] = &RelatedPage{
			Title:    link.Title,
			PageID:   link.PageID,
			Relation: "linked_from",
			Score:    2,
		}
	}
}

// addBacklinkRelations populates relatedMap with pages that link to the source page.
func (c *Client) addBacklinkRelations(ctx context.Context, normalizedTitle string, limit int, relatedMap map[string]*RelatedPage) {
	backlinks, err := c.GetBacklinks(ctx, GetBacklinksArgs{
		Title: normalizedTitle,
		Limit: limit,
	})
	if err != nil {
		return
	}
	for _, bl := range backlinks.Backlinks {
		if existing, ok := relatedMap[bl.Title]; ok {
			existing.Relation = "bidirectional_link"
			existing.Score += 3
			continue
		}
		relatedMap[bl.Title] = &RelatedPage{
			Title:    bl.Title,
			PageID:   bl.PageID,
			Relation: "links_to",
			Score:    1,
		}
	}
}

// sortAndLimitRelated converts the relatedMap to a sorted slice capped at limit.
func sortAndLimitRelated(relatedMap map[string]*RelatedPage, limit int) []RelatedPage {
	related := make([]RelatedPage, 0, len(relatedMap))
	for _, rp := range relatedMap {
		related = append(related, *rp)
	}
	for i := 0; i < len(related)-1; i++ {
		for j := i + 1; j < len(related); j++ {
			if related[j].Score > related[i].Score {
				related[i], related[j] = related[j], related[i]
			}
		}
	}
	if len(related) > limit {
		related = related[:limit]
	}
	return related
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

	if method == "categories" || method == "all" {
		result.Categories = c.addCategoryRelations(ctx, normalizedTitle, limit, relatedMap)
	}
	if method == "links" || method == "all" {
		c.addLinkRelations(ctx, normalizedTitle, limit, relatedMap)
	}
	if method == "backlinks" || method == "all" {
		c.addBacklinkRelations(ctx, normalizedTitle, limit, relatedMap)
	}

	related := sortAndLimitRelated(relatedMap, limit)
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

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected API response: missing 'query' object")
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected API response: missing 'pages' object")
	}

	var categories []string
	for _, p := range pages {
		page, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if cats, ok := page["categories"].([]interface{}); ok {
			for _, cat := range cats {
				c, ok := cat.(map[string]interface{})
				if !ok {
					continue
				}
				catTitle := getString(c["title"])
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

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected API response: missing 'query' object")
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected API response: missing 'pages' object")
	}

	var links []PageSummary
	for _, p := range pages {
		page, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if linksList, ok := page["links"].([]interface{}); ok {
			for _, l := range linksList {
				link, ok := l.(map[string]interface{})
				if !ok {
					continue
				}
				links = append(links, PageSummary{
					Title: getString(link["title"]),
				})
			}
		}
	}

	return links, nil
}
