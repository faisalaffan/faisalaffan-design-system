package handler

import (
	"strconv"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/search-autocomplete/trie"
	"github.com/gin-gonic/gin"
)

type AutocompleteHandler struct {
	trie *trie.Trie
}

func NewAutocompleteHandler(t *trie.Trie) *AutocompleteHandler {
	return &AutocompleteHandler{trie: t}
}

func (h *AutocompleteHandler) Search(c *gin.Context) {
	q := c.Query("q")
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 10
	}

	results := h.trie.Search(q, limit)
	if results == nil {
		results = []trie.Result{}
	}
	kit.OK(c, gin.H{"query": q, "results": results})
}

type trainRequest struct {
	Term string `json:"term" binding:"required"`
}

func (h *AutocompleteHandler) Train(c *gin.Context) {
	var req trainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "term is required")
		return
	}
	h.trie.Increment(req.Term)
	kit.OK(c, gin.H{"trained": req.Term})
}

type bulkTrainRequest struct {
	Terms []string `json:"terms" binding:"required"`
}

func (h *AutocompleteHandler) TrainBulk(c *gin.Context) {
	var req bulkTrainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "terms array is required")
		return
	}
	for _, term := range req.Terms {
		h.trie.Increment(term)
	}
	kit.OK(c, gin.H{"trained": len(req.Terms)})
}

func (h *AutocompleteHandler) Register(r *gin.RouterGroup) {
	r.GET("/autocomplete", h.Search)
	r.POST("/train", h.Train)
	r.POST("/train/bulk", h.TrainBulk)
}
