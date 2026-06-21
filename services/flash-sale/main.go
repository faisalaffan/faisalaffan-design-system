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
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8102"

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	store := NewStore(rdb, kit.LoadConfig().HmacSecret)
	if err := store.Init(ctx); err != nil {
		log.Fatalf("store init: %v", err)
	}

	svc := NewService(store)
	h := NewHandler(svc)

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
