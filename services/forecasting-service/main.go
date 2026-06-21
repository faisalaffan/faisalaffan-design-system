package main

import (
	"log"
	"os"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/service"
)

func main() {
	cfg := kit.LoadConfig()

	// Initialise all service layers.
	hwEngine := service.NewHoltWintersEngine()
	coldStart := service.NewColdStartHandler()
	replenish := service.NewReplenishmentCalculator()
	collector := service.NewFeatureCollector()
	accuracy := service.NewAccuracyMonitor()

	// Override Holt-Winters params per SKU category.
	hwEngine.SetParams("FNB", model.HoltWintersParams{Alpha: 0.4, Beta: 0.15, Gamma: 0.25, SeasonLength: 7})
	hwEngine.SetParams("FRZ", model.HoltWintersParams{Alpha: 0.2, Beta: 0.05, Gamma: 0.1, SeasonLength: 7})

	forecastSvc := service.NewForecastingService(hwEngine, coldStart, replenish, collector, accuracy)
	h := handler.New(forecastSvc)

	srv := kit.NewServer(cfg)
	h.RegisterRoutes(srv)

	port := getEnv("PORT", "8110")
	log.Printf("forecasting-service starting on :%s", port)
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
