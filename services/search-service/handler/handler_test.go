package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/service"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func setupTestService() *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewSearchService(nil)

	svc.AddProduct(model.Product{
		SKU:             "TP-001",
		Name:            "Wireless Bluetooth Headphones",
		Description:     "High-quality wireless headphones with noise cancelling",
		Category:        "Electronics",
		Brand:           "Sony",
		Price:           149.99,
		MarginPct:       45.0,
		PopularityScore: 0.92,
		Tags:            []string{"headphones", "wireless", "bluetooth", "audio"},
		Active:          true,
	})
	svc.AddProduct(model.Product{
		SKU:             "TP-002",
		Name:            "Organic Cotton T-Shirt",
		Description:     "Comfortable organic cotton t-shirt",
		Category:        "Fashion",
		Brand:           "Nike",
		Price:           29.99,
		MarginPct:       55.0,
		PopularityScore: 0.78,
		Tags:            []string{"t-shirt", "cotton", "organic", "casual"},
		Active:          true,
	})
	svc.AddProduct(model.Product{
		SKU:             "TP-003",
		Name:            "Artisan Coffee Beans",
		Description:     "Premium medium roast coffee beans",
		Category:        "Food",
		Brand:           "Starbucks",
		Price:           18.99,
		MarginPct:       35.0,
		PopularityScore: 0.85,
		Tags:            []string{"coffee", "beans", "roast", "beverage"},
		Active:          true,
	})
	svc.AddProduct(model.Product{
		SKU:             "TP-004",
		Name:            "Mechanical Keyboard",
		Description:     "RGB mechanical keyboard with blue switches",
		Category:        "Electronics",
		Brand:           "Logitech",
		Price:           89.99,
		MarginPct:       40.0,
		PopularityScore: 0.65,
		Tags:            []string{"keyboard", "mechanical", "rgb", "gaming"},
		Active:          true,
	})

	h := NewSearchHandler(svc)
	r := gin.New()
	h.Register(&r.RouterGroup)
	return r
}

// searchResponse mirrors the JSON shape returned by the /search endpoint.
type searchResponse struct {
	Data struct {
		Products []model.RankedProduct `json:"products"`
		Total    int                   `json:"total"`
		Page     int                   `json:"page"`
		PageSize int                   `json:"page_size"`
		Query    string                `json:"query"`
	} `json:"data"`
}

func getSearchResponse(t *testing.T, body []byte) *searchResponse {
	t.Helper()
	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal search response: %v\nbody: %s", err, string(body))
	}
	return &resp
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestSearch_ByName(t *testing.T) {
	r := setupTestService()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/search?q=headphones", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := getSearchResponse(t, w.Body.Bytes())
	if resp.Data.Total == 0 {
		t.Fatal("expected at least 1 result for 'headphones'")
	}
	if resp.Data.Products[0].Name != "Wireless Bluetooth Headphones" {
		t.Fatalf("expected top result to be 'Wireless Bluetooth Headphones', got %q", resp.Data.Products[0].Name)
	}
}

