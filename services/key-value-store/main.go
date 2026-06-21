package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/key-value-store/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/key-value-store/shard"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8085"

	nodes := shard.NodesFromEnv(os.Getenv("KV_NODES"))
	mgr := shard.NewManager(nodes)
	h := handler.NewKVHandler(mgr)

	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("key-value-store listening on :%s (nodes=%s)", cfg.Port, strings.Join(nodes, ","))
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
