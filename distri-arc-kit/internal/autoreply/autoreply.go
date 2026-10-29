// Package autoreply answers customers while their sales is on a break (ADR 0066). With "seperti sales" on, Lyra, the
// company's Hermes Agent, answers any message the way a sales would, from the facts below; a reply whose numbers are
// not in them is not sent (ADR 0078). Otherwise, or when that fails: a product question is read by Lyra
// and answered with the stock and DPP price from the system (ADR 0077); another question
// about a fact the system computes (stock, price, order status) gets the AI Trigger draft — numbers from the system,
// checked by CheckDraft — and anything else gets the template a person approved once. The approval of the setting is the recorded human
// decision (proposals.decided_by); every message still goes through the outbox and the anti-ban guard.
package autoreply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/hermes"
	"distri-arc/internal/jobs"
	"distri-arc/internal/outbox"
	"distri-arc/internal/replyai"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// PolicyKey is the policies row of the setting.
const PolicyKey = "chat.auto_reply"

// Agent names the automatic answers (proposals.agent); a person's answer is anything else.
const Agent = "Balas otomatis"

// Guards: a person who answered lately is at work; a message waiting longer is the sales' to answer.
const (
	HumanQuiet  = 10 * time.Minute
	MaxAge      = 15 * time.Minute
	MaxFactsPer = 5  // fact answers per conversation per break
	MaxLyraPer  = 10 // Lyra's answers per conversation per break: she keeps up the conversation
	// MaxSalesReply keeps Lyra's reply with its footer short enough to read as an automatic message (is_auto_reply ≤ 700)
	MaxSalesReply = 640
	// LyraWait is how long a product question waits for Lyra (one agent loop) before the other answers are tried.
	LyraWait = 4 * time.Minute
)

// Send window of the outbox for automatic messages (outbox.DefaultRules SendFrom–SendTo): a break outside it would
// be answered the next morning.
const (
	SendFrom = "08:00"
	SendTo   = "18:00"
)

// Facts are the AI Trigger actions an automatic answer may give.
var Facts = []string{replyrule.ActionStock, replyrule.ActionPrice, replyrule.ActionOrder, replyrule.ActionCredit}

// Window is one break: on Days (1 = Senin … 7 = Minggu), from Start to End ("12:00"), WIB.
type Window struct {
	Days  []int  `json:"days"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// Settings is the page Pengaturan → Balas otomatis.
type Settings struct {
	Enabled  bool     `json:"enabled"`
	Breaks   []Window `json:"breaks"`
	Numbers  []string `json:"numbers"` // the sales numbers it answers for; empty: every number
	Template string   `json:"template"`
	// Facts are the topics an automatic reply may give, by Lyra and by AI Trigger alike (ADR 0080): without "price"
	// no price reaches the customer, without "stock" no stock quantity
	Facts []string `json:"facts"`
	Lyra  bool     `json:"lyra"` // product questions answered by Lyra (Hermes Agent), stock and price from the system
	// SalesStyle: Lyra answers every message like a sales, from the facts the system computed (ADR 0078)
	SalesStyle bool `json:"sales_style"`
	// the person whose approval sends the messages, set on every save
	ApprovedBy      *uuid.UUID `json:"approved_by,omitempty"`
	ApprovedProfile *uuid.UUID `json:"approved_profile,omitempty"`
	ApprovedName    string     `json:"approved_name,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
}

// DefaultTemplate reads as an automatic message (chat_messages.auto_reply), so the chat stays "Belum dibalas".
const DefaultTemplate = "{salam} {nama}, terima kasih sudah menghubungi kami. Sales kami sedang istirahat dan akan membalas pesan Anda sekitar pukul {kembali} WIB. (Pesan otomatis)"

// Default is off, Senin–Sabtu 12.00–13.00, products answered by Lyra, stock / price / order by AI.
func Default() Settings {
	return Settings{Breaks: []Window{{Days: []int{1, 2, 3, 4, 5, 6}, Start: "12:00", End: "13:00"}}, Template: DefaultTemplate,
		Facts: []string{replyrule.ActionStock, replyrule.ActionPrice, replyrule.ActionOrder}, Lyra: true, SalesStyle: true}
}

// Load reads the setting; the default when none is saved.
func Load(ctx context.Context, q *gen.Queries) Settings {
	if p, err := q.GetPolicy(ctx, PolicyKey); err == nil {
		s := Settings{}
		if json.Unmarshal(p.Value, &s) == nil {
			return s
		}
	}
	return Default()
}

