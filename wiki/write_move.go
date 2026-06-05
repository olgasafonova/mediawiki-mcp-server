package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (c *Client) performMove(ctx context.Context, args MovePageArgs) (map[string]interface{}, error) {
	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	params := url.Values{}
	params.Set("action", "move")
	params.Set("from", args.From)
	params.Set("to", args.To)
	params.Set("token", token)

	if args.Reason != "" {
		params.Set("reason", args.Reason)
	}

	if args.NoRedirect {
		params.Set("noredirect", "1")
	}

	// Default: move talk page
	if !args.MoveTalk {
		// MediaWiki moves talk by default, so we only set movetalk=0 if explicitly disabled
	} else {
		params.Set("movetalk", "1")
	}

	if args.MoveSubpages {
		params.Set("movesubpages", "1")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	// Check for badtoken error so caller can retry
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		code := getString(errInfo["code"])
		if code == "badtoken" {
			return nil, fmt.Errorf("%s: %s", code, getString(errInfo["info"]))
		}
	}

	return resp, nil
}

// MovePage moves (renames) a wiki page
func (c *Client) MovePage(ctx context.Context, args MovePageArgs) (MovePageResult, error) {
	if args.From == "" {
		return MovePageResult{}, &ValidationError{
			Field:   "from",
			Message: "source page title is required",
		}
	}
	if args.To == "" {
		return MovePageResult{}, &ValidationError{
			Field:   "to",
			Message: "target page title is required",
		}
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return MovePageResult{}, fmt.Errorf("authentication required for page moves: %w", err)
	}

	resp, err := c.performMove(ctx, args)
	if err != nil && strings.Contains(err.Error(), "badtoken") {
		c.invalidateCSRFToken()
		resp, err = c.performMove(ctx, args)
	}
	if err != nil {
		return MovePageResult{}, err
	}

	// Check for errors
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return MovePageResult{
			Success: false,
			From:    args.From,
			To:      args.To,
			Message: fmt.Sprintf("Move failed: %s", getString(errInfo["info"])),
		}, nil
	}

	moveData, ok := resp["move"].(map[string]interface{})
	if !ok {
		return MovePageResult{
			Success: false,
			From:    args.From,
			To:      args.To,
			Message: "Unexpected response format",
		}, nil
	}

	result := MovePageResult{
		Success: true,
		From:    getString(moveData["from"]),
		To:      getString(moveData["to"]),
		Reason:  args.Reason,
		Message: fmt.Sprintf("Page moved from '%s' to '%s'", getString(moveData["from"]), getString(moveData["to"])),
	}

	// Check if talk page was moved
	if _, hasTalkFrom := moveData["talkfrom"]; hasTalkFrom {
		result.TalkMoved = true
	}

	// Build redirect URL
	wikiBaseURL := strings.TrimSuffix(c.config.BaseURL, "api.php") + "index.php"
	encodedFrom := url.QueryEscape(strings.ReplaceAll(result.From, " ", "_"))
	result.RedirectURL = fmt.Sprintf("%s?title=%s&redirect=no", wikiBaseURL, encodedFrom)

	// Log the move
	c.logAudit(AuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Operation: "move",
		Title:     result.From + " → " + result.To,
		Summary:   args.Reason,
		WikiURL:   c.config.BaseURL,
		Success:   true,
	})

	return result, nil
}

// ManageCategories adds or removes categories from a page
// categoryTagRegex matches "[[Category:Name|sortkey]]" anywhere in wikitext.
