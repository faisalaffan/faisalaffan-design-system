package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	kitmw "github.com/faisalaffan/faisalaffan-design-system/pkg/kit/middleware"
	"github.com/faisalaffan/faisalaffan-design-system/rate-limiter/algorithm"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8081"

	algo := algorithm.NewSlidingWindow()
	srv := kit.NewServer(cfg)

	srv.GET("/limited", kitmw.RateLimitPerSecond(algo, 5), func(c *gin.Context) {
		kit.OK(c, gin.H{"message": "within rate limit"})
	})

	srv.GET("/unlimited", func(c *gin.Context) {
		kit.OK(c, gin.H{"message": "no rate limit"})
	})

	srv.POST("/admin/reset", func(c *gin.Context) {
		var body struct {
			IP string `json:"ip" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			kit.BadRequest(c, "ip is required")
			return
		}
		kit.OK(c, gin.H{"message": "rate limit reset for " + body.IP})
	})

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("rate-limiter listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}
