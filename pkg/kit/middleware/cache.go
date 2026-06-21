package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CacheConfig struct {
	MaxAge               int // seconds, default 30
	StaleWhileRevalidate int // seconds, default 300
	StaleIfError         int // seconds, default 3600
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{MaxAge: 30, StaleWhileRevalidate: 300, StaleIfError: 3600}
}

// CDNCache sets Cache-Control, Surrogate-Control, and CDN-Cache-Control headers.
// Only applied to GET/HEAD requests with 2xx/3xx responses.
func CDNCache(cfg CacheConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			return
		}
		if c.Writer.Status() < 200 || c.Writer.Status() >= 400 {
			return
		}
		val := fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d, stale-if-error=%d",
			cfg.MaxAge, cfg.StaleWhileRevalidate, cfg.StaleIfError)
		c.Header("Cache-Control", val)
		c.Header("Surrogate-Control", fmt.Sprintf("max-age=%d, stale-while-revalidate=%d", cfg.MaxAge, cfg.StaleWhileRevalidate))
		c.Header("CDN-Cache-Control", fmt.Sprintf("max-age=%d", cfg.MaxAge))
	}
}
