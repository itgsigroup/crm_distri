package health

import "testing"

func TestComputeCases(t *testing.T) {
	cases := []struct {
		name string
		in   Components
		want int
		band string
	}{
		// RSUD breakdown from the fixture: acceptance requires 41 ± 3.
		{"rsud at risk", Components{28, 15, 40, 85, 55}, 43, "bad"},
		{"bsd verbal commit", Components{90, 75, 85, 95, 90}, 87, "good"},
		{"sleman attention", Components{60, 45, 55, 85, 70}, 63, "warn"},
		{"empty", Components{}, 0, "bad"},
	}
	for _, c := range cases {
		got := Compute(c.in)
		if got.Health != c.want || got.Band != c.band {
			t.Errorf("%s: got %d/%s want %d/%s", c.name, got.Health, got.Band, c.want, c.band)
		}
	}
	if h := Compute(Components{28, 15, 40, 85, 55}).Health; h < 38 || h > 44 {
		t.Fatalf("RSUD health %d outside 41±3", h)
	}
}

func TestComponentScorers(t *testing.T) {
	if e := Engagement(EngagementInput{Interactions30d: 1, NormalRhythmDays: 5, DaysSinceLast: 16}); e > 20 {
		t.Errorf("silent account should score low engagement, got %d", e)
	}
	if e := Engagement(EngagementInput{Interactions30d: 10, NormalRhythmDays: 5, DaysSinceLast: 1}); e != 100 {
		t.Errorf("busy account engagement = %d", e)
	}
	single := Multithreading([]Contact{{Tag: "champion", Active: true, Strength: 3}})
	if single > 20 {
		t.Errorf("single-threaded must cap at 20, got %d", single)
	}
	multi := Multithreading([]Contact{{Tag: "champion", Active: true, Strength: 3}, {Tag: "decision", Active: true, Strength: 1}})
	if multi <= single+30-1 {
		t.Errorf("decision-maker should add 30: single=%d multi=%d", single, multi)
	}
	if m := Momentum(MomentumInput{TheirCommitsLate: 2}); m >= 50 {
		t.Errorf("late commitments should reduce momentum, got %d", m)
	}
	if s := Sentiment([]float64{1, 1}); s != 100 {
		t.Errorf("sentiment = %d", s)
	}
}
