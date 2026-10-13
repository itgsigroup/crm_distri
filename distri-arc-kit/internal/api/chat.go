package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
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
	"distri-arc/internal/waanalysis"
)

func (s *Server) chatRoutes(r chi.Router) {
	r.Get("/chat/threads", s.chatThreads)
	r.Get("/chat/threads/{id}", s.chatThread)
	r.Post("/chat/threads/{id}/read", s.chatRead)
	r.Get("/chat/threads/{id}/context", s.chatContext)
	r.Post("/chat/threads/{id}/messages", s.chatSend)
	r.Patch("/chat/threads/{id}/messages/{mid}", s.chatEdit)
	r.Delete("/chat/threads/{id}/messages/{mid}", s.chatDelete)
	r.Get("/chat/threads/{id}/messages/{mid}/media", s.chatMedia)
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

// threadPageSize caps the first page of a conversation: far above a year of an ordinary chat, so the whole
// retention window arrives at once; olderPageSize is each further page of a busier one.
const (
	threadPageSize = 3000
	olderPageSize  = 500
)

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
	LastDirection string          `json:"last_direction"`
	// WaitingSince: the oldest customer message not answered yet (null when the last word was ours)
	WaitingSince *time.Time `json:"waiting_since"`
}

// timeOrNil reads a nullable timestamp sqlc could not type (min/max over a subquery).
func timeOrNil(v any) *time.Time {
	if t, ok := v.(time.Time); ok {
		return &t
	}
	return nil
}

// phoneOfJID is the contact's number of a one-to-one chat ("62812…@s.whatsapp.net"); groups have none.
func phoneOfJID(jid *string) string {
	if jid == nil || !strings.HasSuffix(*jid, "@s.whatsapp.net") {
		return ""
	}
	return wa.Digits(strings.TrimSuffix(*jid, "@s.whatsapp.net"))
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
		external := kind == "group" && deref(t.GroupKind) == "external"
		if (tab == "dealer" && kind != "dealer") || (tab == "group_internal" && (kind != "group" || external)) ||
			(tab == "group_external" && !external) || (tab == "new" && kind != "new") {
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
			Account: deref(t.Account), AccountLabel: t.AccountLabel, LastDirection: deref(t.LastDirection), WaitingSince: timeOrNil(t.WaitingSince)}
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
	Analysis   *MsgAnalysis    `json:"analysis,omitempty"`
	Media      *MediaView      `json:"media,omitempty"`
	EditedAt   *time.Time      `json:"edited_at,omitempty"`
	// CanEdit: a text this number sent (or still queued); a sent one only within WhatsApp's 15 minutes
	CanEdit bool `json:"can_edit,omitempty"`
	// Quote: the bubble this message replies to (WhatsApp "Balas"); ID is empty when that message is not kept
	Quote *MsgQuote `json:"quote,omitempty"`
	// CanReply: the message has its WhatsApp id, so a reply can quote it
	CanReply bool `json:"can_reply,omitempty"`
	// Deleted: deleted for everyone (by the customer, on the phone, or from Chat); the text is not shown
	Deleted bool `json:"deleted,omitempty"`
	// CanDelete: a message this number sent (or still queued); a sent one within DeleteWindow
	CanDelete bool `json:"can_delete,omitempty"`
}

// MsgQuote is the quoted bubble shown above a reply.
type MsgQuote struct {
	ID        *uuid.UUID `json:"id"`
	Direction string     `json:"direction"`
	FromName  string     `json:"from_name"`
	Body      string     `json:"body"`
	MediaKind string     `json:"media_kind,omitempty"`
}

