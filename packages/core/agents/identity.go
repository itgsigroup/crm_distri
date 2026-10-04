package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"arc/packages/core/domain"
	"arc/packages/core/llm"
	"arc/packages/core/prompts"
	"arc/packages/core/storage"
)

// IdentSource is one identification source line ("Siapa ini").
type IdentSource struct {
	S string `json:"s"` // source label
	C string `json:"c"` // css class: gc wa tc od web
	V string `json:"v"` // value
}

// IdentityInput is handed to the provider.
type IdentityInput struct {
	Phone        string        `json:"phone"`
	FirstMessage string        `json:"first_message"`
	Sources      []IdentSource `json:"sources"`
}

// IdentityOut is the identification result.
type IdentityOut struct {
	Name       string  `json:"name"`
	Role       string  `json:"role"`
	Company    string  `json:"company"`
	Status     string  `json:"status"`
	FitScore   *int    `json:"fit_score"`
	Confidence float64 `json:"confidence"`
	Overview   string  `json:"overview"`
}

// unidentifiedName is the placeholder shown until a source names the number.
const unidentifiedName = "Belum teridentifikasi"

// IdentifyInbound runs the source chain for an inbound number (only numbers that
// contacted GSI first): ARC Person → WhatsApp profile → Truecaller → Getcontact
// (manual import) → public web → combine ≥ 2 sources → status, fit, overview.
func (a *Agents) IdentifyInbound(ctx context.Context, id string) (IdentityOut, error) {
	var phone, first, viaSession, chatJID string
	var identRaw []byte
	err := a.DB.Pool.QueryRow(ctx, `SELECT i.phone, i.first_message, i.identification, COALESCE(t.session_id,''), COALESCE(t.chat_jid,'')
		FROM inbound_contacts i LEFT JOIN chat_threads t ON t.id=i.thread_id WHERE i.id=$1`, id).Scan(&phone, &first, &identRaw, &viaSession, &chatJID)
	if err != nil {
		return IdentityOut{}, err
	}
	var prev struct {
		Name    string        `json:"name"`
		Sources []IdentSource `json:"sources"`
	}
	_ = json.Unmarshal(identRaw, &prev)
	norm := domain.NormalizePhone(phone)
	var sources []IdentSource
	// (1) Odoo/ARC contact match.
	var pname, prole, pacc string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT p.name, p.role, COALESCE(a.name,'') FROM people p LEFT JOIN accounts a ON a.id=p.account_id
		WHERE EXISTS (SELECT 1 FROM unnest(p.phones) ph WHERE regexp_replace(ph,'[^0-9]','','g') IN ($1, '0'||substr($1,3)))`, norm).Scan(&pname, &prole, &pacc); err == nil {
		sources = append(sources, IdentSource{"Odoo", "od", fmt.Sprintf("%s · %s · %s", pname, prole, pacc)})
	} else {
		sources = append(sources, IdentSource{"Odoo", "od", "Tidak ada"})
	}
	// (2) WhatsApp Business profile via bridge.
	if a.Profiles != nil && viaSession != "" {
		if p, err := a.Profiles.Profile(ctx, viaSession, firstNonEmpty(chatJID, norm+"@s.whatsapp.net")); err == nil && p.Name != "" {
			v := p.Name
			if p.About != "" {
				v += " · “" + p.About + "”"
			}
			sources = append(sources, IdentSource{"Profil WA Business", "wa", v})
		}
	}
	// (3) Truecaller.
	if a.Truecaller != nil {
		if c, err := a.Truecaller.Lookup(ctx, norm); err == nil {
			v := c.Name
			if c.SpamCount > 0 {
				v = fmt.Sprintf("Ditandai spam oleh %d pengguna", c.SpamCount)
			} else {
				v += " · bukan spam"
			}
			sources = append(sources, IdentSource{"Truecaller", "tc", v})
		}
	}
	// (4) Getcontact: only manual imports already stored on the contact.
	for _, s := range prev.Sources {
		if s.C == "gc" {
			sources = append(sources, s)
		}
	}
	// (5) Public web via Research.
	if a.Web != nil {
		q := first
		for _, s := range sources {
			if s.C == "wa" || s.C == "gc" {
				q = s.V
			}
		}
		if res, err := a.Web.Search(ctx, q); err == nil && len(res) > 0 {
			sources = append(sources, IdentSource{"Web & berita", "web", res[0].Title + " — " + res[0].Snippet})
		}
	}
	in := IdentityInput{Phone: phone, FirstMessage: first, Sources: sources}
	raw, _ := json.Marshal(in)
	var out IdentityOut
	resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "identity_overview", System: prompts.Get("identity/v1"),
		Messages: []llm.Message{{Role: "user", Content: string(raw)}},
		Schema: obj([]string{"name", "role", "company", "status", "fit_score", "confidence", "overview"}, map[string]any{"name": str(), "role": str(), "company": str(),
			"status": map[string]any{"type": "string", "enum": []string{"identified", "unknown", "not_prospect"}}, "fit_score": map[string]any{"type": []string{"integer", "null"}},
			"confidence": map[string]any{"type": "number"}, "overview": str()}),
		MaxTokens: 2000, FakeInput: in}, &out)
	if err != nil {
		return out, err
	}
	// Guardrail: identified needs at least two consistent non-empty sources.
	useful := 0
	for _, s := range sources {
		if s.V != "" && s.V != "Tidak ada" && s.V != "—" && !strings.HasPrefix(s.V, "Tidak ada") {
			useful++
		}
	}
	if out.Status == "identified" && useful < 2 {
		out.Status = "unknown"
		out.FitScore = nil
	}
	if len(sources) == 0 {
		sources = append(sources, IdentSource{"Odoo", "od", "Tidak ada"})
	}
	// Keep the sender's WhatsApp push name when no source identifies the number.
	if out.Status != "identified" && (out.Name == "" || out.Name == unidentifiedName) && prev.Name != "" && prev.Name != unidentifiedName {
		out.Name = prev.Name
	}
	ident := map[string]any{"name": out.Name, "role": out.Role, "company": out.Company, "sources": sources, "confidence": out.Confidence, "model": resp.Model}
	status := out.Status
	if _, err := a.DB.Pool.Exec(ctx, `UPDATE inbound_contacts SET identification=$2, overview=$3, fit_score=$4, status=CASE WHEN status IN ('lead','qualified') THEN status ELSE $5 END, updated_at=now() WHERE id=$1`,
		id, storage.JSONObj(ident), out.Overview, out.FitScore, status); err != nil {
		return out, err
	}
	if status == "identified" {
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO funnel_events(inbound_id,subject_key,stage) VALUES ($1,$1,'teridentifikasi') ON CONFLICT DO NOTHING`, id)
		if out.FitScore != nil && *out.FitScore >= 30 {
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO funnel_events(inbound_id,subject_key,stage) VALUES ($1,$1,'relevan') ON CONFLICT DO NOTHING`, id)
		}
		if out.FitScore != nil && *out.FitScore >= 60 {
			if err := a.Research(ctx, id); err != nil {
				a.logger().Warn("research failed", "inbound", id, "err", err)
			}
		}
	}
	if status == "not_prospect" {
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO funnel_events(inbound_id,subject_key,stage) VALUES ($1,$1,'teridentifikasi') ON CONFLICT DO NOTHING`, id)
	}
	_ = storage.Audit(ctx, a.DB.Pool, storage.Actor{ID: "identity", Type: "agent"}, "identify", "inbound_contact", id, map[string]any{"status": status, "sources": len(sources)})
	return out, nil
}

