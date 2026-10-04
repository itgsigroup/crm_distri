package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/llm"
	"arc/packages/core/prompts"
	"arc/packages/core/storage"
)

// MemoryInput is the context for the account memory.
type MemoryInput struct {
	AccountID    string   `json:"account_id"`
	Account      string   `json:"account"`
	Previous     string   `json:"previous"`
	Interactions []string `json:"interactions"`
	IDs          []int64  `json:"ids"`
}

// UpdateMemory rewrites the living account memory (≤ 180 words) from recent interactions.
func (a *Agents) UpdateMemory(ctx context.Context, accountID string) error {
	in := MemoryInput{AccountID: accountID}
	if err := a.DB.Pool.QueryRow(ctx, `SELECT name, memory FROM accounts WHERE id=$1`, accountID).Scan(&in.Account, &in.Previous); err != nil {
		return err
	}
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, channel, to_char(occurred_at,'DD Mon'), participants_label || sender_name, body_text FROM interactions
		WHERE account_id=$1 AND channel <> 'wa_aggregate' ORDER BY occurred_at DESC LIMIT 20`, accountID)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for rows.Next() {
		var id int64
		var ch, d, who, body string
		if err := rows.Scan(&id, &ch, &d, &who, &body); err != nil {
			rows.Close()
			return err
		}
		counts[ch]++
		in.IDs = append(in.IDs, id)
		in.Interactions = append(in.Interactions, fmt.Sprintf("#%d %s %s %s: %s", id, d, ch, who, trunc(body, 300)))
	}
	rows.Close()
	if len(in.IDs) == 0 {
		return nil
	}
	raw, _ := json.Marshal(in)
	var out struct {
		Memory   string            `json:"memory"`
		Evidence []domain.Evidence `json:"evidence"`
	}
	resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "memory", System: prompts.Get("memory/v1"),
		Messages:  []llm.Message{{Role: "user", Content: string(raw)}},
		Schema:    obj([]string{"memory", "evidence"}, map[string]any{"memory": str(), "evidence": arr(obj([]string{"interaction_id", "quote"}, map[string]any{"interaction_id": map[string]any{"type": "integer"}, "quote": str()}))}),
		MaxTokens: 2000, FakeInput: in}, &out)
	if err != nil {
		return err
	}
	words := strings.Fields(out.Memory)
	if len(words) > 180 {
		out.Memory = strings.Join(words[:180], " ") + "…"
	}
	if len(out.Evidence) == 0 {
		out.Evidence = []domain.Evidence{{InteractionID: in.IDs[0], Quote: "ringkasan 20 interaksi terakhir"}}
	}
	if strings.TrimSpace(out.Memory) == strings.TrimSpace(in.Previous) {
		return nil
	}
	prov := []map[string]string{}
	label := map[string]string{"email": "email", "meeting": "meeting", "document": "dokumen", "wa_message": "WhatsApp", "wa_group_message": "WhatsApp grup", "call": "telepon"}
	icon := map[string]string{"email": "i-mail", "meeting": "i-people", "document": "i-doc", "wa_message": "i-chat", "wa_group_message": "i-chat", "call": "i-phone"}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if l, ok := label[k]; ok {
			prov = append(prov, map[string]string{"icon": icon[k], "label": fmt.Sprintf("%d %s", counts[k], l)})
		}
	}
	return a.DB.Tx(ctx, func(tx pgxTx) error {
		var ver int
		if err := tx.QueryRow(ctx, `UPDATE accounts SET memory=$2, memory_version=memory_version+1, memory_provenance=$3, memory_updated_at=$4, updated_at=now() WHERE id=$1 RETURNING memory_version`,
			accountID, out.Memory, storage.JSON(prov), domain.Now()).Scan(&ver); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO account_memory_history(account_id,version,memory,evidence,model,prompt_version) VALUES ($1,$2,$3,$4,$5,'memory/v1') ON CONFLICT DO NOTHING`,
			accountID, ver, out.Memory, storage.JSON(out.Evidence), resp.Model)
		return err
	})
}

