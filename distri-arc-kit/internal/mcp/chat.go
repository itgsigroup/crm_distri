package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
)

// teamLabel names the team of the GSI number holding a conversation (ADR 0033).
var teamLabel = map[string]string{"sales": "Sales", "admin": "Admin", "billing": "Penagihan"}

// chatConversation is chat.thread for one conversation by id: a dealer, a new number or a customer group. A group
// reads the messages every GSI number in it received; a GSI employee's message there is our side ("out"). Internal
// groups are never read (09-policies).
func (s *Server) chatConversation(ctx context.Context, in threadIn) (any, result, error) {
	id, err := uuid.Parse(strings.TrimSpace(in.ThreadID))
	if err != nil {
		return nil, result{}, errors.New("thread_id bukan id percakapan")
	}
	t, err := s.St.Q.GetThread(ctx, id)
	if err != nil {
		return nil, result{}, errors.New("percakapan tidak ditemukan")
	}
	group := deref(t.Kind) == "group"
	if group && deref(t.GroupKind) != "external" {
		return nil, result{}, errors.New("grup internal tidak dibaca — hanya grup pelanggan")
	}
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.St.Q.ListThreadMessages(ctx, gen.ListThreadMessagesParams{ThreadID: &id, SentAt: s.Clock.Now().Add(24 * time.Hour), Lim: int32(limit)})
	if err != nil {
		return nil, result{}, err
	}
	_, _, mask, _ := s.permissions(ctx)
	names := []string{}
	if p, _, ok := strings.Cut(deref(t.Title), " · "); ok {
		names = append(names, strings.TrimSpace(p))
	}
	for _, r := range rows {
		if n := strings.TrimSpace(deref(r.FromName)); n != "" && deref(r.Direction) == "in" && !r.Internal {
			names = append(names, n)
		}
	}
	m := llm.NewMasker()
	msgs := []map[string]any{}
	for _, r := range rows {
		if r.Status == "failed" || r.DeletedAt != nil {
			continue
		}
		dir := deref(r.Direction)
		if r.Internal {
			dir = "out"
		}
		body, from, who := deref(r.Body), deref(r.FromNumber), deref(r.FromName)
		if mask {
			body, from, who = m.Mask(body, names), m.Mask(from, nil), m.Mask(who, names)
		}
		mm := map[string]any{"at": r.SentAt, "direction": dir, "from": who, "from_number": from, "text": body}
		if dir == "out" && deref(r.Holding) {
			mm["jawaban_sementara"] = true // "tunggu sebentar": the customer is not answered yet (ADR 0034)
		}
		msgs = append(msgs, mm)
	}
	title := deref(t.Title)
	if mask && !group {
		title = m.Mask(title, names)
	}
	jenis := map[string]string{"dealer": "dealer", "new": "nomor baru", "group": "grup pelanggan"}[deref(t.Kind)]
	return map[string]any{"thread_id": id, "judul": title, "jenis": jenis, "dealer": deref(t.DealerName), "sales": deref(t.SalesName),
			"tim": teamLabel[t.AccountTeam], "masked": mask, "messages": msgs},
		result{Summary: fmt.Sprintf("%d pesan · %s", len(msgs), jenis)}, nil
}
