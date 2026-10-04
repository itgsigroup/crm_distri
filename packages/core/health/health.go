// Package health implements the v1 health score (docs/knowledge/02-domain-model.md):
//
//	health = 0.25·engagement + 0.20·multithreading + 0.20·momentum + 0.20·fit + 0.15·sentiment
//
// Component scorers are pure functions so they can be unit-tested and reused by
// the scoring job.
package health

import "math"

// Weights of each component.
var Weights = map[string]float64{
	"engagement":     0.25,
	"multithreading": 0.20,
	"momentum":       0.20,
	"fit":            0.20,
	"sentiment":      0.15,
}

// Components is the 0–100 value of each component.
type Components struct {
	Engagement     int `json:"engagement"`
	Multithreading int `json:"multithreading"`
	Momentum       int `json:"momentum"`
	Fit            int `json:"fit"`
	Sentiment      int `json:"sentiment"`
}

// Result is the health score with its breakdown and band.
type Result struct {
	Health     int        `json:"health"`
	Band       string     `json:"band"`
	Components Components `json:"components"`
}

// Compute applies the weighted formula and rounds half up.
func Compute(c Components) Result {
	v := Weights["engagement"]*float64(c.Engagement) +
		Weights["multithreading"]*float64(c.Multithreading) +
		Weights["momentum"]*float64(c.Momentum) +
		Weights["fit"]*float64(c.Fit) +
		Weights["sentiment"]*float64(c.Sentiment)
	h := int(math.Floor(v + 0.5))
	return Result{Health: clamp(h), Band: band(h), Components: c}
}

// Get returns a component by key.
func (c Components) Get(key string) int {
	switch key {
	case "engagement":
		return c.Engagement
	case "multithreading":
		return c.Multithreading
	case "momentum":
		return c.Momentum
	case "fit":
		return c.Fit
	case "sentiment":
		return c.Sentiment
	}
	return 0
}

// Set assigns a component by key.
func (c *Components) Set(key string, v int) {
	v = clamp(v)
	switch key {
	case "engagement":
		c.Engagement = v
	case "multithreading":
		c.Multithreading = v
	case "momentum":
		c.Momentum = v
	case "fit":
		c.Fit = v
	case "sentiment":
		c.Sentiment = v
	}
}

// EngagementInput: interaction frequency in the last 30 days versus the
// account's normal rhythm (median gap between interactions, last 90 days).
type EngagementInput struct {
	Interactions30d  int
	NormalRhythmDays int
	DaysSinceLast    int
}

// Engagement scores frequency against rhythm, penalising silence beyond rhythm.
func Engagement(in EngagementInput) int {
	rhythm := in.NormalRhythmDays
	if rhythm <= 0 {
		rhythm = 7
	}
	expected := 30.0 / float64(rhythm)
	freq := math.Min(1, float64(in.Interactions30d)/expected)
	score := 100 * freq
	if in.DaysSinceLast > rhythm {
		score *= float64(rhythm) / float64(in.DaysSinceLast)
	}
	return clamp(int(math.Round(score)))
}

// Contact is an active stakeholder considered for multithreading.
type Contact struct {
	Tag      string
	Active   bool // ≥ 1 interaction in 30 days
	Strength int  // 0–3
}

// Multithreading weights active contacts by tag; a single contact caps at 20 and
// an active decision-maker adds 30.
func Multithreading(contacts []Contact) int {
	tagWeight := map[string]float64{"decision": 1.0, "champion": 0.9, "influencer": 0.7, "procurement": 0.7, "user": 0.5, "ghost": 0.2, "former": 0.1}
	active := 0
	score := 0.0
	decision := false
	for _, c := range contacts {
		if !c.Active {
			continue
		}
		active++
		w := tagWeight[c.Tag]
		if w == 0 {
			w = 0.4
		}
		score += w * (10 + 5*float64(c.Strength))
		if c.Tag == "decision" {
			decision = true
		}
	}
	if active == 0 {
		return 0
	}
	if active == 1 {
		score = math.Min(score, 20)
	}
	if decision {
		score += 30
	}
	return clamp(int(math.Round(score)))
}

// MomentumInput captures the 14-day trend and commitment outcomes.
type MomentumInput struct {
	Trend14          float64 // change in weekly interaction count (−1..+1 ratio)
	TheirCommitsKept int
	TheirCommitsLate int
	OurCommitsKept   int
	OurCommitsLate   int
}

// Momentum starts at 50, adds the trend and rewards/penalises commitments.
func Momentum(in MomentumInput) int {
	s := 50 + 30*math.Max(-1, math.Min(1, in.Trend14))
	s += 10*float64(in.TheirCommitsKept) - 15*float64(in.TheirCommitsLate)
	s += 5*float64(in.OurCommitsKept) - 10*float64(in.OurCommitsLate)
	return clamp(int(math.Round(s)))
}

// Sentiment averages sentiment of the last interactions (−1..1) into 0–100.
func Sentiment(scores []float64) int {
	if len(scores) == 0 {
		return 50
	}
	sum := 0.0
	for _, s := range scores {
		sum += s
	}
	avg := sum / float64(len(scores))
	return clamp(int(math.Round((avg + 1) * 50)))
}

func band(h int) string {
	switch {
	case h >= 70:
		return "good"
	case h >= 50:
		return "warn"
	default:
		return "bad"
	}
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
