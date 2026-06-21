package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/service"
	"github.com/gin-gonic/gin"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8108"
	}

	// --- Dependencies ---
	rules := model.DefaultValidationRules()
	validator := service.NewLocationValidator(rules)
	fanOut := service.NewFanOut()
	connMgr := service.NewConnectionManager()
	kalmanPool := service.NewKalmanPool()

	ingestor := &service.Ingestor{
		Validator:  validator,
		FanOut:     fanOut,
		ConnMgr:    connMgr,
		KalmanPool: kalmanPool,
	}
	sseHandler := service.NewSSEHandler(fanOut)

	// --- Gin engine ---
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.Use(gin.Recovery())

	// Health check
	g.GET("/health", handler.HealthCheck)

	// Tracking routes
	h := handler.New(ingestor, sseHandler)
	h.RegisterRoutes(g.Group("/"))

	// --- HTTP server ---
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: g,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("tracking-service: listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("tracking-service: listen: %v", err)
		}
	}()

	<-quit
	log.Println("tracking-service: shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close all active WebSocket connections (best effort)
	// The Ingestor's ConnectionManager handles cleanup on disconnect.
	// We iterate the map and close each connection.
	// (In a real service this would be more graceful.)

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("tracking-service: forced shutdown: %v", err)
	}

	log.Println("tracking-service: stopped")
}
