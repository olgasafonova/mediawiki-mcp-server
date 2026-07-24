package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// ListCategories lists all categories in the wiki
func (c *Client) ListCategories(ctx context.Context, args ListCategoriesArgs) (ListCategoriesResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ListCategoriesResult{}, err
	}

	resp, err := c.apiRequest(ctx, listCategoriesParams(args))
	if err != nil {
		return ListCategoriesResult{}, err
	}

	categories, err := parseCategoryList(resp)
	if err != nil {
		return ListCategoriesResult{}, err
	}

	result := ListCategoriesResult{
		Categories: categories,
	}
	applyCategoryContinuation(resp, "accontinue", &result.HasMore, &result.ContinueFrom)
	return result, nil
}

// listCategoriesParams builds the allcategories query parameters.
func listCategoriesParams(args ListCategoriesArgs) url.Values {
	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)
	return categoryQueryParams("allcategories", "aclimit", limit, map[string]string{
		"acprop":     "size",
		"acprefix":   args.Prefix,
		"accontinue": args.ContinueFrom,
	})
}

// parseCategoryList extracts category entries from an allcategories response.
func parseCategoryList(resp map[string]interface{}) ([]CategoryInfo, error) {
	query := getMap(resp["query"])
	if query == nil {
		return nil, fmt.Errorf("unexpected response format: missing query")
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
	return categories, nil
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

	resp, err := c.apiRequest(ctx, categoryMembersParams(category, args))
	if err != nil {
		return CategoryMembersResult{}, err
	}

	pages, err := parseCategoryMembers(resp)
	if err != nil {
		return CategoryMembersResult{}, err
	}

	result := CategoryMembersResult{
		Category: category,
		Members:  pages,
	}
	applyCategoryContinuation(resp, "cmcontinue", &result.HasMore, &result.ContinueFrom)
	return result, nil
}

// categoryMembersParams builds the categorymembers query parameters.
func categoryMembersParams(category string, args CategoryMembersArgs) url.Values {
	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)
	return categoryQueryParams("categorymembers", "cmlimit", limit, map[string]string{
		"cmtitle":    category,
		"cmtype":     args.Type,
		"cmcontinue": args.ContinueFrom,
	})
}

// categoryQueryParams builds a query action's parameters for a list module,
// applying only the non-empty optional values.
func categoryQueryParams(list, limitKey string, limit int, optional map[string]string) url.Values {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", list)
	params.Set(limitKey, strconv.Itoa(limit))
	for key, value := range optional {
		if value != "" {
			params.Set(key, value)
		}
	}
	return params
}

// parseCategoryMembers extracts member page summaries from a categorymembers response.
func parseCategoryMembers(resp map[string]interface{}) ([]PageSummary, error) {
	query := getMap(resp["query"])
	if query == nil {
		return nil, fmt.Errorf("unexpected response format: missing query")
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
	return pages, nil
}

// applyCategoryContinuation copies a continuation token from the response into the
// result fields when the API signals more data is available.
func applyCategoryContinuation(resp map[string]interface{}, key string, hasMore *bool, continueFrom *string) {
	cont := getMap(resp["continue"])
	if cont == nil {
		return
	}
	if token := getString(cont[key]); token != "" {
		*hasMore = true
		*continueFrom = token
	}
}
