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
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/handler"
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/service"
)

func main() {
	cfg := kit.LoadConfig()
	cfg.Port = "8107"

	svc := service.NewSearchService(nil)
	seedProducts(svc)

	h := handler.NewSearchHandler(svc)

	srv := kit.NewServer(cfg)
	h.Register(&srv.RouterGroup)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv}

	go func() {
		log.Printf("q-commerce search-service listening on :%s", cfg.Port)
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

func seedProducts(svc *service.SearchService) {
	products := []model.Product{
		{
			SKU:             "SKU-001",
			Name:            "Wireless Bluetooth Headphones",
			Description:     "High-quality wireless headphones with active noise cancelling and 30-hour battery life.",
			Category:        "Electronics",
			Brand:           "Sony",
			Price:           149.99,
			MarginPct:       45.0,
			PopularityScore: 0.92,
			Tags:            []string{"headphones", "wireless", "bluetooth", "audio", "noise-cancelling"},
		},
		{
			SKU:             "SKU-002",
			Name:            "Organic Cotton T-Shirt",
			Description:     "Comfortable organic cotton t-shirt available in multiple colors.",
			Category:        "Fashion",
			Brand:           "Nike",
			Price:           29.99,
			MarginPct:       55.0,
			PopularityScore: 0.78,
			Tags:            []string{"t-shirt", "cotton", "organic", "casual", "apparel"},
		},
		{
			SKU:             "SKU-003",
			Name:            "Artisan Coffee Beans - Medium Roast",
			Description:     "Premium single-origin medium roast coffee beans with chocolate and citrus notes.",
			Category:        "Food",
			Brand:           "Starbucks",
			Price:           18.99,
			MarginPct:       35.0,
			PopularityScore: 0.85,
			Tags:            []string{"coffee", "beans", "roast", "beverage", "arabica"},
		},
		{
			SKU:             "SKU-004",
			Name:            "4K Ultra HD Monitor 27-inch",
			Description:     "27-inch 4K UHD IPS monitor with HDR support and USB-C connectivity.",
			Category:        "Electronics",
			Brand:           "Dell",
			Price:           449.99,
			MarginPct:       30.0,
			PopularityScore: 0.72,
			Tags:            []string{"monitor", "4k", "display", "usb-c", "hdr"},
		},
		{
			SKU:             "SKU-005",
			Name:            "Running Shoes Pro",
			Description:     "Lightweight running shoes with responsive cushioning and breathable mesh upper.",
			Category:        "Fashion",
			Brand:           "Adidas",
			Price:           129.99,
			MarginPct:       50.0,
			PopularityScore: 0.88,
			Tags:            []string{"shoes", "running", "athletic", "sneakers", "sport"},
		},
		{
			SKU:             "SKU-006",
			Name:            "Dark Chocolate Bar 70% Cocoa",
			Description:     "Rich dark chocolate bar made with sustainably sourced cocoa beans.",
			Category:        "Food",
			Brand:           "Lindt",
			Price:           4.99,
			MarginPct:       60.0,
			PopularityScore: 0.65,
			Tags:            []string{"chocolate", "dark", "snack", "sweet", "organic"},
		},
		{
			SKU:             "SKU-007",
			Name:            "Mechanical Keyboard RGB",
			Description:     "Full-size mechanical keyboard with per-key RGB lighting and blue mechanical switches.",
			Category:        "Electronics",
			Brand:           "Logitech",
			Price:           89.99,
			MarginPct:       40.0,
			PopularityScore: 0.82,
			Tags:            []string{"keyboard", "mechanical", "rgb", "gaming", "typing"},
		},
		{
			SKU:             "SKU-008",
			Name:            "Denim Jacket Classic",
			Description:     "Classic denim jacket with a modern fit, perfect for layering.",
			Category:        "Fashion",
			Brand:           "Levi's",
			Price:           79.99,
			MarginPct:       48.0,
			PopularityScore: 0.70,
			Tags:            []string{"jacket", "denim", "outerwear", "classic", "casual"},
		},
		{
			SKU:             "SKU-009",
			Name:            "Green Tea Matcha Powder",
			Description:     "Ceremonial grade matcha green tea powder from Japan.",
			Category:        "Food",
			Brand:           "Ito En",
			Price:           24.99,
			MarginPct:       42.0,
			PopularityScore: 0.60,
			Tags:            []string{"tea", "matcha", "green", "japanese", "beverage"},
		},
		{
			SKU:             "SKU-010",
			Name:            "USB-C Hub 7-in-1",
			Description:     "Compact 7-in-1 USB-C hub with HDMI, USB-A, SD card reader, and PD charging.",
			Category:        "Electronics",
			Brand:           "Anker",
			Price:           34.99,
			MarginPct:       38.0,
			PopularityScore: 0.75,
			Tags:            []string{"usb-c", "hub", "adapter", "accessories", "charging"},
		},
		{
			SKU:             "SKU-011",
			Name:            "Premium Yoga Mat",
			Description:     "Extra thick non-slip yoga mat with carrying strap. Eco-friendly TPE material.",
			Category:        "Sports",
			Brand:           "Lululemon",
			Price:           68.00,
			MarginPct:       52.0,
			PopularityScore: 0.80,
			Tags:            []string{"yoga", "mat", "fitness", "exercise", "wellness"},
		},
		{
			SKU:             "SKU-012",
			Name:            "Wireless Ergonomic Mouse",
			Description:     "Ergonomically designed wireless mouse with adjustable DPI and silent clicks.",
			Category:        "Electronics",
			Brand:           "Logitech",
			Price:           49.99,
			MarginPct:       44.0,
			PopularityScore: 0.68,
			Tags:            []string{"mouse", "wireless", "ergonomic", "computer", "accessories"},
		},
		{
			SKU:             "SKU-013",
			Name:            "Cashmere Scarf",
			Description:     "Luxurious pure cashmere scarf. Soft, warm, and lightweight.",
			Category:        "Fashion",
			Brand:           "Burberry",
			Price:           195.00,
			MarginPct:       65.0,
			PopularityScore: 0.55,
			Tags:            []string{"scarf", "cashmere", "luxury", "winter", "accessories"},
		},
		{
			SKU:             "SKU-014",
			Name:            "Pure Organic Honey",
			Description:     "Raw unfiltered organic honey sourced from wildflower farms.",
			Category:        "Food",
			Brand:           "Nature's Way",
			Price:           12.99,
			MarginPct:       32.0,
			PopularityScore: 0.58,
			Tags:            []string{"honey", "organic", "natural", "sweetener", "superfood"},
		},
		{
			SKU:             "SKU-015",
			Name:            "Silicone Smartphone Case",
			Description:     "Shockproof silicone case with raised bezel for screen protection.",
			Category:        "Electronics",
			Brand:           "Spigen",
			Price:           19.99,
			MarginPct:       55.0,
			PopularityScore: 0.62,
			Tags:            []string{"case", "phone", "silicone", "protective", "accessories"},
		},
		{
			SKU:             "SKU-016",
			Name:            "Leather Bifold Wallet",
			Description:     "Genuine leather bifold wallet with RFID blocking technology.",
			Category:        "Fashion",
			Brand:           "Fossil",
			Price:           55.00,
			MarginPct:       58.0,
			PopularityScore: 0.67,
			Tags:            []string{"wallet", "leather", "rfid", "accessories", "mens"},
		},
		{
			SKU:             "SKU-017",
			Name:            "Protein Bars Variety Pack",
			Description:     "12-pack of high-protein bars with 20g protein each. Mix of chocolate, peanut butter, and cookie dough.",
			Category:        "Food",
			Brand:           "Quest",
			Price:           29.99,
			MarginPct:       38.0,
			PopularityScore: 0.74,
			Tags:            []string{"protein", "bars", "snack", "fitness", "nutrition"},
		},
		{
			SKU:             "SKU-018",
			Name:            "Noise Cancelling Earbuds",
			Description:     "True wireless earbuds with adaptive noise cancelling and spatial audio.",
			Category:        "Electronics",
			Brand:           "Sony",
			Price:           199.99,
			MarginPct:       42.0,
			PopularityScore: 0.95,
			Tags:            []string{"earbuds", "wireless", "noise-cancelling", "audio", "portable"},
		},
		{
			SKU:             "SKU-019",
			Name:            "Designer Aviator Sunglasses",
			Description:     "Classic aviator sunglasses with UV400 protection and gold-tone frame.",
			Category:        "Fashion",
			Brand:           "Ray-Ban",
			Price:           154.00,
			MarginPct:       60.0,
			PopularityScore: 0.83,
			Tags:            []string{"sunglasses", "aviator", "designer", "uv-protection", "accessories"},
		},
		{
			SKU:             "SKU-020",
			Name:            "Extra Virgin Olive Oil",
			Description:     "Cold-pressed extra virgin olive oil from Italy. Rich, peppery finish.",
			Category:        "Food",
			Brand:           "Bertolli",
			Price:           15.99,
			MarginPct:       28.0,
			PopularityScore: 0.50,
			Tags:            []string{"olive-oil", "cooking", "italian", "organic", "condiment"},
		},
	}

	svc.SetProducts(products)
	log.Printf("seeded %d products", len(products))
}
