package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/news-feed/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/news-feed/store"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8087"

	s := store.NewMemoryStore()
	h := handler.NewNewsFeedHandler(s)

	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("news-feed listening on :%s", cfg.Port)
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
