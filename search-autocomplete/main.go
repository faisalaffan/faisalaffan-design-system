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
	"github.com/faisalaffan/faisalaffan-design-system/search-autocomplete/handler"
	"github.com/faisalaffan/faisalaffan-design-system/search-autocomplete/trie"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8086"

	t := trie.New()

	// Seed with common terms
	seeds := []string{"design", "developer", "database", "distributed", "docker", "deploy"}
	for _, s := range seeds {
		t.Insert(s, 10)
	}

	h := handler.NewAutocompleteHandler(t)
	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("search-autocomplete listening on :%s", cfg.Port)
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
