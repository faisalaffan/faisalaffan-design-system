package handler

import (
	"os"
	"strconv"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/web-crawler/crawler"
	"github.com/gin-gonic/gin"
)

type CrawlHandler struct {
	crawler *crawler.Crawler
}

func NewCrawlHandler(c *crawler.Crawler) *CrawlHandler {
	return &CrawlHandler{crawler: c}
}

type crawlRequest struct {
	URL      string `json:"url" binding:"required,url"`
	MaxPages int    `json:"max_pages"`
}

func (h *CrawlHandler) StartCrawl(c *gin.Context) {
	var req crawlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "valid URL is required")
		return
	}
	if req.MaxPages <= 0 {
		req.MaxPages = 10
	}
	if req.MaxPages > 100 {
		req.MaxPages = 100
	}

	id := h.crawler.StartCrawl(req.URL, req.MaxPages)
	kit.Created(c, gin.H{"job_id": id, "status": "started"})
}

func (h *CrawlHandler) GetJob(c *gin.Context) {
	id := c.Param("id")
	job := h.crawler.GetJob(id)
	if job == nil {
		kit.NotFound(c, "crawl job not found")
		return
	}
	kit.OK(c, gin.H{"job": job})
}

func (h *CrawlHandler) Register(r *gin.RouterGroup) {
	r.POST("/crawl", h.StartCrawl)
	r.GET("/crawl/:id", h.GetJob)
}

func DurationFromEnv(key string, defaultDur time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultDur
	}
	d, err := strconv.Atoi(v)
	if err != nil {
		return defaultDur
	}
	return time.Duration(d) * time.Millisecond
}
