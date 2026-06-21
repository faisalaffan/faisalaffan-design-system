package service

import (
	"context"
	"sort"

	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/repository"
)

// ---------------------------------------------------------------------------
// PromoResolver — applies best promos after validation
// ---------------------------------------------------------------------------

// CounterReserver is the interface for reserving usage counters & budget.
type CounterReserver interface {
	IncrementAndCheck(ctx context.Context, ruleID, userID string, limits repository.LimitParams) repository.LimitResult
	BudgetReserve(ctx context.Context, ruleID string, amount, cap float64) repository.LimitResult
}

// PromoResolver sorts validated rules, resolves exclusivity, selects best
// discounts per group, and applies stackable rules sequentially.
type PromoResolver struct {
	counter CounterReserver
}

// NewPromoResolver creates a resolver.
func NewPromoResolver(counter CounterReserver) *PromoResolver {
	return &PromoResolver{counter: counter}
}

// ResolveResult carries the final set of applied promotions.
type ResolveResult struct {
	AppliedRules  []AppliedRule `json:"applied_rules"`
	FinalAmount   float64       `json:"final_amount"`
	TotalDiscount float64       `json:"total_discount"`
}

type AppliedRule struct {
	RuleID         string       `json:"rule_id"`
	RuleName       string       `json:"rule_name"`
	Effect         model.Effect `json:"effect"`
	DiscountAmount float64      `json:"discount_amount"`
}

// Resolve sorts validated rules by priority, resolves exclusivity groups,
// picks the best discount per group, then applies stackable rules sequentially.
func (r *PromoResolver) Resolve(ctx context.Context, validated []*model.Rule, cartCtx model.EvalContext) ResolveResult {
	if len(validated) == 0 {
		return ResolveResult{
			FinalAmount: cartCtx.CartTotal,
		}
	}

	// 1. Sort by priority descending
	sort.Slice(validated, func(i, j int) bool {
		return validated[i].Priority > validated[j].Priority
	})

	// 2. Split into exclusive groups and stackable (non-exclusive) rules
	type groupRule struct {
		rule *model.Rule
		discount float64
	}
	groups := make(map[string][]groupRule) // key = ExclusivityGroup
	var stackable []*model.Rule

	for _, rule := range validated {
		if rule.ExclusivityGroup != "" {
			disc := rule.Effect.CalculateDiscount(cartCtx.CartTotal)
			groups[rule.ExclusivityGroup] = append(groups[rule.ExclusivityGroup], groupRule{rule: rule, discount: disc})
		} else {
			stackable = append(stackable, rule)
		}
	}

	var applied []AppliedRule
	remaining := cartCtx.CartTotal

	// 3. For each exclusivity group, pick the best (max discount)
	for _, grp := range groups {
		if len(grp) == 0 {
			continue
		}
		// sort by discount desc
		sort.Slice(grp, func(i, j int) bool {
			return grp[i].discount > grp[j].discount
		})
		best := grp[0]

		// attempt counter reservation
		if ok := r.tryReserve(ctx, best.rule, cartCtx.UserID, best.rule.Effect.CalculateDiscount(remaining)); !ok {
			continue
		}

		finalDisc := best.rule.Effect.CalculateDiscount(remaining)
		applied = append(applied, AppliedRule{
			RuleID:         best.rule.ID,
			RuleName:       best.rule.Name,
			Effect:         best.rule.Effect,
			DiscountAmount: model.Round2(finalDisc),
		})
		remaining = model.Round2(remaining - finalDisc)
	}

	// 4. Apply stackable rules sequentially up to MaxStackable
	stackApplied := 0
	for _, rule := range stackable {
		if !rule.Stackable {
			continue
		}
		if rule.MaxStackable > 0 && stackApplied >= rule.MaxStackable {
			continue
		}

		disc := rule.Effect.CalculateDiscount(remaining)

		// attempt counter reservation
		if ok := r.tryReserve(ctx, rule, cartCtx.UserID, disc); !ok {
			continue
		}

		applied = append(applied, AppliedRule{
			RuleID:         rule.ID,
			RuleName:       rule.Name,
			Effect:         rule.Effect,
			DiscountAmount: model.Round2(disc),
		})
		remaining = model.Round2(remaining - disc)
		stackApplied++
	}

	totalDiscount := model.Round2(cartCtx.CartTotal - remaining)
	if remaining < 0 {
		remaining = 0
	}

	return ResolveResult{
		AppliedRules:  applied,
		FinalAmount:   remaining,
		TotalDiscount: totalDiscount,
	}
}

// tryReserve attempts to atomically increment usage counters and reserve budget.
// Returns false if any limit is hit.
func (r *PromoResolver) tryReserve(ctx context.Context, rule *model.Rule, userID string, discountAmount float64) bool {
	if r.counter == nil {
		return true
	}

	if rule.Limits.HasAny() {
		lp := repository.LimitParams{
			MaxTotal:   rule.Limits.MaxTotal,
			MaxDaily:   rule.Limits.MaxDaily,
			MaxPerUser: rule.Limits.MaxPerUser,
		}
		res := r.counter.IncrementAndCheck(ctx, rule.ID, userID, lp)
		if !res.Allowed {
			return false
		}
	}

	return true
}
