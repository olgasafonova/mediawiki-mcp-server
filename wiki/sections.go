package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

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
	if wantsSectionContent(args) {
		return c.getSectionContent(ctx, normalizedTitle, args.Section, args.Format)
	}

	return c.listSections(ctx, normalizedTitle)
}

// wantsSectionContent reports whether the caller asked for one section's
// content (explicit section index, or the lead section with a format).
func wantsSectionContent(args GetSectionsArgs) bool {
	return args.Section > 0 || (args.Section == 0 && args.Format != "")
}

// listSections returns the section structure of a page, using the cache.
func (c *Client) listSections(ctx context.Context, normalizedTitle string) (GetSectionsResult, error) {
	cacheKey := fmt.Sprintf("sections:%s", normalizedTitle)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(GetSectionsResult), nil
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", normalizedTitle)
	params.Set("prop", "sections")

	parse, err := c.requestParseObject(ctx, params)
	if err != nil {
		return GetSectionsResult{}, err
	}

	sectionsRaw, _ := parse["sections"].([]interface{})
	sections := parseSectionList(sectionsRaw)

	result := GetSectionsResult{
		Title:    getString(parse["title"]),
		PageID:   getInt(parse["pageid"]),
		Sections: sections,
		Message:  fmt.Sprintf("Found %d sections. Use section parameter to get specific section content.", len(sections)),
	}

	c.setCache(cacheKey, result, "page_content")
	return result, nil
}

// requestParseObject performs a parse-action API request and returns the
// 'parse' object from the response.
func (c *Client) requestParseObject(ctx context.Context, params url.Values) (map[string]interface{}, error) {
	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("%s", errInfo["info"])
	}

	parse, ok := resp["parse"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected API response: missing 'parse' object")
	}
	return parse, nil
}

// parseSectionList converts the raw API section entries into SectionInfo values.
func parseSectionList(sectionsRaw []interface{}) []SectionInfo {
	sections := make([]SectionInfo, 0, len(sectionsRaw))
	for _, s := range sectionsRaw {
		sec, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		sections = append(sections, parseSectionInfo(sec))
	}
	return sections
}

// parseSectionInfo converts one raw API section entry into a SectionInfo.
func parseSectionInfo(sec map[string]interface{}) SectionInfo {
	index, _ := strconv.Atoi(getString(sec["index"]))
	level, _ := strconv.Atoi(getString(sec["level"]))
	lineNum := 0
	if line, ok := sec["line"].(float64); ok {
		lineNum = int(line)
	}

	return SectionInfo{
		Index:   index,
		Level:   level,
		Title:   stripHTMLTags(getString(sec["line"])),
		Anchor:  getString(sec["anchor"]),
		LineNum: lineNum,
	}
}

// sectionRequest identifies one section of a page and the desired output format.
type sectionRequest struct {
	title   string
	section int
	format  string
}

// prop selects the parse properties for the requested format.
func (r sectionRequest) prop() string {
	if r.format == "html" {
		return "text|displaytitle"
	}
	return "wikitext|displaytitle"
}

// getSectionContent retrieves the content of a specific section
func (c *Client) getSectionContent(ctx context.Context, title string, section int, format string) (GetSectionsResult, error) {
	req := sectionRequest{title: title, section: section, format: format}
	if req.format == "" {
		req.format = "wikitext"
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", req.title)
	params.Set("section", strconv.Itoa(req.section))
	params.Set("prop", req.prop())

	parse, err := c.requestParseObject(ctx, params)
	if err != nil {
		return GetSectionsResult{}, err
	}

	content := extractSectionBody(parse, req)

	return GetSectionsResult{
		Title:          getString(parse["title"]),
		PageID:         getInt(parse["pageid"]),
		SectionContent: content,
		SectionTitle:   extractSectionTitle(content, req),
		Format:         req.format,
	}, nil
}

// extractSectionBody pulls the rendered or raw section body from the parse
// object, depending on the requested format.
func extractSectionBody(parse map[string]interface{}, req sectionRequest) string {
	key := "wikitext"
	if req.format == "html" {
		key = "text"
	}
	if body, ok := parse[key].(map[string]interface{}); ok {
		return getString(body["*"])
	}
	return ""
}

// extractSectionTitle extracts the section heading from wikitext content that
// starts with "==" markers.
func extractSectionTitle(content string, req sectionRequest) string {
	if req.format != "wikitext" {
		return ""
	}
	if !strings.HasPrefix(strings.TrimSpace(content), "==") {
		return ""
	}
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	return strings.Trim(lines[0], "= \t")
}
