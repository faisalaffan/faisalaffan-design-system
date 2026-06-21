package repository

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/promo-engine/model"
	"github.com/google/uuid"
)

// RuleStore is an in-memory rule repository with CRUD operations.
// Rules are stored as JSON bytes for extensibility — loaded rules are cached
// as parsed model.Rule on read.
type RuleStore struct {
	mu    sync.RWMutex
	rules map[string]*model.Rule // keyed by rule ID
}

// NewRuleStore creates an empty rule store.
func NewRuleStore() *RuleStore {
	return &RuleStore{
		rules: make(map[string]*model.Rule),
	}
}

// Save persists a rule, generating an ID if missing.
func (s *RuleStore) Save(rule *model.Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	if rule.Status == "" {
		rule.Status = model.RuleActive
	}

	s.rules[rule.ID] = rule
	return nil
}

// Get retrieves a single rule by ID.
func (s *RuleStore) Get(id string) (*model.Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.rules[id]
	if !ok {
		return nil, fmt.Errorf("rule %s: not found", id)
	}
	return r, nil
}

// List returns all rules. Optionally filter to active rules only.
func (s *RuleStore) List(activeOnly bool) []*model.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().UTC()
	out := make([]*model.Rule, 0, len(s.rules))
	for _, r := range s.rules {
		if activeOnly && !r.IsActive(now) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Update replaces an existing rule by ID. Returns error if not found.
func (s *RuleStore) Update(id string, rule *model.Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rules[id]; !ok {
		return fmt.Errorf("rule %s: not found", id)
	}
	rule.ID = id
	s.rules[id] = rule
	return nil
}

// Delete removes a rule by ID. No-op if not found.
func (s *RuleStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rules, id)
}

// MarshalJSON dumps all rules as JSON (for debugging / admin).
func (s *RuleStore) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*model.Rule, 0, len(s.rules))
	for _, r := range s.rules {
		list = append(list, r)
	}
	return json.Marshal(list)
}