// MediaView describes a bubble's photo, video, voice note or document; the file itself comes from
// GET /chat/threads/{id}/messages/{mid}/media when the bubble is opened (ADR 0029).
type MediaView struct {
	Kind     string `json:"kind"`
	Mime     string `json:"mime,omitempty"`
	FileName string `json:"file_name,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Seconds  int    `json:"seconds,omitempty"`
	PTT      bool   `json:"ptt,omitempty"`
	// Viewable: the file can be fetched from WhatsApp (messages stored before ADR 0029 have no reference)
	Viewable bool `json:"viewable"`
}

func mediaView(raw json.RawMessage) *MediaView {
	var m wa.Media
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil || m.Kind == "" {
		return nil
	}
	return &MediaView{Kind: m.Kind, Mime: m.Mime, FileName: m.FileName, Size: m.Size, Seconds: m.Seconds, PTT: m.PTT, Viewable: m.Viewable()}
}

// quotesFor resolves the messages quoted by replies on a page: from the page itself, else from the database (an older
// bubble, or one stored under another GSI number's thread of a group).
func (s *Server) quotesFor(ctx context.Context, msgs []gen.ChatMessage) (map[string]*MsgQuote, error) {
	out := map[string]*MsgQuote{}
	want := map[string]bool{}
	for _, m := range msgs {
		if m.QuotedWaID != nil && *m.QuotedWaID != "" {
			want[*m.QuotedWaID] = true
		}
	}
	if len(want) == 0 {
		return out, nil
	}
	quote := func(id uuid.UUID, dir, from, body string, media json.RawMessage, deleted *time.Time) *MsgQuote {
		if deleted != nil {
			body, media = "Pesan ini dihapus", nil
		}
		q := &MsgQuote{ID: &id, Direction: dir, FromName: from, Body: body}
		if mv := mediaView(media); mv != nil {
			q.MediaKind = mv.Kind
		}
		return q
	}
	for _, m := range msgs {
		if m.WaMsgID != nil && want[*m.WaMsgID] {
			out[*m.WaMsgID] = quote(m.ID, deref(m.Direction), deref(m.FromName), deref(m.Body), m.Media, m.DeletedAt)
		}
	}
	var missing []string
	for id := range want {
		if _, ok := out[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	rows, err := s.st.Q.MessagesByWAIDs(ctx, missing)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		out[m.WaMsgID] = quote(m.ID, deref(m.Direction), deref(m.FromName), deref(m.Body), m.Media, m.DeletedAt)
	}
	return out, nil
}

// realWAID: the message has its WhatsApp id (not a reply still queued, not a local row).
func realWAID(id *string) bool {
	return id != nil && *id != "" && !strings.HasPrefix(*id, "outbox:") && !strings.HasPrefix(*id, "local:")
}

// canDelete: an own bubble of this number's thread, queued or sent within DeleteWindow (delete for everyone).
func canDelete(dir, status string, own bool, waID *string, sentAt, now time.Time) bool {
	if dir != "out" || !own {
		return false
	}
	return status == "pending" || (status == "sent" && realWAID(waID) && now.Sub(sentAt) < wa.DeleteWindow)
}

// canEdit: an own text bubble of this number's thread, queued or sent within WhatsApp's edit window.
func canEdit(dir, status string, own bool, media *MediaView, waID *string, sentAt, now time.Time) bool {
	if dir != "out" || !own || media != nil {
		return false
	}
	return status == "pending" || (status == "sent" && realWAID(waID) && now.Sub(sentAt) < wa.EditWindow)
}

// MsgAnalysis is Analisa WhatsApp of one bubble (ADR 0027): the request and the AI note from the last analysis that
// read it, the reply state live (a reply sent after the analysis counts at once).
type MsgAnalysis struct {
	State       string            `json:"state"` // replied | waiting | needs_reply | closing | reply | follow_up
	WaitMin     *int              `json:"wait_min"`
	Analyzed    bool              `json:"analyzed"` // false: written after the last analysis
	Intent      string            `json:"intent,omitempty"`
	Items       []waanalysis.Item `json:"items,omitempty"`
	Note        string            `json:"note,omitempty"`
	Confidence  *int16            `json:"confidence,omitempty"`
	AnalyzedAt  *time.Time        `json:"analyzed_at,omitempty"`
	CycleNumber *int64            `json:"cycle_number,omitempty"`
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
	// The first page is the whole conversation kept (retention.chat_days, a year): no "load more" per chat. Only
	// a very busy chat goes past it and pages on (older pages of olderPageSize).
	before, lim := s.clock.Now().Add(24*365*time.Hour), int32(threadPageSize)
	if b := r.URL.Query().Get("before"); b != "" {
		if x, err := time.Parse(time.RFC3339, b); err == nil {
			before, lim = x, olderPageSize
		}
	}
	msgs, err := s.st.Q.ListThreadMessages(r.Context(), gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: before, Lim: lim})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	hasMore := len(msgs) == int(lim)
	keepDays := 0
	if pol, err := policy.Load(r.Context(), s.st.Q); err == nil {
		keepDays = pol.Retention.ChatDays
	}
	var historyFrom *time.Time
	if !hasMore && len(msgs) > 0 {
		historyFrom = &msgs[0].SentAt
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
	// a group: messages stored under another GSI number's thread of the group; one that number sent is shown as
	// "in" from that number (this number did not send it)
	others := map[uuid.UUID]string{}
	if deref(t.Kind) == "group" {
		accs, err := s.st.Q.GroupThreadAccounts(r.Context(), t.ID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		for _, a := range accs {
			if a.ID != t.ID {
				others[a.ID] = a.AccountLabel
			}
		}
	}
	quotes, err := s.quotesFor(r.Context(), msgs)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]MessageView, 0, len(msgs))
	now := s.clock.Now()
	for _, m := range msgs {
		v := MessageView{ID: m.ID, Direction: deref(m.Direction), FromName: deref(m.FromName), Body: deref(m.Body), SentAt: m.SentAt, Status: m.Status, Internal: m.Internal, Annotation: m.Annotation, SignalID: m.SignalID,
			Media: mediaView(m.Media), EditedAt: m.EditedAt}
		if label, ok := others[deref(m.ThreadID)]; ok && v.Direction == "out" {
			v.Direction, v.FromName, v.Internal = "in", label, true
		}
		v.CanEdit = canEdit(v.Direction, v.Status, deref(m.ThreadID) == t.ID, v.Media, m.WaMsgID, m.SentAt, now)
		v.CanReply = realWAID(m.WaMsgID) && v.Status != "failed"
		v.CanDelete = canDelete(v.Direction, v.Status, deref(m.ThreadID) == t.ID, m.WaMsgID, m.SentAt, now)
		if m.DeletedAt != nil {
			v.Deleted, v.Body, v.Media, v.CanEdit, v.CanReply, v.CanDelete, v.Analysis = true, "", nil, false, false, false, nil
		}
		if m.QuotedWaID != nil {
			q, ok := quotes[*m.QuotedWaID]
			if !ok {
				q = &MsgQuote{Body: "Pesan tidak tersimpan di GSI Orbit"}
			}
			v.Quote = q
		}
		if m.SignalID != nil {
			v.Proposal = bySig[*m.SignalID]
		}
		out = append(out, v)
	}
	var lastAnalysis *time.Time
	if k := deref(t.Kind); k == "dealer" || k == "new" {
		var err error
		if lastAnalysis, err = s.attachMsgAnalysis(r.Context(), t.ID, out, r.URL.Query().Get("before") == ""); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"thread": map[string]any{
		"id": t.ID, "kind": deref(t.Kind), "title": deref(t.Title), "subtitle": deref(t.Subtitle), "dealer_id": deref(t.DealerSlug),
		"sales": deref(t.SalesName), "sales_wa": deref(t.SalesWa), "account_label": t.AccountLabel, "account_masked": wa.MaskNumber(deref(t.SalesWa)), "suggestions": t.Suggestions, "tag": t.Tag, "unread": t.Unread,
		"phone": phoneOfJID(t.WaJid), "dealer_code": deref(t.DealerCode), "dealer_city": deref(t.DealerCity), "waiting_since": timeOrNil(t.WaitingSince),
		"analyzed_at": lastAnalysis,
	}, "messages": out, "has_more": hasMore, "history_from": historyFrom, "history_days": keepDays})
}

// attachMsgAnalysis adds Analisa WhatsApp to each bubble of a one-to-one conversation. The newest page reads the
// reply state live from its own messages (a reply sent after the analysis counts at once); an older page keeps the
// state the analysis stored, since the reply may sit on a newer page.
func (s *Server) attachMsgAnalysis(ctx context.Context, thread uuid.UUID, msgs []MessageView, newest bool) (*time.Time, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(msgs))
	th := waanalysis.Thread{ID: thread}
	for i, m := range msgs {
		ids[i] = m.ID
		if m.Status != "failed" {
			th.Messages = append(th.Messages, waanalysis.Message{ID: m.ID, In: m.Direction == "in", Text: m.Body, At: m.SentAt})
		}
	}
	rows, err := s.st.Q.WAMessageInsights(ctx, gen.WAMessageInsightsParams{ThreadID: thread, MessageIds: ids})
	if err != nil {
		return nil, err
	}
	stored := map[uuid.UUID]gen.WAMessageInsightsRow{}
	for _, x := range rows {
		stored[x.MessageID] = x
	}
	live := map[uuid.UUID]waanalysis.MessageInsight{}
	if newest {
		for _, x := range waanalysis.AnalyzeMessages(th, nil, s.clock.Now()) {
			live[x.MessageID] = x
		}
	}
	for i := range msgs {
		a := &MsgAnalysis{}
		x, ok := stored[msgs[i].ID]
		if ok {
			at := x.AnalyzedAt
			a.Analyzed, a.Intent, a.AnalyzedAt, a.CycleNumber, a.Confidence = true, x.Intent, &at, x.CycleNumber, x.Confidence
			a.State = x.State
			if x.WaitMin != nil {
				w := int(*x.WaitMin)
				a.WaitMin = &w
			}
			_ = json.Unmarshal(x.Items, &a.Items)
			a.Note = deref(x.Note)
		}
		if l, ok := live[msgs[i].ID]; ok {
			a.State, a.WaitMin = l.State, l.WaitMin
		}
		if a.State == "" {
			continue // failed send, or an older bubble never analysed
		}
		msgs[i].Analysis = a
	}
	last, err := s.st.Q.LastWAMessageAnalysis(ctx, thread)
	if err != nil || last.IsZero() {
		return nil, nil
	}
	return &last, nil
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
	msgs, err := s.st.Q.ListThreadMessages(r.Context(), gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: s.clock.Now().Add(24 * 365 * time.Hour), Lim: 100})
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

// humanDecision records what a person does in Chat as an approved proposal of their own (decided_by): the reply or
// edit they typed is the decision (CLAUDE.md §2). It hangs off the conversation's latest signal (provenance), or a
// manual one when nothing came in yet.
func (s *Server) humanDecision(ctx context.Context, q *gen.Queries, t gen.GetThreadRow, u User, title, body string, now time.Time) (gen.Proposal, error) {
	sig, err := q.LatestThreadSignal(ctx, &t.ID)
	if err != nil || sig == nil {
		payload, _ := json.Marshal(map[string]any{"thread": t.ID, "reason": "balasan manusia tanpa pesan masuk"})
		sm := "Balasan dari aplikasi"
		id, err := q.InsertManualSignal(ctx, gen.InsertManualSignalParams{DealerID: t.DealerID, SalesID: u.SalesUserID, OccurredAt: now, DedupeKey: "reply:" + uuid.NewString(), Summary: &sm, Payload: payload})
		if err != nil {
			return gen.Proposal{}, err
		}
		sig = &id
	}
	reason := "Dikirim langsung dari Chat oleh " + deref(u.Name)
	return q.InsertProposal(ctx, gen.InsertProposalParams{Agent: "Manusia", DealerID: t.DealerID, Kind: "reply", Title: title, Why: reason,
		Preview: &body, Confidence: 1, SignalIds: []uuid.UUID{*sig}, Autonomy: "approve", Status: "approved", DecidedBy: u.SalesUserID, DecidedAt: &now, DecisionReason: &reason})
}

// chatAllowed: the checks every send or edit from Chat passes (a sales identity, a number, not in pilot shadow).
func (s *Server) chatAllowed(w http.ResponseWriter, r *http.Request, t gen.GetThreadRow) (User, bool) {
	u, _ := CurrentUser(r.Context())
	if u.SalesUserID == nil {
		httpx.Fail(w, http.StatusForbidden, "no_sales_user", "Pengguna tidak terhubung ke data sales")
		return u, false
	}
	if t.SalesWa == nil || t.WaJid == nil {
		httpx.Fail(w, http.StatusConflict, "no_number", "Percakapan ini belum terhubung ke nomor sales")
		return u, false
	}
	if pol, err := policy.Load(r.Context(), s.st.Q); err == nil && pol.Pilot.Shadow() {
		httpx.Fail(w, http.StatusConflict, "pilot_shadow", "Mode bayangan pilot: balas dari WhatsApp di ponsel — GSI Orbit belum mengirim")
		return u, false
	}
	return u, true
}

// chatSend: a human typing a reply is itself the decision. It is recorded as a proposal kind=reply approved
// by the sender (decided_by), written to the outbox and delivered by the worker (07-api › Chat).
func (s *Server) chatSend(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	in, file, why := readChatSend(r)
	if why != "" {
		code := http.StatusBadRequest
		if strings.Contains(why, "25 MB") {
			code = http.StatusRequestEntityTooLarge
		}
		httpx.Fail(w, code, "invalid", why)
		return
	}
	u, ok := s.chatAllowed(w, r, t)
	if !ok {
		return
	}
	var quote *wa.Quote
	if in.ReplyTo != "" {
		q, why := s.replyQuote(r.Context(), t, in.ReplyTo)
		if why != "" {
			httpx.Fail(w, http.StatusConflict, "not_quotable", why)
			return
		}
		quote = q
	}
	now := s.clock.Now()
	caption := strings.TrimSpace(in.Body)
	body := caption // the bubble's text: "[gambar] caption", "[polling] Q • a • b", like received media
	var media json.RawMessage
	if in.Content != nil {
		c := in.Content
		label := wa.MediaLabel(c.Kind, map[bool]string{true: c.FileName}[c.Kind == "document"])
		if c.Kind == "audio" && !c.PTT {
			label = "[audio]"
		}
		if c.Kind == "poll" {
			body = label + " " + c.Poll.Name + "\n• " + strings.Join(c.Poll.Options, "\n• ")
		} else {
			body = strings.TrimSpace(label + " " + caption)
		}
		media, _ = json.Marshal(wa.Media{Kind: c.Kind, Mime: c.Mime, FileName: c.FileName, Size: c.Size, Seconds: c.Seconds, PTT: c.PTT})
	}
	var res map[string]any
	err := s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
		p, err := s.humanDecision(r.Context(), q, t, u, "Balasan ke "+deref(t.Title), body, now)
		if err != nil {
			return err
		}
		dir, from := "out", *t.SalesWa
		name := deref(t.SalesName)
		pendingID := "outbox:" + p.ID.String()
		var quoted *string
		if quote != nil {
			quoted = &quote.ID
		}
		mid, err := q.InsertChatMessage(r.Context(), gen.InsertChatMessageParams{ThreadID: &t.ID, WaMsgID: &pendingID, Direction: &dir, FromNumber: &from, FromName: &name, Body: &body, Media: media, SentAt: now, Status: "pending", ProposalID: &p.ID, QuotedWaID: quoted})
		if err != nil {
			return err
		}
		text := body
		if in.Content != nil {
			text = caption
			if file != nil {
				fid, err := q.InsertOutgoingFile(r.Context(), gen.InsertOutgoingFileParams{MessageID: mid, Mime: in.Content.Mime, FileName: in.Content.FileName, Size: int32(len(file)), Data: file})
				if err != nil {
					return err
				}
				in.Content.FileID = fid.String()
			}
		}
		payload, _ := json.Marshal(outbox.WAPayload{From: from, To: *t.WaJid, Text: text, Kind: "reply", MessageID: mid.String(), ThreadID: t.ID.String(), Quote: quote, Content: in.Content})
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

// replyQuote is the WhatsApp key and text of a bubble a reply quotes. The bubble must belong to the conversation and
// have its WhatsApp id; in a group, a member's message is addressed by its sender. why is the refusal for the person.
func (s *Server) replyQuote(ctx context.Context, t gen.GetThreadRow, id string) (q *wa.Quote, why string) {
	mid, err := uuid.Parse(id)
	if err != nil {
		return nil, "Pesan yang dibalas tidak ditemukan"
	}
	m, err := s.st.Q.GetThreadMessage(ctx, gen.GetThreadMessageParams{ThreadID: t.ID, ID: mid})
	if err != nil {
		return nil, "Pesan yang dibalas tidak ditemukan"
	}
	if !realWAID(m.WaMsgID) {
		return nil, "Pesan itu belum terkirim — tunggu terkirim, lalu balas"
	}
	// sent by this number: its own thread's outgoing message; anything else came from someone else
	fromMe := deref(m.Direction) == "out" && deref(m.ThreadID) == t.ID
	q = &wa.Quote{ID: *m.WaMsgID, FromMe: fromMe, Text: deref(m.Body)}
	if wa.IsGroupJID(deref(t.WaJid)) && !fromMe && deref(m.FromNumber) != "" {
		q.Participant = wa.UserJID(deref(m.FromNumber))
	}
	return q, ""
}

// chatSendInput is a send from Chat: text (or caption), an optional bubble to quote, and an optional file, voice
// note or poll.
type chatSendInput struct {
	Body    string             `json:"body"`
	ReplyTo string             `json:"reply_to"` // a bubble of this conversation to quote (WhatsApp "Balas")
	Poll    *wa.Poll           `json:"poll"`
	Content *outbox.OutContent `json:"-"`
}

// maxChatFile is the largest file Chat sends (WhatsApp takes more for documents; uploads stay small).
const maxChatFile = 25 << 20

// readChatSend reads JSON {body, reply_to, poll} or multipart {body, reply_to, file, voice, seconds}. why is the
// refusal for the person; the file's bytes come back separately (stored until the outbox sends them).
func readChatSend(r *http.Request) (in chatSendInput, file []byte, why string) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(nil, r.Body, maxChatFile+(1<<20))
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			return in, nil, "File lebih dari 25 MB — kirim lewat HP atau perkecil dulu"
		}
		in.Body, in.ReplyTo = r.FormValue("body"), r.FormValue("reply_to")
		f, h, err := r.FormFile("file")
		if err != nil {
			return in, nil, "File tidak terbaca"
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxChatFile+1))
		if err != nil || len(data) == 0 {
			return in, nil, "File kosong"
		}
		if len(data) > maxChatFile {
			return in, nil, "File lebih dari 25 MB — kirim lewat HP atau perkecil dulu"
		}
		ct, _, _ := strings.Cut(h.Header.Get("Content-Type"), ";")
		ct = strings.ToLower(strings.TrimSpace(ct))
		if ct == "" || ct == "application/octet-stream" {
			ct, _, _ = strings.Cut(http.DetectContentType(data), ";")
		}
		c := &outbox.OutContent{Mime: ct, FileName: filepathBase(h.Filename), Size: int64(len(data))}
		switch {
		case r.FormValue("voice") == "1":
			c.Kind, c.PTT = "audio", true
			c.Mime = strings.TrimSpace(h.Header.Get("Content-Type")) // keep "audio/webm;codecs=opus" for the conversion
			if sec, err := strconv.Atoi(r.FormValue("seconds")); err == nil && sec > 0 {
				c.Seconds = sec
			}
		case ct == "image/jpeg" || ct == "image/png" || ct == "image/webp":
			c.Kind = "image"
		case ct == "video/mp4" || ct == "video/3gpp" || ct == "video/quicktime":
			c.Kind = "video"
		case strings.HasPrefix(ct, "audio/"):
			c.Kind = "audio"
		default:
			c.Kind = "document"
		}
		in.Content = c
		return in, data, ""
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return in, nil, "Pesan kosong"
	}
	if in.Poll != nil {
		p := &wa.Poll{Name: strings.TrimSpace(in.Poll.Name), Multi: in.Poll.Multi}
		for _, o := range in.Poll.Options {
			if o = strings.TrimSpace(o); o != "" && !slices.Contains(p.Options, o) {
				p.Options = append(p.Options, o)
			}
		}
		if p.Name == "" || len(p.Options) < 2 || len(p.Options) > 12 {
			return in, nil, "Polling butuh pertanyaan dan 2–12 pilihan yang berbeda"
		}
		in.Content = &outbox.OutContent{Kind: "poll", Poll: p}
		return in, nil, ""
	}
	if strings.TrimSpace(in.Body) == "" {
		return in, nil, "Pesan kosong"
	}
	return in, nil, ""
}

// filepathBase is a file name without any directory a browser may send.
func filepathBase(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSpace(name)
}

// errEditGone: the queued reply left the outbox between reading and editing it.
var errEditGone = errors.New("reply already sending")

// chatEdit edits a text this number sent, like WhatsApp's Edit. A reply still queued changes in the outbox; a sent
// one (within 15 minutes) is a new human decision — proposal, outbox row kind=edit, audit — that the worker carries
// out through the bridge, which confirms the approval as for a send.
func (s *Server) chatEdit(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "mid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Body) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pesan kosong")
		return
	}
	u, ok := s.chatAllowed(w, r, t)
	if !ok {
		return
	}
	m, err := s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ThreadID: t.ID, ID: mid})
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	now := s.clock.Now()
	body := strings.TrimSpace(in.Body)
	if !canEdit(deref(m.Direction), m.Status, deref(m.ThreadID) == t.ID, mediaView(m.Media), m.WaMsgID, m.SentAt, now) {
		msg := "Hanya pesan teks dari nomor ini yang bisa diedit"
		if deref(m.Direction) == "out" && m.Status == "sent" && now.Sub(m.SentAt) >= wa.EditWindow {
			msg = "Lewat 15 menit sejak dikirim — WhatsApp tidak menerima edit lagi"
		}
		httpx.Fail(w, http.StatusConflict, "not_editable", msg)
		return
	}
	if body == deref(m.Body) {
		httpx.JSON(w, http.StatusOK, map[string]any{"message_id": mid, "status": m.Status})
		return
	}
	actor, kind, entity := deref(u.Email), "user", "chat_message"
	if m.Status == "pending" {
		err := s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
			if _, err := q.EditQueuedReply(r.Context(), gen.EditQueuedReplyParams{Text: body, MessageID: mid.String()}); errors.Is(err, pgx.ErrNoRows) {
				return errEditGone
			} else if err != nil {
				return err
			}
			if err := q.SetChatMessageBody(r.Context(), gen.SetChatMessageBodyParams{ID: mid, Body: body}); err != nil {
				return err
			}
			action := "chat.edit_queued"
			after, _ := json.Marshal(map[string]any{"thread_id": t.ID, "before": deref(m.Body), "body": body})
			if err := q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &mid, After: after}); err != nil {
				return err
			}
			_, err := tx.Exec(r.Context(), "select pg_notify('chat_message', $1)", `{"thread_id":"`+t.ID.String()+`"}`)
			return err
		})
		if errors.Is(err, errEditGone) {
			httpx.Fail(w, http.StatusConflict, "sending", "Pesan sedang dikirim — tunggu terkirim, lalu edit lagi")
			return
		}
		if err != nil {
			s.log.Error("chat edit queued", "err", err)
			httpx.Fail(w, http.StatusInternalServerError, "internal", "Gagal mengubah pesan")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"message_id": mid, "status": "pending"})
		return
	}
	var res map[string]any
	err = s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
		p, err := s.humanDecision(r.Context(), q, t, u, "Edit pesan ke "+deref(t.Title), body, now)
		if err != nil {
			return err
		}
		sentAt := m.SentAt
		payload, _ := json.Marshal(outbox.WAPayload{From: *t.SalesWa, To: *t.WaJid, Text: body, Kind: "edit", MessageID: mid.String(), ThreadID: t.ID.String(), EditOf: *m.WaMsgID, EditSentAt: &sentAt})
		ob, err := q.InsertOutbox(r.Context(), gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: t.WaJid, Payload: payload})
		if err != nil {
			return err
		}
		action := "chat.edit"
		after, _ := json.Marshal(map[string]any{"thread_id": t.ID, "outbox_id": ob.ID, "message_id": mid, "before": deref(m.Body), "body": body})
		if err := q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &mid, After: after}); err != nil {
			return err
		}
		if s.jobs != nil {
			if _, err := s.jobs.InsertTx(r.Context(), tx, jobs.OutboxSendArgs{OutboxID: ob.ID.String()}, nil); err != nil {
				return err
			}
		}
		res = map[string]any{"proposal_id": p.ID, "outbox_id": ob.ID, "message_id": mid, "status": "editing"}
		return nil
	})
	if err != nil {
		s.log.Error("chat edit", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Gagal mencatat edit")
		return
	}
	httpx.JSON(w, http.StatusAccepted, res)
}

// chatDelete deletes a message this number sent, for everyone (WhatsApp "Hapus untuk semua orang"). A reply still
// queued is cancelled in the outbox; a sent one (within DeleteWindow) is a human decision — proposal, outbox row
// kind=delete, audit — that the worker carries out through the bridge. The text stays for the audit trail.
func (s *Server) chatDelete(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "mid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	u, ok := s.chatAllowed(w, r, t)
	if !ok {
		return
	}
	m, err := s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ThreadID: t.ID, ID: mid})
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	now := s.clock.Now()
	if m.DeletedAt != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"message_id": mid, "status": "deleted"})
		return
	}
	if !canDelete(deref(m.Direction), m.Status, deref(m.ThreadID) == t.ID, m.WaMsgID, m.SentAt, now) {
		msg := "Hanya pesan dari nomor ini yang bisa dihapus untuk semua orang"
		if deref(m.Direction) == "out" && m.Status == "sent" && now.Sub(m.SentAt) >= wa.DeleteWindow {
			msg = "Lewat 48 jam sejak dikirim — hapus untuk semua orang tidak tersedia lagi"
		}
		httpx.Fail(w, http.StatusConflict, "not_deletable", msg)
		return
	}
	actor, kind, entity := deref(u.Email), "user", "chat_message"
	if m.Status == "pending" {
		err := s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
			if _, err := q.CancelQueuedReply(r.Context(), mid.String()); errors.Is(err, pgx.ErrNoRows) {
				return errEditGone
			} else if err != nil {
				return err
			}
			if err := q.SetChatMessageDeleted(r.Context(), gen.SetChatMessageDeletedParams{ID: mid, DeletedAt: now}); err != nil {
				return err
			}
			action := "chat.delete_queued"
			after, _ := json.Marshal(map[string]any{"thread_id": t.ID, "body": deref(m.Body)})
			if err := q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &mid, After: after}); err != nil {
				return err
			}
			_, err := tx.Exec(r.Context(), "select pg_notify('chat_message', $1)", `{"thread_id":"`+t.ID.String()+`"}`)
			return err
		})
		if errors.Is(err, errEditGone) {
			httpx.Fail(w, http.StatusConflict, "sending", "Pesan sedang dikirim — tunggu terkirim, lalu hapus")
			return
		}
		if err != nil {
			s.log.Error("chat delete queued", "err", err)
			httpx.Fail(w, http.StatusInternalServerError, "internal", "Gagal menghapus pesan")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"message_id": mid, "status": "deleted"})
		return
	}
	var res map[string]any
	err = s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
		p, err := s.humanDecision(r.Context(), q, t, u, "Hapus pesan ke "+deref(t.Title), "Hapus untuk semua orang: "+deref(m.Body), now)
		if err != nil {
			return err
		}
		sentAt := m.SentAt
		payload, _ := json.Marshal(outbox.WAPayload{From: *t.SalesWa, To: *t.WaJid, Kind: "delete", MessageID: mid.String(), ThreadID: t.ID.String(), EditOf: *m.WaMsgID, EditSentAt: &sentAt})
		ob, err := q.InsertOutbox(r.Context(), gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: t.WaJid, Payload: payload})
		if err != nil {
			return err
		}
		action := "chat.delete"
		after, _ := json.Marshal(map[string]any{"thread_id": t.ID, "outbox_id": ob.ID, "message_id": mid, "body": deref(m.Body)})
		if err := q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &mid, After: after}); err != nil {
			return err
		}
		if s.jobs != nil {
			if _, err := s.jobs.InsertTx(r.Context(), tx, jobs.OutboxSendArgs{OutboxID: ob.ID.String()}, nil); err != nil {
				return err
			}
		}
		res = map[string]any{"proposal_id": p.ID, "outbox_id": ob.ID, "message_id": mid, "status": "deleting"}
		return nil
	})
	if err != nil {
		s.log.Error("chat delete", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Gagal mencatat hapus")
		return
	}
	httpx.JSON(w, http.StatusAccepted, res)
}

// inlineMedia: types a browser may show in the page; anything else is downloaded as a file.
var inlineMedia = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
	"video/mp4": true, "video/3gpp": true, "audio/ogg": true, "audio/mpeg": true, "audio/mp4": true, "audio/aac": true, "audio/amr": true, "application/pdf": true}

// mediaFetcher is how the API reaches WhatsApp for a file (the Baileys bridge; tests set their own).
func (s *Server) mediaFetcher() wa.MediaFetcher {
	if s.media != nil {
		return s.media
	}
	if s.cfg.WATransport != "baileys" {
		return nil
	}
	return &wa.Baileys{BridgeURL: s.cfg.BridgeURL, Secret: s.cfg.BridgeSecret, Session: func(ctx context.Context, account string) string {
		if n, err := s.st.Q.GetWANumber(ctx, account); err == nil {
			return n.SessionID
		}
		return ""
	}}
}

// chatMedia streams a bubble's photo, video, voice note or document from WhatsApp through the number that received
// it (ADR 0029). Nothing is stored on the server; the browser keeps it in its private cache.
func (s *Server) chatMedia(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "mid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	m, err := s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ThreadID: t.ID, ID: mid})
	var md wa.Media
	if err != nil || json.Unmarshal(m.Media, &md) != nil || !md.Viewable() {
		httpx.Fail(w, http.StatusNotFound, "no_media", "Media pesan ini tidak tersimpan — buka di WhatsApp ponsel")
		return
	}
	f := s.mediaFetcher()
	if f == nil {
		httpx.Fail(w, http.StatusConflict, "media_unavailable", "Media hanya bisa dibuka lewat WhatsApp Baileys")
		return
	}
	body, err := f.Media(r.Context(), deref(m.HolderAccount), md)
	switch {
	case errors.Is(err, wa.ErrMediaGone):
		httpx.Fail(w, http.StatusGone, "media_gone", "Media sudah tidak tersedia di WhatsApp")
		return
	case errors.Is(err, wa.ErrMediaUnavailable):
		httpx.Fail(w, http.StatusServiceUnavailable, "media_busy", "Nomor penerima belum terhubung atau sedang sibuk — coba lagi sebentar")
		return
	case err != nil:
		s.log.Warn("chat media", "err", err, "message", mid)
		httpx.Fail(w, http.StatusBadGateway, "media_failed", "Gagal mengambil media dari WhatsApp")
		return
	}
	defer body.Close()
	ct, _, _ := strings.Cut(md.Mime, ";")
	ct = strings.ToLower(strings.TrimSpace(ct))
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("Cache-Control", "private, max-age=604800, immutable")
	if inlineMedia[ct] {
		h.Set("Content-Type", ct)
		h.Set("Content-Disposition", "inline")
	} else {
		name := md.FileName
		if name == "" {
			name = "lampiran"
		}
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	if _, err := io.Copy(w, body); err != nil {
		s.log.Debug("chat media copy", "err", err)
	}
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
			"user_id": n.UserID, "user_name": deref(n.UserName), "user_email": deref(n.UserEmail), "mine": mine,
			"user_role": deref(n.UserRole), "user_branch": deref(n.UserBranch), "session_id": n.SessionID,
			"thread_count": n.ThreadCount, "unread_count": n.UnreadCount, "last_message_at": n.LastMessageAt}
		if isAdmin(u) || mine {
			v["phone"] = "+" + n.WaNumber // the full number only for managers and the holder; others see it masked
		}
		// the bridge counts per session: numbers linked from Chat run under a "link-…" session, not their number
		if l, ok := limits[n.SessionID]; ok && n.SessionID != "" {
			v["limits"] = l
		} else if l, ok := limits[n.WaNumber]; ok {
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
