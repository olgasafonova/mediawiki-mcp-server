package wiki

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

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

// compareValuePair returns an Inconsistency when two page-value refs disagree
// after normalization. The second return is false when there is nothing to report.
func compareValuePair(valueType string, a, b pageValueRef) (Inconsistency, bool) {
	if a.page == b.page {
		return Inconsistency{}, false
	}
	v1 := normalizeValue(a.value)
	v2 := normalizeValue(b.value)
	if v1 == v2 || v1 == "" || v2 == "" {
		return Inconsistency{}, false
	}
	return Inconsistency{
		Type:        valueType,
		Description: fmt.Sprintf("%s values differ", valueType),
		PageA:       a.page,
		PageB:       b.page,
		ValueA:      a.value,
		ValueB:      b.value,
	}, true
}

// collectInconsistenciesForType returns all inconsistent value pairs within a
// single value type's page-value refs.
func collectInconsistenciesForType(valueType string, pageValues []pageValueRef) []Inconsistency {
	var out []Inconsistency
	for i := 0; i < len(pageValues)-1; i++ {
		for j := i + 1; j < len(pageValues); j++ {
			if inc, ok := compareValuePair(valueType, pageValues[i], pageValues[j]); ok {
				out = append(out, inc)
			}
		}
	}
	return out
}

// detectValueInconsistencies pairs up cross-page values of the same type and
// reports any that disagree once normalized.
func detectValueInconsistencies(allValues map[string][]pageValueRef) []Inconsistency {
	inconsistencies := make([]Inconsistency, 0)
	for valueType, pageValues := range allValues {
		if len(pageValues) < 2 {
			continue
		}
		inconsistencies = append(inconsistencies, collectInconsistenciesForType(valueType, pageValues)...)
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
