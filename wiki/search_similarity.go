package wiki

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type scoredCandidate struct {
	title     string
	score     float64
	terms     []string
	isLinked  bool
	linksBack bool
}

// similarityQuery identifies the candidate pool for a similarity search.
type similarityQuery struct {
	source      string
	category    string
	searchQuery string
}

// collectSimilarityCandidates returns candidate page titles for the similarity
// search, drawn either from a category or from a full-text search query.
// Excludes the source page itself.
func (c *Client) collectSimilarityCandidates(ctx context.Context, q similarityQuery) []string {
	var titles []string
	if q.category != "" {
		titles = c.categoryMemberTitles(ctx, q)
	} else {
		titles = c.searchHitTitles(ctx, q)
	}
	return excludeTitle(titles, q.source)
}

// categoryMemberTitles lists the titles of all members of the query category.
func (c *Client) categoryMemberTitles(ctx context.Context, q similarityQuery) []string {
	catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{Category: q.category, Limit: 100})
	if err != nil {
		return nil
	}
	return titlesOf(catResult.Members, func(member PageSummary) string { return member.Title })
}

// searchHitTitles lists the titles of full-text search hits for the query.
func (c *Client) searchHitTitles(ctx context.Context, q similarityQuery) []string {
	searchResult, err := c.Search(ctx, SearchArgs{Query: q.searchQuery, Limit: 50})
	if err != nil {
		return nil
	}
	return titlesOf(searchResult.Results, func(hit SearchHit) string { return hit.Title })
}

// titlesOf maps a slice of items to their page titles.
func titlesOf[T any](items []T, title func(T) string) []string {
	titles := make([]string, 0, len(items))
	for _, item := range items {
		titles = append(titles, title(item))
	}
	return titles
}

// excludeTitle filters out the given title from a list of page titles.
func excludeTitle(titles []string, exclude string) []string {
	var out []string
	for _, title := range titles {
		if title != exclude {
			out = append(out, title)
		}
	}
	return out
}

// loadOutgoingLinkSet returns the set of titles that the query's source page
// links to. On error, returns an empty set so callers can use it as a "no
// links" sentinel.
func (c *Client) loadOutgoingLinkSet(ctx context.Context, q similarityQuery) map[string]bool {
	set := make(map[string]bool)
	links, err := c.getPageLinks(ctx, q.source, 500)
	if err != nil {
		return set
	}
	for _, link := range links {
		set[link.Title] = true
	}
	return set
}

// candidateLinksBackTo reports whether the candidate page contains a link to
// the source title.
func (c *Client) candidateLinksBackTo(ctx context.Context, candidate string, simCtx similarityContext) bool {
	candLinks, err := c.getPageLinks(ctx, candidate, 500)
	if err != nil {
		return false
	}
	for _, link := range candLinks {
		if link.Title == simCtx.source {
			return true
		}
	}
	return false
}

// similarityContext bundles the source-page data needed to score candidates.
type similarityContext struct {
	source   string
	terms    []string
	links    map[string]bool
	minScore float64
	limit    int
}

// scoreSimilarityCandidate computes the similarity score and link metadata for
// one candidate page. Returns ok=false if the candidate fails to fetch or
// scores below the threshold.
func (c *Client) scoreSimilarityCandidate(ctx context.Context, candidate string, simCtx similarityContext) (scoredCandidate, bool) {
	candContent, err := c.GetPage(ctx, GetPageArgs{Title: candidate})
	if err != nil {
		return scoredCandidate{}, false
	}
	candTerms := extractKeyTerms(candContent.Content)
	similarity := calculateJaccardSimilarity(simCtx.terms, candTerms)
	if similarity < simCtx.minScore {
		return scoredCandidate{}, false
	}
	return scoredCandidate{
		title:     candidate,
		score:     similarity,
		terms:     findCommonTerms(simCtx.terms, candTerms, 10),
		isLinked:  simCtx.links[candidate],
		linksBack: c.candidateLinksBackTo(ctx, candidate, simCtx),
	}, true
}

// recommendation returns advice for the user given the link state between the
// source and candidate pages.
func (sp scoredCandidate) recommendation(simCtx similarityContext) string {
	switch {
	case sp.isLinked && sp.linksBack:
		return "Already cross-linked"
	case sp.isLinked:
		return fmt.Sprintf("Add backlink from '%s' to '%s'", sp.title, simCtx.source)
	case sp.linksBack:
		return fmt.Sprintf("Add link from '%s' to '%s'", simCtx.source, sp.title)
	case sp.score > 0.6:
		return "Possible duplicate - high similarity but no links between pages"
	default:
		return "Consider cross-linking - related content but no links"
	}
}

func (c *Client) FindSimilarPages(ctx context.Context, args FindSimilarPagesArgs) (FindSimilarPagesResult, error) {
	if args.Page == "" {
		return FindSimilarPagesResult{}, fmt.Errorf("page is required")
	}
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindSimilarPagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 10, 50)
	minScore := args.MinScore
	if minScore <= 0 {
		minScore = 0.1
	}
	source := normalizePageTitle(args.Page)

	sourceContent, err := c.GetPage(ctx, GetPageArgs{Title: source})
	if err != nil {
		return FindSimilarPagesResult{}, fmt.Errorf("failed to get source page: %w", err)
	}
	sourceTerms := extractKeyTerms(sourceContent.Content)
	if len(sourceTerms) == 0 {
		return FindSimilarPagesResult{
			SourcePage:   source,
			SimilarPages: []SimilarPage{},
			Message:      "Source page has no significant terms for comparison",
		}, nil
	}

	query := similarityQuery{
		source:      source,
		category:    args.Category,
		searchQuery: strings.Join(extractTopTerms(sourceContent.Content, 5), " "),
	}
	candidates := c.collectSimilarityCandidates(ctx, query)
	if len(candidates) == 0 {
		return FindSimilarPagesResult{
			SourcePage:   source,
			SimilarPages: []SimilarPage{},
			Message:      "No candidate pages found for comparison",
		}, nil
	}

	simCtx := similarityContext{
		source:   source,
		terms:    sourceTerms,
		links:    c.loadOutgoingLinkSet(ctx, query),
		minScore: minScore,
		limit:    limit,
	}
	scored := c.scoreCandidates(ctx, candidates, simCtx)

	return FindSimilarPagesResult{
		SourcePage:    source,
		SimilarPages:  buildSimilarPages(scored, simCtx),
		TotalCompared: len(candidates),
	}, nil
}

// scoreCandidates scores every candidate against the source page and returns
// the survivors sorted by descending similarity.
func (c *Client) scoreCandidates(ctx context.Context, candidates []string, simCtx similarityContext) []scoredCandidate {
	scored := make([]scoredCandidate, 0)
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		if sc, ok := c.scoreSimilarityCandidate(ctx, candidate, simCtx); ok {
			scored = append(scored, sc)
		}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	return scored
}

// buildSimilarPages converts the top scored candidates into the result shape,
// capped at the context's limit.
func buildSimilarPages(scored []scoredCandidate, simCtx similarityContext) []SimilarPage {
	similarPages := make([]SimilarPage, 0, simCtx.limit)
	for i, sp := range scored {
		if i >= simCtx.limit {
			break
		}
		similarPages = append(similarPages, SimilarPage{
			Title:           sp.title,
			SimilarityScore: sp.score,
			CommonTerms:     sp.terms,
			IsLinked:        sp.isLinked,
			LinksBack:       sp.linksBack,
			Recommendation:  sp.recommendation(simCtx),
		})
	}
	return similarPages
}

// CompareTopic compares how a topic is described across multiple pages
