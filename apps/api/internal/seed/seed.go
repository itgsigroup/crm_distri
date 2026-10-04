package seed

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"arc/packages/core/domain"
	"arc/packages/core/health"
	"arc/packages/core/storage"
)

// DefaultPassword is the development password for seeded users (documented in README).
const DefaultPassword = "arc12345"

// Seeder writes fixtures into the database.
type Seeder struct {
	DB  *storage.DB
	F   *Fixtures
	C   Clock
	ctx context.Context
	tx  pgx.Tx
	// derived lookups
	personByName map[string]string // display name -> person id
	stageID      map[string]int
	seq          int
}

// Run seeds everything inside one transaction. It is idempotent.
func Run(ctx context.Context, db *storage.DB, f *Fixtures) error {
	mn, err := time.Parse(time.RFC3339, f.Extra.MockupNow)
	if err != nil {
		return fmt.Errorf("mockup_now: %w", err)
	}
	s := &Seeder{DB: db, F: f, C: Clock{MockupNow: mn, Anchor: domain.Now()}, ctx: ctx, personByName: map[string]string{}, stageID: map[string]int{}}
	return db.Tx(ctx, func(tx pgx.Tx) error {
		s.tx = tx
		steps := []struct {
			name string
			fn   func() error
		}{
			{"users", s.users}, {"stages", s.stages}, {"policies", s.policies}, {"accounts", s.accounts},
			{"people", s.people}, {"opportunities", s.opportunities}, {"wa", s.whatsapp}, {"interactions", s.interactions},
			{"chats", s.chats}, {"commitments", s.commitments}, {"signals", s.signals}, {"cash", s.cash},
			{"actions", s.actions}, {"installed", s.installed}, {"inbound", s.inbound}, {"internal", s.internal},
			{"ops", s.ops}, {"history", s.history}, {"health", s.health},
		}
		for _, st := range steps {
			if err := st.fn(); err != nil {
				return fmt.Errorf("seed %s: %w", st.name, err)
			}
		}
		tag, err := tx.Exec(ctx, `INSERT INTO settings(key,value) VALUES ('seeded_at', to_jsonb(now()::text)) ON CONFLICT DO NOTHING`)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already seeded: every insert above was a no-op
		}
		return storage.Audit(ctx, tx, storage.SystemActor, "seed", "database", "fixtures", map[string]any{"anchor": s.C.Anchor})
	})
}

func (s *Seeder) exec(sql string, args ...any) error {
	_, err := s.tx.Exec(s.ctx, sql, args...)
	return err
}

// ts returns strictly increasing creation timestamps so lists keep fixture order.
func (s *Seeder) ts() time.Time {
	s.seq++
	return s.C.Anchor.Add(-240 * time.Hour).Add(time.Duration(s.seq) * time.Second)
}

func (s *Seeder) user(salesName string) any {
	if id, ok := s.F.Extra.SalesUser[salesName]; ok {
		return id
	}
	return nil
}

