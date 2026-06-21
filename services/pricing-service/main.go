package main

import (
	"log"
	"os"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/pricing-service/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/pricing-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/pricing-service/service"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := kit.LoadConfig()

	// Redis client for price locking
	rdb := redis.NewClient(&redis.Options{
		Addr: getEnv("REDIS_ADDR", "localhost:6379"),
	})

	signalCfg := service.SignalCollectorConfig{}
	collector := service.NewSignalCollector(signalCfg)
	detector := service.NewSurgeDetector(model.DefaultSurgeConfig(), 15)
	lock := service.NewPriceLockService(rdb)
	elasticity := service.NewElasticityTracker()
	abtest := service.NewABTest()

	pricingSvc := service.NewPricingService(collector, detector, lock, elasticity, abtest)
	h := handler.New(pricingSvc)

	srv := kit.NewServer(cfg)
	h.RegisterRoutes(srv)

	port := getEnv("PORT", "8109")
	log.Printf("pricing-service starting on :%s", port)
	if err := srv.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
