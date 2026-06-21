package handler

import (
	"os"
	"strconv"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/unique-id-generator/snowflake"
	"github.com/gin-gonic/gin"
)

type IDHandler struct {
	gen *snowflake.Generator
}

func NewIDHandler(gen *snowflake.Generator) *IDHandler {
	return &IDHandler{gen: gen}
}

func (h *IDHandler) Generate(c *gin.Context) {
	id, err := h.gen.Next()
	if err != nil {
		kit.InternalError(c, "failed to generate ID")
		return
	}
	kit.OK(c, gin.H{"id": id})
}

func (h *IDHandler) Register(r *gin.RouterGroup) {
	r.GET("/id", h.Generate)
}

func WorkerIDFromEnv() int64 {
	v := os.Getenv("WORKER_ID")
	if v == "" {
		return 0
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
