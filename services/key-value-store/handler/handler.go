package handler

import (
	"io"

	"github.com/faisalaffan/faisalaffan-design-system/services/key-value-store/shard"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
)

type KVHandler struct {
	mgr *shard.Manager
}

func NewKVHandler(mgr *shard.Manager) *KVHandler {
	return &KVHandler{mgr: mgr}
}

func (h *KVHandler) Get(c *gin.Context) {
	key := c.Param("key")
	v, err := h.mgr.Get(c.Request.Context(), key)
	if err != nil {
		kit.NotFound(c, "key not found")
		return
	}
	kit.OK(c, gin.H{"key": key, "value": v})
}

func (h *KVHandler) Put(c *gin.Context) {
	key := c.Param("key")
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		kit.BadRequest(c, "request body is required")
		return
	}
	if err := h.mgr.Put(c.Request.Context(), key, string(body)); err != nil {
		kit.InternalError(c, "failed to store value")
		return
	}
	kit.OK(c, gin.H{"key": key, "stored": true})
}

func (h *KVHandler) Delete(c *gin.Context) {
	key := c.Param("key")
	if err := h.mgr.Delete(c.Request.Context(), key); err != nil {
		kit.InternalError(c, "failed to delete key")
		return
	}
	kit.OK(c, gin.H{"key": key, "deleted": true})
}

func (h *KVHandler) Register(r *gin.RouterGroup) {
	r.GET("/:key", h.Get)
	r.PUT("/:key", h.Put)
	r.DELETE("/:key", h.Delete)
}
