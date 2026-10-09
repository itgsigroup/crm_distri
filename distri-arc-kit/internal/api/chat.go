package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	qrcode "github.com/skip2/go-qrcode"

	"distri-arc/internal/httpx"
	"distri-arc/internal/jobs"
	"distri-arc/internal/outbox"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
)

func (s *Server) chatRoutes(r chi.Router) {
	r.Get("/chat/threads", s.chatThreads)
	r.Get("/chat/threads/{id}", s.chatThread)
	r.Post("/chat/threads/{id}/read", s.chatRead)
	r.Get("/chat/threads/{id}/context", s.chatContext)
	r.Post("/chat/threads/{id}/messages", s.chatSend)
	r.Get("/internal-numbers", s.internalNumbers)
	r.Post("/internal-numbers", s.addInternalNumber)
	r.Delete("/internal-numbers/{wa}", s.deleteInternalNumber)
	r.Get("/wa/groups", s.waGroups)
	r.Patch("/wa/groups/{id}", s.patchWAGroup)
	r.Get("/wa/status", s.waStatus)
	r.Post("/wa/pair", s.waPair)
	r.Post("/wa/links", s.createWALink)
	r.Get("/wa/links/{id}", s.getWALink)
	r.Put("/wa/numbers/{wa}/user", s.assignWANumber)
	r.Delete("/wa/numbers/{wa}", s.deleteWANumber)
}

// ThreadView is one row of the chat list.
type ThreadView struct {
	ID            uuid.UUID       `json:"id"`
	Kind          string          `json:"kind"`
	Title         string          `json:"title"`
	Subtitle      string          `json:"subtitle"`
	DealerID      string          `json:"dealer_id,omitempty"`
	DealerName    string          `json:"dealer_name,omitempty"`
	Sales         string          `json:"sales"`
	LastMessageAt *time.Time      `json:"last_message_at"`
	Unread        int32           `json:"unread"`
	LastBody      string          `json:"last_body"`
	LastFrom      string          `json:"last_from"`
	Tag           json.RawMessage `json:"tag"`
	GroupKind     string          `json:"group_kind,omitempty"`
	Account       string          `json:"account"`       // the WhatsApp number holding the conversation
	AccountLabel  string          `json:"account_label"` // its label (CS Kantor, Nomor Andi …)
}

