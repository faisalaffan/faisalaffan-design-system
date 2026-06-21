package model

import "time"

type Product struct {
	SKU             string    `json:"sku"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Category        string    `json:"category"`
	Brand           string    `json:"brand"`
	Price           float64   `json:"price"`
	MarginPct       float64   `json:"margin_pct"`
	PopularityScore float64   `json:"popularity_score"`
	Tags            []string  `json:"tags"`
	ImageURL        string    `json:"image_url"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
}

type SearchRequest struct {
	Query    string `form:"q"`
	HubID    string `form:"hub_id"`
	Category string `form:"category"`
	Sort     string `form:"sort"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

type SearchResult struct {
	Products []RankedProduct `json:"products"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Query    string          `json:"query"`
}

type RankedProduct struct {
	Product  `json:",inline"`
	Score    float64 `json:"score"`
	InStock  bool    `json:"in_stock"`
	StockQty int     `json:"stock_qty"`
}

type Suggestion struct {
	Text  string `json:"text"`
	Score int    `json:"score"`
}

type RankingWeights struct {
	Relevance  float64 `json:"relevance"`
	Stock      float64 `json:"stock"`
	Margin     float64 `json:"margin"`
	Popularity float64 `json:"popularity"`
}

func DefaultWeights() RankingWeights {
	return RankingWeights{
		Relevance:  0.40,
		Stock:      0.25,
		Margin:     0.20,
		Popularity: 0.15,
	}
}
