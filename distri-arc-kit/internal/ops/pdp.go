package ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// ExportDealer returns everything Distri ARC holds about a dealer and its people (arc ctl pdp export).
func ExportDealer(ctx context.Context, st *store.Store, slugOrID string) (json.RawMessage, error) {
	d, err := st.Q.GetDealer(ctx, &slugOrID)
	if err != nil {
		return nil, err
	}
	return st.Q.PDPDealerExport(ctx, d.ID)
}

// DeleteReport counts what a subject deletion removed.
type DeleteReport struct {
	Contacts        int64 `json:"contacts"`
	Threads         int64 `json:"threads"`
	Messages        int64 `json:"messages"`
	Signals         int64 `json:"signals"`
	Identifications int64 `json:"identifications"`
}

// ErrUnknownNumber is returned when nothing is stored for the number.
var ErrUnknownNumber = errors.New("nomor tidak ditemukan")

// DeleteContact removes a person (UU PDP erasure): the contact, its conversations and conversation signals, its
// lines in groups and its identification. Orders, invoices, payments and metrics stay (aggregates, legal records);
// the dealer memo is rewritten in the next cycle. The audit row keeps a hash of the number, not the number.
func DeleteContact(ctx context.Context, st *store.Store, number, by string) (DeleteReport, error) {
	var r DeleteReport
	n := wa.Digits(number)
	if n == "" {
		return r, ErrUnknownNumber
	}
	jid := wa.UserJID(n)
	err := st.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		cs, err := q.ContactsByNumber(ctx, n)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, 0, len(cs))
		var dealers []uuid.UUID
		for _, c := range cs {
			ids = append(ids, c.ID)
			if c.DealerID != nil {
				dealers = append(dealers, *c.DealerID)
			}
		}
		if r.Messages, err = q.PDPDeleteMessages(ctx, gen.PDPDeleteMessagesParams{WaNumber: n, ContactIds: ids, Jid: jid}); err != nil {
			return err
		}
		if r.Threads, err = q.PDPDeleteThreads(ctx, gen.PDPDeleteThreadsParams{ContactIds: ids, Jid: jid}); err != nil {
			return err
		}
		if r.Signals, err = q.PDPDeleteSignals(ctx, gen.PDPDeleteSignalsParams{ContactIds: ids, WaNumber: n}); err != nil {
			return err
		}
		if r.Contacts, err = q.PDPDeleteContacts(ctx, ids); err != nil {
			return err
		}
		if r.Identifications, err = q.PDPDeleteIdentification(ctx, n); err != nil {
			return err
		}
		if r.Contacts+r.Threads+r.Messages+r.Signals+r.Identifications == 0 {
			return ErrUnknownNumber
		}
		if len(dealers) > 0 {
			if err := q.MarkMemoStale(ctx, dealers); err != nil {
				return err
			}
		}
		sum := sha256.Sum256([]byte(n))
		after, _ := json.Marshal(map[string]any{"number_sha256": hex.EncodeToString(sum[:]), "report": r})
		actor, kind, action, entity := by, "user", "pdp.delete", "contact"
		return q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, After: after})
	})
	return r, err
}
