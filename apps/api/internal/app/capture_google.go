package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"arc/packages/connectors/google"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

// MailResult reports a Gmail capture run.
type MailResult struct {
	Fetched    int  `json:"fetched"`
	Stored     int  `json:"stored"`
	Linked     int  `json:"linked"`
	Unresolved int  `json:"unresolved"`
	Ignored    int  `json:"ignored"`
	Mock       bool `json:"mock"`
}

// googleToken returns the decrypted OAuth access token of a user (real Gmail mode).
func (a *App) googleToken(ctx context.Context, mailbox string) (string, error) {
	var enc string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT enc_token FROM oauth_google_tokens WHERE user_id=$1`, mailbox).Scan(&enc); err != nil {
		return "", errors.New("gmail belum dihubungkan untuk pengguna ini")
	}
	return decrypt(a.Cfg.EncryptKey, enc)
}

var noisy = []string{"newsletter", "no-reply", "noreply", "unsubscribe", "promo"}

// CaptureGmail ingests mail incrementally for every connected mailbox
// (or the .eml fixtures in mock mode) with identity resolution:
// exact address → Person, domain → Account, new Person only with ≥ 2 signals.
func (a *App) CaptureGmail(ctx context.Context) (MailResult, error) {
	res := MailResult{Mock: a.GoogleMock}
	mailboxes := []string{}
	if a.GoogleMock {
		mailboxes = []string{"fixtures"}
	} else {
		rows, err := a.DB.Pool.Query(ctx, `SELECT user_id FROM oauth_google_tokens`)
		if err != nil {
			return res, err
		}
		for rows.Next() {
			var u string
			_ = rows.Scan(&u)
			mailboxes = append(mailboxes, u)
		}
		rows.Close()
	}
	for _, mb := range mailboxes {
		var since string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT watermark FROM sync_watermarks WHERE system='gmail' AND model=$1 AND company=0`, mb).Scan(&since)
		mails, next, err := a.Mail.Fetch(ctx, mb, since)
		if err != nil {
			return res, err
		}
		res.Fetched += len(mails)
		for _, m := range mails {
			// First run = backfill: mail older than 3 days is stored as history, not re-analysed.
			backfill := since == "" && !m.Date.IsZero() && domain.Now().Sub(m.Date) > 72*time.Hour
			if err := a.storeMail(ctx, m, backfill, &res); err != nil {
				return res, err
			}
		}
		if next != "" && next != since {
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO sync_watermarks(system,model,company,watermark) VALUES ('gmail',$1,0,$2) ON CONFLICT (system,model,company) DO UPDATE SET watermark=EXCLUDED.watermark, updated_at=now()`, mb, next)
		}
	}
	return res, nil
}

func (a *App) storeMail(ctx context.Context, m google.Mail, backfill bool, res *MailResult) error {
	if m.MessageID == "" {
		return nil
	}
	if a.exists(ctx, `SELECT 1 FROM interactions WHERE raw_ref=$1`, "email:"+m.MessageID) {
		return nil
	}
	// Which GSI user owns the mailbox / sent it?
	var userID string
	addrs := append([]string{m.From}, append(m.To, m.Cc...)...)
	for _, ad := range addrs {
		if strings.HasSuffix(ad, "@gsi.co.id") {
			_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email)=$1`, ad).Scan(&userID)
			if userID != "" {
				break
			}
		}
	}
	outbound := strings.HasSuffix(m.From, "@gsi.co.id")
	external := m.From
	if outbound && len(m.To) > 0 {
		external = m.To[0]
	}
	lower := strings.ToLower(m.From + " " + m.Subject)
	ignored := false
	for _, n := range noisy {
		if strings.Contains(lower, n) {
			ignored = true
		}
	}
	var personID, accountID string
	if !ignored {
		_ = a.DB.Pool.QueryRow(ctx, `SELECT id, COALESCE(account_id,'') FROM people WHERE $1 = ANY(emails) LIMIT 1`, external).Scan(&personID, &accountID)
		if personID == "" {
			domainPart := external[strings.LastIndex(external, "@")+1:]
			_ = a.DB.Pool.QueryRow(ctx, `SELECT p.account_id FROM people p, unnest(p.emails) e WHERE split_part(e,'@',2)=$1 AND p.account_id IS NOT NULL LIMIT 1`, domainPart).Scan(&accountID)
			if accountID != "" && m.FromName != "" && !outbound {
				// Known person of that account without this address yet → attach the email.
				_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM people WHERE account_id=$1 AND lower(name)=lower($2) LIMIT 1`, accountID, m.FromName).Scan(&personID)
				if personID != "" {
					_, _ = a.DB.Pool.Exec(ctx, `UPDATE people SET emails = array_append(emails, $2) WHERE id=$1 AND NOT ($2 = ANY(emails))`, personID, external)
				}
			}
			if personID == "" && accountID != "" && m.FromName != "" && !outbound {
				// Two identity signals (display name + company domain) → create the Person.
				personID = "p-" + storage.Hash(external)[:12]
				if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO people(id,name,account_id,emails,stakeholder_tag,strength) VALUES ($1,$2,$3,$4,'user',1) ON CONFLICT DO NOTHING`,
					personID, m.FromName, accountID, []string{external}); err != nil {
					return err
				}
			}
		}
	}
	var acc, pids any
	pids = []string{}
	if accountID != "" {
		acc = accountID
	}
	if personID != "" {
		pids = []string{personID}
	}
	dir := "in"
	if outbound {
		dir = "out"
	}
	users := []string{}
	if userID != "" {
		users = []string{userID}
	}
	at := m.Date
	if at.IsZero() {
		at = domain.Now()
	}
	summary := ""
	if ignored {
		summary = "diabaikan: bukan korespondensi pelanggan"
	}
	var id int64
	err := a.DB.Pool.QueryRow(ctx, `INSERT INTO interactions(channel,direction,occurred_at,person_ids,user_ids,subject,body_text,attachments,raw_ref,account_id,sender_name,thread_id,transport,extracted,summary)
		VALUES ('email',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULL,'gmail',$11,$12) ON CONFLICT (raw_ref) DO NOTHING RETURNING id`,
		dir, at, pids, users, m.Subject, m.Body, storage.JSON(m.Attachments), "email:"+m.MessageID, acc, defaultStr(m.FromName, m.From), ignored || backfill, summary).Scan(&id)
	if err != nil {
		return nil // conflict: already stored
	}
	res.Stored++
	switch {
	case ignored:
		res.Ignored++
	case accountID != "":
		res.Linked++
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE accounts SET last_interaction_at=GREATEST(COALESCE(last_interaction_at,$2),$2), last_via='mail' WHERE id=$1`, accountID, at)
	default:
		res.Unresolved++
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO unresolved_identities(kind,value,interaction_id) VALUES ('email',$1,$2) ON CONFLICT DO NOTHING`, external, id)
	}
	return nil
}

