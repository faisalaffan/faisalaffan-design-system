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
	"github.com/faisalaffan/faisalaffan-design-system/unique-id-generator/handler"
	"github.com/faisalaffan/faisalaffan-design-system/unique-id-generator/snowflake"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8084"

	workerID := handler.WorkerIDFromEnv()
	gen, err := snowflake.New(workerID)
	if err != nil {
		log.Fatalf("snowflake init: %v", err)
	}

	h := handler.NewIDHandler(gen)
	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("unique-id-generator listening on :%s (worker=%d)", cfg.Port, workerID)
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
