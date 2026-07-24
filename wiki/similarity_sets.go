package wiki

import "sort"

// stringSet builds a set from a slice of strings.
func stringSet(terms []string) map[string]bool {
	s := make(map[string]bool, len(terms))
	for _, t := range terms {
		s[t] = true
	}
	return s
}

// setIntersectionSize returns the count of elements present in both sets.
func setIntersectionSize(a, b map[string]bool) int {
	n := 0
	for term := range a {
		if b[term] {
			n++
		}
	}
	return n
}

// calculateJaccardSimilarity calculates Jaccard similarity between two term sets
func calculateJaccardSimilarity(termsA, termsB []string) float64 {
	if len(termsA) == 0 && len(termsB) == 0 {
		return 0
	}

	setA := stringSet(termsA)
	setB := stringSet(termsB)

	intersection := setIntersectionSize(setA, setB)
	union := len(setA) + len(setB) - intersection

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// findCommonTerms returns terms present in both slices
func findCommonTerms(termsA, termsB []string, limit int) []string {
	setA := make(map[string]bool)
	for _, term := range termsA {
		setA[term] = true
	}

	common := make([]string, 0)
	seen := make(map[string]bool)
	for _, term := range termsB {
		if setA[term] && !seen[term] {
			common = append(common, term)
			seen[term] = true
		}
	}

	// Sort by length (longer = more significant)
	sort.Slice(common, func(i, j int) bool {
		return len(common[i]) > len(common[j])
	})

	if limit > 0 && len(common) > limit {
		return common[:limit]
	}
	return common
}
