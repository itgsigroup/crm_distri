package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"arc/packages/core/llm"
	"arc/packages/core/prompts"
)

// SuggestInput is the context for reply suggestions.
type SuggestInput struct {
	ThreadID string   `json:"thread_id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Owner    string   `json:"owner"`
	Last     []string `json:"last"`
}

// Suggestions returns three short replies for a chat thread (Follow-up agent v0, heavy tier).
func (a *Agents) Suggestions(ctx context.Context, threadID string) ([]string, error) {
	in := SuggestInput{ThreadID: threadID}
	if err := a.DB.Pool.QueryRow(ctx, `SELECT t.name, t.type, COALESCE(u.name,'') FROM chat_threads t JOIN wa_sessions w ON w.id=t.session_id LEFT JOIN users u ON u.id=w.user_id WHERE t.id=$1`,
		threadID).Scan(&in.Name, &in.Type, &in.Owner); err != nil {
		return nil, err
	}
	if in.Type == "internal" {
		return nil, nil
	}
	rows, err := a.DB.Pool.Query(ctx, `SELECT CASE WHEN direction='out' THEN 'GSI' ELSE sender_name END || ': ' || body_text FROM interactions WHERE thread_id=$1 ORDER BY occurred_at DESC LIMIT 10`, threadID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		in.Last = append([]string{s}, in.Last...)
	}
	rows.Close()
	raw, _ := json.Marshal(in)
	var out struct {
		Suggestions []string `json:"suggestions"`
	}
	_, err = a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "suggest_replies", System: prompts.Get("followup/v1"),
		Messages: []llm.Message{{Role: "user", Content: "Saran balasan untuk percakapan berikut:\n" + string(raw)}},
		Schema:   obj([]string{"suggestions"}, map[string]any{"suggestions": arr(str())}), MaxTokens: 1000, FakeInput: in}, &out)
	if err != nil {
		return nil, err
	}
	if len(out.Suggestions) > 3 {
		out.Suggestions = out.Suggestions[:3]
	}
	return out.Suggestions, nil
}

func (a *Agents) fakeSuggestions(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(SuggestInput)
	if s, ok := a.Fx.Suggestions[in.ThreadID]; ok && len(s) > 0 {
		return map[string]any{"suggestions": s}, nil
	}
	last := ""
	if len(in.Last) > 0 {
		last = strings.ToLower(in.Last[len(in.Last)-1])
	}
	sug := []string{"Terima kasih, kami tindak lanjuti", "Boleh kami jadwalkan panggilan singkat?", "Kami kirim detailnya hari ini"}
	switch {
	case strings.Contains(last, "harga") || strings.Contains(last, "penawaran"):
		sug = []string{"Kami siapkan penawaran hari ini", "Boleh tahu kebutuhan & ukurannya?", "Kapan waktu yang pas untuk diskusi?"}
	case strings.Contains(last, "jadwal") || strings.Contains(last, "kapan"):
		sug = []string{"Konfirmasi jadwalnya besok pagi", "Kami cek ketersediaan tim dulu", "Apakah ada PIC di lokasi?"}
	}
	return map[string]any{"suggestions": sug}, nil
}

// DraftInput is the context for a follow-up draft.
type DraftInput struct {
	Kind      string `json:"kind"`
	Account   string `json:"account"`
	Contact   string `json:"contact"`
	Owner     string `json:"owner"`
	Context   string `json:"context"`
	Channel   string `json:"channel"`
	DaysQuiet int    `json:"days_quiet"`
	Rhythm    int    `json:"rhythm"`
}

// DraftOut is a proposal body produced by Follow-up / Collection.
type DraftOut struct {
	Title   string   `json:"title"`
	Why     string   `json:"why"`
	Prep    string   `json:"prep"`
	Preview string   `json:"preview"`
	Steps   []string `json:"steps"`
}

// Draft asks the Follow-up agent for a draft (heavy tier).
func (a *Agents) Draft(ctx context.Context, in DraftInput) (DraftOut, string, error) {
	raw, _ := json.Marshal(in)
	var out DraftOut
	resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "followup_draft", System: prompts.Get("followup/v1"),
		Messages:  []llm.Message{{Role: "user", Content: "Siapkan draf tindak lanjut:\n" + string(raw)}},
		Schema:    obj([]string{"title", "why", "prep", "preview", "steps"}, map[string]any{"title": str(), "why": str(), "prep": str(), "preview": str(), "steps": arr(str())}),
		MaxTokens: 3000, FakeInput: in}, &out)
	return out, resp.Model, err
}

func (a *Agents) fakeFollowupDraft(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(DraftInput)
	switch in.Kind {
	case "reengage":
		return DraftOut{
			Title:   "Sapa kembali " + in.Account,
			Why:     fmt.Sprintf("Sunyi %d hari; ritme normal akun ini %d hari. %s", in.DaysQuiet, in.Rhythm, in.Context),
			Prep:    fmt.Sprintf("Draft %s singkat dari nomor %s ke %s, menanyakan kabar dan menawarkan bantuan langkah berikutnya.", in.Channel, in.Owner, in.Contact),
			Preview: fmt.Sprintf("%s, selamat siang. Menyusul diskusi kita sebelumnya, apakah ada perkembangan yang bisa kami bantu siapkan? Tim kami siap kapan saja.", in.Contact),
			Steps:   []string{"Pesan dikirim dari nomor " + in.Owner, "ARC memantau balasan; kalau sunyi 7 hari lagi, usulkan kunjungan"},
		}, nil
	case "commitment_due":
		return DraftOut{
			Title:   "Penuhi janji ke " + in.Contact,
			Why:     "Komitmen kita jatuh tempo: " + in.Context,
			Prep:    "Draft dalam gaya tulis " + in.Owner + " dengan lampiran dokumen terkait dari Odoo.",
			Preview: fmt.Sprintf("%s, sesuai janji kami, berikut %s. Mohon kabari bila ada yang perlu disesuaikan.", in.Contact, in.Context),
			Steps:   []string{"Draft masuk antrean approval", "Komitmen ledger ditandai terpenuhi setelah terkirim"},
		}, nil
	}
	return DraftOut{Title: "Tindak lanjut " + in.Account, Why: in.Context, Prep: "Draft singkat disiapkan Follow-up agent.",
		Preview: in.Contact + ", kami ingin menindaklanjuti diskusi terakhir.", Steps: []string{"Masuk antrean approval"}}, nil
}
