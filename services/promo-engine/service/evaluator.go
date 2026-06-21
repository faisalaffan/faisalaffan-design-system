package service

import (
	"context"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/repository"
)

// ---------------------------------------------------------------------------
// RuleEvaluator — evaluates condition trees against cart context
// ---------------------------------------------------------------------------

// RuleStoreReader is the minimal interface the evaluator needs from a rule store.
type RuleStoreReader interface {
	List(activeOnly bool) []*model.Rule
	Get(id string) (*model.Rule, error)
}

// LimitChecker is the minimal interface for usage-limit checks (Redis or mock).
type LimitChecker interface {
	IncrementAndCheck(ctx context.Context, ruleID, userID string, limits repository.LimitParams) repository.LimitResult
	CheckAllLimits(ctx context.Context, ruleID, userID string, limits repository.LimitParams) repository.LimitResult
}

// RuleEvaluator fetches & filters rules and evaluates their condition trees.
type RuleEvaluator struct {
	store  RuleStoreReader
	limits LimitChecker
}

// NewRuleEvaluator creates a new evaluator.
func NewRuleEvaluator(store RuleStoreReader, limits LimitChecker) *RuleEvaluator {
	return &RuleEvaluator{store: store, limits: limits}
}

// NewRuleEvaluatorNoop creates an evaluator that skips limit checks (e.g. for admin validation).
func NewRuleEvaluatorNoop(store RuleStoreReader) *RuleEvaluator {
	return &RuleEvaluator{store: store, limits: nil}
}

// EvaluateActiveRules returns all rules that:
//  1. Are within their time window (IsActive)
//  2. Have a condition tree that evaluates to true for the given context
//  3. Pass usage-limit checks (read-only, no increment)
func (ev *RuleEvaluator) EvaluateActiveRules(ctx context.Context, cartCtx model.EvalContext) ([]*model.Rule, error) {
	if cartCtx.Now.IsZero() {
		cartCtx.Now = time.Now().UTC()
	}

	all := ev.store.List(true) // active only
	passed := make([]*model.Rule, 0, len(all))

	for _, rule := range all {
		// time window (already filtered by store.List(true), but double-check)
		if !rule.IsActive(cartCtx.Now) {
			continue
		}

		// condition tree evaluation with short-circuit
		if rule.Conditions != nil && !rule.Conditions.Eval(cartCtx) {
			continue
		}

		// usage-limit read-only check
		if rule.Limits.HasAny() && ev.limits != nil {
			lp := repository.LimitParams{
				MaxTotal:   rule.Limits.MaxTotal,
				MaxDaily:   rule.Limits.MaxDaily,
				MaxPerUser: rule.Limits.MaxPerUser,
			}
			res := ev.limits.CheckAllLimits(ctx, rule.ID, cartCtx.UserID, lp)
			if !res.Allowed {
				continue
			}
		}

		passed = append(passed, rule)
	}

	return passed, nil
}

// EvaluateSingleRule evaluates one specific rule against the given context.
// Returns nil if the rule is not found, inactive, or fails conditions.
func (ev *RuleEvaluator) EvaluateSingleRule(ctx context.Context, ruleID string, cartCtx model.EvalContext) (*model.Rule, error) {
	if cartCtx.Now.IsZero() {
		cartCtx.Now = time.Now().UTC()
	}

	rule, err := ev.store.Get(ruleID)
	if err != nil {
		return nil, nil // nolint: nilnil — caller treats nil as "not applicable"
	}

	if !rule.IsActive(cartCtx.Now) {
		return nil, nil
	}

	if rule.Conditions != nil && !rule.Conditions.Eval(cartCtx) {
		return nil, nil
	}

	return rule, nil
}

// Ensure time imported.
var _ = time.Second