func defaultStr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// CaptureCalendar stores events H-7..H+14 and links accounts via attendees.
func (a *App) CaptureCalendar(ctx context.Context) (int, error) {
	now := domain.Now()
	mailboxes := []string{"fixtures"}
	if !a.GoogleMock {
		mailboxes = nil
		rows, err := a.DB.Pool.Query(ctx, `SELECT user_id FROM oauth_google_tokens`)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var u string
			_ = rows.Scan(&u)
			mailboxes = append(mailboxes, u)
		}
		rows.Close()
	}
	n := 0
	for _, mb := range mailboxes {
		evs, err := a.Calendar.Events(ctx, mb, now.AddDate(0, 0, -7), now.AddDate(0, 0, 14))
		if err != nil {
			return n, err
		}
		for _, e := range evs {
			var acc any
			var accID string
			for _, at := range e.Attendees {
				_ = a.DB.Pool.QueryRow(ctx, `SELECT account_id FROM people WHERE $1 = ANY(emails) AND account_id IS NOT NULL LIMIT 1`, strings.ToLower(at)).Scan(&accID)
				if accID != "" {
					break
				}
			}
			internal := accID == ""
			if accID != "" {
				acc = accID
			}
			owner := any(nil)
			if mb != "fixtures" {
				owner = mb
			}
			tag, err := a.DB.Pool.Exec(ctx, `INSERT INTO calendar_events(id,raw_ref,title,starts_at,duration_min,location,attendees,account_id,internal,owner_user_id,source)
				VALUES ($1,$1,$2,$3,$4,$5,$6,$7,$8,$9,'google') ON CONFLICT (raw_ref) DO UPDATE SET starts_at=EXCLUDED.starts_at, title=EXCLUDED.title, updated_at=now()`,
				e.ID, e.Title, e.Start, int(e.Duration/time.Minute), e.Location, strings.Join(e.Attendees, ", "), acc, internal, owner)
			if err != nil {
				return n, err
			}
			n += int(tag.RowsAffected())
			if accID != "" {
				_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO interactions(channel,direction,occurred_at,participants_label,subject,body_text,raw_ref,account_id,transport,extracted)
					VALUES ('meeting','out',$1,$2,$3,$3,$4,$5,'gcal',true) ON CONFLICT (raw_ref) DO NOTHING`, e.Start, strings.Join(e.Attendees, ", "), e.Title, "gcal:"+e.ID, accID)
			}
		}
	}
	return n, nil
}

func (a *App) mustFmt(format string, v ...any) string { return fmt.Sprintf(format, v...) }
