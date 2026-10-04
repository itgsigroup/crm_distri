package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/llm"
	"arc/packages/core/prompts"
	"arc/packages/core/storage"
)

func (a *Agents) exists(ctx context.Context, q string, args ...any) bool {
	var b bool
	_ = a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(`+q+`)`, args...).Scan(&b)
	return b
}

// ---------------- Meeting prep ----------------

// MeetingPrepInput is the context for a meeting brief.
type MeetingPrepInput struct {
	Title, Account, Attendees, Location, Time string
	OpenCommitments                           []string
	Signals                                   []string
}

// MeetingPrep prepares H-1 briefs for tomorrow's meetings (not internal < 30 min).
func (a *Agents) MeetingPrep(ctx context.Context) (int, error) {
	now := domain.Now()
	from, to := domain.StartOfDay(now).AddDate(0, 0, 1), domain.StartOfDay(now).AddDate(0, 0, 2)
	rows, err := a.DB.Pool.Query(ctx, `SELECT e.id, e.title, e.starts_at, e.duration_min, e.location, e.attendees, COALESCE(e.account_id,''), e.internal, COALESCE(a.name,''), COALESCE(u.name,'')
		FROM calendar_events e LEFT JOIN accounts a ON a.id=e.account_id LEFT JOIN users u ON u.id=e.owner_user_id WHERE e.starts_at >= $1 AND e.starts_at < $2`, from, to)
	if err != nil {
		return 0, err
	}
	type ev struct {
		id, title, loc, att, acc, accName, owner string
		start                                    time.Time
		dur                                      int
		internal                                 bool
	}
	var evs []ev
	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.id, &e.title, &e.start, &e.dur, &e.loc, &e.att, &e.acc, &e.internal, &e.accName, &e.owner); err != nil {
			rows.Close()
			return 0, err
		}
		evs = append(evs, e)
	}
	rows.Close()
	n := 0
	for _, e := range evs {
		if e.internal && e.dur < 30 {
			continue // calibration: no prep for short internal meetings
		}
		if a.exists(ctx, `SELECT 1 FROM actions WHERE type='meeting_brief' AND payload->>'event_id'=$1`, e.id) {
			continue
		}
		in := MeetingPrepInput{Title: e.title, Account: e.accName, Attendees: e.att, Location: e.loc, Time: domain.ClockID(e.start)}
		if e.acc != "" {
			r, _ := a.DB.Pool.Query(ctx, `SELECT who || ': ' || text FROM commitments WHERE account_id=$1 AND status IN ('open','late')`, e.acc)
			for r != nil && r.Next() {
				var s string
				_ = r.Scan(&s)
				in.OpenCommitments = append(in.OpenCommitments, s)
			}
			if r != nil {
				r.Close()
			}
			r, _ = a.DB.Pool.Query(ctx, `SELECT title FROM signals WHERE account_id=$1 AND resolved_at IS NULL`, e.acc)
			for r != nil && r.Next() {
				var s string
				_ = r.Scan(&s)
				in.Signals = append(in.Signals, s)
			}
			if r != nil {
				r.Close()
			}
		}
		raw, _ := json.Marshal(in)
		var out struct {
			Title        string   `json:"title"`
			Why          string   `json:"why"`
			Prep         string   `json:"prep"`
			OpeningPoint string   `json:"opening_point"`
			Steps        []string `json:"steps"`
		}
		resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "meeting_prep", System: prompts.Get("meeting_prep/v1"),
			Messages:  []llm.Message{{Role: "user", Content: string(raw)}},
			Schema:    obj([]string{"title", "why", "prep", "opening_point", "steps"}, map[string]any{"title": str(), "why": str(), "prep": str(), "opening_point": str(), "steps": arr(str())}),
			MaxTokens: 2000, FakeInput: in}, &out)
		if err != nil {
			return n, err
		}
		_, created, err := a.Actions.Propose(ctx, actions.Proposal{Agent: domain.AgentMeetingPrep, Type: "meeting_brief", Kind: "task", Icon: "i-cal", ButtonLabel: "Kirim brief",
			AccountID: e.acc, DueLabel: "Besok " + domain.ClockID(e.start), Title: out.Title, Why: out.Why, Prep: out.Prep + " Poin pembuka: " + out.OpeningPoint, Steps: out.Steps,
			Payload: map[string]any{"event_id": e.id}, Model: resp.Model, Confidence: 0.84,
			Evidence: []domain.Evidence{{DocumentID: "calendar:" + e.id, Quote: e.title}}}, storage.Actor{ID: "meeting_prep", Type: "agent"})
		if err != nil {
			return n, err
		}
		if created {
			n++
			_, _ = a.DB.Pool.Exec(ctx, `UPDATE calendar_events SET prep_status='ready', prep_pills = CASE WHEN prep_pills = '[]'::jsonb THEN $2 ELSE prep_pills END WHERE id=$1`,
				e.id, storage.JSON([]map[string]string{{"k": "good", "t": "Prep siap", "icon": "i-check"}}))
		}
	}
	return n, nil
}

func (a *Agents) fakeMeetingPrep(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(MeetingPrepInput)
	opening := "Konfirmasi tujuan dan langkah berikutnya"
	for _, c := range in.OpenCommitments {
		if strings.Contains(c, "PO") {
			opening = "PO belum masuk — angkat di awal"
		}
	}
	return map[string]any{
		"title":         "Brief meeting: " + in.Title,
		"why":           fmt.Sprintf("Meeting besok %s di %s dengan %s.", in.Time, defaultStr(in.Location, "lokasi pelanggan"), in.Attendees),
		"prep":          fmt.Sprintf("Brief 1 halaman: %d komitmen terbuka, %d sinyal aktif, status deal dan risiko.", len(in.OpenCommitments), len(in.Signals)),
		"opening_point": opening,
		"steps":         []string{"Brief dikirim ke peserta GSI pukul 07.00", "Setelah meeting, ARC mengekstrak komitmen dari catatan"},
	}, nil
}

// ---------------- Collection ----------------

// Collection proposes create_invoice, payment reminders (tone from the payment
// pattern), SPM document checks for government, BAST scheduling and referral requests.
func (a *Agents) Collection(ctx context.Context) (int, error) {
	now := domain.Now()
	bastBench := int(a.Ins.PolicyFloat(ctx, "bast_benchmark_days", 3))
	govMin := int(a.Ins.PolicyFloat(ctx, "gov_reminder_min_days", 30))
	items, err := a.Ins.L2C(ctx)
	if err != nil {
		return 0, err
	}
	patterns, _ := a.Ins.PayPatterns(ctx)
	n := 0
	propose := func(p actions.Proposal) error {
		if a.Actions.Blocked(ctx, p.Agent, p.Type) {
			return nil
		}
		if p.CashItemID != "" && a.exists(ctx, `SELECT 1 FROM actions WHERE cash_item_id=$1 AND type=$2 AND status NOT IN ('rejected','cancelled')`, p.CashItemID, p.Type) {
			return nil
		}
		if inv, _ := p.Payload["invoice_id"].(string); inv != "" && a.exists(ctx, `SELECT 1 FROM actions WHERE type=$2 AND (payload->>'invoice_id'=$1 OR cash_item_id IN (SELECT id FROM cash_items WHERE invoice_id=$1)) AND status NOT IN ('rejected','cancelled')`, inv, p.Type) {
			return nil
		}
		_, created, err := a.Actions.Propose(ctx, p, storage.Actor{ID: "collection", Type: "agent"})
		if created {
			n++
		}
		return err
	}
	for _, it := range items {
		ev := []domain.Evidence{{DocumentID: it.SO, Quote: fmt.Sprintf("%s · %d hari di %s (benchmark %d)", it.Project, it.Days, it.Stage, it.Bench)}}
		switch {
		case it.Stage == "bast" && it.Days > bastBench:
			if err := propose(actions.Proposal{Agent: domain.AgentCollection, Type: "create_invoice", Kind: "task", Icon: "i-doc", ButtonLabel: "Buat invoice",
				AccountID: it.AccountID, CashItemID: it.ID, DueLabel: "Hari ini", Title: "Buat invoice dari " + it.SO + " sekarang",
				Why:      fmt.Sprintf("BAST clear sudah %d hari, benchmark GSI %d hari. Setiap hari tanpa invoice = 1 hari DSO tambahan pada %s.", it.Days, bastBench, domain.FormatRp(it.Value)),
				Prep:     "Draft invoice di Odoo dari SO dengan BAST terlampir, email pengantar ke bagian keuangan pelanggan.",
				Steps:    []string{"Invoice draft dibuat di Odoo · finance memvalidasi", "Email pengantar + BAST terkirim", "Collection agent memantau; pengingat ramah H-3 jatuh tempo"},
				Evidence: ev, Confidence: 0.9}); err != nil {
				return n, err
			}
		case it.Stage == "pemasangan" && it.Bench-it.Days <= 5:
			if err := propose(actions.Proposal{Agent: domain.AgentCollection, Type: "schedule_bast", Kind: "task", Icon: "i-cal", ButtonLabel: "Jadwalkan BAST",
				AccountID: it.AccountID, CashItemID: it.ID, DueLabel: "Minggu ini", Title: "Kunci jadwal BAST " + it.Account,
				Why:   fmt.Sprintf("Pemasangan hari ke-%d dari estimasi %d. BAST yang dijadwalkan sekarang mempercepat invoice termin berikutnya.", it.Days, it.Bench),
				Prep:  "Usulan 2 slot BAST dan checklist BAST dari template; foto sebelum-sesudah dikumpulkan dari grup project.",
				Steps: []string{"Undangan BAST terkirim", "Checklist BAST jadi tugas teknisi di Basecamp", "Invoice termin disiapkan saat BAST clear"}, Evidence: ev, Confidence: 0.85}); err != nil {
				return n, err
			}
		}
	}
	invs, err := a.Ins.OpenInvoices(ctx, "project")
	if err != nil {
		return n, err
	}
	for _, inv := range invs {
		late := domain.DaysBetween(inv.DueDate, now)
		if late <= 0 {
			continue
		}
		pat := patterns[inv.AccountID]
		ev := []domain.Evidence{{DocumentID: inv.Number, Quote: fmt.Sprintf("%s jatuh tempo %s, lewat %d hari", inv.Number, domain.ShortDate(inv.DueDate), late)}}
		if inv.Gov {
			if late < govMin {
				continue // government budget terms: no reminder before H+30
			}
			if err := propose(actions.Proposal{Agent: domain.AgentCollection, Type: "ask_spm_documents", Kind: "send", Icon: "i-chat", ButtonLabel: "Draf WhatsApp",
				AccountID: inv.AccountID, DueLabel: "Besok", Title: "Tanyakan status SPM ke bendahara", Payload: map[string]any{"invoice_id": inv.ID},
				Why:   fmt.Sprintf("Piutang %d hari, pola termin anggaran daerah — yang bisa dipercepat hanya kelengkapan dokumen SPM.", late),
				Prep:  "Draft WhatsApp ke bendahara menanyakan dokumen yang masih kurang, dengan checklist dokumen SPM.",
				Steps: []string{"Pesan masuk antrean approval", "Jawaban diekstrak; dokumen yang kurang jadi tugas admin", "Prediksi kas diperbarui"}, Evidence: ev, Confidence: 0.86}); err != nil {
				return n, err
			}
			continue
		}
		tone := "tegas namun sopan"
		if pat.Kind == "tepat" {
			tone = "ramah: konfirmasi jadwal pembayaran, bukan teguran"
		}
		if err := propose(actions.Proposal{Agent: domain.AgentCollection, Type: "payment_reminder", Kind: "send", Icon: "i-mail", ButtonLabel: "Kirim pengingat",
			AccountID: inv.AccountID, DueLabel: "Hari ini", Title: "Kirim pengingat pembayaran (" + strings.Split(tone, ":")[0] + ")", Payload: map[string]any{"invoice_id": inv.ID},
			Why:   fmt.Sprintf("Invoice lewat %d hari; pola bayar akun ini %d hari.", late, pat.MedianDays),
			Prep:  "Email singkat dengan nada " + tone + ", salinan invoice dan nomor rekening.",
			Steps: []string{"Email masuk antrean approval finance", "Kalau tidak ada balasan H+5, sales diminta menelepon", "Prediksi kas masuk diperbarui saat ada jawaban"}, Evidence: ev, Confidence: 0.88}); err != nil {
			return n, err
		}
	}
	// Referral request H+3 after BAST.
	rows, err := a.DB.Pool.Query(ctx, `SELECT c.id, COALESCE(c.account_id,''), c.account_name FROM cash_items c WHERE c.bast_at IS NOT NULL AND c.bast_at <= $1`, now.AddDate(0, 0, -3))
	if err == nil {
		type r struct{ id, acc, name string }
		var list []r
		for rows.Next() {
			var x r
			_ = rows.Scan(&x.id, &x.acc, &x.name)
			list = append(list, x)
		}
		rows.Close()
		for _, x := range list {
			if err := propose(actions.Proposal{Agent: domain.AgentCollection, Type: "request_referral", Kind: "send", Icon: "i-edit", ButtonLabel: "Draf permintaan",
				AccountID: x.acc, CashItemID: x.id, DueLabel: "Minggu ini", Title: "Minta referensi & testimoni " + x.name,
				Why: "BAST sudah ditandatangani ≥ 3 hari. Referensi dari pelanggan puas menaikkan win rate referral.", Prep: "Draft email permintaan testimoni + izin menyebut sebagai referensi.",
				Steps: []string{"Email masuk antrean approval", "Testimoni masuk library referensi"}, Evidence: []domain.Evidence{{DocumentID: x.id, Quote: "BAST H+3"}}, Confidence: 0.8}); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// ---------------- Growth: tenders, renewal, coaching, forecast ----------------

// TenderInput is the context for tender matching.
type TenderInput struct {
	ID, Title, Agency, Description string
	HPS                            float64
	Deadline                       string
	Keywords                       []string
}

// TenderRadar scores unscored tenders and proposes qualification for good matches.
func (a *Agents) TenderRadar(ctx context.Context) (int, error) {
	var kws []string
	a.Ins.Setting(ctx, "tender_keywords", &kws)
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, title, agency, description, hps::float8, COALESCE(to_char(deadline,'YYYY-MM-DD'),'') FROM tenders WHERE match_score IS NULL`)
	if err != nil {
		return 0, err
	}
	var list []TenderInput
	for rows.Next() {
		var t TenderInput
		_ = rows.Scan(&t.ID, &t.Title, &t.Agency, &t.Description, &t.HPS, &t.Deadline)
		t.Keywords = kws
		list = append(list, t)
	}
	rows.Close()
	n := 0
	for _, t := range list {
		raw, _ := json.Marshal(t)
		var out struct {
			Score   int    `json:"score"`
			Reasons string `json:"reasons"`
		}
		resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "tender_match", System: prompts.Get("tender/v1"),
			Messages: []llm.Message{{Role: "user", Content: string(raw)}}, Schema: obj([]string{"score", "reasons"}, map[string]any{"score": map[string]any{"type": "integer"}, "reasons": str()}),
			MaxTokens: 1000, FakeInput: t}, &out)
		if err != nil {
			return n, err
		}
		if _, err := a.DB.Pool.Exec(ctx, `UPDATE tenders SET match_score=$2, reasons=CASE WHEN reasons='' THEN $3 ELSE reasons END, updated_at=now() WHERE id=$1`, t.ID, out.Score, out.Reasons); err != nil {
			return n, err
		}
		n++
		if out.Score >= 70 {
			_, _, _ = a.Actions.Propose(ctx, actions.Proposal{Agent: domain.AgentResearch, Type: "qualify_tender", Kind: "task", Icon: "i-radar", ButtonLabel: "Kualifikasi",
				Title: "Kualifikasi tender: " + t.Title, Why: fmt.Sprintf("Skor kecocokan %d. %s", out.Score, out.Reasons), Prep: "Lead dibuat di stage Baru dan dokumen kualifikasi disiapkan dari tender serupa.",
				Steps: []string{"Lead dibuat", "Sales cabang ditugaskan"}, Payload: map[string]any{"tender_id": t.ID}, Model: resp.Model, Confidence: float64(out.Score) / 100,
				Evidence: []domain.Evidence{{DocumentID: "tender:" + t.ID, Quote: t.Title}}}, storage.Actor{ID: "research", Type: "agent"})
		}
	}
	return n, nil
}

