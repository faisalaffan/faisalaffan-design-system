package middleware

import (
	"net/http"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/rate-limiter/algorithm"
	"github.com/gin-gonic/gin"
)

func RateLimitPerSecond(algo algorithm.Algorithm, limit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !algo.Allow(key, limit, time.Second) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}
