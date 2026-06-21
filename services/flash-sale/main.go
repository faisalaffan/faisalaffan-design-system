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
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit/circuitbreaker"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit/middleware"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/event"
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

	// Event channel (buffered, non-blocking to not hold up checkout)
	eventCh := make(chan []byte, 100)
	eventPub := event.NewChannelPublisher(eventCh)

	// Consumer goroutine: reads events from channel (stand-in for Kafka consumer)
	go func() {
		for data := range eventCh {
			_ = data // events processed here (e.g., Kafka producer, webhook dispatch)
		}
	}()

	// Circuit breaker: wraps Redis operations. Opens after 3 consecutive failures
	// with a 10-second reset timeout.
	cb := circuitbreaker.New("redis", 3, 10*time.Second)

	svc := service.New(repo, hmacSecret, eventPub)

	// Start background jobs: reaper, waiting room cleanup, admission consumer
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	svc.StartBackgroundJobs(bgCtx, "") // empty productID = scan all

	h := handler.New(svc, repo, hmacSecret, cb)

	srv := kit.NewServer(cfg)
	// Metrics middleware: tracks request duration and status codes for all routes
	srv.Use(middleware.Metrics())
	// CDN cache middleware: only applied to GET/HEAD 2xx/3xx (filtered internally)
	srv.Use(middleware.CDNCache(middleware.DefaultCacheConfig()))
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

	close(eventCh)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
	rdb.Close()
}
