package handler

import (
	"context"
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/service"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Handler wires HTTP endpoints to the service layer.
// ---------------------------------------------------------------------------

type Handler struct {
	evaluator *service.RuleEvaluator
	resolver  *service.PromoResolver
	store     *repository.RuleStore
	counter   *repository.RedisCounter
}

// New creates a new Handler.
func New(evaluator *service.RuleEvaluator, resolver *service.PromoResolver, store *repository.RuleStore, counter *repository.RedisCounter) *Handler {
	return &Handler{
		evaluator: evaluator,
		resolver:  resolver,
		store:     store,
		counter:   counter,
	}
}

// RegisterRoutes registers all promo-engine endpoints on the given engine.
func (h *Handler) RegisterRoutes(g gin.IRouter) {
	g.POST("/validate", h.Validate)
	g.POST("/apply", h.Apply)
	g.POST("/admin/rules", h.CreateRule)
}

// ---------------------------------------------------------------------------
// POST /validate — evaluate all active promos for a cart (no consumption)
// ---------------------------------------------------------------------------

func (h *Handler) Validate(c *gin.Context) {
	var req model.ValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "invalid request body: "+err.Error())
		return
	}
	req.Cart.UserID = req.UserID

	ctx := context.Background()
	rules, err := h.evaluator.EvaluateActiveRules(ctx, req.Cart)
	if err != nil {
		kit.InternalError(c, "evaluation failed: "+err.Error())
		return
	}

	results := make([]model.EvalResult, len(rules))
	for i, r := range rules {
		disc := r.Effect.CalculateDiscount(req.Cart.CartTotal)
		results[i] = model.EvalResult{
			RuleID:         r.ID,
			RuleName:       r.Name,
			Effect:         r.Effect,
			DiscountAmount: model.Round2(disc),
			Approved:       true,
		}
	}

	kit.OK(c, model.ValidateResponse{
		EligibleRules: results,
		TotalRules:    len(rules),
	})
}

// ---------------------------------------------------------------------------
// POST /apply — evaluate, resolve exclusivity, apply best promos, consume
// ---------------------------------------------------------------------------

func (h *Handler) Apply(c *gin.Context) {
	var req model.ApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "invalid request body: "+err.Error())
		return
	}
	req.Cart.UserID = req.UserID

	ctx := context.Background()
	validated, err := h.evaluator.EvaluateActiveRules(ctx, req.Cart)
	if err != nil {
		kit.InternalError(c, "evaluation failed: "+err.Error())
		return
	}

	result := h.resolver.Resolve(ctx, validated, req.Cart)

	resp := model.ApplyResponse{
		FinalAmount:   result.FinalAmount,
		TotalDiscount: result.TotalDiscount,
		AppliedRules:  make([]model.EvalResult, len(result.AppliedRules)),
	}
	for i, ar := range result.AppliedRules {
		resp.AppliedRules[i] = model.EvalResult{
			RuleID:         ar.RuleID,
			RuleName:       ar.RuleName,
			Effect:         ar.Effect,
			DiscountAmount: ar.DiscountAmount,
			Approved:       true,
		}
	}

	kit.OK(c, resp)
}

// ---------------------------------------------------------------------------
// POST /admin/rules — create a new rule
// ---------------------------------------------------------------------------

func (h *Handler) CreateRule(c *gin.Context) {
	var req model.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "invalid request body: "+err.Error())
		return
	}
	if len(req.ConditionsRaw) == 0 {
		kit.BadRequest(c, "conditions is required")
		return
	}
	if req.Name == "" {
		kit.BadRequest(c, "name is required")
		return
	}

	cond, err := model.UnmarshalConditionRaw(req.ConditionsRaw)
	if err != nil {
		kit.BadRequest(c, "invalid conditions: "+err.Error())
		return
	}

	rule := &model.Rule{
		Name:             req.Name,
		Priority:         req.Priority,
		Conditions:       cond,
		Effect:           req.Effect,
		Limits:           req.Limits,
		Stackable:        req.Stackable,
		ExclusivityGroup: req.ExclusivityGroup,
		MaxStackable:     req.MaxStackable,
		StartAt:          req.StartAt,
		EndAt:            req.EndAt,
	}

	if err := h.store.Save(rule); err != nil {
		kit.InternalError(c, "failed to save rule: "+err.Error())
		return
	}

	kit.Created(c, rule)
}

// make handler import net/http
var _ = http.StatusOK