func (s *Server) chatThreads(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListThreads(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	tab := r.URL.Query().Get("tab")
	account := wa.Digits(r.URL.Query().Get("account"))
	term := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	u, _ := CurrentUser(r.Context())
	salesOnly := deref(u.Role) == "sales"
	out := []ThreadView{}
	for _, t := range rows {
		kind := deref(t.Kind)
		if (tab == "dealer" && kind != "dealer") || (tab == "group_internal" && kind != "group") || (tab == "new" && kind != "new") {
			continue
		}
		if account != "" && deref(t.Account) != account {
			continue
		}
		// a sales user reads only the conversations of their own numbers
		if salesOnly && (u.SalesUserID == nil || t.SalesID == nil || *t.SalesID != *u.SalesUserID) {
			continue
		}
		v := ThreadView{ID: t.ID, Kind: kind, Title: deref(t.Title), Subtitle: deref(t.Subtitle), DealerID: deref(t.DealerSlug), DealerName: deref(t.DealerName),
			Sales: deref(t.SalesName), LastMessageAt: t.LastMessageAt, Unread: t.Unread, LastBody: deref(t.LastBody), Tag: t.Tag, GroupKind: deref(t.GroupKind),
			Account: deref(t.Account), AccountLabel: t.AccountLabel}
		if t.LastFrom != nil && kind == "group" {
			v.LastFrom = *t.LastFrom
		}
		if term != "" && !strings.Contains(strings.ToLower(v.Title+" "+v.Subtitle+" "+v.LastBody), term) {
			continue
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) threadParam(w http.ResponseWriter, r *http.Request) (gen.GetThreadRow, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Percakapan tidak ditemukan")
		return gen.GetThreadRow{}, false
	}
	t, err := s.st.Q.GetThread(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Percakapan tidak ditemukan")
		return gen.GetThreadRow{}, false
	}
	if u, _ := CurrentUser(r.Context()); deref(u.Role) == "sales" && (u.SalesUserID == nil || t.SalesID == nil || *t.SalesID != *u.SalesUserID) {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Percakapan tidak ditemukan") // another number's conversation
		return gen.GetThreadRow{}, false
	}
	return t, true
}

// MessageView is one chat bubble.
type MessageView struct {
	ID         uuid.UUID       `json:"id"`
	Direction  string          `json:"direction"`
	FromName   string          `json:"from_name"`
	Body       string          `json:"body"`
	SentAt     time.Time       `json:"sent_at"`
	Status     string          `json:"status"`
	Internal   bool            `json:"internal"`
	Annotation json.RawMessage `json:"annotation"`
	SignalID   *uuid.UUID      `json:"signal_id"`
	Proposal   *MsgProposal    `json:"proposal,omitempty"`
}

// MsgProposal is the agent proposal triggered by a message (the button on its annotation).
type MsgProposal struct {
	ID         uuid.UUID  `json:"id"`
	Button     string     `json:"button"`
	Status     string     `json:"status"`
	ExecutedAt *time.Time `json:"executed_at"`
	DecidedAt  *time.Time `json:"decided_at"`
}

func (s *Server) chatThread(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	before := s.clock.Now().Add(24 * 365 * time.Hour)
	if b := r.URL.Query().Get("before"); b != "" {
		if x, err := time.Parse(time.RFC3339, b); err == nil {
			before = x
		}
	}
	msgs, err := s.st.Q.ListThreadMessages(r.Context(), gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: before, Limit: 30})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var sigIDs []uuid.UUID
	for _, m := range msgs {
		if m.SignalID != nil {
			sigIDs = append(sigIDs, *m.SignalID)
		}
	}
	bySig := map[uuid.UUID]*MsgProposal{}
	if len(sigIDs) > 0 {
		ps, err := s.st.Q.ProposalsForSignals(r.Context(), sigIDs)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		for _, p := range ps {
			for _, sid := range p.SignalIds {
				if _, ok := bySig[sid]; !ok {
					bySig[sid] = &MsgProposal{ID: p.ID, Button: deref(p.Button), Status: p.Status, ExecutedAt: p.ExecutedAt, DecidedAt: p.DecidedAt}
				}
			}
		}
	}
	out := make([]MessageView, 0, len(msgs))
	for _, m := range msgs {
		v := MessageView{ID: m.ID, Direction: deref(m.Direction), FromName: deref(m.FromName), Body: deref(m.Body), SentAt: m.SentAt, Status: m.Status, Internal: m.Internal, Annotation: m.Annotation, SignalID: m.SignalID}
		if m.SignalID != nil {
			v.Proposal = bySig[*m.SignalID]
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"thread": map[string]any{
		"id": t.ID, "kind": deref(t.Kind), "title": deref(t.Title), "subtitle": deref(t.Subtitle), "dealer_id": deref(t.DealerSlug),
		"sales": deref(t.SalesName), "sales_wa": deref(t.SalesWa), "account_label": t.AccountLabel, "account_masked": wa.MaskNumber(deref(t.SalesWa)), "suggestions": t.Suggestions, "tag": t.Tag, "unread": t.Unread,
	}, "messages": out})
}

func (s *Server) chatRead(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	if err := s.st.Q.MarkThreadRead(r.Context(), t.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// chatContext is the right pane: the dealer, what was extracted from this chat, favourite products.
func (s *Server) chatContext(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	msgs, err := s.st.Q.ListThreadMessages(r.Context(), gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: s.clock.Now().Add(24 * 365 * time.Hour), Limit: 100})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	type ext struct {
		Annotation json.RawMessage `json:"annotation"`
		SentAt     time.Time       `json:"sent_at"`
		FromName   string          `json:"from_name"`
	}
	extracted := []ext{}
	for _, m := range msgs {
		if len(m.Annotation) > 0 && string(m.Annotation) != "null" {
			extracted = append(extracted, ext{Annotation: m.Annotation, SentAt: m.SentAt, FromName: deref(m.FromName)})
		}
	}
	res := map[string]any{"kind": deref(t.Kind), "extracted": extracted, "identification": t.Identification}
	if deref(t.Kind) == "new" {
		rows, err := s.st.Q.ThreadProposals(r.Context(), t.ID.String())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		next := map[string]views.NextAction{}
		for _, p := range rows {
			if _, ok := next[p.Kind]; !ok {
				next[p.Kind] = views.NextAction{ID: p.ID, Kind: p.Kind, Title: p.Title, Button: deref(p.Button), Icon: deref(p.Icon), Agent: p.Agent,
					DueLabel: deref(p.DueLabel), Status: p.Status, Why: p.Why, DecidedAt: p.DecidedAt, ExecutedAt: p.ExecutedAt, Autonomy: p.Autonomy}
			}
		}
		res["proposals"] = next
	}
	if t.DealerSlug != nil {
		b, ok := s.fullBoard(w, r)
		if !ok {
			return
		}
		if it, ok := b.Get(*t.DealerSlug); ok {
			res["dealer"] = it
		}
	}
	httpx.JSON(w, http.StatusOK, res)
}

// chatSend: a human typing a reply is itself the decision. It is recorded as a proposal kind=reply approved
// by the sender (decided_by), written to the outbox and delivered by the worker (07-api › Chat).
func (s *Server) chatSend(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Body) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pesan kosong")
		return
	}
	u, _ := CurrentUser(r.Context())
	if u.SalesUserID == nil {
		httpx.Fail(w, http.StatusForbidden, "no_sales_user", "Pengguna tidak terhubung ke data sales")
		return
	}
	if t.SalesWa == nil || t.WaJid == nil {
		httpx.Fail(w, http.StatusConflict, "no_number", "Percakapan ini belum terhubung ke nomor sales")
		return
	}
	if pol, err := policy.Load(r.Context(), s.st.Q); err == nil && pol.Pilot.Shadow() {
		httpx.Fail(w, http.StatusConflict, "pilot_shadow", "Mode bayangan pilot: balas dari WhatsApp di ponsel — GSI Orbit belum mengirim")
		return
	}
	now := s.clock.Now()
	body := strings.TrimSpace(in.Body)
	var res map[string]any
	err := s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
		sig, err := q.LatestThreadSignal(r.Context(), &t.ID)
		if err != nil || sig == nil {
			payload, _ := json.Marshal(map[string]any{"thread": t.ID, "reason": "balasan manusia tanpa pesan masuk"})
			sm := "Balasan dari aplikasi"
			id, err := q.InsertManualSignal(r.Context(), gen.InsertManualSignalParams{DealerID: t.DealerID, SalesID: u.SalesUserID, OccurredAt: now, DedupeKey: "reply:" + uuid.NewString(), Summary: &sm, Payload: payload})
			if err != nil {
				return err
			}
			sig = &id
		}
		reason := "Dikirim langsung dari Chat oleh " + deref(u.Name)
		p, err := q.InsertProposal(r.Context(), gen.InsertProposalParams{Agent: "Manusia", DealerID: t.DealerID, Kind: "reply", Title: "Balasan ke " + deref(t.Title), Why: reason,
			Preview: &body, Confidence: 1, SignalIds: []uuid.UUID{*sig}, Autonomy: "approve", Status: "approved", DecidedBy: u.SalesUserID, DecidedAt: &now, DecisionReason: &reason})
		if err != nil {
			return err
		}
		dir, from := "out", *t.SalesWa
		name := deref(t.SalesName)
		pendingID := "outbox:" + p.ID.String()
		mid, err := q.InsertChatMessage(r.Context(), gen.InsertChatMessageParams{ThreadID: &t.ID, WaMsgID: &pendingID, Direction: &dir, FromNumber: &from, FromName: &name, Body: &body, SentAt: now, Status: "pending", ProposalID: &p.ID})
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(outbox.WAPayload{From: from, To: *t.WaJid, Text: body, Kind: "reply", MessageID: mid.String(), ThreadID: t.ID.String()})
		to := *t.WaJid
		ob, err := q.InsertOutbox(r.Context(), gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: &to, Payload: payload})
		if err != nil {
			return err
		}
		if err := q.TouchThread(r.Context(), gen.TouchThreadParams{ID: t.ID, LastMessageAt: &now}); err != nil {
			return err
		}
		actor, kind, action, entity := deref(u.Email), "user", "chat.reply", "proposal"
		after, _ := json.Marshal(map[string]any{"thread_id": t.ID, "outbox_id": ob.ID, "body": body})
		if err := q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &p.ID, After: after}); err != nil {
			return err
		}
		if s.jobs != nil {
			if _, err := s.jobs.InsertTx(r.Context(), tx, jobs.OutboxSendArgs{OutboxID: ob.ID.String()}, nil); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(r.Context(), "select pg_notify('chat_message', $1)", `{"thread_id":"`+t.ID.String()+`"}`); err != nil {
			return err
		}
		res = map[string]any{"proposal_id": p.ID, "outbox_id": ob.ID, "message_id": mid, "status": "pending"}
		return nil
	})
	if err != nil {
		s.log.Error("chat send", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Gagal mencatat balasan")
		return
	}
	httpx.JSON(w, http.StatusAccepted, res)
}