var reHM = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// Validate checks the setting as the page saves it. The template must read as an automatic message.
func Validate(ctx context.Context, q *gen.Queries, s Settings) error {
	if len(s.Breaks) == 0 || len(s.Breaks) > 7 {
		return errors.New("isi 1–7 jam istirahat")
	}
	for _, w := range s.Breaks {
		if !reHM.MatchString(w.Start) || !reHM.MatchString(w.End) || w.Start >= w.End {
			return errors.New("jam istirahat: isi mulai dan selesai (JJ:MM), selesai setelah mulai")
		}
		if w.Start < SendFrom || w.End > SendTo {
			return fmt.Errorf("jam istirahat harus di antara %s–%s WIB (jendela kirim WhatsApp otomatis)", SendFrom, SendTo)
		}
		if len(w.Days) == 0 {
			return errors.New("pilih hari untuk setiap jam istirahat")
		}
		for _, d := range w.Days {
			if d < 1 || d > 7 {
				return errors.New("hari tidak dikenal")
			}
		}
	}
	for _, f := range s.Facts {
		if !slices.Contains(Facts, f) {
			return errors.New("jenis jawaban AI tidak dikenal")
		}
	}
	t := strings.TrimSpace(s.Template)
	if n := len([]rune(t)); n < 20 || n > 500 {
		return errors.New("pesan template 20–500 karakter")
	}
	auto, err := q.IsAutoReplyText(ctx, Fill(t, "Pak Budi", "13.00", "siang"))
	if err != nil {
		return err
	}
	if !auto {
		return errors.New(`template harus terbaca sebagai pesan otomatis — sertakan "terima kasih sudah menghubungi" atau "(Pesan otomatis)" supaya chat tetap "Belum dibalas" untuk sales`)
	}
	return nil
}

// Fill writes the template for one conversation.
func Fill(t, name, back, partOfDay string) string {
	salam := "Selamat " + partOfDay
	return strings.TrimSpace(strings.NewReplacer("{salam}", salam, "{nama}", name, "{kembali}", back).Replace(t))
}

// Active is the break now is in for a sales number: its start and end, ok false outside a break.
func (s Settings) Active(now time.Time, account string) (start, end time.Time, ok bool) {
	if !s.Enabled || s.ApprovedProfile == nil || (len(s.Numbers) > 0 && !slices.Contains(s.Numbers, account)) {
		return start, end, false
	}
	local := now.In(clock.WIB)
	day := int(local.Weekday())
	if day == 0 {
		day = 7
	}
	hm := local.Format("15:04")
	for _, w := range s.Breaks {
		if slices.Contains(w.Days, day) && hm >= w.Start && hm < w.End {
			return at(local, w.Start), at(local, w.End), true
		}
	}
	return start, end, false
}

func at(day time.Time, hm string) time.Time {
	t, _ := time.ParseInLocation("15:04", hm, clock.WIB)
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, clock.WIB)
}

func partOfDay(t time.Time) string {
	switch h := t.In(clock.WIB).Hour(); {
	case h < 11:
		return "pagi"
	case h < 15:
		return "siang"
	case h < 18:
		return "sore"
	}
	return "malam"
}

// Inserter enqueues the outbox job in the transaction that writes the message (river client).
type Inserter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Service answers one customer message.
type Service struct {
	St    *store.Store
	Reply *replyai.Service
	Lyra  *hermes.Service // nil: product questions go to AI Trigger
	Clock clock.Clock
	Jobs  Inserter
	Log   *slog.Logger
}

// Wants reports whether a break is on now for some number (the ingest hook enqueues only then; Handle checks the
// message's number).
func (s *Service) Wants(ctx context.Context) bool {
	set := Load(ctx, s.St.Q)
	set.Numbers = nil
	_, _, ok := set.Active(s.Clock.Now(), "")
	return ok
}

// Outcome is what Handle did with a message.
type Outcome string

const (
	Skipped  Outcome = ""
	Fact     Outcome = "fact"
	Template Outcome = "template"
	Lyra     Outcome = "lyra"
)

