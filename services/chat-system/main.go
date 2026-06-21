package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/chat-system/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/chat-system/room"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8082"

	mgr := room.NewManager()
	// Pre-create default room
	mgr.GetOrCreate("general")

	h := handler.NewChatHandler(mgr)
	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("chat-system listening on :%s", cfg.Port)
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
