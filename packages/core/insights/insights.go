// Package insights computes ARC's read models (forecast, weighted pipeline,
// cash forecast, funnel, win rates, network, team, calibration, business pulse)
// directly from the database. All numbers on the screens come from here.
package insights

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

// Service wraps the database.
type Service struct{ DB *storage.DB }

// New returns the insights service.
func New(db *storage.DB) *Service { return &Service{DB: db} }

// Policy reads a policy value into out; returns false when missing.
func (s *Service) Policy(ctx context.Context, key string, out any) bool {
	var raw []byte
	if err := s.DB.Pool.QueryRow(ctx, `SELECT value FROM policies WHERE key=$1`, key).Scan(&raw); err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

// PolicyFloat reads a numeric policy with a default.
func (s *Service) PolicyFloat(ctx context.Context, key string, def float64) float64 {
	var v float64
	if s.Policy(ctx, key, &v) {
		return v
	}
	return def
}

// Setting reads a settings value into out.
func (s *Service) Setting(ctx context.Context, key string, out any) bool {
	var raw []byte
	if err := s.DB.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key=$1`, key).Scan(&raw); err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

// SetSetting upserts a settings value.
func (s *Service) SetSetting(ctx context.Context, key string, v any) error {
	_, err := s.DB.Pool.Exec(ctx, `INSERT INTO settings(key,value) VALUES ($1,$2) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, key, storage.JSONObj(v))
	return err
}

// Deal is an open (or won) opportunity with its ARC layer, used by many read models.
type Deal struct {
	ID         string
	AccountID  string
	Account    string
	Name       string
	Value      float64
	ManualProb int
	Stage      string
	StageSeq   int
	StageID    int
	Health     int
	HealthOK   bool
	Trend      int
	Signal     string
	Status     string
	Owner      string
	OwnerName  string
	Branch     string
	Gov        bool
	Deadline   *time.Time
	WonAt      *time.Time
	Locked     bool
	StageWhy   string
	NextAction string
	Tags       []string
	Closing    string
	Priority   int
	Activity   string
	Note       string
	Source     string
}

// Scope restricts reads to what a user may see (branch access rules).
type Scope struct {
	All    bool
	Branch string
	UserID string
}

// SQL returns a condition on an accounts alias.
func (sc Scope) SQL(alias string, argN int) (string, []any) {
	if sc.All {
		return "TRUE", nil
	}
	return fmt.Sprintf("(%s.branch = $%d OR %s.owner_user_id = $%d)", alias, argN, alias, argN+1), []any{sc.Branch, sc.UserID}
}

