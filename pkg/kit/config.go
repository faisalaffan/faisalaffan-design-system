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
	// Search .env.local from CWD upward (handles go test, delve debug, normal run)
	loaded := false
	for _, path := range []string{".env.local", "..", "../.."} {
		if path == ".." || path == "../.." {
			path = path + "/.env.local"
		}
		if _, err := os.Stat(path); err == nil {
			if err := godotenv.Load(path); err == nil {
				loaded = true
				break
			}
		}
	}
	if !loaded {
		log.Printf("info: .env.local not found, using OS env or defaults")
	}

	port := EnvOrDefault("PORT", "8080")
	env := EnvOrDefault("ENV", "development")

	return Config{
		Port:          port,
		Env:           env,
		RedisAddr:     EnvOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		HmacSecret:    EnvOrDefault("HMAC_SECRET", "change-me-in-production"),
		ServicePorts: map[string]string{
			"inventory":    EnvOrDefault("PORT_INVENTORY", "8100"),
			"geo":          EnvOrDefault("PORT_GEO", "8101"),
			"flash_sale":   EnvOrDefault("PORT_FLASH_SALE", "8102"),
			"checkout":     EnvOrDefault("PORT_CHECKOUT", "8103"),
			"promo":        EnvOrDefault("PORT_PROMO", "8104"),
			"dispatch":     EnvOrDefault("PORT_DISPATCH", "8105"),
			"eta":          EnvOrDefault("PORT_ETA", "8106"),
			"search":       EnvOrDefault("PORT_SEARCH", "8107"),
			"tracking":     EnvOrDefault("PORT_TRACKING", "8108"),
			"pricing":      EnvOrDefault("PORT_PRICING", "8109"),
			"forecasting":  EnvOrDefault("PORT_FORECASTING", "8110"),
		},
	}
}

func EnvOrDefault(key, fallback string) string {
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
