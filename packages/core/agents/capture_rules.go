package agents

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"arc/packages/core/llm"
)

type pgxTx = pgx.Tx

// Deterministic extraction rules. They back the FakeProvider (mock mode, tests,
// eval) and double as a sanity baseline for the LLM output.

var (
	reCompetitor     = regexp.MustCompile(`(?i)(vendor lain|kompetitor|pesaing|penawaran lain|harga lebih (rendah|murah))`)
	reMoved          = regexp.MustCompile(`(?i)(mulai (bulan )?\w+ saya di|saya pindah|dimutasi|mutasi ke|pindah tugas|koordinasi dengan pak \w+ ya)`)
	rePaid           = regexp.MustCompile(`(?i)(sudah (kami )?(transfer|bayar)|pembayaran .{0,40}sudah|sudah dibayar|bukti transfer)`)
	reStock          = regexp.MustCompile(`(?i)stok .{0,40}(kurang|habis)|lead time`)
	reNightOnly      = regexp.MustCompile(`(?i)hanya bisa (malam|pagi|siang)|akses .{0,30}hanya`)
	reNeed           = regexp.MustCompile(`(?i)butuh ([a-z0-9 ]{3,30}?)(,|\.|$| saya)`)
	rePlease         = regexp.MustCompile(`(?i)tolong ([^.?!]{5,80})`)
	reVerbKami       = regexp.MustCompile(`(?i)\b(kirim|kami kirim|siapkan|jadwalkan|kabari|antar|pasang|survey|presentasikan|kami siapkan)\b`)
	reInboundPromise = regexp.MustCompile(`(?i)\b(menyusul|dikirim|kami kirim|saya kirim|saya kabari|dikabari|nanti saya|akan kami)\b`)
	nouns            = []struct {
		re    *regexp.Regexp
		label string
	}{
		{regexp.MustCompile(`(?i)\brevisi`), "revisi"}, {regexp.MustCompile(`(?i)\bpenawaran`), "penawaran"}, {regexp.MustCompile(`\bPO\b`), "PO"},
		{regexp.MustCompile(`(?i)\bBAST\b`), "BAST"}, {regexp.MustCompile(`(?i)\blaporan`), "laporan"}, {regexp.MustCompile(`(?i)\bproposal`), "proposal"},
		{regexp.MustCompile(`(?i)\binvoice`), "invoice"}, {regexp.MustCompile(`(?i)\bjadwal`), "jadwal"}, {regexp.MustCompile(`(?i)\bspek`), "spesifikasi"},
		{regexp.MustCompile(`(?i)\bBoQ\b`), "BoQ"}, {regexp.MustCompile(`(?i)\bkontrak`), "kontrak"}, {regexp.MustCompile(`(?i)\bmockup`), "mockup"},
		{regexp.MustCompile(`(?i)\bfoto`), "foto"}, {regexp.MustCompile(`(?i)\bdata\b`), "data"}, {regexp.MustCompile(`(?i)\bkabari`), "feedback"},
	}
	reDayPhrase = regexp.MustCompile(`(?i)\b(senin|selasa|rabu|kamis|jumat|sabtu|minggu depan|besok|hari ini|lusa|\d{1,2} (jan|feb|mar|apr|mei|jun|jul|agu|sep|okt|nov|des))\b`)
)

// firstNoun returns the deliverable mentioned earliest in the text
// ("proposal revisinya" → proposal, not revisi).
func firstNoun(t string) string {
	best, at := "", -1
	for _, n := range nouns {
		if loc := n.re.FindStringIndex(t); loc != nil && (at < 0 || loc[0] < at) {
			best, at = n.label, loc[0]
		}
	}
	return best
}

func firstName(n string) string {
	f := strings.Fields(strings.TrimPrefix(strings.TrimPrefix(n, "Pak "), "Bu "))
	if len(f) == 0 {
		return n
	}
	return f[0]
}