// ImportGetcontact stores tags pasted by a sales user (source getcontact_manual) and re-identifies.
func (a *Agents) ImportGetcontact(ctx context.Context, id, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("teks tag kosong")
	}
	var raw []byte
	if err := a.DB.Pool.QueryRow(ctx, `SELECT identification FROM inbound_contacts WHERE id=$1`, id).Scan(&raw); err != nil {
		return err
	}
	ident := map[string]any{}
	_ = json.Unmarshal(raw, &ident)
	var sources []IdentSource
	if b, err := json.Marshal(ident["sources"]); err == nil {
		_ = json.Unmarshal(b, &sources)
	}
	var kept []IdentSource
	for _, s := range sources {
		if s.C != "gc" {
			kept = append(kept, s)
		}
	}
	tags := strings.Split(strings.ReplaceAll(text, "\n", ","), ",")
	var quoted []string
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			quoted = append(quoted, "“"+t+"”")
		}
	}
	kept = append([]IdentSource{{"Getcontact", "gc", strings.Join(quoted, " · ") + " — impor manual"}}, kept...)
	ident["sources"] = kept
	if _, err := a.DB.Pool.Exec(ctx, `UPDATE inbound_contacts SET identification=$2, updated_at=now() WHERE id=$1`, id, storage.JSONObj(ident)); err != nil {
		return err
	}
	_, err := a.IdentifyInbound(ctx, id)
	return err
}

