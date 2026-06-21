package crawler

import (
	"net/url"
	"testing"
	"time"
)

func TestExtractTitle(t *testing.T) {
	html := `<html><head><title>Test Page</title></head><body></body></html>`
	title := extractTitle(html)
	if title != "Test Page" {
		t.Errorf("expected 'Test Page', got '%s'", title)
	}
}

func TestExtractTitle_Empty(t *testing.T) {
	html := `<html><head></head><body></body></html>`
	title := extractTitle(html)
	if title != "" {
		t.Errorf("expected empty title, got '%s'", title)
	}
}

func TestExtractLinks(t *testing.T) {
	html := `<html><body><a href="https://example.com/page1">Link</a><a href="/page2">Relative</a></body></html>`
	links := extractLinks(mustParse("https://example.com"), html)
	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d: %v", len(links), links)
	}
}

func TestExtractLinks_NoDuplicates(t *testing.T) {
	html := `<html><body><a href="https://x.com/a">1</a><a href="https://x.com/a">2</a></body></html>`
	links := extractLinks(mustParse("https://x.com"), html)
	if len(links) != 1 {
		t.Errorf("expected 1 unique link, got %d: %v", len(links), links)
	}
}

func TestNewCrawler(t *testing.T) {
	c := New(100 * time.Millisecond)
	if c.delay != 100*time.Millisecond {
		t.Error("delay not set correctly")
	}
	if len(c.jobs) != 0 {
		t.Error("expected no initial jobs")
	}
}

func TestVisitedURLs(t *testing.T) {
	v := newVisited()
	if v.len() != 0 {
		t.Errorf("expected 0, got %d", v.len())
	}

	if !v.tryVisit("https://example.com") {
		t.Error("expected first visit to return true")
	}
	if v.len() != 1 {
		t.Errorf("expected 1, got %d", v.len())
	}

	if v.tryVisit("https://example.com") {
		t.Error("expected duplicate visit to return false")
	}
	if v.len() != 1 {
		t.Errorf("expected still 1 after duplicate, got %d", v.len())
	}

	if !v.tryVisit("https://example.org") {
		t.Error("expected new URL to return true")
	}
	if v.len() != 2 {
		t.Errorf("expected 2, got %d", v.len())
	}
}

func mustParse(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}
