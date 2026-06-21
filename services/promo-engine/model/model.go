package model

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Core types
// ---------------------------------------------------------------------------

type EffectType string

const (
	EffectPercentage  EffectType = "percentage"
	EffectFixed       EffectType = "fixed"
	EffectFreeShipping EffectType = "free_shipping"
	EffectCashback    EffectType = "cashback"
)

type RuleStatus string

const (
	RuleActive   RuleStatus = "active"
	RuleInactive RuleStatus = "inactive"
	RuleExpired  RuleStatus = "expired"
)

// ---------------------------------------------------------------------------
// Effect
// ---------------------------------------------------------------------------

type Effect struct {
	Type  EffectType `json:"type"`
	Value float64    `json:"value"` // percentage (0-100) or fixed amount in cents
}

func (e Effect) CalculateDiscount(amount float64) float64 {
	switch e.Type {
	case EffectPercentage:
		return amount * e.Value / 100.0
	case EffectFixed:
		if e.Value > amount {
			return amount
		}
		return e.Value
	case EffectFreeShipping:
		return 0 // shipping cost is computed by caller
	case EffectCashback:
		return e.Value // flat cashback amount
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// Condition — AST nodes
// ---------------------------------------------------------------------------

type ConditionType string

const (
	CondAND            ConditionType = "and"
	CondOR             ConditionType = "or"
	CondNOT            ConditionType = "not"
	CondMinTransaction ConditionType = "min_transaction"
	CondArea           ConditionType = "area"
	CondCategory       ConditionType = "category"
	CondFirstNOrder    ConditionType = "first_n_order"
	CondTimeSlot       ConditionType = "time_slot"
)

type Condition interface {
	Type() ConditionType
	Eval(ctx EvalContext) bool
}

// --- composite nodes ---

type CompositeCondition struct {
	CondType   ConditionType `json:"type"`
	Children   []Condition   `json:"conditions,omitempty"`    // AND / OR
	Child      Condition     `json:"condition,omitempty"`     // NOT (single child)
}

func (c *CompositeCondition) Type() ConditionType { return c.CondType }

func (c *CompositeCondition) Eval(ctx EvalContext) bool {
	switch c.CondType {
	case CondAND:
		for _, child := range c.Children {
			if !child.Eval(ctx) {
				return false // short-circuit
			}
		}
		return true
	case CondOR:
		for _, child := range c.Children {
			if child.Eval(ctx) {
				return true // short-circuit
			}
		}
		return false
	case CondNOT:
		if c.Child == nil {
			return true
		}
		return !c.Child.Eval(ctx)
	default:
		return false
	}
}

// --- leaf nodes ---

type MinTransactionCondition struct {
	MinAmount float64 `json:"min_amount"`
}

func (m *MinTransactionCondition) Type() ConditionType { return CondMinTransaction }
func (m *MinTransactionCondition) Eval(ctx EvalContext) bool {
	return ctx.CartTotal >= m.MinAmount
}

type AreaCondition struct {
	Areas []string `json:"areas"`
}

func (a *AreaCondition) Type() ConditionType { return CondArea }
func (a *AreaCondition) Eval(ctx EvalContext) bool {
	for _, allowed := range a.Areas {
		if strings.EqualFold(allowed, ctx.Area) {
			return true
		}
	}
	return false
}

type CategoryCondition struct {
	Categories []string `json:"categories"`
}

func (c *CategoryCondition) Type() ConditionType { return CondCategory }
func (c *CategoryCondition) Eval(ctx EvalContext) bool {
	for _, cat := range c.Categories {
		for _, itemCat := range ctx.Categories {
			if strings.EqualFold(cat, itemCat) {
				return true
			}
		}
	}
	return len(c.Categories) == 0 // empty means unrestricted
}

type FirstNOrderCondition struct {
	MaxOrders int `json:"max_orders"`
}

func (f *FirstNOrderCondition) Type() ConditionType { return CondFirstNOrder }
func (f *FirstNOrderCondition) Eval(ctx EvalContext) bool {
	return ctx.UserOrderCount <= f.MaxOrders
}

type TimeSlotCondition struct {
	Start string         `json:"start"` // "HH:MM" in 24h
	End   string         `json:"end"`   // "HH:MM"
	Days  []time.Weekday `json:"days"`  // 0=Sunday, 6=Saturday
}

func (t *TimeSlotCondition) Type() ConditionType { return CondTimeSlot }
func (t *TimeSlotCondition) Eval(ctx EvalContext) bool {
	now := ctx.Now
	if now.IsZero() {
		now = time.Now()
	}
	// day check
	if len(t.Days) > 0 {
		matched := false
		for _, d := range t.Days {
			if now.Weekday() == d {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	// time check
	if t.Start != "" && t.End != "" {
		startMin := parseMinutes(t.Start)
		endMin := parseMinutes(t.End)
		curMin := now.Hour()*60 + now.Minute()
		if startMin <= endMin {
			// normal interval, e.g. 08:00-18:00
			if curMin < startMin || curMin > endMin {
				return false
			}
		} else {
			// overnight, e.g. 22:00-06:00
			if curMin < startMin && curMin > endMin {
				return false
			}
		}
	}
	return true
}

func parseMinutes(hhmm string) int {
	parts := strings.Split(hhmm, ":")
	if len(parts) != 2 {
		return 0
	}
	h, m := 0, 0
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	return h*60 + m
}

// ---------------------------------------------------------------------------
// Usage limits
// ---------------------------------------------------------------------------

type UsageLimits struct {
	MaxTotal   int `json:"max_total,omitempty"`
	MaxDaily   int `json:"max_daily,omitempty"`
	MaxPerUser int `json:"max_per_user,omitempty"`
}

func (u UsageLimits) HasAny() bool {
	return u.MaxTotal > 0 || u.MaxDaily > 0 || u.MaxPerUser > 0
}

// ---------------------------------------------------------------------------
// Rule
// ---------------------------------------------------------------------------

type Rule struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Priority        int          `json:"priority"`        // higher = more important
	Conditions      Condition    `json:"conditions"`       // AST root
	Effect          Effect       `json:"effect"`
	Limits          UsageLimits  `json:"limits,omitempty"`
	Stackable       bool         `json:"stackable"`
	ExclusivityGroup string      `json:"exclusivity_group,omitempty"` // empty = no exclusivity
	MaxStackable    int          `json:"max_stackable,omitempty"`     // max stackable rules in same group
	StartAt         time.Time    `json:"start_at,omitempty"`
	EndAt           time.Time    `json:"end_at,omitempty"`
	Status          RuleStatus   `json:"status"`
	CreatedAt       time.Time    `json:"created_at"`
}

// IsActive returns true if the rule is within its time window and has active status.
func (r *Rule) IsActive(now time.Time) bool {
	if r.Status != RuleActive {
		return false
	}
	if !r.StartAt.IsZero() && now.Before(r.StartAt) {
		return false
	}
	if !r.EndAt.IsZero() && now.After(r.EndAt) {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Condition JSON deserialization (type discriminator)
// ---------------------------------------------------------------------------

type conditionJSON struct {
	Type       ConditionType    `json:"type"`
	Conditions []json.RawMessage `json:"conditions,omitempty"`
	Condition  *json.RawMessage `json:"condition,omitempty"`
	MinAmount  *float64         `json:"min_amount,omitempty"`
	Areas      []string         `json:"areas,omitempty"`
	Categories []string         `json:"categories,omitempty"`
	MaxOrders  *int             `json:"max_orders,omitempty"`
	Start      string           `json:"start,omitempty"`
	End        string           `json:"end,omitempty"`
	Days       []time.Weekday   `json:"days,omitempty"`
}

func unmarshalCondition(raw json.RawMessage) (Condition, error) {
	var base struct {
		Type ConditionType `json:"type"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil, fmt.Errorf("condition type discriminator: %w", err)
	}

	var aux conditionJSON
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("condition payload: %w", err)
	}

	switch base.Type {
	case CondAND, CondOR:
		comp := &CompositeCondition{CondType: base.Type}
		for _, childRaw := range aux.Conditions {
			child, err := unmarshalCondition(childRaw)
			if err != nil {
				return nil, fmt.Errorf("and/or child: %w", err)
			}
			comp.Children = append(comp.Children, child)
		}
		return comp, nil

	case CondNOT:
		comp := &CompositeCondition{CondType: CondNOT}
		if aux.Condition != nil {
			child, err := unmarshalCondition(*aux.Condition)
			if err != nil {
				return nil, fmt.Errorf("not child: %w", err)
			}
			comp.Child = child
		}
		return comp, nil

	case CondMinTransaction:
		if aux.MinAmount == nil {
			return nil, fmt.Errorf("min_transaction: missing min_amount")
		}
		return &MinTransactionCondition{MinAmount: *aux.MinAmount}, nil

	case CondArea:
		return &AreaCondition{Areas: aux.Areas}, nil

	case CondCategory:
		return &CategoryCondition{Categories: aux.Categories}, nil

	case CondFirstNOrder:
		if aux.MaxOrders == nil {
			return nil, fmt.Errorf("first_n_order: missing max_orders")
		}
		return &FirstNOrderCondition{MaxOrders: *aux.MaxOrders}, nil

	case CondTimeSlot:
		return &TimeSlotCondition{Start: aux.Start, End: aux.End, Days: aux.Days}, nil

	default:
		return nil, fmt.Errorf("unknown condition type: %s", base.Type)
	}
}

// ---------------------------------------------------------------------------
// Custom JSON for Rule (conditions need special handling)
// ---------------------------------------------------------------------------

type ruleJSON struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Priority         int             `json:"priority"`
	Conditions       json.RawMessage `json:"conditions"`
	Effect           Effect          `json:"effect"`
	Limits           UsageLimits     `json:"limits,omitempty"`
	Stackable        bool            `json:"stackable"`
	ExclusivityGroup string          `json:"exclusivity_group,omitempty"`
	MaxStackable     int             `json:"max_stackable,omitempty"`
	StartAt          time.Time       `json:"start_at,omitempty"`
	EndAt            time.Time       `json:"end_at,omitempty"`
	Status           RuleStatus      `json:"status"`
	CreatedAt        time.Time       `json:"created_at"`
}

func (r *Rule) UnmarshalJSON(data []byte) error {
	var aux ruleJSON
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	cond, err := unmarshalCondition(aux.Conditions)
	if err != nil {
		return fmt.Errorf("rule %s: %w", aux.ID, err)
	}
	r.ID = aux.ID
	r.Name = aux.Name
	r.Priority = aux.Priority
	r.Conditions = cond
	r.Effect = aux.Effect
	r.Limits = aux.Limits
	r.Stackable = aux.Stackable
	r.ExclusivityGroup = aux.ExclusivityGroup
	r.MaxStackable = aux.MaxStackable
	r.StartAt = aux.StartAt
	r.EndAt = aux.EndAt
	r.Status = aux.Status
	r.CreatedAt = aux.CreatedAt
	return nil
}

func (r *Rule) MarshalJSON() ([]byte, error) {
	condRaw, err := marshalCondition(r.Conditions)
	if err != nil {
		return nil, err
	}
	return json.Marshal(ruleJSON{
		ID:               r.ID,
		Name:             r.Name,
		Priority:         r.Priority,
		Conditions:       condRaw,
		Effect:           r.Effect,
		Limits:           r.Limits,
		Stackable:        r.Stackable,
		ExclusivityGroup: r.ExclusivityGroup,
		MaxStackable:     r.MaxStackable,
		StartAt:          r.StartAt,
		EndAt:            r.EndAt,
		Status:           r.Status,
		CreatedAt:        r.CreatedAt,
	})
}

func marshalCondition(c Condition) (json.RawMessage, error) {
	switch v := c.(type) {
	case *CompositeCondition:
		m := map[string]interface{}{
			"type": v.CondType,
		}
		if len(v.Children) > 0 {
			children := make([]json.RawMessage, len(v.Children))
			for i, child := range v.Children {
				var err error
				children[i], err = marshalCondition(child)
				if err != nil {
					return nil, err
				}
			}
			m["conditions"] = children
		}
		if v.Child != nil {
			childRaw, err := marshalCondition(v.Child)
			if err != nil {
				return nil, err
			}
			m["condition"] = childRaw
		}
		return json.Marshal(m)
	case *MinTransactionCondition:
		return json.Marshal(map[string]interface{}{
			"type":       CondMinTransaction,
			"min_amount": v.MinAmount,
		})
	case *AreaCondition:
		return json.Marshal(map[string]interface{}{
			"type":  CondArea,
			"areas": v.Areas,
		})
	case *CategoryCondition:
		return json.Marshal(map[string]interface{}{
			"type":       CondCategory,
			"categories": v.Categories,
		})
	case *FirstNOrderCondition:
		return json.Marshal(map[string]interface{}{
			"type":       CondFirstNOrder,
			"max_orders": v.MaxOrders,
		})
	case *TimeSlotCondition:
		return json.Marshal(map[string]interface{}{
			"type":  CondTimeSlot,
			"start": v.Start,
			"end":   v.End,
			"days":  v.Days,
		})
	default:
		return nil, fmt.Errorf("unknown condition type for marshal: %T", c)
	}
}

// ---------------------------------------------------------------------------
// EvalContext — what the rule engine evaluates against
// ---------------------------------------------------------------------------

type EvalContext struct {
	UserID         string    `json:"user_id"`
	CartTotal      float64   `json:"cart_total"`
	CartItems      []CartItem `json:"cart_items"`
	Area           string    `json:"area"`
	Categories     []string  `json:"categories"`      // union of all item categories
	UserOrderCount int       `json:"user_order_count"` // total orders placed
	DeviceID       string    `json:"device_id,omitempty"`
	Phone          string    `json:"phone,omitempty"`
	Address        string    `json:"address,omitempty"`
	Now            time.Time `json:"-"`
}

type CartItem struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
	Category string  `json:"category"`
}

// ---------------------------------------------------------------------------
// EvalResult — result returned after evaluation / application
// ---------------------------------------------------------------------------

type EvalResult struct {
	RuleID         string    `json:"rule_id"`
	RuleName       string    `json:"rule_name"`
	Effect         Effect    `json:"effect"`
	DiscountAmount float64   `json:"discount_amount"`
	Approved       bool      `json:"approved"`
	RejectReason   string    `json:"reject_reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Apply / Validate request & response
// ---------------------------------------------------------------------------

type ValidateRequest struct {
	Cart   EvalContext `json:"cart"`
	UserID string      `json:"user_id"`
}

type ValidateResponse struct {
	EligibleRules []EvalResult `json:"eligible_rules"`
	TotalRules    int          `json:"total_rules"`
}

type ApplyRequest struct {
	Cart   EvalContext `json:"cart"`
	UserID string      `json:"user_id"`
}

type ApplyResponse struct {
	AppliedRules  []EvalResult `json:"applied_rules"`
	FinalAmount   float64      `json:"final_amount"`
	TotalDiscount float64      `json:"total_discount"`
}

type CreateRuleRequest struct {
	Name             string      `json:"name"`
	Priority         int         `json:"priority"`
	Conditions       Condition   `json:"-"`
	ConditionsRaw    json.RawMessage `json:"conditions"`
	Effect           Effect      `json:"effect"`
	Limits           UsageLimits `json:"limits,omitempty"`
	Stackable        bool        `json:"stackable"`
	ExclusivityGroup string      `json:"exclusivity_group,omitempty"`
	MaxStackable     int         `json:"max_stackable,omitempty"`
	StartAt          time.Time   `json:"start_at,omitempty"`
	EndAt            time.Time   `json:"end_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Fraud detection
// ---------------------------------------------------------------------------

type FraudConfig struct {
	DeviceHashEnabled       bool    `json:"device_hash_enabled"`
	PhoneHashEnabled        bool    `json:"phone_hash_enabled"`
	AddressJaccardThreshold float64 `json:"address_jaccard_threshold"` // 0.0-1.0; checked if >0
}

type FraudResult struct {
	Fraudulent       bool     `json:"fraudulent"`
	DeviceMatch      float64  `json:"device_match,omitempty"`
	PhoneMatch       bool     `json:"phone_match,omitempty"`
	AddressSimilarity float64 `json:"address_similarity,omitempty"`
	Reasons          []string `json:"reasons,omitempty"`
}

// JaccardSimilarity computes Jaccard index on character bigrams of two strings.
func JaccardSimilarity(a, b string) float64 {
	if a == "" && b == "" {
		return 1.0
	}
	bigramsA := bigramSet(a)
	bigramsB := bigramSet(b)
	if len(bigramsA) == 0 && len(bigramsB) == 0 {
		return 1.0
	}
	intersection := 0
	for bg := range bigramsA {
		if bigramsB[bg] {
			intersection++
		}
	}
	union := len(bigramsA) + len(bigramsB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func bigramSet(s string) map[string]bool {
	s = strings.ToLower(strings.TrimSpace(s))
	set := make(map[string]bool)
	runes := []rune(s)
	for i := 0; i < len(runes)-1; i++ {
		if runes[i] == ' ' || runes[i+1] == ' ' {
			continue
		}
		set[string(runes[i:i+2])] = true
	}
	return set
}

// NormalizedDeviceID returns a hash-like normalized form. In production this
// would be a SHA256; here we use a simple repeatable transform for demo.
func NormalizedDeviceID(deviceID string) string {
	// In production: sha256(deviceID + salt)
	return fmt.Sprintf("fp_%x", hashString(deviceID))
}

// NormalizedPhone returns a consistent hash for phone dedup.
func NormalizedPhone(phone string) string {
	// Strip non-digits
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return fmt.Sprintf("ph_%x", hashString(b.String()))
}

func hashString(s string) uint64 {
	if s == "" {
		return 0
	}
	// FNV-1a 64-bit
	var h uint64 = 14695981039346656037
	for _, r := range s {
		h ^= uint64(r)
		h *= 1099511628211
	}
	return h
}

// Max returns the larger of two float64 values.
func Max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// Min returns the smaller of two float64 values.
func Min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Round2 rounds a float64 to 2 decimal places (cents).
func Round2(val float64) float64 {
	return math.Round(val*100) / 100
}

// UnmarshalConditionRaw is a public wrapper for unmarshaling a Condition from
// a json.RawMessage (used by the handler package).
func UnmarshalConditionRaw(raw json.RawMessage) (Condition, error) {
	return unmarshalCondition(raw)
}

// Ensure utf8 imported
var _ = utf8.ValidString