func (a *Agents) fakeTender(req llm.Request) (any, error) {
	t, _ := req.FakeInput.(TenderInput)
	text := strings.ToLower(t.Title + " " + t.Description)
	score := 30
	var hit []string
	for _, k := range t.Keywords {
		if strings.Contains(text, strings.ToLower(k)) {
			score += 12
			hit = append(hit, k)
		}
	}
	if score > 95 {
		score = 95
	}
	reasons := "cocok kata kunci: " + strings.Join(hit, ", ")
	if len(hit) == 0 {
		reasons = "tidak ada kata kunci yang cocok"
	}
	// Fixture tenders keep the score of the approved mockup; the reason is always given.
	if s, ok := a.Fx.TenderScore[t.ID]; ok {
		score = s
	}
	return map[string]any{"score": score, "reasons": reasons}, nil
}

// Renewal proposes maintenance offers for warranties ending within 90 days.
func (a *Agents) Renewal(ctx context.Context) (int, error) {
	now := domain.Now()
	rows, err := a.DB.Pool.Query(ctx, `SELECT s.account_id, a.name, s.system, s.warranty_end, s.units FROM installed_systems s JOIN accounts a ON a.id=s.account_id
		WHERE s.warranty_end BETWEEN $1 AND $2 AND s.service_contract NOT ILIKE 'kontrak%'`, now, now.AddDate(0, 0, 90))
	if err != nil {
		return 0, err
	}
	type r struct {
		acc, name, system string
		end               time.Time
		units             int
	}
	var list []r
	for rows.Next() {
		var x r
		_ = rows.Scan(&x.acc, &x.name, &x.system, &x.end, &x.units)
		list = append(list, x)
	}
	rows.Close()
	n := 0
	for _, x := range list {
		if a.exists(ctx, `SELECT 1 FROM actions WHERE account_id=$1 AND status IN ('proposed','snoozed') AND (type IN ('offer_maintenance','create_quotation') OR title ILIKE '%maintenance%')`, x.acc) {
			continue
		}
		_, created, err := a.Actions.Propose(ctx, actions.Proposal{Agent: domain.AgentResearch, Type: "offer_maintenance", Kind: "task", Icon: "i-edit", ButtonLabel: "Buat penawaran",
			AccountID: x.acc, DueLabel: "Minggu ini", Title: "Tawarkan paket maintenance " + x.system,
			Why:      fmt.Sprintf("Garansi %s habis %s %d dan belum ada kontrak service.", x.system, domain.MonthLong(x.end.Month()), x.end.Year()),
			Prep:     "Draft penawaran maintenance 12 bulan dari template, harga dari Odoo, disesuaikan dengan unit terpasang.",
			Steps:    []string{"Quotation dibuat di Odoo sebagai draft", "Email penawaran masuk antrean approval", "Opportunity maintenance dibuat di stage Penawaran"},
			Evidence: []domain.Evidence{{Source: "installed_systems", Quote: fmt.Sprintf("%s · garansi habis %s", x.system, domain.ShortDate(x.end))}}, Confidence: 0.83}, storage.Actor{ID: "research", Type: "agent"})
		if err != nil {
			return n, err
		}
		if created {
			n++
		}
	}
	return n, nil
}

