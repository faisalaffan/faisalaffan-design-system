package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
)

// SearchService is the core orchestrator: it holds an in-memory product
// catalogue, a prefix-trie for autocomplete, and delegates ranking and stock
// filtering to dedicated components.
type SearchService struct {
	products    []model.Product
	ranker      *CompositeRanker
	stockFilter *StockFilter
	trie        *autocompleteTrie
	mu          sync.RWMutex
}

// NewSearchService creates a search service. Pass nil for redisClient to
// disable stock filtering (all products treated as in-stock).
func NewSearchService(redisClient redis.UniversalClient) *SearchService {
	return &SearchService{
		ranker:      NewCompositeRanker(),
		stockFilter: NewStockFilter(redisClient),
		trie:        newAutocompleteTrie(),
	}
}

// SetProducts replaces the entire catalogue and rebuilds the trie. Useful for
// initial seeding.
func (s *SearchService) SetProducts(products []model.Product) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.products = products
	s.trie = newAutocompleteTrie()
	for _, p := range products {
		if !p.Active {
			continue
		}
		s.trie.Insert(strings.ToLower(p.Name), 1)
		s.trie.Insert(strings.ToLower(p.Brand), 1)
		for _, tag := range p.Tags {
			s.trie.Insert(strings.ToLower(tag), 1)
		}
	}
}

// AddProduct appends a new product and updates the autocomplete trie.
func (s *SearchService) AddProduct(p model.Product) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p.CreatedAt = time.Now()
	p.Active = true
	s.products = append(s.products, p)

	s.trie.Insert(strings.ToLower(p.Name), 1)
	s.trie.Insert(strings.ToLower(p.Brand), 1)
	for _, tag := range p.Tags {
		s.trie.Insert(strings.ToLower(tag), 1)
	}
}

// Products returns a snapshot of the current catalogue.
func (s *SearchService) Products() []model.Product {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Product, len(s.products))
	copy(out, s.products)
	return out
}

// Search runs the full search pipeline: multi-match, fallback, stock
// filtering, composite ranking, sort override, and pagination.
func (s *SearchService) Search(ctx context.Context, req model.SearchRequest) (*model.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Phase 1 — multi-match scoring
	ranked := s.matchProducts(req.Query, req.Category)

	// Phase 2 — fallback chain for zero results
	if len(ranked) == 0 && req.Query != "" {
		ranked = s.fallbackProducts(req.Query, req.Category)
	}

	if len(ranked) == 0 && req.Query != "" {
		return s.emptyResult(req), nil
	}

	// Phase 3 — stock enrichment
	if req.HubID != "" && s.stockFilter != nil {
		ranked = s.stockFilter.Filter(ctx, ranked, req.HubID)
	}

	// Phase 4 — composite ranking (re-scores and sorts desc)
	ranked = s.ranker.Rank(ranked)

	// Phase 5 — sort override (only when user picks a non-relevance sort)
	ranked = s.applySort(ranked, req.Sort)

	// Phase 6 — pagination
	total := len(ranked)
	start, end := paginate(total, req.Page, req.PageSize)
	if start >= total {
		ranked = []model.RankedProduct{}
	} else {
		ranked = ranked[start:end]
	}

	return &model.SearchResult{
		Products: ranked,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
		Query:    req.Query,
	}, nil
}

// Autocomplete returns prefix-based suggestions.
func (s *SearchService) Autocomplete(prefix string, limit int) []model.Suggestion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trie.Search(prefix, limit)
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// matchProducts scores every active product against the query and optional
// category filter. Products that don't match at all are omitted.
func (s *SearchService) matchProducts(query, category string) []model.RankedProduct {
	terms := tokenize(query)
	var matched []model.RankedProduct

	for _, p := range s.products {
		if !p.Active {
			continue
		}
		if category != "" && !strings.EqualFold(p.Category, category) {
			continue
		}
		score := scoreProduct(&p, terms)
		if len(terms) > 0 && score == 0 {
			continue
		}
		// No query AND no category = no reason to include; the caller should
		// use getPopularProducts directly.
		if len(terms) == 0 && category == "" {
			continue
		}
		if len(terms) == 0 {
			score = 1.0
		}

		matched = append(matched, model.RankedProduct{
			Product: p,
			Score:   score,
		})
	}
	return matched
}

// getPopularProducts returns at most limit products sorted by popularity
// descending.
func (s *SearchService) getPopularProducts(limit int) []model.RankedProduct {
	var all []model.RankedProduct
	for _, p := range s.products {
		if !p.Active {
			continue
		}
		all = append(all, model.RankedProduct{
			Product: p,
			Score:   p.PopularityScore,
		})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].PopularityScore > all[j].PopularityScore
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// applySort re-sorts results when the sort parameter is set (the default
// relevance sort is already handled by CompositeRanker.Rank).
func (s *SearchService) applySort(ranked []model.RankedProduct, sortBy string) []model.RankedProduct {
	out := make([]model.RankedProduct, len(ranked))
	copy(out, ranked)

	switch sortBy {
	case "price_asc":
		sort.Slice(out, func(i, j int) bool { return out[i].Price < out[j].Price })
	case "price_desc":
		sort.Slice(out, func(i, j int) bool { return out[i].Price > out[j].Price })
	case "popularity":
		sort.Slice(out, func(i, j int) bool { return out[i].PopularityScore > out[j].PopularityScore })
	}
	return out
}

func (s *SearchService) emptyResult(req model.SearchRequest) *model.SearchResult {
	return &model.SearchResult{
		Products: []model.RankedProduct{},
		Total:    0,
		Page:     req.Page,
		PageSize: req.PageSize,
		Query:    req.Query,
	}
}

// tokenize lowercases the query and splits on whitespace.
func tokenize(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(strings.ToLower(s))
}

// scoreProduct computes a raw relevance score for a product given the query
// terms:
//   - name^3 (exact), name^2 (contains)
//   - category^2 (exact), category^1.5 (contains)
//   - brand^2 (exact), brand^1.5 (contains)
//   - description^1 (contains)
//   - tags^2 (exact)
func scoreProduct(p *model.Product, terms []string) float64 {
	if len(terms) == 0 {
		return 1.0
	}

	var total float64
	name := strings.ToLower(p.Name)
	desc := strings.ToLower(p.Description)
	cat := strings.ToLower(p.Category)
	brand := strings.ToLower(p.Brand)

	for _, term := range terms {
		if name == term {
			total += 3.0
		} else if strings.Contains(name, term) {
			total += 2.0
		}

		if cat == term {
			total += 2.0
		} else if strings.Contains(cat, term) {
			total += 1.5
		}

		if brand == term {
			total += 2.0
		} else if strings.Contains(brand, term) {
			total += 1.5
		}

		if strings.Contains(desc, term) {
			total += 1.0
		}

		for _, tag := range p.Tags {
			if strings.EqualFold(tag, term) {
				total += 2.0
				break
			}
		}
	}
	return total
}

// paginate computes [start, end) bounds for the given page/size.
func paginate(total, page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= total {
		return total, total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return start, end
}