func (s *Seeder) users() error {
	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	for _, u := range s.F.Extra.Users {
		if err := s.exec(`INSERT INTO users(id,name,email,role,branch,initials,wa_numbers,password_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`,
			u.ID, u.Name, u.Email, u.Role, u.Branch, u.Initials, u.WA, string(hash)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) stages() error {
	for i, st := range []struct {
		n         string
		won, lost bool
	}{{"Baru", false, false}, {"Berkualifikasi", false, false}, {"Penawaran", false, false}, {"Won", true, false}, {"Lost", false, true}} {
		if err := s.exec(`INSERT INTO stage_definitions(name,seq,is_won,is_lost) VALUES ($1,$2,$3,$4) ON CONFLICT (name) DO NOTHING`, st.n, i+1, st.won, st.lost); err != nil {
			return err
		}
	}
	rows, err := s.tx.Query(s.ctx, `SELECT id,name FROM stage_definitions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			return err
		}
		s.stageID[n] = id
	}
	return rows.Err()
}

func (s *Seeder) policies() error {
	desc := map[string]string{
		"discount_max_without_ceo": "Diskon di atas nilai ini (persen) butuh persetujuan CEO",
		"quiet_threshold_days":     "Akun dianggap sunyi setelah N hari (atau 2× ritme normal)",
		"gov_reminder_min_days":    "Pengingat piutang pemerintah tidak sebelum H+N",
		"single_thread_share":      "Single-threaded bila ≥ N% interaksi lewat satu orang",
		"bast_benchmark_days":      "Benchmark BAST clear → invoice (hari)",
		"prep_benchmark_days":      "Benchmark persiapan (hari)",
		"install_benchmark_days":   "Benchmark pemasangan (hari)",
		"quarter_target":           "Target penjualan kuartal berjalan (Rp)",
	}
	keys := make([]string, 0, len(s.F.Extra.Policies))
	for k := range s.F.Extra.Policies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := s.exec(`INSERT INTO policies(key,value,description) VALUES ($1,$2,$3) ON CONFLICT (key) DO NOTHING`, k, storage.JSONObj(s.F.Extra.Policies[k]), desc[k]); err != nil {
			return err
		}
	}
	return nil
}

var memoIcons = []string{"i-mail", "i-people", "i-doc", "i-chat"}

func (s *Seeder) accounts() error {
	for _, d := range s.F.Deals {
		meta := s.F.Extra.AccountMeta[d.ID]
		prov := []map[string]string{}
		for i, p := range d.MemoProv[:len(d.MemoProv)-1] {
			icon := "i-doc"
			if i < len(memoIcons) {
				icon = memoIcons[i]
			}
			if strings.Contains(p, "WhatsApp") {
				icon = "i-chat"
			} else if strings.Contains(p, "pembayaran") {
				icon = "i-box"
			}
			prov = append(prov, map[string]string{"icon": icon, "label": p})
		}
		updated := memoUpdated(s.C.Anchor, d.MemoProv[len(d.MemoProv)-1])
		if err := s.exec(`INSERT INTO accounts(id,name,sector,branch,owner_user_id,normal_rhythm_days,is_government,memory,memory_version,memory_provenance,memory_updated_at,tags,last_interaction_at,last_via,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1,$9,$10,$11,$12,$13,$14) ON CONFLICT (id) DO NOTHING`,
			d.ID, d.Account, d.Sector, d.Branch, s.user(d.Owner), meta.Rhythm, meta.Gov, d.Memo, storage.JSON(prov), updated, d.Tags,
			s.C.At(meta.LastAt), d.LastVia, s.ts()); err != nil {
			return err
		}
		if err := s.exec(`INSERT INTO account_memory_history(account_id,version,memory,evidence,model,prompt_version) VALUES ($1,1,$2,$3,'fixture','memory/v1') ON CONFLICT DO NOTHING`,
			d.ID, d.Memo, storage.JSON([]domain.Evidence{{Source: "fixture", Quote: strings.Join(d.MemoProv, ", ")}})); err != nil {
			return err
		}
	}
	for _, a := range s.F.Extra.AccountsExtra {
		if err := s.exec(`INSERT INTO accounts(id,name,sector,branch,owner_user_id,is_government,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
			a.ID, a.Name, a.Sector, a.Branch, s.user(a.Owner), a.Gov, s.ts()); err != nil {
			return err
		}
	}
	return nil
}

var reAgo = regexp.MustCompile(`(\d+) (jam|hari) lalu`)

func memoUpdated(now time.Time, label string) time.Time {
	switch {
	case strings.Contains(label, "kemarin"):
		return now.Add(-24 * time.Hour)
	case strings.Contains(label, "hari ini"):
		return now.Add(-2 * time.Hour)
	}
	if m := reAgo.FindStringSubmatch(label); m != nil {
		n, _ := strconv.Atoi(m[1])
		if m[2] == "jam" {
			return now.Add(-time.Duration(n) * time.Hour)
		}
		return now.AddDate(0, 0, -n)
	}
	return now
}

func personID(accountID, name string) string {
	n := strings.ToLower(name)
	for _, p := range []string{"pak ", "bu ", "dr. "} {
		n = strings.TrimPrefix(n, p)
	}
	n = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, n)
	return accountID + "-" + strings.Trim(n, "-")
}

func (s *Seeder) people() error {
	byName := map[string]Contact{}
	for _, c := range s.F.Contacts {
		byName[c.N] = c
	}
	used := map[string]bool{}
	insert := func(id, name, role, acc, tag string, strength int, note string, decision bool) error {
		var phones, emails []string
		if p, ok := s.F.Extra.Phones[id]; ok {
			phones = []string{p}
		}
		if e, ok := s.F.Extra.Emails[id]; ok {
			emails = []string{e}
		}
		waIDs := []string{}
		for _, p := range phones {
			waIDs = append(waIDs, domain.NormalizePhone(p)+"@s.whatsapp.net")
		}
		var accArg any = acc
		if acc == "" {
			accArg = nil
		}
		s.personByName[name] = id
		return s.exec(`INSERT INTO people(id,name,role,account_id,phones,emails,wa_ids,stakeholder_tag,strength,stakeholder_note,is_decision,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (id) DO NOTHING`,
			id, name, role, accArg, nz(phones), nz(emails), waIDs, tag, strength, note, decision, s.ts())
	}
	for _, d := range s.F.Deals {
		for _, sh := range d.Stakeholders {
			id := personID(d.ID, sh.N)
			if c, ok := byName[sh.N]; ok {
				id = c.ID
				used[c.ID] = true
			}
			if err := insert(id, sh.N, sh.Role, d.ID, sh.Tag, sh.S, sh.Note, sh.Tag == "decision"); err != nil {
				return err
			}
		}
	}
	for _, c := range s.F.Contacts {
		if used[c.ID] {
			continue
		}
		acc := ""
		if c.Acc != nil {
			acc = *c.Acc
		} else if a, ok := s.F.Extra.ContactAccounts[c.ID]; ok {
			acc = a
		}
		if err := insert(c.ID, c.N, c.Role, acc, "user", 1, "", c.Decision); err != nil {
			return err
		}
	}
	// Internal staff appear as people flagged internal (excluded from stakeholders and the network).
	for _, in := range s.F.Internal {
		id := "int-" + strings.Trim(strings.ToLower(strings.ReplaceAll(in.N, " ", "-")), "-")
		phone := s.F.Extra.InternalFull[in.N]
		s.personByName[in.N] = id
		if err := s.exec(`INSERT INTO people(id,name,role,phones,wa_ids,is_internal,internal_unit,created_at) VALUES ($1,$2,$3,$4,$5,true,$6,$7) ON CONFLICT (id) DO NOTHING`,
			id, in.N, in.Unit+" · "+in.Branch, []string{phone}, []string{domain.NormalizePhone(phone) + "@s.whatsapp.net"}, in.Unit, s.ts()); err != nil {
			return err
		}
	}
	return nil
}

func nz(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

var signalKey = map[string]string{"Negosiasi": "negosiasi", "Verbal commit": "verbal_commit", "Kontrak": "kontrak"}

func (s *Seeder) opportunities() error {
	for _, d := range s.F.Deals {
		meta := s.F.Extra.AccountMeta[d.ID]
		if err := s.exec(`INSERT INTO opportunities(id,account_id,name,expected_revenue,probability,stage_id,date_deadline,closing_label,tags,priority,activity_state,owner_user_id,source,status,lead_at,signal,stage_evidence,product_line,created_at)
			VALUES ($1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'open',$13,$14,$15,$16,$17) ON CONFLICT (id) DO NOTHING`,
			d.ID, d.Opp, d.Value, d.ManualProb, s.stageID[d.OdooStage], s.C.At(s.F.Extra.DealDeadlines[d.ID]), d.Closing, d.Tags, d.Prio, d.Activity,
			s.user(d.Owner), meta.Source, s.C.At(s.F.Extra.DealLeadAt[d.ID]), signalKey[d.Signal], d.StageWhy, firstTag(d.Tags), s.ts()); err != nil {
			return err
		}
		for _, b := range d.Breakdown {
			label, _ := b[0].(string)
			v, _ := b[1].(float64)
			key := componentKey(label)
			if err := s.exec(`INSERT INTO health_components(opportunity_id,component,value,origin,evidence,confidence,model,prompt_version)
				VALUES ($1,$2,$3,'fixture',$4,0.85,'fixture','health/v1') ON CONFLICT DO NOTHING`,
				d.ID, key, int(v), storage.JSON([]domain.Evidence{{Source: "fixture:breakdown", Quote: label}})); err != nil {
				return err
			}
		}
	}
	for _, w := range s.F.Won {
		m := s.F.Extra.Won[w.Aid]
		label := "Won " + w.Won
		if err := s.exec(`INSERT INTO opportunities(id,account_id,name,expected_revenue,probability,stage_id,closing_label,tags,owner_user_id,source,status,lead_at,won_at,product_line,created_at)
			VALUES ($1,$2,$3,$4,100,$5,$6,$7,$8,$9,'won',$10,$11,$12,$13) ON CONFLICT (id) DO NOTHING`,
			w.Aid, m.Account, w.Opp, w.Value, s.stageID["Won"], label, w.Tags, s.user(w.Owner), m.Source, s.C.At(m.LeadAt), s.C.At(m.WonAt), firstTag(w.Tags), s.ts()); err != nil {
			return err
		}
	}
	for _, l := range s.F.Leads {
		m := s.F.Extra.Lead[l.Aid]
		if err := s.exec(`INSERT INTO opportunities(id,account_id,name,expected_revenue,probability,stage_id,date_deadline,closing_label,tags,priority,owner_user_id,source,status,lead_at,note,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'open',$13,$14,$15) ON CONFLICT (id) DO NOTHING`,
			l.Aid, m.Account, l.Opp, l.Value, l.ManualProb, s.stageID["Baru"], s.C.At(m.Deadline), l.Closing, l.Tags, l.Prio, s.user(l.Owner), m.Source, s.C.At(m.LeadAt), l.Note, s.ts()); err != nil {
			return err
		}
	}
	return nil
}

func firstTag(t []string) string {
	if len(t) == 0 {
		return ""
	}
	return t[0]
}

func componentKey(label string) string {
	for _, c := range domain.HealthComponents {
		if c.Label == label {
			return c.Key
		}
	}
	return strings.ToLower(label)
}

var viaChannel = map[string]string{"mail": "email", "chat": "wa_message", "people": "meeting", "doc": "document", "form": "form", "box": "erp_event", "phone": "call"}

func (s *Seeder) interactions() error {
	for _, d := range s.F.Deals {
		for i, t := range d.Timeline {
			at, ok := s.C.ShortDate(t.D, 10)
			if !ok {
				at = s.C.Anchor
			}
			ch := viaChannel[t.Via]
			if ch == "" {
				ch = "note"
			}
			dir := "in"
			if strings.HasPrefix(t.Who, "Dewi") || strings.HasPrefix(t.Who, "Andi") || strings.HasPrefix(t.Who, "Rizky") || strings.HasPrefix(t.Who, "Proposal") {
				dir = "out"
			}
			sent := 0.2
			if t.Hot {
				sent = -0.3
			}
			if err := s.exec(`INSERT INTO interactions(channel,direction,occurred_at,participants_label,body_text,raw_ref,account_id,opportunity_id,inference,hot,extracted,extraction_version,sentiment,summary)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8,$9,true,1,$10,$5) ON CONFLICT (raw_ref) DO NOTHING`,
				ch, dir, at, t.Who, t.T, fmt.Sprintf("fixture:timeline:%s:%d", d.ID, i), d.ID, t.X, t.Hot, sent); err != nil {
				return err
			}
		}
	}
	// WhatsApp monthly aggregates per sales number × contact (network map history).
	salesUser := map[string]string{}
	for _, sv := range s.F.Sales {
		salesUser[sv.ID] = s.F.Extra.SalesUser[sv.N]
	}
	contactAcc := map[string]string{}
	for _, c := range s.F.Contacts {
		if c.Acc != nil {
			contactAcc[c.ID] = *c.Acc
		} else {
			contactAcc[c.ID] = s.F.Extra.ContactAccounts[c.ID]
		}
	}
	n := len(s.F.Edges.Months)
	for _, e := range s.F.Edges.Edges {
		sid, _ := e[0].(string)
		cid, _ := e[1].(string)
		counts, _ := e[2].([]any)
		for i, cv := range counts {
			c := int(cv.(float64))
			if c == 0 {
				continue
			}
			monthsBack := n - 1 - i
			at := s.C.Anchor.AddDate(0, 0, -10-30*monthsBack)
			var acc any
			if a := contactAcc[cid]; a != "" {
				acc = a
			}
			ref := fmt.Sprintf("agg:%s:%s:%d", sid, cid, monthsBack)
			if err := s.exec(`INSERT INTO interactions(channel,direction,occurred_at,person_ids,user_ids,raw_ref,account_id,message_count,transport,is_history,extracted,summary)
				VALUES ('wa_aggregate','in',$1,$2,$3,$4,$5,$6,'bridge',true,true,$7) ON CONFLICT (raw_ref) DO NOTHING`,
				at, []string{cid}, []string{salesUser[sid]}, ref, acc, c, fmt.Sprintf("%d pesan WhatsApp (%s)", c, s.F.Edges.Months[i])); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Seeder) whatsapp() error {
	for _, w := range s.F.Extra.WASessions {
		var phone string
		for _, u := range s.F.Extra.Users {
			if u.ID == w.User && len(u.WA) > 0 {
				phone = u.WA[0]
			}
		}
		var last, qrExp *time.Time
		if w.Status == "connected" {
			t := s.C.Anchor.Add(-time.Duration(w.LastEventMin) * time.Minute)
			last = &t
		}
		if w.Status == "pairing" {
			t := s.C.Anchor.Add(time.Duration(w.QRSeconds) * time.Second)
			qrExp = &t
		}
		qr := ""
		if w.Status == "pairing" {
			qr = "2@ARC-DEMO-" + w.ID + ",fixture-pairing-code"
		}
		if err := s.exec(`INSERT INTO wa_sessions(id,label,user_id,phone,transport,status,history_days,messages_30d,last_event_at,qr_code,qr_expires_at)
			VALUES ($1,$2,$3,$4,'bridge',$5,30,$6,$7,$8,$9) ON CONFLICT (id) DO NOTHING`,
			w.ID, w.Label, w.User, phone, w.Status, w.Messages30d, last, qr, qrExp); err != nil {
			return err
		}
	}
	chatsByID := map[string]ChatFixture{}
	for _, c := range s.F.Chats {
		chatsByID[c.ID] = c
	}
	for _, g := range s.F.Extra.Groups {
		members := []map[string]any{}
		project := map[string]any{}
		summary := []string{}
		for _, c := range s.F.Chats {
			if c.ID == g.ID {
				for _, m := range c.Members {
					members = append(members, map[string]any{"n": m.N, "r": m.R, "int": m.Int, "person_id": s.personByName[m.N]})
				}
				if c.Project != nil {
					project = map[string]any{"id": c.Project.ID, "stage": c.Project.Stage, "next": c.Project.Next}
				}
				summary = c.Summary
			}
		}
		if len(members) == 0 && g.Members > 0 {
			for i := 0; i < g.Members; i++ {
				members = append(members, map[string]any{"n": fmt.Sprintf("Anggota %d", i+1), "r": "Internal", "int": true})
			}
			if g.ID == "g-teknisi" {
				members = []map[string]any{}
				for _, in := range s.F.Internal {
					if in.Branch == "Semarang" {
						members = append(members, map[string]any{"n": in.N, "r": in.Unit, "int": true, "person_id": s.personByName[in.N]})
					}
				}
				for len(members) < g.Members {
					members = append(members, map[string]any{"n": fmt.Sprintf("Teknisi %d", len(members)+1), "r": "Teknisi", "int": true})
				}
			}
		}
		var acc any
		if g.Account != "" {
			acc = g.Account
		}
		if err := s.exec(`INSERT INTO chat_groups(id,session_id,jid,name,type,read_policy,members,account_id,project,summary)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (id) DO NOTHING`,
			g.ID, g.Session, g.JID, g.Name, g.Type, g.Read, storage.JSON(members), acc, storage.JSONObj(project), storage.JSON(summary)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) dayTime(day, tm string) time.Time {
	d := s.F.Extra.ChatDays[day]
	if d == "" {
		d = "2026-09-28"
	}
	tm = strings.ReplaceAll(tm, ".", ":")
	return s.C.At(d + "T" + tm)
}

func annKind(t string) string {
	l := strings.ToLower(t)
	switch {
	case strings.HasPrefix(l, "komitmen"):
		return "commitment"
	case strings.HasPrefix(l, "sinyal"):
		return "signal"
	case strings.HasPrefix(l, "risiko"), strings.HasPrefix(l, "kendala"):
		return "risk"
	case strings.HasPrefix(l, "tugas"), strings.Contains(l, "tugas →"):
		return "task"
	case strings.HasPrefix(l, "milestone"):
		return "milestone"
	case strings.HasPrefix(l, "stage"):
		return "stage"
	case strings.HasPrefix(l, "syarat"):
		return "task"
	}
	return "note"
}

func (s *Seeder) chats() error {
	for _, c := range s.F.Chats {
		meta := s.F.Extra.ChatThreads[c.ID]
		session, _ := meta["session"].(string)
		var person, group, acc any
		jid := c.ID
		if p, ok := meta["person"].(string); ok {
			person = p
			if ph, ok := s.F.Extra.Phones[p]; ok {
				jid = domain.NormalizePhone(ph) + "@s.whatsapp.net"
			}
		}
		if g, ok := meta["group"].(string); ok {
			group = g
			for _, gg := range s.F.Extra.Groups {
				if gg.ID == g {
					jid = gg.JID
				}
			}
		}
		if n, ok := meta["internal"].(string); ok {
			if ph, ok := s.F.Extra.InternalFull[n]; ok {
				jid = domain.NormalizePhone(ph) + "@s.whatsapp.net"
			}
			person = s.personByName[n]
		}
		if c.Acc != "" {
			acc = c.Acc
		}
		unread := 0
		if u, ok := meta["unread"].(float64); ok {
			unread = int(u)
		}
		var last time.Time
		curDay := "Hari ini"
		for _, m := range c.Msgs {
			if m.D != "" {
				curDay = m.D
				continue
			}
			last = s.dayTime(curDay, m.Tm)
		}
		if last.IsZero() {
			last = s.dayTime("Hari ini", strings.ReplaceAll(c.Time, ".", ":"))
		}
		if err := s.exec(`INSERT INTO chat_threads(id,session_id,chat_jid,type,name,subtitle,account_id,person_id,group_id,unread,last_at,is_private)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (id) DO NOTHING`,
			c.ID, session, jid, c.Type, c.Name, c.Sub, acc, person, group, unread, last, c.Private); err != nil {
			return err
		}
		curDay = "Hari ini"
		var salesUser string
		for _, w := range s.F.Extra.WASessions {
			if w.ID == session {
				salesUser = w.User
			}
		}
		for i, m := range c.Msgs {
			if m.D != "" {
				curDay = m.D
				continue
			}
			at := s.dayTime(curDay, m.Tm)
			channel := "wa_message"
			if c.Type == "gext" || c.Type == "gint" {
				channel = "wa_group_message"
			}
			dir := m.F
			sender := m.Who
			if sender == "" {
				if dir == "out" {
					sender = c.Via
				} else {
					sender = c.Name
				}
			}
			var pids []string
			if pid, ok := s.personByName[sender]; ok {
				pids = []string{pid}
			} else if p, ok := person.(string); ok && dir == "in" {
				pids = []string{p}
			}
			wamid := fmt.Sprintf("fixture-%s-%d", c.ID, i)
			ref := "fixture:wa:" + wamid
			var id int64
			err := s.tx.QueryRow(s.ctx, `INSERT INTO interactions(channel,direction,occurred_at,person_ids,user_ids,thread_id,group_id,body_text,raw_ref,wamid,account_id,sender_name,sender_internal,transport,extracted,extraction_version,delivery_status)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$14,$10,$11,$12,'bridge',true,1,$13) ON CONFLICT (raw_ref) DO UPDATE SET updated_at=interactions.updated_at RETURNING id`,
				channel, dir, at, nz(pids), []string{salesUser}, c.ID, group, m.T, ref, acc, sender, m.Int || dir == "out", map[bool]string{true: "terkirim", false: ""}[dir == "out"], wamid).Scan(&id)
			if err != nil {
				return err
			}
			if m.Ann != nil {
				if err := s.exec(`INSERT INTO extractions(interaction_id,kind,tone,text,action_label,evidence,confidence,model,prompt_version)
					VALUES ($1,$2,$3,$4,$5,$6,0.88,'fixture','capture/v1') ON CONFLICT DO NOTHING`,
					id, annKind(m.Ann.T), m.Ann.K, m.Ann.T, m.Ann.Act, storage.JSON([]domain.Evidence{{InteractionID: id, Quote: trunc(m.T, 200), At: at.Format(time.RFC3339)}})); err != nil {
					return err
				}
			}
		}
		for i, t := range c.Tasks {
			srcTm := strings.TrimPrefix(t.Src, "pesan ")
			var src any
			if err := s.tx.QueryRow(s.ctx, `SELECT id FROM interactions WHERE thread_id=$1 AND to_char(occurred_at AT TIME ZONE 'Asia/Jakarta','HH24.MI')=$2 LIMIT 1`, c.ID, srcTm).Scan(&src); err != nil {
				src = nil
			}
			if err := s.exec(`INSERT INTO tasks(id,title,assignee,source_interaction_id,source_label,group_id,account_id,status,dedupe_hash)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'detected',$8) ON CONFLICT (id) DO NOTHING`,
				fmt.Sprintf("task-%s-%d", c.ID, i), t.T, t.Who, src, t.Src, group, acc, storage.Hash(c.ID, t.T)); err != nil {
				return err
			}
		}
	}
	return nil
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

var reLate = regexp.MustCompile(`Lewat (\d+)`)

func (s *Seeder) commitments() error {
	for _, d := range s.F.Deals {
		for _, side := range []struct {
			who  string
			rows []Commit
		}{{"kami", d.Commits.Kami}, {"mereka", d.Commits.Mereka}} {
			for i, c := range side.rows {
				key := fmt.Sprintf("%s:%s:%d", d.ID, side.who, i)
				status := map[string]string{"done": "done", "open": "open", "late": "late"}[c.St]
				var due *time.Time
				draft := false
				text, detail := c.T, c.S
				if o, ok := s.F.Extra.TodayCommitments[key]; ok {
					due = s.C.AtPtr(o.Due)
					draft = o.DraftReady
					text, detail = o.Text, o.Detail
					if o.Status != "" {
						status = o.Status
					}
				}
				if text == "" {
					text, detail = c.T, c.S
				} else if m := reLate.FindStringSubmatch(c.D); m != nil {
					n, _ := strconv.Atoi(m[1])
					if strings.Contains(c.D, "+") {
						n++
					}
					t := s.C.Anchor.AddDate(0, 0, -n)
					due = &t
				} else if strings.Contains(c.S, "Hari ini") {
					t := domain.StartOfDay(s.C.Anchor).Add(23 * time.Hour)
					due = &t
				} else if strings.Contains(c.S, "Besok") {
					t := domain.StartOfDay(s.C.Anchor).Add(24*time.Hour + 15*time.Hour)
					due = &t
				} else if t, ok := s.C.ShortDate(c.S, 17); ok {
					due = &t
				}
				owner := s.user(d.Owner)
				if err := s.exec(`INSERT INTO commitments(id,dedupe_hash,account_id,opportunity_id,who,text,detail,due_at,status,owner_user_id,evidence,confidence,model,draft_ready,created_at)
					VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,$9,$10,0.86,'fixture',$11,$12) ON CONFLICT (id) DO NOTHING`,
					"cm-"+strings.ReplaceAll(key, ":", "-"), storage.Hash(d.ID, side.who, strings.ToLower(c.T)), d.ID, side.who, text, detail, due, status, owner,
					storage.JSON([]domain.Evidence{{Source: "fixture:ledger", Quote: c.S}}), draft, s.C.Anchor.AddDate(0, 0, -3)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Seeder) signals() error {
	today := map[string]int{}
	for i, st := range s.F.Extra.SignalsToday {
		today[st.Key] = i
	}
	for _, d := range s.F.Deals {
		for i, f := range d.Flags {
			typ := ""
			for prefix, t := range s.F.Extra.FlagTypes {
				if strings.HasPrefix(f.T, prefix) {
					typ = t
				}
			}
			if typ == "" {
				typ = "note"
			}
			key := typ + ":" + d.ID
			detected := s.C.Anchor.Add(-time.Duration(48+i*6) * time.Hour)
			headline := ""
			if idx, ok := today[key]; ok {
				st := s.F.Extra.SignalsToday[idx]
				detected = s.C.At(st.DetectedAt)
				headline = st.TodayDetail
			}
			if err := s.exec(`INSERT INTO signals(type,severity,account_id,opportunity_id,title,detail,suggested_action,headline,evidence,confidence,dedupe_key,detected_at)
				VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,0.88,$9,$10) ON CONFLICT (dedupe_key) DO NOTHING`,
				typ, f.K, d.ID, f.T, f.S, f.Act, headline, storage.JSON([]domain.Evidence{{Source: "fixture:flag", Quote: f.S}}), key, detected); err != nil {
				return err
			}
		}
	}
	for _, st := range s.F.Extra.SignalsToday {
		if err := s.exec(`INSERT INTO signals(type,severity,account_id,opportunity_id,title,detail,headline,evidence,confidence,dedupe_key,detected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$6,$7,0.9,$8,$9) ON CONFLICT (dedupe_key) DO NOTHING`,
			st.Type, st.Severity, st.Account, st.Opportunity, st.Title, st.TodayDetail,
			storage.JSON([]domain.Evidence{{Source: "fixture:signal", Quote: st.TodayDetail}}), st.Key, s.C.At(st.DetectedAt)); err != nil {
			return err
		}
	}
	return nil
}

// fixtureConfidence reproduces the mockup's deterministic confidence per action id.
func fixtureConfidence(aid string) float64 {
	sum := 0
	for _, r := range aid {
		sum += int(r)
	}
	return math.Round((0.78+float64(sum%15)/100)*100) / 100
}

func (s *Seeder) agentFor(aid string, a FixtureAction) string {
	if strings.HasPrefix(aid, "l2c_") {
		return domain.AgentCollection
	}
	switch a.Icon {
	case "i-cal":
		return domain.AgentMeetingPrep
	case "i-doc", "i-edit":
		return domain.AgentResearch
	}
	return domain.AgentFollowUp
}

func (s *Seeder) actions() error {
	l2cByAid := map[string]string{}
	for aid, m := range s.F.Extra.L2CMeta {
		l2cByAid[aid] = m.ID
	}
	ids := make([]string, 0, len(s.F.Actions))
	for k := range s.F.Actions {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	for _, aid := range ids {
		a := s.F.Actions[aid]
		var acc, opp, cash any
		switch {
		case strings.HasPrefix(aid, "l2c_"):
			acc = s.F.Extra.L2CMeta[aid].Account
			cash = l2cByAid[aid]
		case strings.HasPrefix(aid, "won_"):
			acc = s.F.Extra.Won[aid].Account
			opp = aid
			if c, ok := l2cByAid[aid]; ok {
				cash = c
			}
		case aid == "kendal":
			acc, opp = "kendal", "kendal"
		default:
			acc, opp = aid, aid
		}
		kind := "send"
		switch a.Icon {
		case "i-cal", "i-doc", "i-edit", "i-phone":
			kind = "task"
		}
		title, button, summary, previewFrom, preview, contextNote, result := a.T, a.Btn, "", "", a.Preview, "", ""
		inQueue := false
		tags := []domainPill{}
		if v, ok := s.F.Extra.ActionSummary[aid]; ok {
			summary = v
		}
		if q, ok := s.F.Extra.Queue[aid]; ok {
			inQueue = true
			kind = q.Kind
			if q.ButtonLabel != "" {
				button = q.ButtonLabel
			}
			summary, previewFrom, contextNote, result = q.Summary, q.PreviewFrom, q.ContextNote, q.ResultText
			if q.Preview != "" {
				preview = q.Preview
			}
			tags = q.Tags
		}
		payload := map[string]any{}
		for k, v := range s.F.Extra.ActionPayload[aid] {
			payload[k] = v
		}
		if q, ok := s.F.Extra.Queue[aid]; ok && q.Toast != "" {
			payload["toast"] = q.Toast
		}
		if n, ok := queueOrder[aid]; ok {
			payload["queue_order"] = n
		}
		if previewFrom == "" && preview != "" {
			if p := s.F.Extra.ActionPayload[aid]; p != nil {
				if p["session"] != "" {
					previewFrom = "WhatsApp · dari nomor " + sessionOwner(p["session"])
				} else if p["to"] != "" {
					previewFrom = "Kepada: " + p["to"] + " · Dari: " + p["from"]
				}
			}
		}
		agent := s.agentFor(aid, a)
		created := s.C.Anchor.Add(-6 * time.Hour)
		if err := s.exec(`INSERT INTO actions(id,agent,type,kind,title,button_label,icon,account_id,opportunity_id,cash_item_id,due_label,summary,why,prep,preview,preview_from,context_note,steps,tags,payload,evidence,confidence,model,in_queue,result_text,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,'claude-sonnet-5',$23,$24,$25) ON CONFLICT (id) DO NOTHING`,
			aid, agent, s.F.Extra.ActionTypes[aid], kind, title, button, a.Icon, acc, opp, cash, a.Due, summary, a.Why, a.Prep, preview, previewFrom, contextNote,
			storage.JSON(a.Steps), storage.JSON(tags), storage.JSONObj(payload),
			storage.JSON([]domain.Evidence{{Source: "fixture:action", Quote: trunc(a.Why, 200)}}), fixtureConfidence(aid), inQueue, result, created); err != nil {
			return err
		}
		if opp != nil {
			if err := s.exec(`UPDATE opportunities SET next_action_id=$1 WHERE id=$2 AND next_action_id IS NULL`, aid, opp); err != nil {
				return err
			}
		}
	}
	for _, x := range s.F.Extra.ExtraActions {
		if x.Payload == nil {
			x.Payload = kv{}
		}
		if n, ok := queueOrder[x.ID]; ok {
			x.Payload["queue_order"] = n
		}
		var opp any
		if x.Opportunity != nil {
			opp = *x.Opportunity
		}
		if err := s.exec(`INSERT INTO actions(id,agent,type,kind,title,button_label,icon,account_id,opportunity_id,due_label,summary,why,prep,context_note,impact,steps,options,tags,payload,evidence,confidence,model,in_queue,proposed_by,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,'claude-sonnet-5',true,$22,$23) ON CONFLICT (id) DO NOTHING`,
			x.ID, x.Agent, x.Type, x.Kind, x.Title, x.ButtonLabel, x.Icon, x.Account, opp, x.Due, x.Summary, x.Why, x.Prep, x.ContextNote,
			storage.JSON(x.Impact), storage.JSON(x.Steps), storage.JSON(x.Options), storage.JSON(x.Tags), storage.JSONObj(x.Payload),
			storage.JSON([]domain.Evidence{{Source: "fixture:request", Quote: trunc(x.Summary, 200)}}), x.Confidence, x.ProposedBy, s.C.At(x.CreatedAt)); err != nil {
			return err
		}
	}
	// Historical decisions feed the calibration panel (acceptance rate per agent, 30 days).
	for _, cal := range s.F.Extra.Calibration {
		for i := 0; i < cal.Total; i++ {
			id := fmt.Sprintf("hist-%s-%02d", strings.ToLower(strings.Fields(cal.Agent)[0]), i)
			approved := i < cal.Approved
			status, decision, reason := "executed", "approve", ""
			if !approved {
				status, decision, reason = "rejected", "reject", domain.RejectReasons[i%len(domain.RejectReasons)]
			}
			at := s.C.Anchor.AddDate(0, 0, -1-(i%27))
			if err := s.exec(`INSERT INTO actions(id,agent,type,kind,title,status,model,created_at,executed_at,decision) VALUES ($1,$2,'historical','internal',$3,$4,'claude-sonnet-5',$5,$5,$6) ON CONFLICT (id) DO NOTHING`,
				id, cal.Agent, "Saran historis "+cal.Agent, status, at, storage.JSONObj(map[string]any{"user": "sam", "decision": decision, "reason": reason})); err != nil {
				return err
			}
			if err := s.exec(`INSERT INTO action_decisions(action_id,agent,type,user_id,decision,reason,decided_at)
				SELECT $1,$2,'historical','sam',$3,$4,$5 WHERE NOT EXISTS (SELECT 1 FROM action_decisions WHERE action_id=$1)`,
				id, cal.Agent, decision, reason, at); err != nil {
				return err
			}
		}
	}
	for _, r := range s.F.Extra.LearnedRules {
		if err := s.exec(`INSERT INTO learned_rules(agent,pattern,text,notified,created_at) VALUES ($1,$2,$3,true,$4) ON CONFLICT (pattern) DO NOTHING`,
			r.Agent, r.Pattern, r.Text, s.C.Anchor.AddDate(0, 0, -12)); err != nil {
			return err
		}
	}
	return nil
}

var queueOrder = map[string]int{"semarang": 1, "pricing_rsud_7": 2, "unmer": 3, "credit_graha_24cam": 4}

func sessionOwner(session string) string {
	return strings.Title(strings.TrimPrefix(session, "s-")) //nolint:staticcheck // ASCII names only
}

func (s *Seeder) health() error {
	rows, err := s.tx.Query(s.ctx, `SELECT opportunity_id, component, value FROM health_components`)
	if err != nil {
		return err
	}
	comps := map[string]*health.Components{}
	for rows.Next() {
		var opp, key string
		var v int
		if err := rows.Scan(&opp, &key, &v); err != nil {
			rows.Close()
			return err
		}
		if comps[opp] == nil {
			comps[opp] = &health.Components{}
		}
		comps[opp].Set(key, v)
	}
	rows.Close()
	trend := map[string]int{}
	for _, d := range s.F.Deals {
		trend[d.ID] = d.Trend
	}
	for opp, c := range comps {
		r := health.Compute(*c)
		bd := storage.JSONObj(r.Components)
		if err := s.exec(`UPDATE opportunities SET health=$1, health_breakdown=$2, health_trend_30d=$3 WHERE id=$4`, r.Health, bd, trend[opp], opp); err != nil {
			return err
		}
		if err := s.exec(`UPDATE accounts SET health=$1, health_trend_30d=$2 WHERE id=(SELECT account_id FROM opportunities WHERE id=$3)`, r.Health, trend[opp], opp); err != nil {
			return err
		}
		for _, snap := range []struct {
			day time.Time
			h   int
		}{{s.C.Anchor.AddDate(0, 0, -30), r.Health - trend[opp]}, {s.C.Anchor, r.Health}} {
			if err := s.exec(`INSERT INTO health_snapshots(opportunity_id,health,breakdown,taken_on) SELECT $1,$2,$3,$4
				WHERE (SELECT count(*) FROM health_snapshots WHERE opportunity_id=$1) < 2 ON CONFLICT DO NOTHING`,
				opp, snap.h, bd, snap.day.Format("2006-01-02")); err != nil {
				return err
			}
		}
	}
	return nil
}