func (a *Agents) fakeIdentity(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(IdentityInput)
	if fx, ok := a.Fx.Inbound[domain.NormalizePhone(in.Phone)]; ok {
		st := map[string]string{"identified": "identified", "unknown": "unknown", "notprospect": "not_prospect"}[fx.Status]
		conf := 0.0
		if fx.Score != nil {
			conf = float64(*fx.Score) / 100
		}
		return IdentityOut{Name: fx.Ident.Name, Role: fx.Ident.Role, Company: fx.Ident.Company, Status: st, FitScore: fx.Score, Confidence: conf, Overview: fx.Overview}, nil
	}
	// Heuristic: spam/vendor wording → not a prospect; otherwise combine available names.
	l := strings.ToLower(in.FirstMessage)
	for _, s := range in.Sources {
		if strings.Contains(strings.ToLower(s.V), "spam") {
			five := 5
			return IdentityOut{Name: "Penelepon ditandai spam", Role: "Vendor", Company: "—", Status: "not_prospect", FitScore: &five, Confidence: 0.9,
				Overview: "Ditandai spam oleh sumber identifikasi. Tidak dibuat lead dan tidak dihitung di funnel; sales bisa membalikkan penandaan ini."}, nil
		}
	}
	if strings.Contains(l, "kerjasama") || strings.Contains(l, "agensi") || strings.Contains(l, "menawarkan") {
		five := 5
		return IdentityOut{Name: "Vendor / penawaran kerja sama", Role: "Vendor", Company: "—", Status: "not_prospect", FitScore: &five, Confidence: 0.7,
			Overview: "Pesan pertama berupa penawaran dari vendor, bukan kebutuhan. Tidak dibuat lead."}, nil
	}
	var name, company string
	useful := 0
	for _, s := range in.Sources {
		if s.V == "" || strings.HasPrefix(s.V, "Tidak ada") {
			continue
		}
		useful++
		if name == "" {
			name = strings.Split(strings.Trim(s.V, "“”"), " · ")[0]
		}
		if s.C == "web" && company == "" {
			company = strings.Split(s.V, " — ")[0]
		}
	}
	if useful < 2 {
		return IdentityOut{Name: "Belum teridentifikasi", Role: "—", Company: "—", Status: "unknown", Confidence: 0,
			Overview: "Tidak ada sumber yang cocok. Jangan kirim materi generik — satu pertanyaan sopan tentang perusahaan dan kebutuhannya cukup; kalau dijawab, identifikasi diulang otomatis."}, nil
	}
	fit := 40
	for _, kw := range []string{"videotron", "cctv", "led", "fire alarm", "command center", "videowall", "signage", "penawaran"} {
		if strings.Contains(l, kw) {
			fit += 15
		}
	}
	if fit > 90 {
		fit = 90
	}
	return IdentityOut{Name: name, Role: "kontak inbound", Company: defaultStr(company, "—"), Status: "identified", FitScore: &fit, Confidence: 0.7,
		Overview: fmt.Sprintf("Teridentifikasi dari %d sumber. Permintaan pertama: “%s”. Jawab cepat yang diminta, lalu gali kebutuhan lain lewat pertanyaan pain point.", useful, trunc(in.FirstMessage, 120))}, nil
}