func (s *Server) internalNumbers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListInternalNumbers(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) addInternalNumber(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WANumber, Label, Department string
		IsSales                     bool `json:"is_sales"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || wa.Digits(in.WANumber) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nomor tidak valid")
		return
	}
	u, _ := CurrentUser(r.Context())
	n := wa.Digits(in.WANumber)
	if err := s.st.Q.UpsertInternalNumber(r.Context(), gen.UpsertInternalNumberParams{WaNumber: n, Label: &in.Label, Department: &in.Department, IsSales: in.IsSales, AddedBy: &u.ID}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "internal_number.add", "internal_number", nil, map[string]any{"wa_number": n, "label": in.Label})
	httpx.JSON(w, http.StatusCreated, map[string]any{"wa_number": n})
}

func (s *Server) deleteInternalNumber(w http.ResponseWriter, r *http.Request) {
	n := wa.Digits(chi.URLParam(r, "wa"))
	if err := s.st.Q.DeleteInternalNumber(r.Context(), n); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "internal_number.delete", "internal_number", nil, map[string]any{"wa_number": n})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) waGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListWAGroups(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) patchWAGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Grup tidak ditemukan")
		return
	}
	var in struct {
		Kind        string `json:"kind"`
		ReadEnabled *bool  `json:"read_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || (in.Kind != "internal" && in.Kind != "external") || in.ReadEnabled == nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "kind (internal|external) dan read_enabled wajib")
		return
	}
	g, err := s.st.Q.UpdateWAGroup(r.Context(), gen.UpdateWAGroupParams{ID: id, Kind: &in.Kind, ReadEnabled: *in.ReadEnabled})
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Grup tidak ditemukan")
		return
	}
	s.audit(r, "wa_group.update", "wa_group", &id, map[string]any{"kind": in.Kind, "read_enabled": *in.ReadEnabled})
	httpx.JSON(w, http.StatusOK, g)
}