// Handle answers a customer message when every guard allows it; re-running it never sends twice.
func (s *Service) Handle(ctx context.Context, messageID uuid.UUID) (Outcome, error) {
	now := s.Clock.Now()
	set := Load(ctx, s.St.Q)
	m, err := s.St.Q.ReplyTriggerMessage(ctx, messageID)
	if err != nil {
		return Skipped, err
	}
	if m.Direction != "in" || m.Internal || m.DeletedAt != nil || m.ThreadID == nil || strings.TrimSpace(m.Body) == "" || now.Sub(m.SentAt) > MaxAge {
		return Skipped, nil
	}
	t, err := s.St.Q.GetThread(ctx, *m.ThreadID)
	if err != nil {
		return Skipped, err
	}
	// one-to-one customer chats only: no groups, internal, personal or supplier chats
	kind := ""
	if t.Kind != nil {
		kind = *t.Kind
	}
	if (kind != "dealer" && kind != "new") || t.WaJid == nil || !strings.HasSuffix(*t.WaJid, "@s.whatsapp.net") || t.SalesWa == nil || t.Account == nil {
		return Skipped, nil
	}
	if ct, err := s.St.Q.ContactTypeOf(ctx, strings.TrimSuffix(*t.WaJid, "@s.whatsapp.net")); err != nil || ct != "" {
		return Skipped, err
	}
	start, end, ok := set.Active(now, *t.Account)
	if !ok {
		return Skipped, nil
	}
	if busy, err := s.St.Q.HumanRepliedSince(ctx, gen.HumanRepliedSinceParams{ThreadID: &t.ID, SentAt: now.Add(-HumanQuiet)}); err != nil || busy {
		return Skipped, err
	}
	// the facts the system computes for this message: Lyra's product card, the AI Trigger answers
	quote, facts := "", ""
	if set.Lyra && s.Lyra != nil && s.Reply != nil {
		if quote, err = s.lyraAnswer(ctx, m, t, set.Facts); err != nil && s.Log != nil {
			s.Log.Warn("auto reply: no Lyra answer", "message", messageID, "err", err)
		}
	}
	if len(set.Facts) > 0 && s.Reply != nil && (quote == "" || set.SalesStyle) {
		if facts, err = s.factAnswer(ctx, m, set.Facts); err != nil && s.Log != nil {
			s.Log.Warn("auto reply: no fact answer", "message", messageID, "err", err)
		}
	}
	answer, out := "", Template
	if set.SalesStyle && s.Lyra != nil {
		if answer, err = s.salesAnswer(ctx, m, t, []string{quote, facts}, set.Facts, end, now); err != nil && s.Log != nil {
			s.Log.Warn("auto reply: no sales-style answer", "message", messageID, "err", err)
		}
		if answer != "" {
			out = Lyra
		}
	}
	switch {
	case answer != "":
	case quote != "":
		answer, out = quote, Lyra
	case facts != "":
		answer, out = facts, Fact
	default:
		answer = Fill(set.Template, nameOf(t), end.Format("15.04"), partOfDay(now))
	}
	if out == Lyra { // Lyra took a while: a person who answered meanwhile is at work
		if busy, err := s.St.Q.HumanRepliedSince(ctx, gen.HumanRepliedSinceParams{ThreadID: &t.ID, SentAt: now.Add(-HumanQuiet)}); err != nil || busy {
			return Skipped, err
		}
	}
	sent := false
	err = s.St.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		// one conversation at a time: two messages in a row must not both get the template
		if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock(hashtext($1))", "auto_reply:"+t.ID.String()); err != nil {
			return err
		}
		limit := map[Outcome]int64{Template: 1, Fact: MaxFactsPer, Lyra: MaxLyraPer}[out]
		n, err := q.CountAutoReplies(ctx, gen.CountAutoRepliesParams{ThreadID: t.ID, BreakStart: start, Kind: string(out)})
		if err != nil || n >= limit {
			return err
		}
		if claimed, err := q.ClaimAutoReply(ctx, gen.ClaimAutoReplyParams{MessageID: m.ID, ThreadID: t.ID, Kind: string(out), BreakStart: start}); err != nil || claimed == 0 {
			return err
		}
		pid, rid, err := s.queue(ctx, q, tx, t, set, out, answer, now)
		if err != nil {
			return err
		}
		sent = true
		return q.SetAutoReplySent(ctx, gen.SetAutoReplySentParams{MessageID: m.ID, ProposalID: &pid, ReplyID: &rid})
	})
	if err != nil || !sent {
		return Skipped, err
	}
	return out, nil
}

