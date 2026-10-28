package wa

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Chat ganda LID (ADR 0072): WhatsApp sometimes delivers a one-to-one message by the contact's LID ("…@lid") before
// the linked device knows their number, so the chat is stored apart from the number's chat ("Nomor baru" next to
// "+62 823…"). Once the device's LID mapping knows the number, the LID chat is merged into the number's chat.

// LIDResolver turns LIDs into phone JIDs through a GSI number's device (its local LID mapping: no WhatsApp query).
type LIDResolver interface {
	PhonesForLIDs(ctx context.Context, account string, lids []string) (map[string]string, error)
}

// IsLID reports whether a JID is a LID ("123456789@lid").
func IsLID(jid string) bool { return strings.HasSuffix(jid, "@lid") }

// PhonesForLIDs asks the bridge for the phone JIDs of LIDs the number's device knows.
func (b *Baileys) PhonesForLIDs(ctx context.Context, account string, lids []string) (map[string]string, error) {
	var out struct {
		Map map[string]string `json:"map"`
	}
	if _, _, err := b.call(ctx, http.MethodPost, "/sessions/"+url.PathEscape(b.session(ctx, account))+"/lids", map[string]any{"lids": lids}, &out); err != nil {
		return nil, err
	}
	return out.Map, nil
}

// MergeLIDThread moves a chat stored under a LID into the chat of the same contact's phone JID on the same GSI
// number, in one transaction: its messages, stars, labels, pins, analysis and Lyra readings, the unread count and the
// customer link; then the LID chat is deleted. Without a phone chat yet, the LID chat simply takes the phone JID.
// It returns the chat that remains (uuid.Nil when the LID chat is gone already: running it again is a no-op).
func MergeLIDThread(ctx context.Context, st *store.Store, lidThread uuid.UUID, phoneJID string) (uuid.UUID, error) {
	if IsLID(phoneJID) || !strings.HasSuffix(phoneJID, "@s.whatsapp.net") {
		return uuid.Nil, errors.New("bukan JID nomor telepon: " + phoneJID)
	}
	var kept uuid.UUID
	err := st.Tx(ctx, func(_ *gen.Queries, tx pgx.Tx) error {
		var account, jid string
		err := tx.QueryRow(ctx, "select coalesce(account, ''), coalesce(wa_jid, '') from chat_threads where id = $1 for update", lidThread).Scan(&account, &jid)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // merged already (another list load did it): nothing to do
		} else if err != nil {
			return err
		}
		if !IsLID(jid) {
			kept = lidThread // merged already, or not a LID chat
			return nil
		}
		err = tx.QueryRow(ctx, "select id from chat_threads where account = $1 and wa_jid = $2 for update", account, phoneJID).Scan(&kept)
		if errors.Is(err, pgx.ErrNoRows) { // no chat under the number yet: the LID chat becomes it
			kept = lidThread
			_, err = tx.Exec(ctx, "update chat_threads set wa_jid = $2 where id = $1", lidThread, phoneJID)
			return err
		} else if err != nil {
			return err
		}
		from, to := lidThread.String(), kept.String()
		for _, q := range []string{
			// the number's chat keeps its own identity; what it lacks comes from the LID chat
			`update chat_threads t set
				kind = case when t.kind = 'new' and l.kind <> 'new' then l.kind else t.kind end,
				dealer_id = coalesce(t.dealer_id, l.dealer_id), contact_id = coalesce(t.contact_id, l.contact_id),
				sales_id = coalesce(t.sales_id, l.sales_id), phone_name = coalesce(t.phone_name, l.phone_name),
				title = coalesce(nullif(t.title, ''), l.title), subtitle = coalesce(nullif(t.subtitle, ''), l.subtitle),
				identification = coalesce(t.identification, l.identification),
				last_message_at = greatest(t.last_message_at, l.last_message_at), unread = coalesce(t.unread, 0) + coalesce(l.unread, 0),
				unreplied_mark = greatest(t.unreplied_mark, l.unreplied_mark), replied_mark = greatest(t.replied_mark, l.replied_mark),
				mentioned = t.mentioned + l.mentioned
			from chat_threads l where t.id = $2::uuid and l.id = $1::uuid`,
			`update chat_messages set thread_id = $2::uuid where thread_id = $1::uuid`,
			`update chat_message_stars set thread_id = $2::uuid where thread_id = $1::uuid`,
			`update wa_message_insights set thread_id = $2::uuid where thread_id = $1::uuid`,
			`delete from wa_insights l where l.thread_id = $1::uuid and exists (select 1 from wa_insights t where t.thread_id = $2::uuid and t.cycle_id = l.cycle_id)`,
			`update wa_insights set thread_id = $2::uuid where thread_id = $1::uuid`,
			`update hermes_product_reads set thread_id = $2::uuid where thread_id = $1::uuid`,
			// labels and pins are keyed by the chat id (text) for one-to-one chats
			`delete from chat_conv_labels l where l.conv = $1 and exists (select 1 from chat_conv_labels t where t.conv = $2 and t.label_id = l.label_id)`,
			`update chat_conv_labels set conv = $2 where conv = $1`,
			`update chat_message_labels set conv = $2 where conv = $1`,
			`update chat_pins set conv = $2 where conv = $1`,
			`delete from chat_thread_pins l where l.conv = $1 and exists (select 1 from chat_thread_pins t where t.conv = $2 and t.user_id = l.user_id)`,
			`update chat_thread_pins set conv = $2 where conv = $1`,
			`delete from chat_threads where id = $1::uuid`,
		} {
			args := []any{from, to}
			if !strings.Contains(q, "$2") {
				args = args[:1]
			}
			if _, err := tx.Exec(ctx, q, args...); err != nil {
				return err
			}
		}
		return nil
	})
	return kept, err
}
