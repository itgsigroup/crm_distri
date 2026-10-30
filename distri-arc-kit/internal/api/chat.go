package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	qrcode "github.com/skip2/go-qrcode"

	"distri-arc/internal/hermes"
	"distri-arc/internal/httpx"
	"distri-arc/internal/jobs"
	"distri-arc/internal/llm"
	"distri-arc/internal/outbox"
	"distri-arc/internal/policy"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
	"distri-arc/internal/waanalysis"
)

func (s *Server) chatRoutes(r chi.Router) {
	r = r.With(s.threads.afterWrite)
	r.Get("/chat/threads", s.chatThreads)
	r.Get("/chat/threads/{id}", s.chatThread)
	r.Post("/chat/threads/{id}/read", s.chatRead)
	r.Get("/chat/threads/{id}/context", s.chatContext)
	r.Get("/chat/threads/{id}/search", s.chatSearch)
	r.Get("/chat/messages/search", s.searchMessages)
	r.Get("/chat/threads/{id}/ai-overview", s.chatAIOverview)
	r.Get("/chat/threads/{id}/stats", s.chatStats)
	r.Post("/chat/threads/{id}/history", s.chatHistory)
	r.Get("/chat/threads/{id}/members", s.chatMembers)
	r.Post("/chat/threads/{id}/members/{number}/open", s.chatOpenMember)
	r.Post("/chat/threads/{id}/messages", s.chatSend)
	r.Patch("/chat/threads/{id}/messages/{mid}", s.chatEdit)
	r.Delete("/chat/threads/{id}/messages/{mid}", s.chatDelete)
	r.Put("/chat/threads/{id}/messages/{mid}/reaction", s.chatReact)
	r.Put("/chat/threads/{id}/contact-type", s.setContactType)
	r.Put("/chat/threads/{id}/alias", s.setThreadAlias)
	r.Get("/chat/threads/{id}/messages/{mid}/media", s.chatMedia)
	r.Get("/internal-numbers", s.internalNumbers)
	r.Post("/internal-numbers", s.addInternalNumber)
	r.Delete("/internal-numbers/{wa}", s.deleteInternalNumber)
	s.chatLabelRoutes(r)
	s.quickReplyRoutes(r)
	s.lyraRoutes(r)
	s.profileRoutes(r)
	s.chatListRoutes(r)
	s.chatBackupRoutes(r)
	r.Get("/wa/groups", s.waGroups)
	r.Patch("/wa/groups/{id}", s.patchWAGroup)
	r.Get("/wa/status", s.waStatus)
	r.Post("/wa/pair", s.waPair)
	r.Post("/wa/links", s.createWALink)
	r.Get("/wa/links/{id}", s.getWALink)
	r.Put("/wa/numbers/{wa}/user", s.assignWANumber)
	r.Put("/wa/numbers/{wa}/team", s.setWANumberTeam)
	r.Delete("/wa/numbers/{wa}", s.deleteWANumber)
}

// threadPageSize caps the first page of a conversation (the chat window); olderPageSize is each page loaded by
// scrolling up; quietChatPage is what a chat with nothing in the window opens with.
const (
	threadPageSize = 3000
	olderPageSize  = 500
	quietChatPage  = 30
	jumpPageSize   = 5000 // a jump to a search result loads at most this many messages
)

// ThreadView is one row of the chat list.
type ThreadView struct {
	ID            uuid.UUID       `json:"id"`
	Kind          string          `json:"kind"`
	Title         string          `json:"title"`
	Subtitle      string          `json:"subtitle"`
	PhoneName     string          `json:"phone_name,omitempty"` // what the phone saved for this number ("Pak Yohanes")
	DealerID      string          `json:"dealer_id,omitempty"`
	DealerName    string          `json:"dealer_name,omitempty"`
	Sales         string          `json:"sales"`
	LastMessageAt *time.Time      `json:"last_message_at"`
	Unread        int32           `json:"unread"`
	Mentioned     int32           `json:"mentioned,omitempty"` // unread messages that tag this number ("@")
	LastBody      string          `json:"last_body"`
	LastFrom      string          `json:"last_from"`
	Tag           json.RawMessage `json:"tag"`
	GroupKind     string          `json:"group_kind,omitempty"`
	Account       string          `json:"account"`       // the WhatsApp number holding the conversation
	AccountLabel  string          `json:"account_label"` // its label (CS Kantor, Nomor Andi …)
	AccountTeam   string          `json:"account_team"`  // its team: sales | admin | billing (ADR 0033)
	// ContactType: supplier (pemasok) — not a customer lead, out of the reply queue (ADR 0035); empty: customer
	ContactType   string `json:"contact_type,omitempty"`
	LastDirection string `json:"last_direction"`
	// WaitingSince: the oldest customer message not answered yet (null when the last word was ours)
	WaitingSince *time.Time `json:"waiting_since"`
	// Labels: the conversation's labels (WhatsApp Business), shown on the row and filterable
	Labels []uuid.UUID `json:"labels,omitempty"`
	// PinnedAt: the person pinned this chat to the top of their list
	PinnedAt *time.Time `json:"pinned_at,omitempty"`
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

// chatViewer is which WhatsApp conversations a user reads (ADR 0033): CEO/admin every number; a sales the numbers of
// their sales profile; Penagihan (base finance) only the number they hold.
type chatViewer struct {
	all      bool
	sales    *uuid.UUID
	accounts map[string]bool // Penagihan: the numbers they hold
	held     map[string]bool // a sales: the numbers they hold (their chats even when a number's sales profile differs)
	// locked: numbers logged out of their linked device — their chat history stays hidden from everyone, CEO/admin
	// included, until the number is linked again (Sam 27 Oct)
	locked map[string]bool
}

// waOwnOnly: the user reads (and manages) only their own WhatsApp numbers — a sales, a Penagihan without the
// management pages, or any role whose Chat WhatsApp is set to its own numbers (Peran & akses, Sam 17 Oct).
func waOwnOnly(u User) bool {
	return deref(u.Role) == "sales" || (deref(u.Role) == "finance" && !isAdmin(u)) || u.RoleChatScope == "own"
}

// heldNumbers are the WhatsApp numbers a user holds: their own number and every number linked to them.
func (s *Server) heldNumbers(ctx context.Context, u User) (map[string]bool, error) {
	held := map[string]bool{}
	if u.WaNumber != nil {
		held[*u.WaNumber] = true
	}
	rows, err := s.st.Pool.Query(ctx, "select wa_number from wa_numbers where user_id = $1", u.ID)
	if err != nil {
		return held, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return held, err
		}
		held[n] = true
	}
	return held, rows.Err()
}