// CoachingInput is the context for one sales coaching note.
type CoachingInput struct {
	UserID string
	Row    insights.TeamRow
}

// Coaching writes a coaching note per sales user (heavy tier) into settings.
func (a *Agents) Coaching(ctx context.Context) (map[string]string, error) {
	team, err := a.Ins.Team(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, t := range team {
		raw, _ := json.Marshal(t)
		var res struct {
			Coach string `json:"coach"`
		}
		if _, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "coaching", System: prompts.Get("coaching/v1"),
			Messages: []llm.Message{{Role: "user", Content: string(raw)}}, Schema: obj([]string{"coach"}, map[string]any{"coach": str()}),
			MaxTokens: 500, FakeInput: CoachingInput{UserID: t.UserID, Row: t}}, &res); err != nil {
			return out, err
		}
		out[t.UserID] = res.Coach
	}
	return out, a.Ins.SetSetting(ctx, "coaching", out)
}

func (a *Agents) fakeCoaching(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(CoachingInput)
	if c, ok := a.Fx.Coaching[in.UserID]; ok {
		c = strings.ReplaceAll(c, "{msgs}", fmt.Sprint(in.Row.WAMessages30d))
		c = strings.ReplaceAll(c, "{contacts}", fmt.Sprint(in.Row.NoOppContacts))
		return map[string]any{"coach": c}, nil
	}
	if in.Row.Pipeline == 0 && in.Row.NoOppContacts > 0 {
		return map[string]any{"coach": fmt.Sprintf("%d pesan WA dengan %d kontak tanpa lead. Buat lead %s hari ini.", in.Row.WAMessages30d, in.Row.NoOppContacts, in.Row.TopNoOppContact)}, nil
	}
	return map[string]any{"coach": "Jaga ritme follow-up dan tambah satu kontak baru di deal terbesar minggu ini."}, nil
}

// ForecastJob refreshes arc_probability for open deals.
func (a *Agents) ForecastJob(ctx context.Context) (int, error) {
	deals, err := a.Ins.Deals(ctx, insights.Scope{All: true})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range deals {
		if !d.IsPipeline() {
			continue
		}
		p := a.Ins.ArcProbability(ctx, d)
		if _, err := a.DB.Pool.Exec(ctx, `UPDATE opportunities SET arc_probability=$2 WHERE id=$1`, d.ID, p); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
