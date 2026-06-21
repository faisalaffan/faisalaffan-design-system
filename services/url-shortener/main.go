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
	"github.com/faisalaffan/faisalaffan-design-system/services/url-shortener/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/url-shortener/service"
	"github.com/faisalaffan/faisalaffan-design-system/services/url-shortener/storage"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8080"

	store := storage.NewMemoryStore()
	svc := service.NewShortenService(store, "http://localhost:"+cfg.Port)
	h := handler.NewShortenHandler(svc)

	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("url-shortener listening on :%s", cfg.Port)
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
