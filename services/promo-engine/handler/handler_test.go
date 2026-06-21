package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/service"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// setup
// ---------------------------------------------------------------------------

func setupTestHandler() *Handler {
	gin.SetMode(gin.TestMode)

	store := repository.NewRuleStore()
	mc := &mockCounter{}

	evaluator := service.NewRuleEvaluator(store, mc)
	resolver := service.NewPromoResolver(mc)

	return New(evaluator, resolver, store, nil)
}

func setupRouter(h *Handler) *gin.Engine {
	g := gin.New()
	h.RegisterRoutes(g)
	return g
}

func seedTestRule(store *repository.RuleStore) string {
	rule := &model.Rule{
		Name:       "min-spend-100",
		Priority:   100,
		Conditions: &model.MinTransactionCondition{MinAmount: 10000},
		Effect:     model.Effect{Type: model.EffectFixed, Value: 5000},
		Stackable:  true,
	}
	if err := store.Save(rule); err != nil {
		panic(err)
	}
	return rule.ID
}

// respData extracts the "data" payload from a kit.Response JSON blob.
func respData(t *testing.T, raw []byte) []byte {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal kit.Response: %v", err)
	}
	return []byte(env.Data)
}

// ---------------------------------------------------------------------------
// mock counter (always-allows)
// ---------------------------------------------------------------------------

type mockCounter struct{}

func (m *mockCounter) IncrementAndCheck(_ context.Context, _, _ string, _ repository.LimitParams) repository.LimitResult {
	return repository.LimitResult{Allowed: true, ErrorCode: "OK"}
}

func (m *mockCounter) CheckAllLimits(_ context.Context, _, _ string, _ repository.LimitParams) repository.LimitResult {
	return repository.LimitResult{Allowed: true, ErrorCode: "OK"}
}

func (m *mockCounter) BudgetReserve(_ context.Context, _ string, _, _ float64) repository.LimitResult {
	return repository.LimitResult{Allowed: true, ErrorCode: "OK"}
}

// ---------------------------------------------------------------------------
// Tests: POST /validate
// ---------------------------------------------------------------------------

func TestValidate_EmptyCart(t *testing.T) {
	h := setupTestHandler()
	router := setupRouter(h)

	body := map[string]interface{}{
		"user_id": "user-1",
		"cart": map[string]interface{}{
			"cart_total": 0,
			"area":       "jakarta",
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.ValidateResponse
	if err := json.Unmarshal(respData(t, w.Body.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.EligibleRules) != 0 {
		t.Errorf("expected 0 eligible, got %d", len(resp.EligibleRules))
	}
}

func TestValidate_WithEligibleRule(t *testing.T) {
	h := setupTestHandler()
	seedTestRule(h.store)
	router := setupRouter(h)

	body := map[string]interface{}{
		"user_id": "user-1",
		"cart": map[string]interface{}{
			"cart_total": 20000,
			"area":       "jakarta",
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.ValidateResponse
	if err := json.Unmarshal(respData(t, w.Body.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.EligibleRules) != 1 {
		t.Fatalf("expected 1 eligible rule, got %d", len(resp.EligibleRules))
	}
	if resp.EligibleRules[0].RuleName != "min-spend-100" {
		t.Errorf("unexpected rule name: %s", resp.EligibleRules[0].RuleName)
	}
}

// ---------------------------------------------------------------------------
// Tests: POST /apply
// ---------------------------------------------------------------------------

func TestApply_Basic(t *testing.T) {
	h := setupTestHandler()
	seedTestRule(h.store)
	router := setupRouter(h)

	body := map[string]interface{}{
		"user_id": "user-1",
		"cart": map[string]interface{}{
			"cart_total": 20000,
			"area":       "jakarta",
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/apply", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.ApplyResponse
	if err := json.Unmarshal(respData(t, w.Body.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.AppliedRules) != 1 {
		t.Fatalf("expected 1 applied, got %d", len(resp.AppliedRules))
	}
	if resp.TotalDiscount != 5000 {
		t.Errorf("expected discount 5000, got %.2f", resp.TotalDiscount)
	}
	if resp.FinalAmount != 15000 {
		t.Errorf("expected final 15000, got %.2f", resp.FinalAmount)
	}
}

// ---------------------------------------------------------------------------
// Tests: POST /admin/rules
// ---------------------------------------------------------------------------

func TestCreateRule(t *testing.T) {
	h := setupTestHandler()
	router := setupRouter(h)

	body := map[string]interface{}{
		"name":     "test-rule",
		"priority": 50,
		"conditions": map[string]interface{}{
			"type":       "min_transaction",
			"min_amount": 5000,
		},
		"effect": map[string]interface{}{
			"type":  "percentage",
			"value": 10,
		},
		"stackable": true,
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/rules", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var rule model.Rule
	if err := json.Unmarshal(respData(t, w.Body.Bytes()), &rule); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rule.Name != "test-rule" {
		t.Errorf("expected 'test-rule', got '%s'", rule.Name)
	}
	if rule.ID == "" {
		t.Errorf("expected non-empty ID")
	}
}

func TestCreateRule_NoConditions(t *testing.T) {
	h := setupTestHandler()
	router := setupRouter(h)

	body := map[string]interface{}{
		"name":     "bad-rule",
		"priority": 10,
		"effect": map[string]interface{}{
			"type":  "fixed",
			"value": 1000,
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/rules", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateRule_MissingName(t *testing.T) {
	h := setupTestHandler()
	router := setupRouter(h)

	body := map[string]interface{}{
		"priority": 10,
		"conditions": map[string]interface{}{
			"type":       "min_transaction",
			"min_amount": 5000,
		},
		"effect": map[string]interface{}{
			"type":  "fixed",
			"value": 1000,
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/rules", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Resolver exclusivity test (direct, no HTTP)
// ---------------------------------------------------------------------------

func TestResolve_ExclusivityGroup(t *testing.T) {
	store := repository.NewRuleStore()
	mc := &mockCounter{}

	eval := service.NewRuleEvaluator(store, mc)
	resolver := service.NewPromoResolver(mc)

	r1 := &model.Rule{
		Name:             "better-rule",
		Priority:         100,
		ExclusivityGroup: "group-A",
		Conditions:       &model.MinTransactionCondition{MinAmount: 0},
		Effect:           model.Effect{Type: model.EffectFixed, Value: 10000},
	}
	r2 := &model.Rule{
		Name:             "worse-rule",
		Priority:         50,
		ExclusivityGroup: "group-A",
		Conditions:       &model.MinTransactionCondition{MinAmount: 0},
		Effect:           model.Effect{Type: model.EffectFixed, Value: 2000},
	}
	store.Save(r1)
	store.Save(r2)

	ctx := context.Background()
	validated, _ := eval.EvaluateActiveRules(ctx, model.EvalContext{
		CartTotal: 50000,
		UserID:    "user-ex",
	})
	result := resolver.Resolve(ctx, validated, model.EvalContext{
		CartTotal: 50000,
		UserID:    "user-ex",
	})

	if len(result.AppliedRules) != 1 {
		t.Fatalf("expected 1 applied (best from group), got %d", len(result.AppliedRules))
	}
	if result.AppliedRules[0].RuleName != "better-rule" {
		t.Errorf("expected better-rule, got %s", result.AppliedRules[0].RuleName)
	}
	if result.TotalDiscount != 10000 {
		t.Errorf("expected discount 10000, got %.2f", result.TotalDiscount)
	}
}

