package api

import (
	"context"
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
			phones, err := res.PhonesForLIDs(ctx, acct, lids)
			if err != nil {
				s.log.Warn("lid merge: resolve", "account", wa.MaskNumber(acct), "err", err)
				continue
			}
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