// lyraAnswer is Lyra's reading of a product question as the message the sales would send from its card: the products
// Lyra named, with the stock per city and the DPP price at the dealer's tier from the system. "" when the message asks
// no product, Lyra found none or is not connected.
func (s *Service) lyraAnswer(ctx context.Context, m gen.ReplyTriggerMessageRow, t gen.GetThreadRow, topics []string) (string, error) {
	stockOK, priceOK := slices.Contains(topics, replyrule.ActionStock), slices.Contains(topics, replyrule.ActionPrice)
	if !stockOK && !priceOK {
		return "", nil
	}
	rules, err := s.Reply.Rules(ctx)
	if err != nil {
		return "", err
	}
	q := hermes.Question{MessageID: m.ID, ThreadID: t.ID, Text: hermes.Caption(m.Body), SentAt: m.SentAt}
	if !hermes.Asks(q, rules) {
		return "", nil
	}
	wait, cancel := context.WithTimeout(ctx, LyraWait)
	defer cancel()
	r, err := s.Lyra.ReadNow(wait, q, nil)
	if errors.Is(err, hermes.ErrNotConnected) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if r.Status != "done" || r.Demo {
		return "", nil
	}
	rows, err := s.St.Q.ListStockItems(ctx)
	if err != nil {
		return "", err
	}
	stock := make([]domain.StockItem, 0, len(rows))
	for _, x := range rows {
		stock = append(stock, domain.StockItem{Branch: x.Branch, SKU: x.Sku, Name: x.Name, Qty: int(x.Qty)})
	}
	if len(r.Items) == 0 { // a brand or kind without a type ("cek stok cctv hilook semarang"): the products in stock
		for _, sku := range Browse(q.Text, stock) {
			r.Items = append(r.Items, hermes.ProductItem{SKU: sku})
		}
	}
	if len(r.Items) == 0 {
		return "", nil
	}
	products, err := s.Reply.Products(ctx)
	if err != nil {
		return "", err
	}
	skus := make([]string, len(r.Items))
	for i, it := range r.Items {
		skus[i] = it.SKU
	}
	wh, err := s.Lyra.Warehouses(ctx, skus)
	if err != nil {
		return "", err
	}
	tier := ""
	if t.DealerID != nil {
		_ = s.St.Pool.QueryRow(ctx, "select coalesce(tier, '') from dealers where id = $1", *t.DealerID).Scan(&tier)
	}
	return hermes.QuoteTextOf(hermes.BuildCard(r, stock, wh, products, tier), stockOK, priceOK), nil
}

// MaxBrowse is how many products a question naming only a brand or kind is answered with.
const MaxBrowse = 6

// Browse is the products a message names without a type: those of the best word match with stock, the city the
// message names first (most stock first), else any city. Nothing when no product word matched.
func Browse(text string, stock []domain.StockItem) []string {
	cands := replyrule.Candidates(text, nil, stock, 200)
	if len(cands) == 0 {
		return nil
	}
	words := " " + strings.ToLower(strings.Join(strings.Fields(reNonWord.ReplaceAllString(text, " ")), " ")) + " "
	city := ""
	inCity := map[string]int{} // SKU → stock in the named city
	for _, x := range stock {
		if b := strings.ToLower(strings.TrimSpace(x.Branch)); b != "" && strings.Contains(words, " "+b+" ") {
			city = b
		}
	}
	for _, x := range stock {
		if city != "" && strings.EqualFold(strings.TrimSpace(x.Branch), city) && x.Qty > 0 {
			inCity[x.SKU] += x.Qty
		}
	}
	top := cands[0].Score
	var best []replyrule.Candidate
	for _, c := range cands {
		if c.Score == top && c.Qty > 0 {
			best = append(best, c)
		}
	}
	if len(inCity) > 0 {
		var here []replyrule.Candidate
		for _, c := range best {
			if inCity[c.SKU] > 0 {
				here = append(here, c)
			}
		}
		if len(here) > 0 {
			best = here
			slices.SortStableFunc(best, func(a, b replyrule.Candidate) int { return inCity[b.SKU] - inCity[a.SKU] })
		}
	} else {
		slices.SortStableFunc(best, func(a, b replyrule.Candidate) int { return b.Qty - a.Qty })
	}
	out := make([]string, 0, MaxBrowse)
	for _, c := range best[:min(len(best), MaxBrowse)] {
		out = append(out, c.SKU)
	}
	return out
}