func TestSearch_ByCategory(t *testing.T) {
	r := setupTestService()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/search?q=coffee&category=Food", nil)
	r.ServeHTTP(w, req)

	resp := getSearchResponse(t, w.Body.Bytes())
	if resp.Data.Total == 0 {
		t.Fatal("expected results for Food category")
	}
	for _, p := range resp.Data.Products {
		if p.Category != "Food" {
			t.Fatalf("expected all results to have category Food, got %q", p.Category)
		}
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	r := setupTestService()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/search?q=", nil)
	r.ServeHTTP(w, req)

	resp := getSearchResponse(t, w.Body.Bytes())
	if resp.Data.Total != 0 {
		t.Fatalf("expected 0 results for empty query, got %d", resp.Data.Total)
	}
}

func TestSearch_SortPriceAsc(t *testing.T) {
	r := setupTestService()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/search?q=keyboard&sort=price_asc", nil)
	r.ServeHTTP(w, req)

	resp := getSearchResponse(t, w.Body.Bytes())
	if len(resp.Data.Products) < 2 {
		t.Skip("need at least 2 matching products to test sort order")
	}
	for i := 1; i < len(resp.Data.Products); i++ {
		if resp.Data.Products[i-1].Price > resp.Data.Products[i].Price {
			t.Fatal("expected prices in ascending order")
		}
	}
}

func TestSearch_SortPopularity(t *testing.T) {
	r := setupTestService()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/search?q=a&sort=popularity", nil)
	r.ServeHTTP(w, req)

	resp := getSearchResponse(t, w.Body.Bytes())
	if len(resp.Data.Products) < 2 {
		t.Skip("need at least 2 matching products to test sort order")
	}
	for i := 1; i < len(resp.Data.Products); i++ {
		if resp.Data.Products[i-1].PopularityScore < resp.Data.Products[i].PopularityScore {
			t.Fatal("expected popularity in descending order")
		}
	}
}

func TestSearch_Pagination(t *testing.T) {
	r := setupTestService()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/search?q=a&page=1&page_size=2", nil)
	r.ServeHTTP(w, req)

	resp := getSearchResponse(t, w.Body.Bytes())
	if len(resp.Data.Products) > 2 {
		t.Fatalf("expected at most 2 products, got %d", len(resp.Data.Products))
	}
	if resp.Data.Page != 1 || resp.Data.PageSize != 2 {
		t.Fatalf("expected page=1, page_size=2, got page=%d, page_size=%d", resp.Data.Page, resp.Data.PageSize)
	}
}

func TestSearch_Fallback(t *testing.T) {
	r := setupTestService()
	w := httptest.NewRecorder()
	// "headphonez" should trigger fallback to "headphones" via longest-term
	req, _ := http.NewRequest("GET", "/search?q=headphonez", nil)
	r.ServeHTTP(w, req)

	resp := getSearchResponse(t, w.Body.Bytes())
	if resp.Data.Total == 0 {
		t.Fatal("expected fallback to return results for misspelled query")
	}
}

// ---------------------------------------------------------------------------
// autocomplete
// ---------------------------------------------------------------------------

func TestAutocomplete(t *testing.T) {
	r := setupTestService()

	t.Run("matches prefix", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/autocomplete?q=wire", nil)
		r.ServeHTTP(w, req)

		var resp struct {
			Data []model.Suggestion `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.Data) == 0 {
			t.Fatal("expected autocomplete suggestions")
		}
		found := false
		for _, s := range resp.Data {
			if strings.Contains(strings.ToLower(s.Text), "wire") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected a suggestion containing 'wire', got %+v", resp.Data)
		}
	})

	t.Run("empty prefix returns empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/autocomplete?q=", nil)
		r.ServeHTTP(w, req)

		var resp struct {
			Data []model.Suggestion `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.Data) != 0 {
			t.Fatalf("expected empty suggestions, got %d", len(resp.Data))
		}
	})
}

// ---------------------------------------------------------------------------
// admin: add product
// ---------------------------------------------------------------------------

func TestAddProduct(t *testing.T) {
	r := setupTestService()

	t.Run("valid product returns 201", func(t *testing.T) {
		body := `{"sku":"NEW-001","name":"Test Product","price":9.99}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp kit.Response
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Error != "" {
			t.Fatalf("unexpected error: %s", resp.Error)
		}
	})

	t.Run("missing sku returns 400", func(t *testing.T) {
		body := `{"name":"No SKU"}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing sku, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("added product appears in search", func(t *testing.T) {
		body := `{"sku":"SEARCH-001","name":"Fresh Mango Juice","category":"Food","price":5.99}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("setup: expected 201, got %d", w.Code)
		}

		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("GET", "/search?q=mango", nil)
		r.ServeHTTP(w2, req2)

		resp := getSearchResponse(t, w2.Body.Bytes())
		if resp.Data.Total == 0 {
			t.Fatal("expected newly added product to appear in search results")
		}
	})
}
