package service

import (
	"context"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/url-shortener/storage"
)

func TestShortenService_ShortenAndLookup(t *testing.T) {
	store := storage.NewMemoryStore()
	svc := NewShortenService(store, "http://localhost:8080")

	result, err := svc.Shorten(context.Background(), "https://example.com/long-url")
	if err != nil {
		t.Fatalf("shorten failed: %v", err)
	}

	if result.Code == "" {
		t.Error("code should not be empty")
	}
	if result.ShortURL == "" {
		t.Error("short_url should not be empty")
	}

	url, err := svc.Lookup(context.Background(), result.Code)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if url != "https://example.com/long-url" {
		t.Errorf("expected original URL, got %s", url)
	}
}
