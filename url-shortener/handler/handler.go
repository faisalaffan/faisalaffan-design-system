package handler

import (
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/url-shortener/service"
	"github.com/gin-gonic/gin"
)

type ShortenHandler struct {
	svc *service.ShortenService
}

func NewShortenHandler(svc *service.ShortenService) *ShortenHandler {
	return &ShortenHandler{svc: svc}
}

type shortenRequest struct {
	URL string `json:"url" binding:"required,url"`
}

func (h *ShortenHandler) Shorten(c *gin.Context) {
	var req shortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "valid URL is required")
		return
	}

	result, err := h.svc.Shorten(c.Request.Context(), req.URL)
	if err != nil {
		kit.InternalError(c, "failed to shorten URL")
		return
	}

	kit.Created(c, result)
}

func (h *ShortenHandler) Lookup(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		kit.BadRequest(c, "code is required")
		return
	}

	url, err := h.svc.Lookup(c.Request.Context(), code)
	if err != nil {
		kit.NotFound(c, "shortcode not found")
		return
	}

	c.Redirect(http.StatusFound, url)
}

func (h *ShortenHandler) Register(r *gin.RouterGroup) {
	r.POST("/shorten", h.Shorten)
	r.GET("/:code", h.Lookup)
}
