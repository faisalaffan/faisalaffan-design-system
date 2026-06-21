package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/service"
)

type SearchHandler struct {
	svc *service.SearchService
}

func NewSearchHandler(svc *service.SearchService) *SearchHandler {
	return &SearchHandler{svc: svc}
}

func (h *SearchHandler) Register(r *gin.RouterGroup) {
	r.GET("/search", h.Search)
	r.GET("/autocomplete", h.Autocomplete)
	r.POST("/admin/products", h.AddProduct)
}

// Search handles GET /search?q=&hub_id=&category=&sort=&page=&page_size=
func (h *SearchHandler) Search(c *gin.Context) {
	var req model.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		kit.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	result, err := h.svc.Search(c.Request.Context(), req)
	if err != nil {
		kit.InternalError(c, "search failed")
		return
	}

	kit.OK(c, result)
}

// Autocomplete handles GET /autocomplete?q=&hub_id=
func (h *SearchHandler) Autocomplete(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		kit.OK(c, []model.Suggestion{})
		return
	}

	suggestions := h.svc.Autocomplete(q, 10)
	kit.OK(c, suggestions)
}

type addProductRequest struct {
	SKU             string   `json:"sku" binding:"required"`
	Name            string   `json:"name" binding:"required"`
	Description     string   `json:"description"`
	Category        string   `json:"category"`
	Brand           string   `json:"brand"`
	Price           float64  `json:"price"`
	MarginPct       float64  `json:"margin_pct"`
	PopularityScore float64  `json:"popularity_score"`
	Tags            []string `json:"tags"`
	ImageURL        string   `json:"image_url"`
}

// AddProduct handles POST /admin/products
func (h *SearchHandler) AddProduct(c *gin.Context) {
	var req addProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "sku and name are required")
		return
	}

	p := model.Product{
		SKU:             req.SKU,
		Name:            req.Name,
		Description:     req.Description,
		Category:        req.Category,
		Brand:           req.Brand,
		Price:           req.Price,
		MarginPct:       req.MarginPct,
		PopularityScore: req.PopularityScore,
		Tags:            req.Tags,
		ImageURL:        req.ImageURL,
	}

	h.svc.AddProduct(p)
	kit.Created(c, p)
}