// RuleExtract is the deterministic extractor.
func RuleExtract(in CaptureInput) CaptureOutput {
	var out CaptureOutput
	t := in.Text
	lower := strings.ToLower(t)
	group := in.ThreadType == "gext" || in.ThreadType == "gint"
	addAnn := func(kind, tone, text, action string) {
		out.Annotations = append(out.Annotations, struct {
			Kind   string `json:"kind"`
			Tone   string `json:"tone"`
			Text   string `json:"text"`
			Action string `json:"action"`
		}{kind, tone, text, action})
	}
	day := reDayPhrase.FindString(t)
	noun := firstNoun(t)
	if in.ThreadType != "gint" {
		// Our commitment needs a deliverable, a deadline (or "siap") and a promise verb;
		// "kami tunggu PO-nya … Selasa" is waiting on them, not a promise of ours.
		promised := reVerbKami.MatchString(t) || reInboundPromise.MatchString(t)
		if in.Direction == "out" && noun != "" && (day != "" || strings.Contains(lower, "siap")) && promised {
			text := strings.TrimSpace(noun + " " + strings.ToLower(day))
			if day != "" {
				text = noun + " " + titleDay(day)
			}
			out.Commitments = append(out.Commitments, commitmentOut("kami", text, quoteAround(t, noun), 0.9))
			addAnn("commitment", "accent", "Komitmen kita: "+text+" → ledger", "")
		} else if in.Direction == "in" && noun != "" && day != "" && reInboundPromise.MatchString(t) {
			text := noun + " " + titleDay(day)
			out.Commitments = append(out.Commitments, commitmentOut("mereka", text, quoteAround(t, noun), 0.88))
			addAnn("commitment", "accent", "Komitmen mereka: "+text+" → ledger", "")
		}
		if reCompetitor.MatchString(t) {
			out.Signals = append(out.Signals, signalOut("competitor_mentioned", "bad", "Kompetitor disebut", "“"+trunc(reCompetitor.FindString(t), 60)+"”", t))
			addAnn("signal", "bad", "Sinyal: kompetitor disebut · belum ada dokumen penawaran", "")
		}
		if reMoved.MatchString(t) && in.Direction == "in" {
			out.Signals = append(out.Signals, signalOut("champion_moved", "warn", "Champion mutasi", in.Sender+" menyebut pindah tugas", t))
			addAnn("signal", "warn", "Sinyal: champion pindah", "")
		}
		if rePaid.MatchString(t) && in.Direction == "in" {
			out.Signals = append(out.Signals, signalOut("payment_on_time", "good", "Pembayaran tepat waktu", "Konfirmasi transfer dari "+in.Sender, t))
			addAnn("signal", "good", "Pembayaran dikonfirmasi → dicocokkan dengan Odoo", "")
		}
	}
	if group {
		if m := reNeed.FindStringSubmatch(t); m != nil && in.SenderInternal {
			obj := strings.TrimSpace(m[1])
			verb := "Siapkan "
			if regexp.MustCompile(`(?i)crane|truk|scaffold|alat|forklift|genset`).MatchString(obj) {
				verb = "Sewa "
			}
			task := verb + obj
			out.Tasks = append(out.Tasks, taskOut(task, in.Sender, m[0]))
			addAnn("task", "indigo", fmt.Sprintf("Tugas → %s: %s", firstName(in.Sender), strings.ToLower(task)), "Kirim ke Basecamp")
		}
		if m := rePlease.FindStringSubmatch(t); m != nil {
			task := strings.ToUpper(m[1][:1]) + m[1][1:]
			out.Tasks = append(out.Tasks, taskOut(task, "Admin Project", m[0]))
			addAnn("task", "indigo", "Syarat dicatat · tugas → Admin Project", "Kirim ke Basecamp")
		}
		if reStock.MatchString(t) && in.ThreadType == "gext" || (reStock.MatchString(t) && in.SenderInternal) {
			out.Tasks = append(out.Tasks, taskOut("Ajukan PO kekurangan stok (cek lead time)", "Purchasing", t))
			if in.ThreadType == "gext" {
				out.Signals = append(out.Signals, signalOut("stock_risk", "bad", "Risiko stok", trunc(t, 120), t))
			}
			addAnn("risk", "bad", "Risiko: kekurangan stok · lead time", "Buat permintaan PO")
		}
		if reNightOnly.MatchString(t) && in.ThreadType == "gext" {
			addAnn("risk", "warn", "Kendala: akses terbatas → jadwal & lembur dicatat di project", "")
		}
	}
	out.Sentiment = 0.2
	if reCompetitor.MatchString(t) {
		out.Sentiment = -0.3
	} else if rePaid.MatchString(t) || strings.Contains(lower, "terima kasih") {
		out.Sentiment = 0.5
	}
	out.Summary = trunc(t, 140)
	return out
}

func titleDay(d string) string {
	d = strings.ToLower(d)
	if len(d) > 0 && d[0] >= 'a' && d[0] <= 'z' && !strings.Contains(d, " ") {
		return strings.ToUpper(d[:1]) + d[1:]
	}
	return d
}

func quoteAround(t, word string) string {
	i := strings.Index(strings.ToLower(t), strings.ToLower(word))
	if i < 0 {
		return trunc(t, 200)
	}
	start := i - 60
	if start < 0 {
		start = 0
	}
	return trunc(strings.TrimSpace(t[start:]), 200)
}

