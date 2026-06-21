package handler

import (
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/youtube/store"
	"github.com/gin-gonic/gin"
)

type YouTubeHandler struct {
	store *store.MemoryStore
}

func NewYouTubeHandler(s *store.MemoryStore) *YouTubeHandler {
	return &YouTubeHandler{store: s}
}

type uploadRequest struct {
	Title       string   `json:"title" binding:"required"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func (h *YouTubeHandler) Upload(c *gin.Context) {
	var req uploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "title is required")
		return
	}

	v := h.store.Create(req.Title, req.Description, req.Tags)
	kit.Created(c, gin.H{"video": v})
}

func (h *YouTubeHandler) Get(c *gin.Context) {
	id := c.Param("id")
	v := h.store.Get(id)
	if v == nil {
		kit.NotFound(c, "video not found")
		return
	}
	h.store.IncrementView(id)
	kit.OK(c, gin.H{"video": v})
}

func (h *YouTubeHandler) List(c *gin.Context) {
	videos := h.store.List()
	if videos == nil {
		videos = []*store.Video{}
	}
	kit.OK(c, gin.H{"videos": videos})
}

func (h *YouTubeHandler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		kit.BadRequest(c, "q query param is required")
		return
	}
	videos := h.store.Search(q)
	if videos == nil {
		videos = []*store.Video{}
	}
	kit.OK(c, gin.H{"videos": videos})
}

func (h *YouTubeHandler) Register(r *gin.RouterGroup) {
	r.POST("/videos", h.Upload)
	r.GET("/videos", h.List)
	r.GET("/videos/:id", h.Get)
	r.GET("/search", h.Search)
}
