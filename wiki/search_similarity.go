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

// collectSimilarityCandidates returns candidate page titles for the similarity
// search, drawn either from a category or from a full-text search query.
// Excludes the source page itself.
func (c *Client) collectSimilarityCandidates(ctx context.Context, source string, category, searchQuery string) []string {
	var candidates []string
	if category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{Category: category, Limit: 100})
		if err != nil {
			return nil
		}
		for _, member := range catResult.Members {
			if member.Title != source {
				candidates = append(candidates, member.Title)
			}
		}
		return candidates
	}
	searchResult, err := c.Search(ctx, SearchArgs{Query: searchQuery, Limit: 50})
	if err != nil {
		return nil
	}
	for _, hit := range searchResult.Results {
		if hit.Title != source {
			candidates = append(candidates, hit.Title)
		}
	}
	return candidates
}

// loadOutgoingLinkSet returns the set of titles that the given page links to.
// On error, returns an empty set so callers can use it as a "no links" sentinel.
func (c *Client) loadOutgoingLinkSet(ctx context.Context, page string, limit int) map[string]bool {
	set := make(map[string]bool)
	links, err := c.getPageLinks(ctx, page, limit)
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
func (c *Client) candidateLinksBackTo(ctx context.Context, candidate, source string) bool {
	candLinks, err := c.getPageLinks(ctx, candidate, 500)
	if err != nil {
		return false
	}
	for _, link := range candLinks {
		if link.Title == source {
			return true
		}
	}
	return false
}

// scoreSimilarityCandidate computes the similarity score and link metadata for
// one candidate page. Returns ok=false if the candidate fails to fetch or
// scores below the threshold.
func (c *Client) scoreSimilarityCandidate(ctx context.Context, candidate, source string, sourceTerms []string, sourceLinks map[string]bool, minScore float64) (scoredCandidate, bool) {
	candContent, err := c.GetPage(ctx, GetPageArgs{Title: candidate})
	if err != nil {
		return scoredCandidate{}, false
	}
	candTerms := extractKeyTerms(candContent.Content)
	similarity := calculateJaccardSimilarity(sourceTerms, candTerms)
	if similarity < minScore {
		return scoredCandidate{}, false
	}
	return scoredCandidate{
		title:     candidate,
		score:     similarity,
		terms:     findCommonTerms(sourceTerms, candTerms, 10),
		isLinked:  sourceLinks[candidate],
		linksBack: c.candidateLinksBackTo(ctx, candidate, source),
	}, true
}

// similarityRecommendation returns advice for the user given the link state
// between the source and candidate pages.
func similarityRecommendation(score float64, isLinked, linksBack bool, source, candidate string) string {
	switch {
	case score > 0.6 && !isLinked && !linksBack:
		return "Possible duplicate - high similarity but no links between pages"
	case !isLinked && !linksBack:
		return "Consider cross-linking - related content but no links"
	case isLinked && !linksBack:
		return fmt.Sprintf("Add backlink from '%s' to '%s'", candidate, source)
	case !isLinked && linksBack:
		return fmt.Sprintf("Add link from '%s' to '%s'", source, candidate)
	default:
		return "Already cross-linked"
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

	searchQuery := strings.Join(extractTopTerms(sourceContent.Content, 5), " ")
	candidates := c.collectSimilarityCandidates(ctx, source, args.Category, searchQuery)
	if len(candidates) == 0 {
		return FindSimilarPagesResult{
			SourcePage:   source,
			SimilarPages: []SimilarPage{},
			Message:      "No candidate pages found for comparison",
		}, nil
	}

	sourceLinks := c.loadOutgoingLinkSet(ctx, source, 500)

	scored := make([]scoredCandidate, 0)
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		if sc, ok := c.scoreSimilarityCandidate(ctx, candidate, source, sourceTerms, sourceLinks, minScore); ok {
			scored = append(scored, sc)
		}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	similarPages := make([]SimilarPage, 0, limit)
	for i, sp := range scored {
		if i >= limit {
			break
		}
		similarPages = append(similarPages, SimilarPage{
			Title:           sp.title,
			SimilarityScore: sp.score,
			CommonTerms:     sp.terms,
			IsLinked:        sp.isLinked,
			LinksBack:       sp.linksBack,
			Recommendation:  similarityRecommendation(sp.score, sp.isLinked, sp.linksBack, source, sp.title),
		})
	}

	return FindSimilarPagesResult{
		SourcePage:    source,
		SimilarPages:  similarPages,
		TotalCompared: len(candidates),
	}, nil
}

// CompareTopic compares how a topic is described across multiple pages
