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

// topicComparer runs a cross-page topic comparison: finding candidate pages,
// collecting mentions, and gathering near-topic values.
type topicComparer struct {
	client   *Client
	topic    string
	category string
	limit    int
}

// analyzedPage is one candidate page's content under analysis, attributed to
// the requested title.
type analyzedPage struct {
	title    string
	content  string
	contexts []string
}

// findPages returns candidate page titles for topic comparison, drawn from
// the category if specified, otherwise from a full-text search.
func (tc topicComparer) findPages(ctx context.Context) ([]string, error) {
	var pageTitles []string
	if tc.category != "" {
		catResult, err := tc.client.GetCategoryMembers(ctx, CategoryMembersArgs{Category: tc.category, Limit: 100})
		if err != nil {
			return pageTitles, nil //nolint:nilerr // category lookup failure falls through to "no candidates"
		}
		for _, member := range catResult.Members {
			pageTitles = append(pageTitles, member.Title)
		}
		return pageTitles, nil
	}
	searchResult, err := tc.client.Search(ctx, SearchArgs{Query: tc.topic, Limit: tc.limit * 2})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	for _, hit := range searchResult.Results {
		pageTitles = append(pageTitles, hit.Title)
	}
	return pageTitles, nil
}

// valuesOn collects values from the analyzed page that appear near a context
// where the topic is mentioned.
func (tc topicComparer) valuesOn(p analyzedPage) map[string][]pageValueRef {
	out := make(map[string][]pageValueRef)
	topicLower := strings.ToLower(tc.topic)
	for _, v := range extractValues(p.content) {
		valueCtxLower := strings.ToLower(v.Context)
		for _, ctxStr := range p.contexts {
			if strings.Contains(strings.ToLower(ctxStr), valueCtxLower) ||
				strings.Contains(valueCtxLower, topicLower) {
				out[v.Type] = append(out[v.Type], pageValueRef{page: p.title, value: v.Value})
				break
			}
		}
	}
	return out
}

// analyzePage fetches the page, checks for the topic, and returns the mention
// summary plus any near-topic values. Returns ok=false when the page either
// fails to fetch or doesn't mention the topic.
func (tc topicComparer) analyzePage(ctx context.Context, title string) (TopicMention, map[string][]pageValueRef, bool) {
	page, err := tc.client.GetPage(ctx, GetPageArgs{Title: title})
	if err != nil {
		return TopicMention{}, nil, false
	}
	contentLower := strings.ToLower(page.Content)
	topicLower := strings.ToLower(tc.topic)
	if !strings.Contains(contentLower, topicLower) {
		return TopicMention{}, nil, false
	}
	contexts := extractContextsForTerm(page.Content, tc.topic, 3)
	info, _ := tc.client.GetPageInfo(ctx, PageInfoArgs{Title: title})
	mention := TopicMention{
		PageTitle:  title,
		Mentions:   strings.Count(contentLower, topicLower),
		Contexts:   contexts,
		LastEdited: info.Touched,
	}
	values := tc.valuesOn(analyzedPage{title: title, content: page.Content, contexts: contexts})
	return mention, values, true
}

// collectMentions walks candidate pages, gathering mentions and near-topic
// values until the mention limit is reached or the context is canceled.
func (tc topicComparer) collectMentions(ctx context.Context, pageTitles []string) ([]TopicMention, map[string][]pageValueRef) {
	mentions := make([]TopicMention, 0)
	allValues := make(map[string][]pageValueRef)
	for _, title := range pageTitles {
		if len(mentions) >= tc.limit || ctx.Err() != nil {
			break
		}
		mention, values, ok := tc.analyzePage(ctx, title)
		if !ok {
			continue
		}
		mentions = append(mentions, mention)
		for valueType, refs := range values {
			allValues[valueType] = append(allValues[valueType], refs...)
		}
	}
	return mentions, allValues
}

// compareValuePair returns an Inconsistency when two page-value refs disagree
// after normalization. The second return is false when there is nothing to report.
func compareValuePair(valueType string, a, b pageValueRef) (Inconsistency, bool) {
	if a.page == b.page {
		return Inconsistency{}, false
	}
	v1 := normalizeValue(a.value)
	v2 := normalizeValue(b.value)
	if v1 == "" || v2 == "" {
		return Inconsistency{}, false
	}
	if v1 == v2 {
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

	tc := topicComparer{
		client:   c,
		topic:    args.Topic,
		category: args.Category,
		limit:    normalizeLimit(args.Limit, 20, 50),
	}

	pageTitles, err := tc.findPages(ctx)
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

	mentions, allValues := tc.collectMentions(ctx, pageTitles)

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
