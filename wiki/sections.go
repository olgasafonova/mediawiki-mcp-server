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

	parse, ok := resp["parse"].(map[string]interface{})
	if !ok {
		return GetSectionsResult{}, fmt.Errorf("unexpected API response: missing 'parse' object")
	}
	pageID := getInt(parse["pageid"])
	title := getString(parse["title"])

	sectionsRaw, _ := parse["sections"].([]interface{})
	sections := make([]SectionInfo, 0, len(sectionsRaw))

	for _, s := range sectionsRaw {
		sec, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		index, _ := strconv.Atoi(getString(sec["index"]))
		level, _ := strconv.Atoi(getString(sec["level"]))
		lineNum := 0
		if line, ok := sec["line"].(float64); ok {
			lineNum = int(line)
		}

		sections = append(sections, SectionInfo{
			Index:   index,
			Level:   level,
			Title:   stripHTMLTags(getString(sec["line"])),
			Anchor:  getString(sec["anchor"]),
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

	parse, ok := resp["parse"].(map[string]interface{})
	if !ok {
		return GetSectionsResult{}, fmt.Errorf("unexpected API response: missing 'parse' object")
	}
	pageID := getInt(parse["pageid"])
	pageTitle := getString(parse["title"])

	var content string
	var sectionTitle string

	if format == "html" {
		if text, ok := parse["text"].(map[string]interface{}); ok {
			content = getString(text["*"])
		}
	} else {
		if wikitext, ok := parse["wikitext"].(map[string]interface{}); ok {
			content = getString(wikitext["*"])
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
