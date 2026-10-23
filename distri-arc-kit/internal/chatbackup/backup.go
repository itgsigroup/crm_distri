package chatbackup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"distri-arc/internal/clock"
)

// File is one month's backup: chat-2026-10.zip holding "<GSI number>/Chat WhatsApp dengan <name>.txt" per chat.
type File struct {
	Name     string    `json:"name"`
	Month    string    `json:"month"` // 2026-10
	Size     int64     `json:"size"`
	At       time.Time `json:"at"` // when it was written
	Chats    int       `json:"chats"`
	Messages int       `json:"messages"`
}

var fileRe = regexp.MustCompile(`^chat-(\d{4})-(\d{2})\.zip$`)

func fileOf(month time.Time) string { return "chat-" + month.Format("2006-01") + ".zip" }

// DownloadName is the name the browser saves a month's backup under.
func DownloadName(name string) string {
	if m := fileRe.FindStringSubmatch(name); m != nil {
		return "Cadangan chat GSI Orbit " + m[1] + "-" + m[2] + ".zip"
	}
	return name
}

// Path is the backup file of a name from the list (nothing else in the folder is ever served).
func Path(dir, name string) (string, bool) {
	if !fileRe.MatchString(name) {
		return "", false
	}
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// monthStart is the first instant of t's month in WIB.
func monthStart(t time.Time) time.Time {
	t = t.In(clock.WIB)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, clock.WIB)
}

// Run writes the backups due: this month and the month of yesterday every time (they still grow), every earlier
// month still stored that has no file yet. A month that is complete and written is never written again — the nightly
// purge empties it later, its file stays. Files older than keepMonths (0 = forever) are removed. Light by design: a
// night writes one month (two on the 1st), streamed row by row.
func Run(ctx context.Context, db DB, dir string, now time.Time, keepMonths int) ([]File, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	if _, err := Prune(dir, now, keepMonths); err != nil {
		return nil, err
	}
	var oldest *time.Time
	if err := db.QueryRow(ctx, `select min(sent_at) from chat_message_keys`).Scan(&oldest); err != nil {
		return nil, err
	}
	cur := monthStart(now)
	if oldest == nil {
		oldest = &cur
	}
	from := monthStart(*oldest)
	if keepMonths > 0 {
		if k := cur.AddDate(0, -(keepMonths - 1), 0); from.Before(k) {
			from = k
		}
	}
	yesterday := monthStart(now.AddDate(0, 0, -1))
	var out []File
	for m := from; !m.After(cur); m = m.AddDate(0, 1, 0) {
		_, err := os.Stat(filepath.Join(dir, fileOf(m)))
		if m.Equal(cur) || m.Equal(yesterday) || errors.Is(err, os.ErrNotExist) {
			f, err := WriteMonth(ctx, db, dir, m, now)
			if err != nil {
				return out, fmt.Errorf("cadangan %s: %w", m.Format("2006-01"), err)
			}
			out = append(out, f)
		}
	}
	return out, nil
}

// monthMsgs: the month's messages in one pass, grouped by conversation (a group is one conversation for every GSI
// number in it), each row carrying its thread so the first row of a conversation names its entry.
const monthMsgs = `select case when x.kind = 'group' and x.wa_jid is not null then 'g:' || x.wa_jid else x.id::text end as conv,
  x.id, coalesce(x.kind, ''), coalesce(x.wa_jid, ''), coalesce(x.title, ''), coalesce(x.phone_name, ''), ` + msgCols + ` ` + msgFrom + `
order by conv, m.sent_at, m.id`

// WriteMonth writes month's zip in a single streamed query (temp file, then renamed: a download never sees half a
// file). Measured: 150k messages over 3,000 chats → 2.3 MB zip in 1.6 s, ~10 MB of memory.
func WriteMonth(ctx context.Context, db DB, dir string, month, now time.Time) (File, error) {
	from := monthStart(month)
	to := from.AddDate(0, 1, 0)
	tmp, err := os.CreateTemp(dir, ".chat-*.tmp")
	if err != nil {
		return File{}, err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }() // nothing left after the rename; cleans a failure
	zw := zip.NewWriter(tmp)
	f := File{Name: fileOf(from), Month: from.Format("2006-01"), At: now}

	rows, err := db.Query(ctx, monthMsgs, from, to)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	used := map[string]bool{}
	var (
		key, cur    string
		c           Conv
		title, nick string
		m           Msg
		x           *Writer
		h           *zip.FileHeader
	)
	// end closes the current entry. Its comment says whose chat it is ("Obrolan pribadi" and UU PDP erasure remove
	// it later, Forget) and how many messages it holds; the zip writer keeps h until Close writes the central
	// directory, so it can be set after the data.
	end := func() error {
		if x == nil {
			return nil
		}
		h.Comment = c.JID + " " + strconv.Itoa(x.Count)
		f.Chats++
		f.Messages += x.Count
		return x.Flush()
	}
	var row Conv // this row's thread; c stays the open entry's conversation until the next one starts
	dests := append([]any{&key, &row.ID, &row.Kind, &row.JID, &title, &nick}, m.dests()...)
	for rows.Next() {
		if err := rows.Scan(dests...); err != nil {
			return f, err
		}
		if x == nil || key != cur {
			if err := end(); err != nil {
				return f, err
			}
			cur, c = key, Conv{ID: row.ID, Kind: row.Kind, JID: row.JID, Title: displayTitle(title, nick, row.Kind, row.JID), Account: m.Account}
			h = &zip.FileHeader{Name: entryName(c, used), Method: zip.Deflate, Modified: now.In(clock.WIB)}
			w, err := zw.CreateHeader(h)
			if err != nil {
				return f, err
			}
			x = NewWriter(w, c, now)
		}
		x.Write(m)
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	if err := end(); err != nil {
		return f, err
	}
	if err := zw.SetComment(summary(f)); err != nil {
		return f, err
	}
	if err := zw.Close(); err != nil {
		return f, err
	}
	if err := tmp.Close(); err != nil {
		return f, err
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return f, err
	}
	if info, err := os.Stat(tmp.Name()); err == nil {
		f.Size = info.Size()
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, f.Name)); err != nil {
		return f, err
	}
	return f, nil
}