func (s *Server) waStatus(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListWANumbers(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	// anti-ban counters per number from the Baileys bridge (today / per day, warm-up of a new device)
	limits := map[string]json.RawMessage{}
	if s.cfg.WATransport == "baileys" {
		b := &wa.Baileys{BridgeURL: s.cfg.BridgeURL, Secret: s.cfg.BridgeSecret}
		if raw, err := b.Health(r.Context()); err == nil {
			var h struct {
				Limits map[string]json.RawMessage `json:"limits"`
			}
			if json.Unmarshal(raw, &h) == nil {
				limits = h.Limits
			}
		}
	}
	u, _ := CurrentUser(r.Context())
	out := []map[string]any{}
	for _, n := range rows {
		mine := (n.UserID != nil && *n.UserID == u.ID) || (u.SalesUserID != nil && n.SalesID != nil && *n.SalesID == *u.SalesUserID)
		if deref(u.Role) == "sales" && !mine {
			continue // a sales user sees and pairs only their own number
		}
		v := map[string]any{"wa_number": n.WaNumber, "masked": wa.MaskNumber(n.WaNumber), "sales": deref(n.SalesName), "branch": deref(n.SalesBranch), "transport": n.Transport,
			"state": n.State, "last_seen_at": n.LastSeenAt, "paired_at": n.PairedAt, "backfill_days": n.BackfillDays, "label": deref(n.Label), "sales_id": n.SalesID,
			"user_id": n.UserID, "user_name": deref(n.UserName), "user_email": deref(n.UserEmail), "mine": mine}
		if l, ok := limits[n.WaNumber]; ok {
			v["limits"] = l
		}
		if n.Qr != nil && n.State == "pairing" && strings.HasPrefix(*n.Qr, wa.LinkingPrefix) {
			v["linking"] = true // scanned: logging in, the first sync starts
		} else if n.Qr != nil && n.State == "pairing" && strings.HasPrefix(*n.Qr, wa.PairCodePrefix) {
			v["pair_code"] = strings.TrimPrefix(*n.Qr, wa.PairCodePrefix)
			v["qr_expires_at"] = n.QrExpiresAt
		} else if n.Qr != nil && n.State == "pairing" {
			if png, err := qrcode.Encode(*n.Qr, qrcode.Medium, 256); err == nil {
				v["qr_png"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
				v["qr_expires_at"] = n.QrExpiresAt
			}
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "transport": s.cfg.WATransport})
}

func (s *Server) waPair(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WANumber string `json:"wa_number"`
		Method   string `json:"method"` // "" (QR) | "code"
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || wa.Digits(in.WANumber) == "" || (in.Method != "" && in.Method != "code") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nomor tidak valid")
		return
	}
	n := wa.Digits(in.WANumber)
	row, err := s.st.Q.GetWANumber(r.Context(), n)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "unknown_number", "Hanya nomor terdaftar yang bisa dipasangkan — tambah nomor dulu")
		return
	}
	if u, _ := CurrentUser(r.Context()); !canManageNumber(u, row) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO, admin, atau pemilik nomor yang bisa memasangkan")
		return
	}
	if s.jobs == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "no_queue", "Antrean tidak tersedia")
		return
	}
	if in.Method == "code" && s.cfg.WATransport != "baileys" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tautan dengan kode hanya untuk transport Baileys — pakai QR")
		return
	}
	if _, err := s.jobs.Insert(r.Context(), jobs.WAPairArgs{WANumber: n, Method: in.Method}, nil); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "wa.pair", "wa_number", nil, map[string]any{"wa_number": n, "method": in.Method})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"wa_number": n, "state": "pairing"})
}

func (s *Server) audit(r *http.Request, action, entity string, id *uuid.UUID, after map[string]any) {
	u, _ := CurrentUser(r.Context())
	actor, kind := deref(u.Email), "user"
	b, _ := json.Marshal(after)
	_ = s.st.Q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: id, After: b})
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
