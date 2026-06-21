package service

import (
	"math"
	"sort"

	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
)

// CompositeRanker computes the final ranking score as a weighted sum of
// normalized relevance, margin, popularity, and stock factors.
//
// Formula:
//
//	score = relScore * 0.40 + marginScore * 0.20 + popScore * 0.15 + stockScore * 0.25
//
// Each factor is normalised to [0, 1] before the weighted sum.
type CompositeRanker struct {
	Weights model.RankingWeights
}

func NewCompositeRanker() *CompositeRanker {
	w := model.DefaultWeights()
	return &CompositeRanker{Weights: w}
}

// Rank re-scores every product using the composite formula and sorts
// descending by final score.
func (r *CompositeRanker) Rank(products []model.RankedProduct) []model.RankedProduct {
	if len(products) == 0 {
		return products
	}

	maxRel := 0.0
	maxMargin := 0.0
	for _, p := range products {
		if p.Score > maxRel {
			maxRel = p.Score
		}
		if p.MarginPct > maxMargin {
			maxMargin = p.MarginPct
		}
	}

	result := make([]model.RankedProduct, len(products))
	copy(result, products)

	for i, p := range result {
		// Normalise raw relevance score to [0, 1]
		relScore := 0.0
		if maxRel > 0 {
			relScore = p.Score / maxRel
		}

		// Normalise margin percentage to [0, 1]
		marginScore := 0.0
		if maxMargin > 0 {
			marginScore = p.MarginPct / maxMargin
		}

		// Popularity is already expected in [0, 1]; clamp defensively.
		popScore := p.PopularityScore
		if popScore < 0 {
			popScore = 0
		}
		if popScore > 1 {
			popScore = 1
		}

		// Stock factor — non-linear: log10(qty+1) / 2.0, capped at 1.0
		//   qty=0  → 0.00,  qty=5  → 0.39,  qty=50 → 0.85,  qty=100 → 1.00
		stockScore := math.Min(1.0, math.Log10(float64(p.StockQty)+1)/2.0)

		total := relScore*r.Weights.Relevance +
			marginScore*r.Weights.Margin +
			popScore*r.Weights.Popularity +
			stockScore*r.Weights.Stock

		result[i].Score = total
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Name < result[j].Name
		}
		return result[i].Score > result[j].Score
	})

	return result
}