// AppendMemory adds a human/MCP note to the account memory (audited).
func (a *Agents) AppendMemory(ctx context.Context, accountID, note string, actor storage.Actor) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return fmt.Errorf("catatan kosong")
	}
	if _, err := a.DB.Pool.Exec(ctx, `UPDATE accounts SET memory = memory || E'\n' || $2, memory_version=memory_version+1, memory_updated_at=$3 WHERE id=$1`, accountID, "Catatan "+actor.ID+": "+note, domain.Now()); err != nil {
		return err
	}
	return storage.Audit(ctx, a.DB.Pool, actor, "memory.append", "account", accountID, map[string]any{"note": note})
}

func (a *Agents) fakeMemory(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(MemoryInput)
	memo := in.Previous
	if memo == "" {
		memo = a.Fx.Memos[in.AccountID]
	}
	if memo == "" && len(in.Interactions) > 0 {
		memo = in.Account + ": " + strings.Join(in.Interactions[:min(3, len(in.Interactions))], " ")
	}
	ev := []map[string]any{}
	if len(in.IDs) > 0 {
		ev = append(ev, map[string]any{"interaction_id": in.IDs[0], "quote": "interaksi terbaru"})
	}
	return map[string]any{"memory": memo, "evidence": ev}, nil
}

// Hygiene merges duplicate people, marks internal numbers, purges expired
// identifications and suggests internal numbers seen in ≥ 3 groups.
func (a *Agents) Hygiene(ctx context.Context) (map[string]int, error) {
	res := map[string]int{}
	// Internal registry → people.is_internal.
	tag, err := a.DB.Pool.Exec(ctx, `UPDATE people p SET is_internal=true, updated_at=now() WHERE NOT p.is_internal AND EXISTS (
		SELECT 1 FROM internal_numbers n, unnest(p.phones) ph WHERE n.phone_norm = regexp_replace(ph,'[^0-9]','','g'))`)
	if err != nil {
		return res, err
	}
	res["marked_internal"] = int(tag.RowsAffected())
	// Retention: identifications older than 90 days that never became a lead.
	tag, err = a.DB.Pool.Exec(ctx, `DELETE FROM inbound_contacts WHERE expires_at < $1 AND status NOT IN ('lead','qualified')`, domain.Now())
	if err != nil {
		return res, err
	}
	res["purged_inbound"] = int(tag.RowsAffected())
	// Internal suspects: phones in ≥ 3 groups that are not external people.
	tag, err = a.DB.Pool.Exec(ctx, `INSERT INTO internal_suspects(phone, phone_norm, name_hint, reason, group_count)
		SELECT max(m->>'phone'), regexp_replace(m->>'phone','[^0-9]','','g'), max(m->>'n'), 'Muncul di '||count(DISTINCT g.id)||' grup project; tidak ada di kontak Odoo maupun Talenta', count(DISTINCT g.id)
		FROM chat_groups g, jsonb_array_elements(g.members) m
		WHERE COALESCE(m->>'phone','') <> '' AND NOT EXISTS (SELECT 1 FROM internal_numbers n WHERE n.phone_norm = regexp_replace(m->>'phone','[^0-9]','','g'))
		  AND NOT EXISTS (SELECT 1 FROM people p, unnest(p.phones) ph WHERE NOT p.is_internal AND regexp_replace(ph,'[^0-9]','','g') = regexp_replace(m->>'phone','[^0-9]','','g'))
		GROUP BY 2 HAVING count(DISTINCT g.id) >= 3 ON CONFLICT (phone_norm) DO NOTHING`)
	if err != nil {
		return res, err
	}
	res["internal_suspects"] = int(tag.RowsAffected())
	// Duplicate people (same account; shared phone/email or very similar name).
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, name, COALESCE(account_id,''), phones, emails FROM people WHERE NOT is_internal ORDER BY created_at`)
	if err != nil {
		return res, err
	}
	type p struct {
		id, name, acc  string
		phones, emails []string
	}
	var ps []p
	for rows.Next() {
		var x p
		if err := rows.Scan(&x.id, &x.name, &x.acc, &x.phones, &x.emails); err != nil {
			rows.Close()
			return res, err
		}
		ps = append(ps, x)
	}
	rows.Close()
	for i := 0; i < len(ps); i++ {
		for j := i + 1; j < len(ps); j++ {
			x, y := ps[i], ps[j]
			if x.acc != y.acc || x.acc == "" {
				continue
			}
			conf := nameSimilarity(x.name, y.name) * 0.6
			if overlap(x.phones, y.phones, true) || overlap(x.emails, y.emails, false) {
				conf += 0.4
			}
			if conf < 0.75 {
				continue
			}
			if conf >= 0.9 {
				if err := a.mergePeople(ctx, x.id, y.id); err != nil {
					return res, err
				}
				res["merged"]++
				continue
			}
			_, created, err := a.Actions.Propose(ctx, actions.Proposal{
				Agent: domain.AgentHygiene, Type: "merge_person", Kind: "internal", Icon: "i-people", AccountID: x.acc,
				Title: fmt.Sprintf("Gabungkan kontak “%s” dan “%s”", x.name, y.name), ButtonLabel: "Gabungkan",
				Why:   fmt.Sprintf("Nama mirip (%.0f%%) di akun yang sama; confidence %.2f di bawah ambang auto-merge 0,9.", nameSimilarity(x.name, y.name)*100, conf),
				Prep:  "Interaksi, nomor, dan email kontak kedua dipindahkan ke kontak pertama; tidak ada yang dihapus permanen sebelum disetujui.",
				Steps: []string{"Kontak digabung", "Peta stakeholder dan health dihitung ulang"}, Payload: map[string]any{"keep": x.id, "drop": y.id},
				Evidence: []domain.Evidence{{Source: "people", Quote: x.name + " ↔ " + y.name}}, Confidence: conf,
			}, storage.Actor{ID: "hygiene", Type: "agent"})
			if err != nil {
				return res, err
			}
			if created {
				res["merge_proposed"]++
			}
		}
	}
	return res, nil
}

// MergePeople merges drop into keep (also used by the merge_person executor).
func (a *Agents) MergePeople(ctx context.Context, keep, drop string) error {
	return a.mergePeople(ctx, keep, drop)
}

func (a *Agents) mergePeople(ctx context.Context, keep, drop string) error {
	return a.DB.Tx(ctx, func(tx pgxTx) error {
		if _, err := tx.Exec(ctx, `UPDATE people k SET phones=(SELECT array_agg(DISTINCT x) FROM unnest(k.phones || d.phones) x), emails=(SELECT array_agg(DISTINCT x) FROM unnest(k.emails || d.emails) x),
			wa_ids=(SELECT array_agg(DISTINCT x) FROM unnest(k.wa_ids || d.wa_ids) x), strength=GREATEST(k.strength,d.strength), updated_at=now() FROM people d WHERE k.id=$1 AND d.id=$2`, keep, drop); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE interactions SET person_ids = array_replace(person_ids, $2, $1) WHERE $2 = ANY(person_ids)`, keep, drop); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE chat_threads SET person_id=$1 WHERE person_id=$2`, keep, drop); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM people WHERE id=$1`, drop); err != nil {
			return err
		}
		return storage.Audit(ctx, tx, storage.Actor{ID: "hygiene", Type: "agent"}, "merge", "person", keep, map[string]any{"dropped": drop})
	})
}

func overlap(a, b []string, phone bool) bool {
	for _, x := range a {
		for _, y := range b {
			if phone {
				if domain.NormalizePhone(x) == domain.NormalizePhone(y) && domain.NormalizePhone(x) != "" {
					return true
				}
			} else if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}

func normName(s string) string {
	s = strings.ToLower(s)
	for _, p := range []string{"pak ", "bu ", "dr. ", "ibu ", "bapak "} {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimSpace(s)
}

// nameSimilarity is 1 − normalized Levenshtein distance.
func nameSimilarity(a, b string) float64 {
	x, y := []rune(normName(a)), []rune(normName(b))
	if len(x) == 0 || len(y) == 0 {
		return 0
	}
	prev := make([]int, len(y)+1)
	cur := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(x); i++ {
		cur[0] = i
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	d := prev[len(y)]
	return 1 - float64(d)/float64(max(len(x), len(y)))
}
