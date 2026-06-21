package service

import (
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
)

// fallbackProducts implements a three-tier zero-result fallback chain:
//
//  1. Simplify query — use the longest term from the original query
//  2. Category browse — return all active products in the requested category
//  3. Popular products — top N by popularity score
//  4. Empty — returns nil when nothing matches
func (s *SearchService) fallbackProducts(query, category string) []model.RankedProduct {
	terms := tokenize(query)

	// Tier 1: keep the longest term, drop the rest
	if len(terms) > 1 {
		longest := ""
		for _, t := range terms {
			if len(t) > len(longest) {
				longest = t
			}
		}
		if longest != "" {
			ranked := s.matchProducts(longest, category)
			if len(ranked) > 0 {
				return ranked
			}
		}
	}

	// Tier 2: drop the query entirely, show everything in the category
	if category != "" {
		ranked := s.matchProducts("", category)
		if len(ranked) > 0 {
			return ranked
		}
	}

	// Tier 3: top popular across all categories
	return s.getPopularProducts(20)
}
