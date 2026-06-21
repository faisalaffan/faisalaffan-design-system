package kit

import (
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit/middleware"
	"github.com/gin-gonic/gin"
)

func NewServer(cfg Config) *gin.Engine {
	gin.SetMode(ginMode(cfg.Env))

	g := gin.New()
	g.Use(middleware.Recovery(), middleware.Logging())

	g.GET("/health", func(c *gin.Context) {
		OK(c, gin.H{"status": "ok"})
	})

	return g
}

func ginMode(env string) string {
	if env == "production" {
		return gin.ReleaseMode
	}
	if env == "test" {
		return gin.TestMode
	}
	return gin.DebugMode
}
