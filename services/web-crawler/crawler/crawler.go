package crawler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// visitedURLs provides thread-safe URL deduplication with length tracking.
type visitedURLs struct {
	mu   sync.Mutex
	urls map[string]struct{}
}

func newVisited() *visitedURLs {
	return &visitedURLs{urls: make(map[string]struct{})}
}

// tryVisit returns true if the URL was not visited before (first visit).
func (v *visitedURLs) tryVisit(rawURL string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.urls[rawURL]; ok {
		return false
	}
	v.urls[rawURL] = struct{}{}
	return true
}

func (v *visitedURLs) len() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.urls)
}

type PageResult struct {
	URL        string   `json:"url"`
	StatusCode int      `json:"status_code"`
	Title      string   `json:"title"`
	Links      []string `json:"links"`
	Error      string   `json:"error,omitempty"`
	CrawledAt  int64    `json:"crawled_at"`
}

type CrawlResult struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"` // pending, running, completed, failed
	SeedURL   string        `json:"seed_url"`
	MaxPages  int           `json:"max_pages"`
	Pages     []*PageResult `json:"pages"`
	StartedAt int64         `json:"started_at"`
	EndedAt   int64         `json:"ended_at,omitempty"`
}

type Crawler struct {
	mu     sync.Mutex
	jobs   map[string]*CrawlResult
	jobSeq int64
	client *http.Client
	delay  time.Duration
}

func New(delay time.Duration) *Crawler {
	return &Crawler{
		jobs:   make(map[string]*CrawlResult),
		client: &http.Client{Timeout: 10 * time.Second},
		delay:  delay,
	}
}

func (c *Crawler) StartCrawl(seedURL string, maxPages int) string {
	c.mu.Lock()
	c.jobSeq++
	id := fmt.Sprintf("crawl_%d", c.jobSeq)
	job := &CrawlResult{
		ID:        id,
		Status:    "pending",
		SeedURL:   seedURL,
		MaxPages:  maxPages,
		StartedAt: time.Now().UnixMilli(),
	}
	c.jobs[id] = job
	c.mu.Unlock()

	go c.run(id)
	return id
}

func (c *Crawler) run(id string) {
	c.mu.Lock()
	job := c.jobs[id]
	job.Status = "running"
	c.mu.Unlock()

	visited := newVisited()
	queue := make(chan string, job.MaxPages*2)
	results := make(chan *PageResult, job.MaxPages)
	var wg sync.WaitGroup

	queue <- job.SeedURL
	visited.tryVisit(job.SeedURL)

	workers := 3
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rawURL := range queue {
				result := c.fetch(rawURL)
				results <- result

				if result.StatusCode == 200 {
					for _, link := range result.Links {
						if visited.tryVisit(link) && visited.len() <= job.MaxPages {
							select {
							case queue <- link:
							default:
							}
						}
					}
				}
			}
		}()
	}

	// Allow queue to drain, then signal workers to stop.
	time.Sleep(2 * time.Second)
	close(queue)
	wg.Wait()
	close(results)

	c.mu.Lock()
	defer c.mu.Unlock()

	for r := range results {
		job.Pages = append(job.Pages, r)
	}
	job.Status = "completed"
	job.EndedAt = time.Now().UnixMilli()
}

func (c *Crawler) fetch(rawURL string) *PageResult {
	result := &PageResult{URL: rawURL, CrawledAt: time.Now().UnixMilli()}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Politeness delay before each request.
	time.Sleep(c.delay)

	resp, err := c.client.Get(rawURL)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.StatusCode != 200 {
		return result
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		result.Error = err.Error()
		return result
	}

	bodyStr := string(body)
	result.Title = extractTitle(bodyStr)
	result.Links = extractLinks(parsed, bodyStr)
	return result
}

func extractTitle(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return ""
	}
	var title string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = n.FirstChild.Data
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return title
}

func extractLinks(base *url.URL, htmlStr string) []string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	links := make(map[string]bool)
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					ref, err := url.Parse(attr.Val)
					if err != nil {
						continue
					}
					resolved := base.ResolveReference(ref)
					if resolved.Scheme == "http" || resolved.Scheme == "https" {
						resolved.Fragment = ""
						links[resolved.String()] = true
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	result := make([]string, 0, len(links))
	for link := range links {
		result = append(result, link)
	}
	return result
}

func (c *Crawler) GetJob(id string) *CrawlResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jobs[id]
}