// entryName is "<GSI number>/Chat WhatsApp dengan <name>.txt", "(2)" for a second chat of the same name.
func entryName(c Conv, used map[string]bool) string {
	base := safeName(nonEmpty(c.Account, "Tanpa nomor")) + "/" + strings.TrimSuffix(FileName(c), ".txt")
	name := base + ".txt"
	for i := 2; used[strings.ToLower(name)]; i++ {
		name = base + " (" + strconv.Itoa(i) + ").txt"
	}
	used[strings.ToLower(name)] = true
	return name
}

// summary is the zip comment: what List shows without reading the chats.
func summary(f File) string {
	return fmt.Sprintf("GSI Orbit chat backup %s chats=%d messages=%d at=%s", f.Month, f.Chats, f.Messages, f.At.UTC().Format(time.RFC3339))
}

func parseSummary(s string, f *File) {
	for _, w := range strings.Fields(s) {
		k, v, ok := strings.Cut(w, "=")
		if !ok {
			continue
		}
		switch k {
		case "chats":
			f.Chats, _ = strconv.Atoi(v)
		case "messages":
			f.Messages, _ = strconv.Atoi(v)
		case "at":
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.At = t
			}
		}
	}
}

// List is every month's backup, newest first.
func List(dir string) ([]File, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []File{}, nil
	} else if err != nil {
		return nil, err
	}
	out := []File{}
	for _, e := range ents {
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		f := File{Name: e.Name(), Month: m[1] + "-" + m[2], Size: info.Size(), At: info.ModTime()}
		if r, err := zip.OpenReader(filepath.Join(dir, e.Name())); err == nil {
			parseSummary(r.Comment, &f)
			_ = r.Close()
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month > out[j].Month })
	return out, nil
}

// Prune removes the backups of months before the last keepMonths (0 keeps every month).
func Prune(dir string, now time.Time, keepMonths int) (int, error) {
	if keepMonths <= 0 {
		return 0, nil
	}
	cut := monthStart(now).AddDate(0, -(keepMonths - 1), 0).Format("2006-01")
	files, err := List(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range files {
		if f.Month < cut {
			if err := os.Remove(filepath.Join(dir, f.Name)); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// Forget removes a chat (its JID) from every backup: a chat marked "Obrolan pribadi" and a UU PDP erasure leave no
// copy here. Other entries are copied as they are (no recompression). It returns how many chats were removed.
func Forget(dir, jid string) (int, error) {
	if jid == "" {
		return 0, nil
	}
	files, err := List(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, f := range files {
		n, err := forgetIn(dir, f, jid)
		if err != nil {
			return removed, err
		}
		removed += n
	}
	return removed, nil
}

func forgetIn(dir string, f File, jid string) (int, error) {
	p := filepath.Join(dir, f.Name)
	r, err := zip.OpenReader(p)
	if err != nil {
		return 0, err
	}
	defer func() { _ = r.Close() }()
	hit := false
	for _, e := range r.File {
		if entryJID(e.Comment) == jid {
			hit = true
			break
		}
	}
	if !hit {
		return 0, nil
	}
	tmp, err := os.CreateTemp(dir, ".chat-*.tmp")
	if err != nil {
		return 0, err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	zw := zip.NewWriter(tmp)
	removed := 0
	g := File{Month: f.Month, At: f.At}
	for _, e := range r.File {
		if entryJID(e.Comment) == jid {
			removed++
			continue
		}
		if err := zw.Copy(e); err != nil {
			return 0, err
		}
		g.Chats++
		g.Messages += entryCount(e.Comment)
	}
	if err := zw.SetComment(summary(g)); err != nil {
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return 0, err
	}
	keepOwner(tmp.Name(), p) // rewritten by root (arc ctl pdp delete): api and worker still read it
	return removed, os.Rename(tmp.Name(), p)
}

// keepOwner gives the new file the owner of the one it replaces (best effort).
func keepOwner(newPath, oldPath string) {
	info, err := os.Stat(oldPath)
	if err != nil {
		return
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		_ = os.Chown(newPath, int(st.Uid), int(st.Gid))
	}
}

func entryJID(comment string) string {
	jid, _, _ := strings.Cut(comment, " ")
	return jid
}

func entryCount(comment string) int {
	_, n, _ := strings.Cut(comment, " ")
	c, _ := strconv.Atoi(n)
	return c
}