// ResearchInput is the context for the solution ladder.
type ResearchInput struct {
	Phone        string `json:"phone"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Company      string `json:"company"`
	FirstMessage string `json:"first_message"`
	Overview     string `json:"overview"`
}

// Research builds the solution ladder and ≤ 5 pain-point questions for an identified inbound contact.
func (a *Agents) Research(ctx context.Context, id string) error {
	var in ResearchInput
	var identRaw []byte
	if err := a.DB.Pool.QueryRow(ctx, `SELECT phone, first_message, overview, identification FROM inbound_contacts WHERE id=$1`, id).Scan(&in.Phone, &in.FirstMessage, &in.Overview, &identRaw); err != nil {
		return err
	}
	var ident map[string]any
	_ = json.Unmarshal(identRaw, &ident)
	in.Name, _ = ident["name"].(string)
	in.Role, _ = ident["role"].(string)
	in.Company, _ = ident["company"].(string)
	raw, _ := json.Marshal(in)
	var out struct {
		Solutions []map[string]any `json:"solutions"`
		Questions []map[string]any `json:"questions"`
	}
	_, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "research_solutions", System: prompts.Get("research/v1"),
		Messages: []llm.Message{{Role: "user", Content: string(raw)}},
		Schema: obj([]string{"solutions", "questions"}, map[string]any{
			"solutions": arr(obj([]string{"t", "v", "k", "why"}, map[string]any{"t": str(), "v": map[string]any{"type": "number"}, "k": map[string]any{"type": "string", "enum": []string{"proses", "peluang", "paket"}}, "why": str()})),
			"questions": arr(obj([]string{"q", "u"}, map[string]any{"q": str(), "u": str()})),
		}), MaxTokens: 4000, FakeInput: in}, &out)
	if err != nil {
		return err
	}
	if len(out.Questions) > 5 {
		out.Questions = out.Questions[:5]
	}
	_, err = a.DB.Pool.Exec(ctx, `UPDATE inbound_contacts SET solutions=$2, pain_questions=$3, updated_at=now() WHERE id=$1`, id, storage.JSON(out.Solutions), storage.JSON(out.Questions))
	return err
}

// linePrices is the indicative price table per product line (config for the Research agent).
var linePrices = []struct {
	kw, label string
	v         float64
	why       string
}{
	{"videotron", "Videotron sesuai permintaan", 280e6, "Permintaan langsung · pintu masuk, jawab cepat"},
	{"cctv", "CCTV + analitik", 900e6, "Keamanan aset & area"},
	{"fire alarm", "Fire alarm addressable", 450e6, "Kepatuhan asuransi & regulasi"},
	{"command center", "Command center mini", 600e6, "Monitoring terpusat"},
	{"led", "LED indoor", 250e6, "Ruang rapat / auditorium"},
	{"signage", "Digital signage", 120e6, "Informasi & branding"},
}

func (a *Agents) fakeResearch(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(ResearchInput)
	if fx, ok := a.Fx.Inbound[domain.NormalizePhone(in.Phone)]; ok && len(fx.Solutions) > 0 {
		return map[string]any{"solutions": fx.Solutions, "questions": fx.Questions}, nil
	}
	l := strings.ToLower(in.FirstMessage)
	var sols []map[string]any
	total := 0.0
	for _, p := range linePrices {
		k := "peluang"
		if strings.Contains(l, p.kw) {
			k = "proses"
		}
		sols = append(sols, map[string]any{"t": p.label, "v": p.v, "k": k, "why": p.why})
		total += p.v
		if len(sols) == 4 {
			break
		}
	}
	sols = append(sols, map[string]any{"t": "Paket lengkap + maintenance 3 tahun", "v": total * 1.1, "k": "paket", "why": "Satu vendor, satu SLA"})
	qs := []map[string]any{
		{"q": "Kebutuhan ini untuk satu lokasi atau ada cabang/gudang lain?", "u": "Skala proyek · cabang"},
		{"q": "Sistem keamanan (CCTV, fire alarm) di lokasi sekarang seperti apa?", "u": "CCTV + fire alarm"},
		{"q": "Bagaimana pemantauan lokasi dilakukan saat ini?", "u": "Command center"},
		{"q": "Anggarannya tahunan atau per proyek, dan siapa yang menyetujui?", "u": "Timing & pengambil keputusan"},
	}
	return map[string]any{"solutions": sols, "questions": qs}, nil
}