func commitmentOut(who, text, quote string, conf float64) struct {
	Who        string  `json:"who"`
	Text       string  `json:"text"`
	Due        *string `json:"due"`
	Quote      string  `json:"quote"`
	Confidence float64 `json:"confidence"`
} {
	return struct {
		Who        string  `json:"who"`
		Text       string  `json:"text"`
		Due        *string `json:"due"`
		Quote      string  `json:"quote"`
		Confidence float64 `json:"confidence"`
	}{who, text, nil, quote, conf}
}

func signalOut(typ, sev, title, detail, quote string) struct {
	Type       string  `json:"type"`
	Severity   string  `json:"severity"`
	Title      string  `json:"title"`
	Detail     string  `json:"detail"`
	Quote      string  `json:"quote"`
	Confidence float64 `json:"confidence"`
} {
	return struct {
		Type       string  `json:"type"`
		Severity   string  `json:"severity"`
		Title      string  `json:"title"`
		Detail     string  `json:"detail"`
		Quote      string  `json:"quote"`
		Confidence float64 `json:"confidence"`
	}{typ, sev, title, detail, trunc(quote, 200), 0.86}
}

func taskOut(text, who, quote string) struct {
	Text         string `json:"text"`
	AssigneeHint string `json:"assignee_hint"`
	Quote        string `json:"quote"`
} {
	return struct {
		Text         string `json:"text"`
		AssigneeHint string `json:"assignee_hint"`
		Quote        string `json:"quote"`
	}{text, who, trunc(quote, 200)}
}

var reAnnCommit = regexp.MustCompile(`(?i)komitmen (kita|mereka)[^:]*: ([^→·]+)`)
var reAnnTask = regexp.MustCompile(`(?i)tugas → ([^:]+): ([^·(]+)`)

// fakeCapture answers from the mockup annotation of a known message, completed
// with the deterministic rules.
func (a *Agents) fakeCapture(req llm.Request) (any, error) {
	in, ok := req.FakeInput.(CaptureInput)
	if !ok {
		raw, _ := json.Marshal(req.FakeInput)
		_ = json.Unmarshal(raw, &in)
	}
	out := RuleExtract(in)
	ann, known := a.Fx.Annotations[in.Text]
	if !known {
		return out, nil
	}
	kind := "note"
	l := strings.ToLower(ann.T)
	switch {
	case strings.HasPrefix(l, "komitmen") || strings.Contains(l, "komitmen mereka") || strings.Contains(l, "komitmen kita"):
		kind = "commitment"
	case strings.HasPrefix(l, "sinyal"):
		kind = "signal"
	case strings.HasPrefix(l, "tugas") || strings.HasPrefix(l, "syarat"):
		kind = "task"
	case strings.HasPrefix(l, "risiko") || strings.HasPrefix(l, "kendala"):
		kind = "risk"
	case strings.HasPrefix(l, "milestone"):
		kind = "milestone"
	case strings.HasPrefix(l, "stage"):
		kind = "stage"
	}
	out.Annotations = out.Annotations[:0]
	out.Annotations = append(out.Annotations, struct {
		Kind   string `json:"kind"`
		Tone   string `json:"tone"`
		Text   string `json:"text"`
		Action string `json:"action"`
	}{kind, ann.K, ann.T, ann.Act})
	if len(out.Commitments) == 0 {
		if m := reAnnCommit.FindStringSubmatch(ann.T); m != nil {
			who := "kami"
			if strings.EqualFold(m[1], "mereka") {
				who = "mereka"
			}
			text := strings.TrimSpace(m[2])
			if strings.Contains(l, "terpenuhi") {
				out.Fulfils = append(out.Fulfils, struct {
					Text  string `json:"text"`
					Quote string `json:"quote"`
				}{text, trunc(in.Text, 200)})
			} else {
				out.Commitments = append(out.Commitments, commitmentOut(who, text, trunc(in.Text, 200), 0.9))
			}
		}
	}
	if strings.Contains(l, "terpenuhi") && len(out.Fulfils) == 0 {
		out.Fulfils = append(out.Fulfils, struct {
			Text  string `json:"text"`
			Quote string `json:"quote"`
		}{firstNoun(in.Text), trunc(in.Text, 200)})
	}
	if len(out.Tasks) == 0 {
		if m := reAnnTask.FindStringSubmatch(ann.T); m != nil {
			out.Tasks = append(out.Tasks, taskOut(strings.TrimSpace(m[2]), strings.TrimSpace(m[1]), in.Text))
		}
	}
	if kind == "signal" && len(out.Signals) == 0 && strings.Contains(l, "kompetitor") {
		out.Signals = append(out.Signals, signalOut("competitor_mentioned", "bad", "Kompetitor disebut", trunc(in.Text, 120), in.Text))
	}
	return out, nil
}