// salesAnswer is Lyra's reply to the message as a sales would write it, from the facts; "" when Lyra is not connected,
// the reply is empty or too long, or it has a number that is neither in the facts nor in the conversation.
func (s *Service) salesAnswer(ctx context.Context, m gen.ReplyTriggerMessageRow, t gen.GetThreadRow, facts, topics []string, end, now time.Time) (string, error) {
	in := hermes.SalesInput{Name: nameOf(t), Message: hermes.Caption(m.Body), Back: end.Format("15.04"), PartOfDay: partOfDay(now),
		NoPrice: !slices.Contains(topics, replyrule.ActionPrice), NoStock: !slices.Contains(topics, replyrule.ActionStock)}
	for _, f := range facts {
		if strings.TrimSpace(f) != "" {
			in.Facts = append(in.Facts, f)
		}
	}
	allowed := []string{in.Message, in.Back, strings.Join(in.Facts, "\n")}
	hist, err := s.St.Q.ReplyTriggerHistory(ctx, gen.ReplyTriggerHistoryParams{ThreadID: t.ID, Before: m.SentAt})
	if err != nil {
		return "", err
	}
	for i := min(len(hist), replyrule.HistoryLines) - 1; i >= 0; i-- { // oldest first
		line := hermes.ChatLine{In: hist[i].Direction == "in", Text: hermes.Caption(hist[i].Body)}
		in.History = append(in.History, line)
		allowed = append(allowed, line.Text)
	}
	wait, cancel := context.WithTimeout(ctx, LyraWait)
	defer cancel()
	a, err := s.Lyra.SalesReply(wait, in)
	if errors.Is(err, hermes.ErrNotConnected) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if n := len([]rune(a.Reply)); n == 0 || n > MaxSalesReply {
		return "", fmt.Errorf("balasan %s kosong atau terlalu panjang (%d karakter)", hermes.AgentName, n)
	}
	if err := replyai.CheckDraft("", strings.Join(allowed, "\n"), a.Reply); err != nil {
		return "", fmt.Errorf("balasan %s tidak dikirim: %w", hermes.AgentName, err)
	}
	if in.NoPrice && HasPrice(a.Reply) {
		return "", fmt.Errorf("balasan %s menyebut harga, padahal harga tidak boleh dijawab otomatis", hermes.AgentName)
	}
	if a.NeedSales && s.Log != nil {
		s.Log.Info("auto reply: needs the sales", "message", m.ID, "note", a.Note)
	}
	return a.Reply + "\n\n" + hermes.SalesFooter, nil
}

