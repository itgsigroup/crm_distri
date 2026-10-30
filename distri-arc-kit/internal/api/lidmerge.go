package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/events"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Chat ganda LID (ADR 0072): a chat list holding one-to-one chats stored under a LID asks the number's device for
// their phone numbers and merges each into the number's chat, in the background; the list then refreshes.

// lidRetry: a LID the device did not know is asked again after this long (the mapping fills in as messages come).
const lidRetry = 30 * time.Minute

var lidTried = struct {
	sync.Mutex
	m map[uuid.UUID]time.Time
}{m: map[uuid.UUID]time.Time{}}

// lidCandidates are the numbers whose LIDs are worth asking: one-to-one chats under a phone number on the same GSI
// number that carry the name of a LID chat (the saved name or the profile name). The name only picks whom to ask;
// a chat is merged only when WhatsApp maps the LID to that number.
func (s *Server) lidCandidates(ctx context.Context, account string, chats map[string]uuid.UUID) []string {
	ids := make([]uuid.UUID, 0, len(chats))
	for _, id := range chats {
		ids = append(ids, id)
	}
	rows, err := s.st.Pool.Query(ctx, `select distinct p.wa_jid from chat_threads l join chat_threads p on p.account = l.account and p.id <> l.id
		where l.id = any($1) and p.account = $2 and p.wa_jid like '%@s.whatsapp.net' and coalesce(p.kind, '') <> 'group'
		  and (lower(nullif(trim(p.phone_name), '')) in (lower(trim(l.phone_name)), lower(trim(l.title)))
		    or lower(nullif(trim(p.title), '')) in (lower(trim(l.phone_name)), lower(trim(l.title))))
		limit 50`, ids, account)
	if err != nil {
		s.log.Warn("lid merge: candidates", "err", err)
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var jid string
		if rows.Scan(&jid) == nil {
			out = append(out, jid)
		}
	}
	return out
}

// mergeLIDChats starts merging the LID chats of a list (each tried at most once per lidRetry).
func (s *Server) mergeLIDChats(rows []gen.ListThreadsRow) {
	res, _ := s.mediaFetcher().(wa.LIDResolver)
	if res == nil {
		return
	}
	byAccount := map[string]map[string]uuid.UUID{} // account → LID → chat
	now := time.Now()
	lidTried.Lock()
	for _, t := range rows {
		jid, acct := deref(t.WaJid), deref(t.Account)
		if deref(t.Kind) == "group" || acct == "" || !wa.IsLID(jid) || now.Sub(lidTried.m[t.ID]) < lidRetry {
			continue
		}
		lidTried.m[t.ID] = now
		if byAccount[acct] == nil {
			byAccount[acct] = map[string]uuid.UUID{}
		}
		byAccount[acct][jid] = t.ID
	}
	lidTried.Unlock()
	if len(byAccount) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for acct, chats := range byAccount {
			lids := make([]string, 0, len(chats))
			for l := range chats {
				lids = append(lids, l)
			}
			cands := s.lidCandidates(ctx, acct, chats)
			phones, err := res.PhonesForLIDs(ctx, acct, lids, cands)
			if err != nil {
				s.log.Warn("lid merge: resolve", "account", wa.MaskNumber(acct), "err", err)
				continue
			}
			s.log.Info("lid merge: resolve", "account", wa.MaskNumber(acct), "lids", len(lids), "candidates", len(cands), "found", len(phones))
			for lid, pn := range phones {
				id, ok := chats[lid]
				if !ok {
					continue
				}
				kept, err := wa.MergeLIDThread(ctx, s.st, id, pn)
				if err != nil {
					s.log.Error("lid merge", "thread", id, "err", err)
					continue
				} else if kept == uuid.Nil {
					continue
				}
				s.log.Info("lid merge", "thread", id, "into", kept, "account", wa.MaskNumber(acct))
				_ = events.Notify(ctx, s.st.Pool, "chat_message", map[string]string{"thread_id": kept.String()})
			}
		}
	}()
}

// chatTwins are the one-to-one chats of the same person on the same GSI number kept apart from this one (ADR 0083):
// a LID chat and the number's chat that WhatsApp has not linked yet, recognised by the same saved or profile name.
// Exactly one twin counts — two or more chats of that name could be different people. Its messages show in this
// chat, so "Muat pesan lama" reads them from GSI Orbit before asking the phone; the LID is also asked of the device
// (in the background, at most once per lidRetry) so the two merge for good once WhatsApp links them (ADR 0072).
func (s *Server) chatTwins(ctx context.Context, id uuid.UUID, kind, account, jid *string) []uuid.UUID {
	j, acct := deref(jid), deref(account)
	lid := wa.IsLID(j)
	if deref(kind) == "group" || acct == "" || (!lid && !strings.HasSuffix(j, "@s.whatsapp.net")) {
		return nil
	}
	other := "%@lid"
	if lid {
		other = "%@s.whatsapp.net"
	}
	rows, err := s.st.Pool.Query(ctx, `select p.id, p.wa_jid from chat_threads l join chat_threads p on p.account = l.account and p.id <> l.id
		where l.id = $1 and p.wa_jid like $2 and coalesce(p.kind, '') <> 'group'
		  and exists (select 1 from unnest(array[lower(trim(p.phone_name)), lower(trim(p.title))]) a(n)
		              join unnest(array[lower(trim(l.phone_name)), lower(trim(l.title))]) b(n) on a.n = b.n
		              where a.n ~ '[a-z]' and a.n not like 'nomor%')
		limit 2`, id, other)
	if err != nil {
		s.log.Warn("chat twins", "thread", id, "err", err)
		return nil
	}
	defer rows.Close()
	var twins []uuid.UUID
	var twinJID string
	for rows.Next() {
		var tid uuid.UUID
		if rows.Scan(&tid, &twinJID) == nil {
			twins = append(twins, tid)
		}
	}
	if rows.Err() != nil || len(twins) != 1 {
		return nil
	}
	lidChat := gen.ListThreadsRow{ID: twins[0], Kind: kind, Account: account, WaJid: &twinJID}
	if lid {
		lidChat = gen.ListThreadsRow{ID: id, Kind: kind, Account: account, WaJid: jid}
	}
	s.mergeLIDChats([]gen.ListThreadsRow{lidChat})
	return twins
}
