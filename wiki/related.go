package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// relationScoring describes how a related-page source affects the map: the
// relation and score bump applied when the page is already tracked, and the
// initial relation/score when it is first seen.
type relationScoring struct {
	upgradeRelation string
	upgradeDelta    int
	newRelation     string
	newScore        int
}

// recordRelated adds a page to relatedMap or upgrades an existing entry
// according to the given scoring rules.
func recordRelated(relatedMap map[string]*RelatedPage, title string, pageID int, scoring relationScoring) {
	if existing, ok := relatedMap[title]; ok {
		existing.Relation = scoring.upgradeRelation
		existing.Score += scoring.upgradeDelta
		return
	}
	relatedMap[title] = &RelatedPage{
		Title:    title,
		PageID:   pageID,
		Relation: scoring.newRelation,
		Score:    scoring.newScore,
	}
}

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

// Scoring rules for the link-based relation sources.
var (
	linkScoring     = relationScoring{upgradeRelation: "linked_and_categorized", upgradeDelta: 2, newRelation: "linked_from", newScore: 2}
	backlinkScoring = relationScoring{upgradeRelation: "bidirectional_link", upgradeDelta: 3, newRelation: "links_to", newScore: 1}
)

// addLinkRelations populates relatedMap with pages linked from the source page.
func (c *Client) addLinkRelations(ctx context.Context, normalizedTitle string, limit int, relatedMap map[string]*RelatedPage) {
	links, err := c.getPageLinks(ctx, normalizedTitle, limit)
	if err != nil {
		return
	}
	for _, link := range links {
		recordRelated(relatedMap, link.Title, link.PageID, linkScoring)
	}
}

// addBacklinkRelations populates relatedMap with pages that link to the source page.
func (c *Client) addBacklinkRelations(ctx context.Context, normalizedTitle string, limit int, relatedMap map[string]*RelatedPage) {
	backlinks, err := c.GetBacklinks(ctx, GetBacklinksArgs{Title: normalizedTitle, Limit: limit})
	if err != nil {
		return
	}
	for _, bl := range backlinks.Backlinks {
		recordRelated(relatedMap, bl.Title, bl.PageID, backlinkScoring)
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

// collectRelated gathers related pages into relatedMap using the requested
// method(s). Returns the source-page categories when categories are consulted.
func (c *Client) collectRelated(ctx context.Context, method, normalizedTitle string, limit int, relatedMap map[string]*RelatedPage) []string {
	var categories []string
	if method == "categories" || method == "all" {
		categories = c.addCategoryRelations(ctx, normalizedTitle, limit, relatedMap)
	}
	if method == "links" || method == "all" {
		c.addLinkRelations(ctx, normalizedTitle, limit, relatedMap)
	}
	if method == "backlinks" || method == "all" {
		c.addBacklinkRelations(ctx, normalizedTitle, limit, relatedMap)
	}
	return categories
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

	result.Categories = c.collectRelated(ctx, method, normalizedTitle, limit, relatedMap)

	related := sortAndLimitRelated(relatedMap, limit)
	result.RelatedPages = related
	result.Count = len(related)
	return result, nil
}

// queryPages performs an API query request and returns the 'pages' object
// from the response.
func (c *Client) queryPages(ctx context.Context, params url.Values) (map[string]interface{}, error) {
	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}
	query := getMap(resp["query"])
	if query == nil {
		return nil, fmt.Errorf("unexpected API response: missing 'query' object")
	}
	pages := getMap(query["pages"])
	if pages == nil {
		return nil, fmt.Errorf("unexpected API response: missing 'pages' object")
	}
	return pages, nil
}

// getPageCategories gets categories for a page
func (c *Client) getPageCategories(ctx context.Context, title string) ([]string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "categories")
	params.Set("cllimit", "50")

	pages, err := c.queryPages(ctx, params)
	if err != nil {
		return nil, err
	}

	var categories []string
	for _, p := range pages {
		categories = append(categories, categoriesFromPage(p)...)
	}
	return categories, nil
}

// categoriesFromPage extracts category titles (without the "Category:" prefix)
// from a single page object in an API response.
func categoriesFromPage(p interface{}) []string {
	page := getMap(p)
	if page == nil {
		return nil
	}
	var categories []string
	for _, cat := range getSlice(page["categories"]) {
		catMap := getMap(cat)
		if catMap == nil {
			continue
		}
		catTitle := strings.TrimPrefix(getString(catMap["title"]), "Category:")
		categories = append(categories, catTitle)
	}
	return categories
}

// getPageLinks gets outgoing links from a page
func (c *Client) getPageLinks(ctx context.Context, title string, limit int) ([]PageSummary, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "links")
	params.Set("pllimit", strconv.Itoa(limit))
	params.Set("plnamespace", "0") // Main namespace only

	pages, err := c.queryPages(ctx, params)
	if err != nil {
		return nil, err
	}

	var links []PageSummary
	for _, p := range pages {
		links = append(links, linksFromPage(p)...)
	}
	return links, nil
}

// linksFromPage extracts outgoing link summaries from a single page object in
// an API response.
func linksFromPage(p interface{}) []PageSummary {
	page := getMap(p)
	if page == nil {
		return nil
	}
	var links []PageSummary
	for _, l := range getSlice(page["links"]) {
		link := getMap(l)
		if link == nil {
			continue
		}
		links = append(links, PageSummary{
			Title: getString(link["title"]),
		})
	}
	return links
}
