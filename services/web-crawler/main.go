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
	"github.com/faisalaffan/faisalaffan-design-system/services/web-crawler/crawler"
	"github.com/faisalaffan/faisalaffan-design-system/services/web-crawler/handler"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8088"

	delay := handler.DurationFromEnv("CRAWL_DELAY_MS", 1000*time.Millisecond)
	c := crawler.New(delay)
	h := handler.NewCrawlHandler(c)

	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("web-crawler listening on :%s (delay=%s)", cfg.Port, delay)
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
