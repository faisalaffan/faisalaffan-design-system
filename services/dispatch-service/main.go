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
	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/service"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8105"

	// --- Configuration ---
	batchCfg := model.DefaultBatchConfig()
	scoreCfg := model.DefaultMatchScore()
	reCfg := service.DefaultReassignmentConfig()

	// --- Services ---
	batcher := service.NewBatchCollector(batchCfg)
	matcher := service.NewGreedyMatcher(scoreCfg)
	stateMachine := service.NewDriverStateMachine()
	reassign := service.NewReassignmentHandler(matcher, stateMachine, reCfg)

	// --- HTTP Handler ---
	h := handler.NewDispatchHandler(batcher, matcher, stateMachine, reassign)

	// --- HTTP Server ---
	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("dispatch-service listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down dispatch-service...")

	batcher.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
