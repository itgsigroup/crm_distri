package seed

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// chatMeta are the presentation hints of the mockup chat threads (db/seed/chats.json).
type chatMeta struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // cust | gint | new
	Name        string            `json:"name"`
	Sub         string            `json:"sub"`
	Via         string            `json:"via"`
	Dealer      string            `json:"dealer"`
	Unread      int32             `json:"unread"`
	Tag         map[string]string `json:"tag"`
	Suggestions []string          `json:"suggestions"`
}

type chatInput struct {
	sales     []salesUser
	salesID   map[string]uuid.UUID
	dealers   []dealer
	dealerID  map[string]uuid.UUID
	contactID map[string]uuid.UUID
	signals   []signal
	signalID  map[string]uuid.UUID
	chats     []chatMeta
}

const (
	gudangJID  = "120363041100000001@g.us"
	projectJID = "120363041100000002@g.us"
	newNumber  = "6282212343310"
	joko, sari = "6281100001001", "6281100001002"
)

// seedChat loads internal numbers, groups, paired sales numbers and the chat threads/messages of the mockup
// (the WhatsApp signals of signals.json with a "thread" key), as the ingest pipeline would have stored them.
func seedChat(ctx context.Context, q *gen.Queries, in chatInput) error {
	salesNo := map[string]string{}
	for _, s := range in.sales {
		salesNo[s.Key] = s.WANumber
		label, dept := s.Name, map[string]string{"sales": "Sales", "ceo": "Direksi", "admin": "Admin", "finance": "Keuangan"}[s.Role]
		if err := q.UpsertInternalNumber(ctx, gen.UpsertInternalNumberParams{WaNumber: s.WANumber, Label: &label, Department: &dept, IsSales: s.Role == "sales"}); err != nil {
			return err
		}
		if s.Role == "sales" {
			sid := in.salesID[s.Key]
			lbl := "Nomor " + s.Name
			if err := q.UpsertWANumber(ctx, gen.UpsertWANumberParams{WaNumber: s.WANumber, SalesID: &sid, Label: &lbl, Transport: "fake", State: "connected"}); err != nil {
				return err
			}
		}
	}
	for _, n := range []struct{ no, label, dept string }{{joko, "Pak Joko", "Gudang Semarang"}, {sari, "Sari", "Gudang Semarang"}} {
		l, d := n.label, n.dept
		if err := q.UpsertInternalNumber(ctx, gen.UpsertInternalNumberParams{WaNumber: n.no, Label: &l, Department: &d}); err != nil {
			return err
		}
	}
	type grp struct {
		jid, name, kind, branch string
		members                 int32
	}
	groupID := map[string]uuid.UUID{}
	for _, g := range []grp{{gudangJID, "Gudang Semarang", "internal", "Semarang", 6}, {projectJID, "Proyek RS Muntilan · Bina Teknik", "external", "Yogyakarta", 5}} {
		jid, name, kind, branch, members := g.jid, g.name, g.kind, g.branch, g.members
		row, err := q.UpsertWAGroup(ctx, gen.UpsertWAGroupParams{Jid: &jid, Name: &name, Kind: &kind, Branch: &branch, Members: &members, ReadEnabled: true})
		if err != nil {
			return err
		}
		groupID[jid] = row.ID
	}

	contactNo := map[string]string{}
	contactName := map[string]string{}
	contactRole := map[string]string{}
	dealerName := map[string]string{}
	for _, d := range in.dealers {
		dealerName[d.Slug] = d.Name
		for _, c := range d.Contacts {
			contactNo[c.Key], contactName[c.Key], contactRole[c.Key] = c.WANumber, c.Name, c.Role
		}
	}
	meta := map[string]chatMeta{}
	for _, c := range in.chats {
		meta[c.ID] = c
	}

	// group the WA signals per thread
	type msg struct {
		sig signal
		p   map[string]any
	}
	threads := map[string][]msg{}
	var order []string
	for _, sg := range in.signals {
		if sg.Kind != "wa" && sg.Kind != "wa_group" {
			continue
		}
		var p map[string]any
		_ = json.Unmarshal(sg.Payload, &p)
		key, _ := p["thread"].(string)
		if hist, _ := p["history"].(bool); key == "" || hist {
			continue // older synthetic history is kept as signals only (it predates chat capture)
		}
		if _, ok := threads[key]; !ok {
			order = append(order, key)
		}
		threads[key] = append(threads[key], msg{sg, p})
	}

	for _, key := range order {
		msgs := threads[key]
		first := msgs[0].sig
		m, hasMeta := meta[key]
		salesKey := first.Sales
		if hasMeta {
			salesKey = m.Via
		}
		sid := in.salesID[salesKey]
		var kind, jid, title, subtitle string
		var dealerID, contactID, gid *uuid.UUID
		var ident json.RawMessage
		contactKey := ""
		for _, x := range msgs {
			if x.sig.Contact != "" {
				contactKey = x.sig.Contact
				break
			}
		}
		switch {
		case hasMeta && m.Type == "gint":
			kind, jid, title, subtitle = "group", gudangJID, m.Name, m.Sub
			g := groupID[gudangJID]
			gid = &g
		case hasMeta && m.Type == "new":
			kind, jid, title, subtitle = "new", wa.UserJID(newNumber), wa.MaskNumber(newNumber), m.Sub
			ident, _ = json.Marshal(map[string]any{
				"best_name": "Toko Mandiri Elektronik", "best_org": "Pati · toko CCTV & sound", "score": 74,
				"sources": []map[string]string{
					{"source": "getcontact", "ok": "true", "value": "Getcontact: “Mandiri Elektronik Pati” (2 tag)"},
					{"source": "wa_business", "ok": "true", "value": "Profil WA Business: “Toko Mandiri – CCTV & Sound”"},
					{"source": "odoo", "ok": "false", "value": "Belum ada di Odoo"},
				},
				"potential": "Toko sejenis di Pati rata-rata order Rp 6–9 jt/bulan (kamera analog & 4MP, DVR). Tawarkan jadi dealer resmi dengan harga tier C dan ongkir subsidi untuk order ≥ Rp 5 jt.",
			})
		default:
			if contactKey == "" {
				continue
			}
			kind, jid = "dealer", wa.UserJID(contactNo[contactKey])
			d := in.dealerID[first.Dealer]
			c := in.contactID[contactKey]
			dealerID, contactID = &d, &c
			title, subtitle = contactName[contactKey]+" · "+shortDealer(dealerName[first.Dealer]), dealerName[first.Dealer]+" · "+contactRole[contactKey]
			if hasMeta {
				title, subtitle = m.Name, m.Sub
			}
		}
		var tag, sugg json.RawMessage
		unread := int32(0)
		if hasMeta {
			tag, _ = json.Marshal(m.Tag)
			sugg, _ = json.Marshal(m.Suggestions)
			unread = m.Unread
		}
		last := msgs[len(msgs)-1].sig.OccurredAt
		seedKey := "seed:thread:" + key
		th, err := q.InsertThread(ctx, gen.InsertThreadParams{Kind: &kind, DealerID: dealerID, GroupID: gid, ContactID: contactID, WaJid: &jid, Title: &title, Subtitle: &subtitle, SalesID: &sid, LastMessageAt: &last, Unread: unread, Identification: ident, Tag: tag, Suggestions: sugg, SeedKey: &seedKey})
		if err != nil {
			return err
		}
		if kind == "new" {
			if err := q.UpsertIdentification(ctx, gen.UpsertIdentificationParams{WaNumber: newNumber, Sources: ident, BestName: ptr("Toko Mandiri Elektronik"), BestOrg: ptr("Pati"), Score: ptr(int16(74))}); err != nil {
				return err
			}
		}
		for _, x := range msgs {
			dir, _ := x.p["direction"].(string)
			from := contactNo[x.sig.Contact]
			name, _ := x.p["from_name"].(string)
			internal, _ := x.p["internal"].(bool)
			switch {
			case dir == "out":
				from, name = salesNo[salesKey], strings.ToUpper(salesKey[:1])+salesKey[1:]
			case kind == "new":
				from = newNumber
			case kind == "group" && name == "Pak Joko":
				from = joko
			case kind == "group":
				from = sari
			}
			if name == "" {
				name = contactName[x.sig.Contact]
			}
			var ann json.RawMessage
			if a, ok := x.p["annotation"]; ok {
				ann, _ = json.Marshal(a)
			}
			id := x.sig.DedupeKey
			body := x.sig.Summary
			sig := in.signalID[x.sig.DedupeKey]
			status := "received"
			if dir == "out" {
				status = "sent"
			}
			if _, err := q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &th.ID, WaMsgID: &id, Direction: &dir, FromNumber: &from, FromName: &name, Body: &body, SentAt: x.sig.OccurredAt, Annotation: ann, SignalID: &sig, Status: status, Internal: internal}); err != nil && !isNoRows(err) {
				return err
			}
		}
	}
	return nil
}

func shortDealer(name string) string {
	for _, p := range []string{"PT ", "CV ", "UD ", "Toko "} {
		name = strings.TrimPrefix(name, p)
	}
	f := strings.Fields(name)
	if len(f) > 2 {
		f = f[:2]
	}
	return strings.Join(f, " ")
}

func ptr[T any](v T) *T { return &v }

func isNoRows(err error) bool { return err != nil && strings.Contains(err.Error(), "no rows") }