func (s *Server) chatViewerOf(ctx context.Context, u User) (chatViewer, error) {
	var v chatViewer
	var err error
	switch {
	case deref(u.Role) == "sales":
		// their sales profile's chats, and the chats on the numbers they hold (19 Oct: a number whose sales profile is
		// another sales still shows its chats to the person holding it)
		v.sales = u.SalesUserID
		v.held, err = s.heldNumbers(ctx, u)
	case waOwnOnly(u):
		v.accounts, err = s.heldNumbers(ctx, u)
	default:
		v.all = true
	}
	if err != nil {
		return v, err
	}
	v.locked, err = s.loggedOutNumbers(ctx)
	return v, err
}

// loggedOutNumbers are the numbers WhatsApp removed from their linked device: they need linking again.
func (s *Server) loggedOutNumbers(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := s.st.Pool.Query(ctx, "select wa_number from wa_numbers where state = 'logged_out'")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return out, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

// forReports is the viewer for Report WhatsApp and Analisa WhatsApp: a logged-out number's history stays counted
// there (Sam 27 Oct) — the lock only hides it from Chat.
func (v chatViewer) forReports() chatViewer {
	v.locked = nil
	return v
}

// lockedOut reports whether a conversation is hidden because its number is logged out.
func (v chatViewer) lockedOut(account *string) bool {
	return account != nil && v.locked[*account]
}

// sees reports whether the viewer reads a conversation held by account (its sales salesID).
func (v chatViewer) sees(salesID *uuid.UUID, account *string) bool {
	switch {
	case v.lockedOut(account):
		return false
	case v.all:
		return true
	case v.accounts != nil:
		return account != nil && v.accounts[*account]
	}
	if account != nil && v.held[*account] {
		return true
	}
	return v.sales != nil && salesID != nil && *salesID == *v.sales
}

// chatSince is where the chat window starts: Chat lists and opens conversations of the last CHAT_WINDOW_DAYS.
func (s *Server) chatSince() time.Time {
	if s.cfg.ChatWindowDays <= 0 {
		return time.Time{}
	}
	return s.clock.Now().AddDate(0, 0, -s.cfg.ChatWindowDays)
}

// chatOpenSince is where an opened conversation starts: the last CHAT_OPEN_DAYS (a month, Sam 27 Oct), within the
// chat window — a year of bubbles at once made the chat slow on slower laptops.
func (s *Server) chatOpenSince() time.Time {
	since := s.chatSince()
	if s.cfg.ChatOpenDays <= 0 {
		return since
	}
	if open := s.clock.Now().AddDate(0, 0, -s.cfg.ChatOpenDays); open.After(since) {
		return open
	}
	return since
}

func (s *Server) chatThreads(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	account := wa.Digits(r.URL.Query().Get("account"))
	team := strings.TrimSpace(r.URL.Query().Get("team"))
	var accountFilter, teamFilter *string
	if account != "" {
		accountFilter = &account
	}
	if team != "" {
		teamFilter = &team
	}
	query := func(ctx context.Context, since time.Time) ([]gen.ListThreadsRow, error) {
		return s.st.Q.ListThreads(ctx, gen.ListThreadsParams{Since: since, Account: accountFilter, Team: teamFilter})
	}
	var rows []gen.ListThreadsRow
	var err error
	if r.URL.Query().Get("all") == "1" && len([]rune(term)) >= 2 {
		// a search reaches every stored conversation, not only the chat window: an older customer is still found
		rows, err = query(r.Context(), time.Time{})
	} else {
		key := account + "\x00" + team
		rows, err = s.threads.getKey(r.Context(), key, func(ctx context.Context) ([]gen.ListThreadsRow, error) {
			return query(ctx, s.chatSince())
		})
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.mergeLIDChats(rows) // a contact's chat stored under a LID joins their number's chat (ADR 0072)
	tab := r.URL.Query().Get("tab")
	// Account/team already narrowed the database query; these checks remain as defense in depth.
	match := threadMatcher(term)
	u, _ := CurrentUser(r.Context())
	viewer, err := s.chatViewerOf(r.Context(), u)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	labels, pins := s.labelsByConv(r), s.threadPins(r)
	out := []ThreadView{}
	for _, t := range rows {
		kind := deref(t.Kind)
		external := kind == "group" && deref(t.GroupKind) == "external"
		if (tab == "dealer" && kind != "dealer") || (tab == "group_internal" && (kind != "group" || external)) ||
			(tab == "group_external" && !external) || (tab == "new" && kind != "new") || (tab == "internal" && kind != "internal") {
			continue
		}
		if (account != "" && deref(t.Account) != account) || (team != "" && t.AccountTeam != team) {
			continue
		}
		// a sales or Penagihan user reads only the conversations of their own numbers
		if !viewer.sees(t.SalesID, t.Account) {
			continue
		}
		v := ThreadView{ID: t.ID, Kind: kind, Title: deref(t.Title), Subtitle: deref(t.Subtitle), DealerID: deref(t.DealerSlug), DealerName: deref(t.DealerName),
			Sales: deref(t.SalesName), LastMessageAt: t.LastMessageAt, Unread: t.Unread, Mentioned: t.Mentioned, LastBody: llm.MaskCredentials(wa.NamedMentions(deref(t.LastBody), t.LastMentions)), Tag: t.Tag, GroupKind: deref(t.GroupKind),
			Account: deref(t.Account), AccountLabel: t.AccountLabel, AccountTeam: t.AccountTeam, ContactType: t.ContactType, LastDirection: deref(t.LastDirection), WaitingSince: timeOrNil(t.WaitingSince)}
		if kind != "group" {
			v.PhoneName = deref(t.PhoneName)
		}
		conv := convKey(t.Kind, t.WaJid, t.ID)
		v.Labels = labels[conv]
		if at, ok := pins[conv]; ok {
			v.PinnedAt = &at
		}
		if t.LastFrom != nil && kind == "group" {
			v.LastFrom = *t.LastFrom
		}
		if match != nil && !match(v.Title+" "+v.Subtitle+" "+v.LastBody+" "+deref(t.PhoneName), wa.NumberOfJID(deref(t.WaJid))) {
			continue
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

// setThreadAlias names the number of a one-to-one conversation as the team knows it ("Pak Yohanes"), shown with the
// master name and found by search; it applies to every conversation with that number.
func (s *Server) setThreadAlias(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	name := strings.Join(strings.Fields(in.Name), " ")
	if len([]rune(name)) > 80 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nama terlalu panjang (maks. 80 huruf)")
		return
	}
	number := wa.NumberOfJID(deref(t.WaJid))
	if deref(t.Kind) == "group" || number == "" {
		httpx.Fail(w, http.StatusConflict, "not_a_contact", "Nama panggilan hanya untuk chat pribadi")
		return
	}
	n, err := s.st.Q.SetNumberAlias(r.Context(), gen.SetNumberAliasParams{Name: name, Number: number})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	// the name also goes to the WhatsApp contacts of the number holding this chat, so it shows on that phone (Sam 17
	// Oct); best effort: a phone offline or a transport without it keeps the name in GSI Orbit only
	synced, note := false, ""
	if name != "" && deref(t.Account) != "" {
		if cs := s.contactSaver(); cs == nil {
			note = "Nama tersimpan di GSI Orbit; transport WhatsApp ini tidak bisa menyimpan kontak ke HP"
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			err := cs.SaveContact(ctx, deref(t.Account), number, name)
			cancel()
			if err != nil {
				note = "Nama tersimpan di GSI Orbit; belum tersimpan ke kontak HP (" + err.Error() + ")"
			} else {
				synced = true
			}
		}
	}
	s.audit(r, "chat.alias", "chat_thread", &t.ID, map[string]any{"number": wa.MaskNumber(number), "name": name, "threads": n, "phone_synced": synced})
	httpx.JSON(w, http.StatusOK, map[string]any{"name": name, "threads": n, "phone_synced": synced, "phone_note": note})
}

// threadMatcher matches a search against a conversation's names and text, forgiving Indonesian spellings of one name
// (Yohanes / Johannes, Yusuf / Joesoef), and against its number ("0812…", "62812…", the last digits). nil: no search.
func threadMatcher(term string) func(text, number string) bool {
	t := searchKey(dropHonorific(term))
	if t == "" {
		return nil
	}
	digits := wa.Digits(term)
	if strings.HasPrefix(digits, "0") {
		digits = "62" + digits[1:]
	}
	return func(text, number string) bool {
		if strings.Contains(searchKey(text), t) {
			return true
		}
		return len(digits) >= 4 && number != "" && strings.Contains(number, digits)
	}
}

// honorifics are how people are addressed, not part of the name the master or the chat title holds ("Pak Yohanes" is
// "JOHANNES YUDHAYANTO"): a search drops them in front, unless that is all it is.
var honorifics = map[string]bool{"pak": true, "bapak": true, "bp": true, "bu": true, "ibu": true, "mas": true, "mbak": true, "mba": true, "kak": true, "ko": true, "koh": true, "ci": true, "cik": true, "bang": true, "om": true, "tante": true, "sdr": true}

func dropHonorific(term string) string {
	f := strings.Fields(term)
	for len(f) > 1 && honorifics[strings.ToLower(strings.TrimSuffix(f[0], "."))] {
		f = f[1:]
	}
	return strings.Join(f, " ")
}

// searchKey folds spelling variants: case, "oe" → "u", "j" → "y", doubled letters (the same rules as web inbox.ts).
func searchKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "oe", "u")
	s = strings.ReplaceAll(s, "j", "y")
	var b strings.Builder
	var prev rune
	for _, r := range s {
		if r == prev && unicode.IsLetter(r) {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
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
	u, _ := CurrentUser(r.Context())
	v, err := s.chatViewerOf(r.Context(), u)
	if err == nil && v.lockedOut(t.Account) {
		httpx.Fail(w, http.StatusLocked, "number_logged_out", "WhatsApp nomor ini keluar dari perangkat tertaut — tautkan ulang untuk membuka riwayat chat")
		return gen.GetThreadRow{}, false
	}
	if err != nil || !v.sees(t.SalesID, t.Account) {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Percakapan tidak ditemukan") // another number's conversation
		return gen.GetThreadRow{}, false
	}
	return t, true
}

// MessageView is one chat bubble.
type MessageView struct {
	ID        uuid.UUID `json:"id"`
	Direction string    `json:"direction"`
	FromName  string    `json:"from_name"`
	// SentBy: the person who sent it from GSI Orbit; FromPhone: sent straight from the number's phone (WhatsApp).
	// Only for those who see every chat (admin, manager — Sam 19 Oct).
	SentBy    string `json:"sent_by,omitempty"`
	FromPhone bool   `json:"from_phone,omitempty"`
	// FromNumber: the sender of a group message (a click on the name opens the one-to-one chat with them)
	FromNumber string    `json:"from_number,omitempty"`
	Body       string    `json:"body"`
	SentAt     time.Time `json:"sent_at"`
	Status     string    `json:"status"`
	Internal   bool      `json:"internal"`
	// Holding: our message only says an answer will come — the response time keeps running (ADR 0034)
	Holding bool `json:"holding,omitempty"`
	// AutoReply: an automatic WhatsApp Business message (greeting, away) — neither an answer nor a request (ADR 0035)
	AutoReply  bool            `json:"auto_reply,omitempty"`
	closing    bool            // the customer's message asks nothing (a polite closing, emoji, sticker)
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
	// Suggest: the AI Trigger cards from the data for a customer message (ADR 0038, 0041); the sales decides
	Suggest []*replyrule.Suggestion `json:"suggest,omitempty"`
	// Lyra: the Hermes Agent's card — stock per city and DPP price of the products the message asks about (ADR 0070)
	Lyra   *hermes.Card `json:"lyra,omitempty"`
	Labels []uuid.UUID  `json:"labels,omitempty"`
	Pinned bool         `json:"pinned,omitempty"`
	// Starred: the person viewing starred it (Pesan berbintang — each person's own, Sam 22 Oct)
	Starred bool `json:"starred,omitempty"`
	// Reactions: the emoji under the bubble, like WhatsApp (who reacted with what, counted per emoji)
	Reactions []ReactionView `json:"reactions,omitempty"`
	// Mentions: the members the text tags — "@<token>" in Body is shown as "@<name>"
	Mentions json.RawMessage `json:"mentions,omitempty"`
	// Held: why a queued message has not gone out yet (the anti-ban guard's reason), shown instead of a bare
	// "mengirim…" (ADR 0064)
	Held       string     `json:"held,omitempty"`
	proposalID *uuid.UUID // the send's proposal (its outbox row)
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
	// Call, Video: a WhatsApp call record (kind "call", ADR 0062) — missed | answered | rejected
	Call  string `json:"call,omitempty"`
	Video bool   `json:"video,omitempty"`
	// Viewable: the file can be fetched from WhatsApp (messages stored before ADR 0029 have no reference)
	Viewable bool `json:"viewable"`
}

func mediaView(raw json.RawMessage) *MediaView {
	var m wa.Media
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil || m.Kind == "" {
		return nil
	}
	return &MediaView{Kind: m.Kind, Mime: m.Mime, FileName: m.FileName, Size: m.Size, Seconds: m.Seconds, PTT: m.PTT, Call: m.Call, Video: m.Video, Viewable: m.Viewable()}
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
		q := &MsgQuote{ID: &id, Direction: dir, FromName: from, Body: llm.MaskCredentials(body)}
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
	return id != nil && *id != "" && !strings.HasPrefix(*id, "outbox:") && !strings.HasPrefix(*id, "local:") && !strings.HasPrefix(*id, "call:")
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
	// The first page is the last month (CHAT_OPEN_DAYS, within CHAT_WINDOW_DAYS), not everything stored.
	// Older messages stay stored and come in pages of olderPageSize when the user scrolls up.
	before, lim, since := s.clock.Now().Add(24*365*time.Hour), int32(threadPageSize), s.chatOpenSince()
	if b := r.URL.Query().Get("before"); b != "" {
		if x, err := time.Parse(time.RFC3339, b); err == nil {
			before, lim, since = x, olderPageSize, time.Time{}
		}
	}
	if f := r.URL.Query().Get("from"); f != "" {
		// a jump to a search result: every message from it up to `before`, so the chat stays continuous
		if x, err := time.Parse(time.RFC3339Nano, f); err == nil {
			lim, since = jumpPageSize, x
		}
	}
	msgs, err := s.st.Q.ListThreadMessages(r.Context(), gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: before, Since: since, Lim: lim})
	if err == nil && len(msgs) == 0 && !since.IsZero() {
		// nothing in the window (a chat opened from a link): its last messages, not an empty page
		since, lim = time.Time{}, quietChatPage
		msgs, err = s.st.Q.ListThreadMessages(r.Context(), gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: before, Lim: lim})
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	hasMore := len(msgs) == int(lim)
	if !hasMore && !since.IsZero() && len(msgs) > 0 {
		// the window ended the page: say whether anything older is stored, so scrolling up can load it
		older, err := s.st.Q.ListThreadMessages(r.Context(), gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: msgs[0].SentAt, Lim: 1})
		hasMore = err == nil && len(older) > 0
	}
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
	var senderNames map[string]string
	if deref(t.Kind) == "group" {
		senderNames = s.rosterNames(r.Context(), wa.Digits(deref(t.SalesWa)), deref(t.WaJid))
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
		v := MessageView{ID: m.ID, Direction: deref(m.Direction), FromName: deref(m.FromName),
			SentBy: deref(m.SentByName), FromPhone: deref(m.Direction) == "out" && m.ProposalID == nil && !m.Internal, Body: llm.MaskCredentials(deref(m.Body)), SentAt: m.SentAt, Status: m.Status, Internal: m.Internal, Holding: deref(m.Holding), AutoReply: deref(m.AutoReply), closing: deref(m.Closing), Annotation: m.Annotation, SignalID: m.SignalID,
			Media: mediaView(m.Media), EditedAt: m.EditedAt, Mentions: m.Mentions, proposalID: m.ProposalID}
		if deref(t.Kind) == "group" && v.Direction == "in" {
			v.FromNumber = deref(m.FromNumber)
			if n := senderNames[wa.Digits(v.FromNumber)]; n != "" {
				v.FromName = n // the contact saved on this number's phone, else their WhatsApp profile name
			}
		}
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
	if err := s.attachSentFiles(r.Context(), out); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.attachHeld(r.Context(), out); err != nil {
		s.log.Warn("held sends", "err", err) // the reason is a hint; the chat loads without it
	}
	// pins and labels of the conversation (a group: the same for every GSI number in it)
	conv := convKey(t.Kind, t.WaJid, t.ID)
	pins, err := s.pinsOf(r, conv)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	convLabels, err := s.st.Q.ConvLabels(r.Context(), conv)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.attachLabelsPins(r.Context(), out, pins); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.attachStars(r, out); err != nil {
		s.log.Warn("message stars", "err", err) // a star never stops the chat from loading
	}
	if err := s.attachReactions(r.Context(), out, deref(t.SalesWa)); err != nil {
		s.log.Warn("message reactions", "err", err) // nor a reaction
	}
	if err := s.attachSuggestions(r.Context(), out, t.ThreadBranch, t.DealerID, now); err != nil {
		s.log.Warn("reply suggestions", "err", err) // a suggestion never stops the chat from loading
	}
	if err := s.attachLyra(r.Context(), t, out, now); err != nil {
		s.log.Warn("lyra cards", "err", err) // nor does Lyra's card
	}
	var lastAnalysis *time.Time
	if k := deref(t.Kind); k == "dealer" || k == "new" || (k == "group" && deref(t.GroupKind) == "external") {
		conv := t.ID // a customer group is analysed under one of its threads (ConversationOf)
		if c, err := s.st.Q.ConversationOf(r.Context(), t.ID); err == nil {
			conv = c
		}
		var err error
		if lastAnalysis, err = s.attachMsgAnalysis(r.Context(), conv, out, r.URL.Query().Get("before") == ""); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	replyClosed, lastIn := s.replyClosed(r.Context(), t)
	httpx.JSON(w, http.StatusOK, map[string]any{"thread": map[string]any{
		"id": t.ID, "kind": deref(t.Kind), "title": deref(t.Title), "subtitle": deref(t.Subtitle), "phone_name": deref(t.PhoneName), "dealer_id": deref(t.DealerSlug),
		"sales": deref(t.SalesName), "sales_wa": deref(t.SalesWa), "account_label": t.AccountLabel, "sender_name": t.SenderName, "account_masked": wa.MaskNumber(deref(t.SalesWa)), "suggestions": t.Suggestions, "tag": t.Tag, "unread": t.Unread, "mentioned": t.Mentioned,
		"phone": phoneOfJID(t.WaJid), "account": deref(t.Account), "dealer_code": deref(t.DealerCode), "dealer_city": deref(t.DealerCity), "waiting_since": timeOrNil(t.WaitingSince),
		"analyzed_at": lastAnalysis, "account_team": t.AccountTeam, "branch": t.ThreadBranch, "contact_type": t.ContactType, "group_id": t.GroupID, "group_kind": deref(t.GroupKind),
		"labels": convLabels, "admin_only": deref(t.Kind) == "group" && s.groupAdminOnly(threadAccount(t), deref(t.WaJid)),
		"reply_closed": replyClosed, "last_in_at": lastIn,
	}, "messages": out, "pins": pins, "stock_as_of": stockAsOf(r.Context(), s, out), "has_more": hasMore, "history_from": historyFrom, "history_days": keepDays})
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
			// a GSI employee's message in a customer group is our side
			th.Messages = append(th.Messages, waanalysis.Message{ID: m.ID, In: m.Direction == "in" && !m.Internal, Text: m.Body, At: m.SentAt,
				Holding: m.Holding || m.AutoReply, Closing: m.closing || m.AutoReply})
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
	// a person opened the chat: the customer sees it read (blue ticks) now, like WhatsApp Web — not only when a reply
	// goes out through the send queue (ADR 0069). Internal DMs are not read; a failure only leaves the ticks for later.
	if b := s.bridge(); b != nil && t.Account != nil && t.WaJid != nil && deref(t.Kind) != "internal" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		if err := b.MarkRead(ctx, *t.Account, *t.WaJid); err != nil {
			s.log.Warn("chat read receipt", "thread", t.ID, "err", err)
		}
		cancel()
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
	u, ok := s.personOf(w, r)
	if !ok {
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
	if deref(t.Kind) == "group" && s.groupAdminOnly(threadAccount(t), deref(t.WaJid)) {
		httpx.Fail(w, http.StatusConflict, "admin_only", "Hanya admin yang dapat mengirim pesan ke grup ini — nomor ini bukan admin")
		return
	}
	if closed, _ := s.replyClosed(r.Context(), t); closed {
		httpx.Fail(w, http.StatusConflict, "reply_window_closed", "Pelanggan belum mengirim pesan dalam 24 jam terakhir — agar nomor tidak diblokir WhatsApp, mulai chat dari WhatsApp Web atau HP")
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
	var forwarded *gen.GetThreadMessageRow
	if in.Forward != nil {
		m, data, code, why := s.forwardSource(r, *in.Forward)
		if why != "" {
			httpx.Fail(w, code, "not_forwardable", why)
			return
		}
		forwarded, file = &m, data
		in.Body, in.ReplyTo, in.Content = forwardText(m), "", nil
		var md wa.Media
		if data != nil && json.Unmarshal(m.Media, &md) == nil {
			in.Content = &outbox.OutContent{Kind: md.Kind, Mime: md.Mime, FileName: md.FileName, Size: int64(len(data)), PTT: md.PTT, Seconds: md.Seconds}
			if in.Content.Kind == "gif" {
				in.Content.Kind = "video"
			}
		}
		if in.Content == nil && strings.TrimSpace(in.Body) == "" {
			httpx.Fail(w, http.StatusConflict, "not_forwardable", "Pesan ini tidak berisi teks atau file yang bisa diteruskan")
			return
		}
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
		switch c.Kind {
		case "poll":
			body = label + " " + c.Poll.Name + "\n• " + strings.Join(c.Poll.Options, "\n• ")
		case "contact":
			body = label + " " + c.Contact.Name + " · +" + c.Contact.Phone
		default:
			body = strings.TrimSpace(label + " " + caption)
		}
		media, _ = json.Marshal(wa.Media{Kind: c.Kind, Mime: c.Mime, FileName: c.FileName, Size: c.Size, Seconds: c.Seconds, PTT: c.PTT})
	}
	mentions, tagged, err := s.sendMentions(r.Context(), t, caption)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var res map[string]any
	err = s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
		title := "Balasan ke " + deref(t.Title)
		if forwarded != nil {
			title = "Teruskan ke " + deref(t.Title)
		}
		p, err := s.humanDecision(r.Context(), q, t, u, title, body, now)
		if err != nil {
			return err
		}
		dir, from := "out", *t.SalesWa
		name := t.SenderName // the holder of the number it goes out from, not the thread's sales (19 Oct bug)
		pendingID := "outbox:" + p.ID.String()
		var quoted *string
		if quote != nil {
			quoted = &quote.ID
		}
		mid, err := q.InsertChatMessage(r.Context(), gen.InsertChatMessageParams{ThreadID: &t.ID, WaMsgID: &pendingID, Direction: &dir, FromNumber: &from, FromName: &name, Body: &body, Media: media, SentAt: now, Status: "pending", ProposalID: &p.ID, QuotedWaID: quoted, Mentions: mentions})
		if err != nil {
			return err
		}
		// "dikirim oleh <nama>": the person who pressed Kirim / Teruskan (Sam 19 Oct)
		if err := q.SetMessageSender(r.Context(), gen.SetMessageSenderParams{ID: mid, SentAt: now, SentBy: u.ID, SentByName: deref(u.Name)}); err != nil {
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
		payload, _ := json.Marshal(outbox.WAPayload{From: from, To: *t.WaJid, Text: text, Kind: "reply", MessageID: mid.String(), ThreadID: t.ID.String(), Quote: quote, Content: in.Content, Mentions: tagged})
		to := *t.WaJid
		ob, err := q.InsertOutbox(r.Context(), gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: &to, Payload: payload})
		if err != nil {
			return err
		}
		if err := q.TouchThread(r.Context(), gen.TouchThreadParams{ID: t.ID, LastMessageAt: &now}); err != nil {
			return err
		}
		actor, kind, action, entity := deref(u.Email), "user", "chat.reply", "proposal"
		audit := map[string]any{"thread_id": t.ID, "outbox_id": ob.ID, "body": body}
		if forwarded != nil {
			action = "chat.forward"
			audit["forwarded_from"] = map[string]any{"thread_id": forwarded.ThreadID, "message_id": forwarded.ID}
		}
		after, _ := json.Marshal(audit)
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

// forwardSource is the bubble a forward sends on, with its file when it has one (fetched from WhatsApp as Chat
// shows it). The person must see that conversation too. code and why are the refusal for the person.
func (s *Server) forwardSource(r *http.Request, f chatForward) (m gen.GetThreadMessageRow, file []byte, code int, why string) {
	tid, err1 := uuid.Parse(f.ThreadID)
	mid, err2 := uuid.Parse(f.MessageID)
	if err1 != nil || err2 != nil {
		return m, nil, http.StatusNotFound, "Pesan yang diteruskan tidak ditemukan"
	}
	src, err := s.st.Q.GetThread(r.Context(), tid)
	if err != nil {
		return m, nil, http.StatusNotFound, "Pesan yang diteruskan tidak ditemukan"
	}
	u, _ := CurrentUser(r.Context())
	if v, err := s.chatViewerOf(r.Context(), u); err != nil || !v.sees(src.SalesID, src.Account) {
		return m, nil, http.StatusNotFound, "Pesan yang diteruskan tidak ditemukan" // another number's conversation
	}
	m, err = s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ThreadID: src.ID, ID: mid})
	if err != nil {
		return m, nil, http.StatusNotFound, "Pesan yang diteruskan tidak ditemukan"
	}
	if m.DeletedAt != nil {
		return m, nil, http.StatusConflict, "Pesan ini sudah dihapus"
	}
	var md wa.Media
	if len(m.Media) == 0 || json.Unmarshal(m.Media, &md) != nil || md.Kind == "" {
		return m, nil, 0, ""
	}
	if md.Kind == "poll" || md.Kind == "location" || md.Kind == "contact" || md.Kind == "sticker" {
		return m, nil, http.StatusConflict, "Polling, lokasi, kontak dan stiker belum bisa diteruskan dari GSI Orbit — teruskan dari HP"
	}
	f2 := s.mediaFetcher()
	if !md.Viewable() || f2 == nil {
		return m, nil, http.StatusConflict, "File pesan ini tidak tersimpan — teruskan dari HP"
	}
	body, err := f2.Media(r.Context(), deref(m.HolderAccount), md)
	switch {
	case errors.Is(err, wa.ErrMediaGone):
		return m, nil, http.StatusGone, "File sudah tidak tersedia di WhatsApp"
	case errors.Is(err, wa.ErrMediaUnavailable):
		return m, nil, http.StatusServiceUnavailable, "Nomor pemegang file belum terhubung atau sedang sibuk — coba lagi sebentar"
	case err != nil:
		s.log.Warn("chat forward media", "err", err, "message", mid)
		return m, nil, http.StatusBadGateway, "Gagal mengambil file dari WhatsApp"
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxChatFile+1))
	if err != nil || len(data) == 0 {
		return m, nil, http.StatusBadGateway, "Gagal mengambil file dari WhatsApp"
	}
	if len(data) > maxChatFile {
		return m, nil, http.StatusRequestEntityTooLarge, "File lebih dari 25 MB — teruskan dari HP"
	}
	return m, data, 0, ""
}

// forwardText is the text a forward carries: the bubble's caption without its media label ("[gambar] …").
func forwardText(m gen.GetThreadMessageRow) string {
	body := deref(m.Body)
	var md wa.Media
	if len(m.Media) > 0 && json.Unmarshal(m.Media, &md) == nil && md.Kind != "" {
		if label := wa.MediaLabel(md.Kind, map[bool]string{true: md.FileName}[md.Kind == "document"]); strings.HasPrefix(body, label) {
			body = strings.TrimPrefix(body, label)
		} else if i := strings.Index(body, "]"); strings.HasPrefix(body, "[") && i > 0 {
			body = body[i+1:]
		}
	}
	return strings.TrimSpace(body)
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
	Contact *wa.ContactCard    `json:"contact"` // a WhatsApp contact card to send (name + number)
	Forward *chatForward       `json:"forward"` // a bubble of another conversation sent on to this one
	Content *outbox.OutContent `json:"-"`
}

// chatForward names the bubble to forward: its conversation and message.
type chatForward struct {
	ThreadID  string `json:"thread_id"`
	MessageID string `json:"message_id"`
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
	if in.Contact != nil {
		name := strings.Join(strings.Fields(in.Contact.Name), " ")
		phone := wa.Digits(in.Contact.Phone)
		if strings.HasPrefix(phone, "0") {
			phone = "62" + phone[1:]
		}
		if name == "" || len([]rune(name)) > 80 || len(phone) < 8 || len(phone) > 15 {
			return in, nil, "Kontak butuh nama (maks. 80 huruf) dan nomor WhatsApp 8–15 digit"
		}
		in.Content = &outbox.OutContent{Kind: "contact", Contact: &wa.ContactCard{Name: name, Phone: phone}}
		return in, nil, ""
	}
	if in.Forward != nil {
		return in, nil, "" // the text and file come from the forwarded bubble
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
	if b := s.bridge(); b != nil {
		return b
	}
	return nil
}

// groupRoster is how the API reads a group's members (the Baileys bridge; tests set their own; nil without it).
func (s *Server) groupRoster() wa.GroupRoster {
	if s.roster != nil {
		return s.roster
	}
	if b := s.bridge(); b != nil {
		return b
	}
	return nil
}

// contactSaver saves names to a number's WhatsApp contacts: the test double, else the Baileys bridge.
func (s *Server) contactSaver() wa.ContactSaver {
	if s.contacts != nil {
		return s.contacts
	}
	if b := s.bridge(); b != nil {
		return b
	}
	return nil
}

// bridge is the Baileys bridge as the API reaches it, nil when another transport runs.
func (s *Server) bridge() *wa.Baileys {
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
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "no_media", "Media pesan ini tidak tersimpan — buka di WhatsApp ponsel")
		return
	}
	// a file sent from Chat: the copy kept with it, as it was sent (also while it waits in the send queue)
	if f, err := s.st.Q.OutgoingFileOf(r.Context(), m.ID); err == nil {
		writeMedia(w, f.Mime, f.FileName, bytes.NewReader(f.Data), s.log)
		return
	}
	var md wa.Media
	if json.Unmarshal(m.Media, &md) != nil || !md.Viewable() {
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
	writeMedia(w, md.Mime, md.FileName, body, s.log)
}

// writeMedia streams a chat file: images, video and audio inline, anything else as a download; never run.
func writeMedia(w http.ResponseWriter, mimeType, fileName string, body io.Reader, log *slog.Logger) {
	ct, _, _ := strings.Cut(mimeType, ";")
	ct = strings.ToLower(strings.TrimSpace(ct))
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("Cache-Control", "private, max-age=604800, immutable")
	if inlineMedia[ct] {
		h.Set("Content-Type", ct)
		h.Set("Content-Disposition", "inline")
	} else {
		name := fileName
		if name == "" {
			name = "lampiran"
		}
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	if _, err := io.Copy(w, body); err != nil {
		log.Debug("chat media copy", "err", err)
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

// setContactType marks the number of a one-to-one chat (ADR 0035): customer (default), supplier — kept, but not a
// lead and not in the reply queue — or personal: the chat leaves the CRM on every GSI number (messages and the signals
// made of them) and later messages are not kept.
func (s *Server) setContactType(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	phone := phoneOfJID(t.WaJid)
	if deref(t.Kind) == "group" || phone == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tipe kontak hanya untuk chat satu nomor")
		return
	}
	var in struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil || (in.Type != "customer" && in.Type != "supplier" && in.Type != "personal") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tipe: customer, supplier, atau personal")
		return
	}
	u, _ := CurrentUser(r.Context())
	purged := 0
	err := s.st.Tx(r.Context(), func(q *gen.Queries, _ pgx.Tx) error {
		if in.Type == "customer" {
			return q.ClearContactType(r.Context(), phone)
		}
		if err := q.SetContactType(r.Context(), gen.SetContactTypeParams{WaNumber: phone, Type: in.Type, SetBy: u.Email}); err != nil {
			return err
		}
		if in.Type == "personal" {
			n, err := q.PurgePersonalChat(r.Context(), deref(t.WaJid))
			purged = int(n)
			return err
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if in.Type == "personal" {
		s.forgetInBackups(deref(t.WaJid)) // Cadangan chat keeps no copy of a private chat either (ADR 0061)
	}
	s.audit(r, "chat.contact_type", "chat_thread", &t.ID, map[string]any{"number": wa.MaskNumber(phone), "type": in.Type, "threads_removed": purged})
	msg := map[string]string{"customer": "Ditandai pelanggan", "supplier": "Ditandai pemasok · tidak masuk antrean perlu dibalas",
		"personal": "Obrolan pribadi dihapus dari CRM · pesan berikutnya tidak disimpan"}[in.Type]
	httpx.JSON(w, http.StatusOK, map[string]any{"type": in.Type, "removed": purged > 0, "message": msg})
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
	// from Chat a sales or Penagihan user marks only a group one of their own numbers is in (a customer group is read
	// by Analisa WhatsApp)
	u, _ := CurrentUser(r.Context())
	if v, err := s.chatViewerOf(r.Context(), u); err != nil || !v.all {
		mine := false
		if err == nil {
			rows, qerr := s.st.Pool.Query(r.Context(), "select sales_id, account from chat_threads where group_id = $1", id)
			if qerr == nil {
				for rows.Next() {
					var sid *uuid.UUID
					var acc *string
					if rows.Scan(&sid, &acc) == nil && v.sees(sid, acc) {
						mine = true
					}
				}
				rows.Close()
			}
		}
		if !mine {
			httpx.Fail(w, http.StatusNotFound, "not_found", "Grup tidak ditemukan")
			return
		}
	}
	g, err := s.st.Q.UpdateWAGroup(r.Context(), gen.UpdateWAGroupParams{ID: id, Kind: &in.Kind, ReadEnabled: *in.ReadEnabled})
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Grup tidak ditemukan")
		return
	}
	sub := map[string]string{"internal": "Grup internal", "external": "Grup pelanggan"}[in.Kind]
	_, _ = s.st.Pool.Exec(r.Context(), "update chat_threads set subtitle = $2 where group_id = $1", id, sub)
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
		if waOwnOnly(u) && !mine {
			continue // a sales or Penagihan user sees and pairs only their own number
		}
		v := map[string]any{"wa_number": n.WaNumber, "masked": wa.MaskNumber(n.WaNumber), "sales": deref(n.SalesName), "branch": deref(n.SalesBranch), "transport": n.Transport,
			"state": n.State, "state_since": n.StateSince, "last_seen_at": n.LastSeenAt, "paired_at": n.PairedAt, "backfill_days": n.BackfillDays, "label": deref(n.Label), "sales_id": n.SalesID,
			"user_id": n.UserID, "user_name": deref(n.UserName), "user_email": deref(n.UserEmail), "mine": mine,
			"user_role": deref(n.UserRole), "user_branch": deref(n.UserBranch), "session_id": n.SessionID,
			"thread_count": n.ThreadCount, "unread_count": n.UnreadCount, "last_message_at": n.LastMessageAt, "team": n.Team}
		if (isAdmin(u) && !waOwnOnly(u)) || mine {
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

// attachHeld tells why a queued message waits: the guard's reason on its outbox row ("Batas 20 pesan/jam …").
func (s *Server) attachHeld(ctx context.Context, msgs []MessageView) error {
	var ids []uuid.UUID
	for _, m := range msgs {
		if m.Status == "pending" && m.Direction == "out" && m.proposalID != nil {
			ids = append(ids, *m.proposalID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.st.Q.HeldSendNotes(ctx, ids)
	if err != nil {
		return err
	}
	why := map[uuid.UUID]string{}
	for _, r := range rows {
		if r.ProposalID != nil && r.Error != nil {
			why[*r.ProposalID] = strings.TrimPrefix(*r.Error, "Penjaga anti-blokir: ")
		}
	}
	for i := range msgs {
		if p := msgs[i].proposalID; p != nil && msgs[i].Status == "pending" {
			msgs[i].Held = why[*p]
		}
	}
	return nil
}

// attachSentFiles: a file sent from Chat is shown from the copy kept with it — while it waits to be sent and after.
func (s *Server) attachSentFiles(ctx context.Context, msgs []MessageView) error {
	var ids []uuid.UUID
	for _, m := range msgs {
		if m.Media != nil && !m.Media.Viewable && m.Direction == "out" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	have, err := s.st.Q.MessagesWithOutgoingFile(ctx, ids)
	if err != nil {
		return err
	}
	kept := map[uuid.UUID]bool{}
	for _, id := range have {
		kept[id] = true
	}
	for i := range msgs {
		if msgs[i].Media != nil && kept[msgs[i].ID] {
			msgs[i].Media.Viewable = true
		}
	}
	return nil
}

// chatHistory is "Muat pesan lama dari HP" (Sam 22 Oct: chats are kept 90 days, older ones load again from the phone
// like WhatsApp Web scrolling up): the linked phone is asked for the 50 messages before the oldest one kept. They come
// back within seconds as history events and show in the chat; the nightly purge removes them again past retention.
func (s *Server) chatHistory(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	b := s.bridge()
	if b == nil || t.Account == nil || t.WaJid == nil {
		httpx.Fail(w, http.StatusConflict, "no_history", "Pesan lama hanya bisa dimuat dari HP untuk nomor yang ditautkan ke GSI Orbit")
		return
	}
	o, err := s.st.Q.OldestThreadMessage(r.Context(), t.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusConflict, "no_history", "Belum ada pesan WhatsApp di chat ini untuk dijadikan titik awal")
		return
	} else if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	anchor := wa.HistoryAnchor{ID: o.WaMsgID, FromMe: o.Direction == "out", At: o.SentAt}
	if err := b.FetchHistory(r.Context(), *t.Account, *t.WaJid, anchor, wa.HistoryBatch); err != nil {
		if errors.Is(err, wa.ErrHistoryUnavailable) {
			httpx.Fail(w, http.StatusTooManyRequests, "history_busy", "HP pemegang nomor belum bisa dimintai riwayat (belum terhubung atau terlalu cepat) — coba lagi sebentar")
			return
		}
		httpx.Fail(w, http.StatusBadGateway, "bridge", err.Error())
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{"requested": wa.HistoryBatch, "before": o.SentAt})
}
