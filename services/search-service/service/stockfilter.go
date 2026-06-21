package service

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"

	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
)

// StockFilter batch-checks product stock via a Redis pipeline and reorders
// results so in-stock items appear first.
type StockFilter struct {
	client redis.UniversalClient
}

func NewStockFilter(client redis.UniversalClient) *StockFilter {
	return &StockFilter{client: client}
}

// Filter enriches each product with stock information. When a Redis client is
// configured it batch-fetches stockQty for every SKU in a single pipeline
// round-trip. Products with qty > 0 are placed first, followed by out-of-stock
// items. When no Redis client is available it falls back to "all in stock".
func (sf *StockFilter) Filter(ctx context.Context, products []model.RankedProduct, hubID string) []model.RankedProduct {
	if sf.client == nil || len(products) == 0 {
		result := make([]model.RankedProduct, len(products))
		copy(result, products)
		for i := range result {
			result[i].InStock = true
			result[i].StockQty = 10
		}
		return result
	}

	skus := make([]string, len(products))
	for i, p := range products {
		skus[i] = p.SKU
	}

	stockMap := sf.batchCheck(ctx, skus, hubID)

	inStock := make([]model.RankedProduct, 0, len(products))
	var outOfStock []model.RankedProduct

	for _, p := range products {
		rp := p
		if qty, ok := stockMap[p.SKU]; ok && qty > 0 {
			rp.InStock = true
			rp.StockQty = qty
			inStock = append(inStock, rp)
		} else {
			rp.InStock = false
			rp.StockQty = 0
			outOfStock = append(outOfStock, rp)
		}
	}

	return append(inStock, outOfStock...)
}

// batchCheck executes a Redis pipeline GET for every SKU under the given hub.
func (sf *StockFilter) batchCheck(ctx context.Context, skus []string, hubID string) map[string]int {
	pipe := sf.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd, len(skus))
	for _, sku := range skus {
		cmds[sku] = pipe.Get(ctx, "stock:"+hubID+":"+sku)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("stockfilter: redis pipeline exec: %v — falling back to default stock", err)
		result := make(map[string]int, len(skus))
		for _, sku := range skus {
			result[sku] = 10
		}
		return result
	}

	result := make(map[string]int, len(skus))
	for sku, cmd := range cmds {
		val, err := cmd.Int64()
		if err != nil {
			// Key not found or type mismatch → out of stock
			result[sku] = 0
			continue
		}
		result[sku] = int(val)
	}
	return result
}
