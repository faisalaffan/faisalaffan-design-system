// Command geo-service provides a q-commerce geo-serviceability API
// with H3 hexagon indexing and tie-breaking.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/geo-service/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/geo-service/service"
)

func main() {
	cfg := kit.Config{Port: "8101", Env: "development"}

	hubStore := handler.NewHubStore()
	svc := service.New(hubStore)
	h := handler.New(svc, hubStore)

	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      srv,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("geo-service listening on :%s", cfg.Port)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
