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
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/service"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8102"

	hmacSecret := os.Getenv("FLASH_SALE_HMAC_SECRET")
	if hmacSecret == "" {
		hmacSecret = "change-me-in-production"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	repo := repository.New(rdb)
	if err := repo.Init(ctx); err != nil {
		log.Fatalf("repo init: %v", err)
	}

	svc := service.New(repo, hmacSecret)

	// Start background jobs: reaper, waiting room cleanup, admission consumer
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	svc.StartBackgroundJobs(bgCtx, "") // empty productID = scan all

	h := handler.New(svc, repo)

	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("flash-sale listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
	rdb.Close()
}
