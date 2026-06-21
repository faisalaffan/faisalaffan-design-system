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
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/service"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = os.Getenv("CHECKOUT_PORT")
	if cfg.Port == "" {
		cfg.Port = "8103"
	}

	// ─── Redis ───────────────────────────────────────────────────────────
	var rdb *redis.Client
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Printf("redis: connection failed, running without Redis: %v", err)
			rdb = nil
		} else {
			log.Println("redis: connected")
		}
	} else {
		log.Println("redis: REDIS_ADDR not set, running without Redis")
	}
	redisRepo := repository.NewRedisClient(rdb)

	// ─── Outbox channel (buffered, consumed by background goroutine) ─────
	outbox := make(chan model.OutboxEvent, 100)
	go consumeOutbox(outbox)

	// ─── Service & Handler ──────────────────────────────────────────────
	svc := service.NewCheckoutService(outbox)
	h := handler.NewHandler(svc, redisRepo)

	// ─── HTTP Server ────────────────────────────────────────────────────
	engine := kit.NewServer(cfg)
	rg := engine.Group("/api/v1")
	h.RegisterRoutes(rg)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: engine,
	}

	// Graceful shutdown.
	go func() {
		log.Printf("server: listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("server: shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server: forced shutdown: %v", err)
	}
	log.Println("server: stopped")
}

// consumeOutbox reads events from the outbox channel and processes them.
// In production this would write to a persistent outbox table or message queue.
func consumeOutbox(ch <-chan model.OutboxEvent) {
	for evt := range ch {
		log.Printf("outbox: processing event id=%s type=%s order=%s",
			evt.ID, evt.EventType, evt.OrderID)
		// TODO: publish to message broker (Kafka / RabbitMQ / SQS)
	}
}
