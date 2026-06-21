package service

import (
	"hash/fnv"
	"log"
)

// Experiment defines an A/B test experiment with traffic allocation and variants.
type Experiment struct {
	Name       string
	TrafficPct int // 0–100: percentage of users in experiment
	Variants   []Variant
}

// Variant defines a single variant within an experiment.
type Variant struct {
	Name   string
	Weight int // relative weight within experiment traffic
}

// ABTest provides deterministic hash-based A/B test bucketing.
type ABTest struct {
	experiments map[string]*Experiment
}

// NewABTest creates a new ABTest with the default experiments pre-configured.
func NewABTest() *ABTest {
	return &ABTest{
		experiments: map[string]*Experiment{
			"surge_v2": {
				Name:       "surge_v2",
				TrafficPct: 50,
				Variants: []Variant{
					{Name: "control", Weight: 50},
					{Name: "treatment", Weight: 50},
				},
			},
		},
	}
}

// GetVariant returns the variant assigned to a user for the given experiment.
// Assignment is deterministic: the same user+experiment always gets the same variant.
// Users outside the traffic percentage always get "control".
// Results are logged for downstream analysis.
func (t *ABTest) GetVariant(userID, experimentName string) string {
	exp, ok := t.experiments[experimentName]
	if !ok {
		return "control"
	}

	h := fnv.New32a()
	h.Write([]byte(userID + experimentName))
	bucket := int(h.Sum32() % 100)

	if bucket >= exp.TrafficPct {
		t.logAssignment(experimentName, userID, "control")
		return "control"
	}

	// Map bucket [0, TrafficPct) into [0, 100) for variant weighting
	pct := bucket * 100 / exp.TrafficPct

	cumulative := 0
	for _, v := range exp.Variants {
		cumulative += v.Weight
		if pct < cumulative {
			t.logAssignment(experimentName, userID, v.Name)
			return v.Name
		}
	}

	t.logAssignment(experimentName, userID, "control")
	return "control"
}

func (t *ABTest) logAssignment(experimentName, userID, variant string) {
	log.Printf("abtest: experiment=%s user=%s variant=%s", experimentName, userID, variant)
}