// factAnswer is the AI Trigger draft of the message for the allowed facts: written now from the current numbers
// (Write keeps drafts of unchanged facts), "" when no allowed trigger matched or no AI kept to the facts.
func (s *Service) factAnswer(ctx context.Context, m gen.ReplyTriggerMessageRow, allowed []string) (string, error) {
	rules, err := s.Reply.Rules(ctx)
	if err != nil {
		return "", err
	}
	action := map[uuid.UUID]string{}
	matched := false
	for _, r := range rules {
		if id, err := uuid.Parse(r.ID); err == nil {
			action[id] = r.Action
		}
		if slices.Contains(allowed, r.Action) && replyrule.Matches(r, m.Body) != "" {
			matched = true
		}
	}
	if !matched {
		return "", nil
	}
	if _, err := s.Reply.Write(ctx, m.ID, nil, ""); err != nil && !errors.Is(err, replyai.ErrNoDraft) {
		return "", err
	}
	drafts, err := s.St.Q.ReplyAIDrafts(ctx, []uuid.UUID{m.ID})
	if err != nil {
		return "", err
	}
	var parts []string
	for _, r := range rules { // in the rules' order
		id, _ := uuid.Parse(r.ID)
		for _, d := range drafts {
			if d.RuleID == id && slices.Contains(allowed, action[id]) && strings.TrimSpace(d.Draft) != "" {
				parts = append(parts, strings.TrimSpace(d.Draft))
			}
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

// queue records the answer as an approved proposal of the person who approved the setting and writes it to the
// outbox (non-live: the anti-ban volume caps apply, ADR 0064).
func (s *Service) queue(ctx context.Context, q *gen.Queries, tx pgx.Tx, t gen.GetThreadRow, set Settings, out Outcome, text string, now time.Time) (uuid.UUID, uuid.UUID, error) {
	sig, err := q.LatestThreadSignal(ctx, &t.ID)
	if err != nil || sig == nil {
		payload, _ := json.Marshal(map[string]any{"thread": t.ID, "reason": "balas otomatis tanpa sinyal pesan masuk"})
		sm := "Balas otomatis saat istirahat"
		id, err := q.InsertManualSignal(ctx, gen.InsertManualSignalParams{DealerID: t.DealerID, SalesID: t.SalesID, OccurredAt: now, DedupeKey: "auto_reply:" + uuid.NewString(), Summary: &sm, Payload: payload})
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		sig = &id
	}
	what := map[Outcome]string{Fact: "jawaban fakta dari sistem, ditulis AI", Template: "pesan template",
		Lyra: "dibalas " + hermes.AgentName + ", angka dari sistem"}[out]
	approved := ""
	if set.ApprovedAt != nil {
		approved = " " + set.ApprovedAt.In(clock.WIB).Format("02/01/2006 15.04")
	}
	reason := fmt.Sprintf("Balas otomatis saat sales istirahat (%s) — aturan disetujui %s%s (ADR 0066)", what, set.ApprovedName, approved)
	conf := 1.0
	if out != Template {
		conf = 0.9
	}
	p, err := q.InsertProposal(ctx, gen.InsertProposalParams{Agent: Agent, DealerID: t.DealerID, Kind: "reply", Title: "Balas otomatis ke " + deref(t.Title),
		Why: reason, Preview: &text, Confidence: conf, SignalIds: []uuid.UUID{*sig}, Autonomy: "auto", Status: "approved",
		DecidedBy: set.ApprovedProfile, DecidedAt: &now, DecisionReason: &reason})
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	dir, from, name := "out", *t.SalesWa, t.SenderName
	pending := "outbox:" + p.ID.String()
	mid, err := q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &t.ID, WaMsgID: &pending, Direction: &dir, FromNumber: &from, FromName: &name,
		Body: &text, SentAt: now, Status: "pending", ProposalID: &p.ID})
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if set.ApprovedBy != nil {
		if err := q.SetMessageSender(ctx, gen.SetMessageSenderParams{ID: mid, SentAt: now, SentBy: *set.ApprovedBy, SentByName: Agent}); err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	}
	payload, _ := json.Marshal(outbox.WAPayload{From: from, To: *t.WaJid, Text: text, Kind: "auto", MessageID: mid.String(), ThreadID: t.ID.String()})
	to := *t.WaJid
	ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: &to, Payload: payload})
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if err := q.TouchThread(ctx, gen.TouchThreadParams{ID: t.ID, LastMessageAt: &now}); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	actor, kind, action, entity := Agent, "system", "chat.auto_reply", "proposal"
	after, _ := json.Marshal(map[string]any{"thread_id": t.ID, "outbox_id": ob.ID, "kind": out, "body": text, "approved_by": set.ApprovedName})
	if err := q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &p.ID, After: after}); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if s.Jobs != nil {
		if _, err := s.Jobs.InsertTx(ctx, tx, jobs.OutboxSendArgs{OutboxID: ob.ID.String()}, nil); err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	}
	if _, err := tx.Exec(ctx, "select pg_notify('chat_message', $1)", `{"thread_id":"`+t.ID.String()+`"}`); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return p.ID, mid, nil
}

// HasPrice reports whether a reply names a rupiah amount ("Rp 2.500.000", "2,5 jt", "250rb").
func HasPrice(text string) bool { return rePrice.MatchString(text) }

var (
	rePrice   = regexp.MustCompile(`(?i)\brp\.?\s*\d|\d\s*(rb|ribu|jt|juta)\b|\brupiah\b`)
	rePhone   = regexp.MustCompile(`^[+\d\s()-]+$`)
	reNonWord = regexp.MustCompile(`[^\pL\pN]+`)
)

// nameOf is how the template greets: the contact's or dealer's name, "Kak" for a number without one.
func nameOf(t gen.GetThreadRow) string {
	n := strings.TrimSpace(deref(t.Title))
	if n == "" || rePhone.MatchString(n) || strings.Contains(n, "•") {
		return "Kak"
	}
	return n
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
