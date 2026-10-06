package api_test

import (
	"math"
	"testing"
)

// Stage 09 acceptance: prediksi kas masuk 30 hari ≈ Rp 1,02 M on the seed (±5%), with the dealers ordered and
// weighted as the mockup (Graha ±55%, Mitra — asked for more time — ±45%).
func TestCreditForecastOnSeed(t *testing.T) {
	srv, _ := chatServer(t)
	_, ov := get(t, srv, "/api/credit/overview", "sam@gsi.co.id")
	total := ov["forecast_30"].(float64)
	if math.Abs(total-1.02e9)/1.02e9 > 0.05 {
		t.Fatalf("forecast %.0f, want ≈ 1,02 M", total)
	}
	_, fc := get(t, srv, "/api/credit/forecast?days=30", "sam@gsi.co.id")
	prob := map[string]float64{}
	for _, x := range fc["items"].([]any) {
		m := x.(map[string]any)
		prob[m["dealer_id"].(string)] = m["probability"].(float64)
	}
	for slug, want := range map[string]float64{"indo": 0.92, "sinar": 0.90, "graha": 0.55, "mitra": 0.45} {
		if math.Abs(prob[slug]-want) > 0.06 {
			t.Errorf("%s probability %.2f, want ≈ %.2f", slug, prob[slug], want)
		}
	}
}
