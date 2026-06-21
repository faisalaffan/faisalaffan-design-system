package kit

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	Env            string
	RedisAddr      string
	RedisPassword  string
	HmacSecret     string
	ServicePorts   map[string]string
}

func LoadConfig() Config {
	// Load .env.local if exists, silently skip if not found
	if err := godotenv.Load(".env.local"); err != nil {
		log.Printf("info: .env.local not found, using OS env or defaults")
	}

	port := envOrDefault("PORT", "8080")
	env := envOrDefault("ENV", "development")

	return Config{
		Port:          port,
		Env:           env,
		RedisAddr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		HmacSecret:    envOrDefault("HMAC_SECRET", "change-me-in-production"),
		ServicePorts: map[string]string{
			"inventory":    envOrDefault("PORT_INVENTORY", "8100"),
			"geo":          envOrDefault("PORT_GEO", "8101"),
			"flash_sale":   envOrDefault("PORT_FLASH_SALE", "8102"),
			"checkout":     envOrDefault("PORT_CHECKOUT", "8103"),
			"promo":        envOrDefault("PORT_PROMO", "8104"),
			"dispatch":     envOrDefault("PORT_DISPATCH", "8105"),
			"eta":          envOrDefault("PORT_ETA", "8106"),
			"search":       envOrDefault("PORT_SEARCH", "8107"),
			"tracking":     envOrDefault("PORT_TRACKING", "8108"),
			"pricing":      envOrDefault("PORT_PRICING", "8109"),
			"forecasting":  envOrDefault("PORT_FORECASTING", "8110"),
		},
	}
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

// EnvInt reads an env var as int, returning fallback if not set or invalid.
func EnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
