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
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8104"

	// --- Redis ---
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("WARN: Redis not reachable at %s: %v — running without rate counters", redisAddr, err)
	}

	// --- Repositories ---
	ruleStore := repository.NewRuleStore()
	counter := repository.NewRedisCounter(rdb)

	// --- Services ---
	evaluator := service.NewRuleEvaluator(ruleStore, counter)
	resolver := service.NewPromoResolver(counter)

	// --- HTTP ---
	srv := kit.NewServer(cfg)
	h := handler.New(evaluator, resolver, ruleStore, counter)

	api := srv.Group("/api/v1/promo")
	h.RegisterRoutes(api)

	// Extra admin route to list all rules (GET)
	srv.GET("/api/v1/promo/admin/rules", func(c *gin.Context) {
		list := ruleStore.List(false)
		kit.OK(c, list)
	})

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("promo-engine listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down promo-engine...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	if err := rdb.Close(); err != nil {
		log.Printf("redis close: %v", err)
	}
}
