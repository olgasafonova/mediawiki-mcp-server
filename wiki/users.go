package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

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
		user, ok := u.(map[string]interface{})
		if !ok {
			continue
		}

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