// Deals returns non-historical opportunities (open and won) visible in scope.
func (s *Service) Deals(ctx context.Context, sc Scope) ([]Deal, error) {
	cond, args := sc.SQL("a", 1)
	rows, err := s.DB.Pool.Query(ctx, `
		SELECT o.id, o.account_id, a.name, o.name, o.expected_revenue::float8, o.probability, sd.name, sd.seq, sd.id,
		       COALESCE(o.health,0), o.health IS NOT NULL, o.health_trend_30d, o.signal, o.status,
		       COALESCE(o.owner_user_id,''), COALESCE(u.name,''), a.branch, a.is_government, o.date_deadline, o.won_at,
		       o.locked_to_source, o.stage_evidence, COALESCE(o.next_action_id,''), o.tags, o.closing_label, o.priority,
		       o.activity_state, o.note, o.source
		FROM opportunities o JOIN accounts a ON a.id=o.account_id JOIN stage_definitions sd ON sd.id=o.stage_id
		LEFT JOIN users u ON u.id=o.owner_user_id
		WHERE NOT o.historical AND o.status IN ('open','won') AND `+cond+`
		ORDER BY a.created_at, o.created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deal
	for rows.Next() {
		var d Deal
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Account, &d.Name, &d.Value, &d.ManualProb, &d.Stage, &d.StageSeq, &d.StageID,
			&d.Health, &d.HealthOK, &d.Trend, &d.Signal, &d.Status, &d.Owner, &d.OwnerName, &d.Branch, &d.Gov, &d.Deadline, &d.WonAt,
			&d.Locked, &d.StageWhy, &d.NextAction, &d.Tags, &d.Closing, &d.Priority, &d.Activity, &d.Note, &d.Source); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// IsPipeline reports whether an open deal counts as pipeline (past the "Baru" stage).
func (d Deal) IsPipeline() bool { return d.Status == "open" && d.StageSeq >= 2 && d.HealthOK }

// SignalLabel renders the ARC sub-status.
func SignalLabel(sig string) string {
	switch sig {
	case "negosiasi":
		return "Negosiasi"
	case "verbal_commit":
		return "Verbal commit"
	case "kontrak":
		return "Kontrak"
	case "":
		return ""
	}
	return strings.ToUpper(sig[:1]) + strings.ReplaceAll(sig[1:], "_", " ")
}

// ForecastResult is the evidence-based quarter forecast.
type ForecastResult struct {
	Quarter      int
	QuarterEnd   time.Time
	Commit       float64
	Best         float64
	Pipeline     float64
	Target       float64
	CommitIDs    []string
	BestIDs      []string
	WorkdaysLeft int
	DaysLeft     int
}

// Forecast computes commit / best / pipeline. Commit = Won this quarter + verbal
// commits with written evidence and health ≥ commit_min_health. Manual sales
// probabilities are never used. Deals in `exclude` are treated as not closing (what-if).
func (s *Service) Forecast(ctx context.Context, sc Scope, exclude ...string) (ForecastResult, error) {
	now := domain.Now()
	q, qend := domain.Quarter(now)
	qstart := domain.QuarterStart(now)
	res := ForecastResult{Quarter: q, QuarterEnd: qend, Target: s.PolicyFloat(ctx, "quarter_target", 5e9)}
	res.WorkdaysLeft = domain.WorkdaysUntil(now.AddDate(0, 0, -1), qend)
	res.DaysLeft = domain.DaysBetween(now, qend)
	commitMin := int(s.PolicyFloat(ctx, "commit_min_health", 80))
	bestMin := int(s.PolicyFloat(ctx, "best_min_health", 75))
	ex := map[string]bool{}
	for _, e := range exclude {
		ex[e] = true
	}
	deals, err := s.Deals(ctx, sc)
	if err != nil {
		return res, err
	}
	for _, d := range deals {
		if d.Status == "won" {
			if d.WonAt != nil && !d.WonAt.Before(qstart) && !d.WonAt.After(qend.AddDate(0, 0, 1)) {
				res.Commit += d.Value
				res.CommitIDs = append(res.CommitIDs, d.ID)
			}
			continue
		}
		if !d.IsPipeline() {
			continue
		}
		res.Pipeline += d.Value
		if ex[d.ID] {
			continue
		}
		verbal := d.Signal == "verbal_commit" || d.Signal == "kontrak"
		switch {
		case verbal && d.Health >= commitMin:
			res.Commit += d.Value
			res.CommitIDs = append(res.CommitIDs, d.ID)
		case d.Health >= bestMin:
			res.Best += d.Value
			res.BestIDs = append(res.BestIDs, d.ID)
		}
	}
	res.Best += res.Commit
	return res, nil
}

// Weighted returns the two readings of the pipeline: manual sales probability vs ARC evidence.
type Weighted struct {
	Sales, ARC float64
	ByDeal     map[string][2]float64
}

// StageWeight returns the ARC probability weight for a deal.
func (s *Service) StageWeight(ctx context.Context) func(Deal) float64 {
	w := map[string]float64{}
	s.Policy(ctx, "forecast_stage_weight", &w)
	return func(d Deal) float64 {
		if d.Signal == "verbal_commit" {
			if v, ok := w["verbal_commit"]; ok {
				return v
			}
			return 1
		}
		if v, ok := w[d.Stage]; ok {
			return v
		}
		return 0.5
	}
}

// ArcProbability is health × stage weight, in percent.
func (s *Service) ArcProbability(ctx context.Context, d Deal) int {
	return int(math.Round(float64(d.Health) * s.StageWeight(ctx)(d)))
}

// WeightedPipeline computes both weighted sums over pipeline deals.
func (s *Service) WeightedPipeline(ctx context.Context, sc Scope) (Weighted, []Deal, error) {
	deals, err := s.Deals(ctx, sc)
	if err != nil {
		return Weighted{}, nil, err
	}
	sw := s.StageWeight(ctx)
	w := Weighted{ByDeal: map[string][2]float64{}}
	var open []Deal
	for _, d := range deals {
		if !d.IsPipeline() {
			continue
		}
		open = append(open, d)
		sales := d.Value * float64(d.ManualProb) / 100
		arc := d.Value * float64(d.Health) / 100 * sw(d)
		w.Sales += sales
		w.ARC += arc
		w.ByDeal[d.ID] = [2]float64{sales, arc}
	}
	return w, open, nil
}

// WeightedNote explains where the gap between the two readings comes from.
func WeightedNote(w Weighted, deals []Deal) string {
	diff := w.Sales - w.ARC
	if diff <= 0 {
		return "Bukti ARC sejalan dengan probabilitas sales — tidak ada deal yang ditulis terlalu optimis."
	}
	type c struct {
		d    Deal
		diff float64
	}
	var cs []c
	for _, d := range deals {
		v := w.ByDeal[d.ID]
		if d.Trend < 0 {
			cs = append(cs, c{d, v[0] - v[1]})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].diff > cs[j].diff })
	if len(cs) > 2 {
		cs = cs[:2]
	}
	if len(cs) == 0 {
		return fmt.Sprintf("Selisih %s antara probabilitas sales dan bukti ARC.", domain.FormatRp1(diff))
	}
	sum := 0.0
	names := []string{}
	probs := []string{}
	for _, x := range cs {
		sum += x.diff
		names = append(names, shortAccount(x.d.Account))
		probs = append(probs, fmt.Sprintf("%d%%", x.d.ManualProb))
	}
	probText := probs[0]
	if len(probs) > 1 && probs[0] != probs[1] {
		probText = strings.Join(probs, " dan ")
	}
	return fmt.Sprintf("Selisih %s; %d%% di antaranya dari %s — %s yang oleh sales masih ditulis %s, padahal sinyalnya melemah.",
		domain.FormatRp1(diff), int(math.Round(sum/diff*100)), strings.Join(names, " dan "),
		map[bool]string{true: "dua deal", false: "deal"}[len(cs) == 2], probText)
}

func shortAccount(name string) string {
	r := strings.NewReplacer("Kota ", "", "PT ", "")
	return r.Replace(name)
}
