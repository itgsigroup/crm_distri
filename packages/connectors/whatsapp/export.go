package whatsapp

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Export lines look like (ID/EN, 24h or 12h, with or without brackets):
//
//	24/09/26 16.20 - Pak Wijaya: Mas Andi, terima kasih…
//	[24/09/2026, 16:20:05] Pak Wijaya: …
//	9/24/26, 4:20 PM - Pak Wijaya: …
var reLine = regexp.MustCompile(`^\[?(\d{1,2})[/.-](\d{1,2})[/.-](\d{2,4}),?\s+(\d{1,2})[.:](\d{2})(?:[.:](\d{2}))?\s*([AaPp][Mm])?\]?\s*(?:-\s*)?([^:]{1,60}):\s(.*)$`)

// ExportOptions control how an exported chat is attributed.
type ExportOptions struct {
	Session    string
	ChatJID    string
	ChatName   string
	IsGroup    bool
	OwnName    string // the sales name as shown in the export (messages from them are from_me)
	MonthFirst bool   // EN exports use M/D/Y
	Location   *time.Location
}

// ParseExport converts a WhatsApp "Ekspor chat" .txt (or .zip containing it)
// into history WaEvents. Wamids are derived from content so re-uploads and
// overlaps with live events do not duplicate.
func ParseExport(data []byte, opt ExportOptions) ([]WaEvent, error) {
	if bytes.HasPrefix(data, []byte("PK")) {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		var found []byte
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".txt") {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				found, err = io.ReadAll(io.LimitReader(rc, 64<<20))
				rc.Close()
				if err != nil {
					return nil, err
				}
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("zip tidak berisi file .txt ekspor chat")
		}
		data = found
	}
	loc := opt.Location
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	var out []WaEvent
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimPrefix(sc.Text(), "\ufeff")
		line = strings.ReplaceAll(line, "\u202f", " ")
		m := reLine.FindStringSubmatch(line)
		if m == nil {
			if len(out) > 0 && strings.TrimSpace(line) != "" {
				out[len(out)-1].Text += "\n" + line
			}
			continue
		}
		var d, mo, y, h, mi, se int
		fmt.Sscanf(m[1], "%d", &d)
		fmt.Sscanf(m[2], "%d", &mo)
		if opt.MonthFirst {
			d, mo = mo, d
		}
		fmt.Sscanf(m[3], "%d", &y)
		if y < 100 {
			y += 2000
		}
		fmt.Sscanf(m[4], "%d", &h)
		fmt.Sscanf(m[5], "%d", &mi)
		if m[6] != "" {
			fmt.Sscanf(m[6], "%d", &se)
		}
		if ap := strings.ToLower(m[7]); ap != "" {
			if ap == "pm" && h < 12 {
				h += 12
			} else if ap == "am" && h == 12 {
				h = 0
			}
		}
		ts := time.Date(y, time.Month(mo), d, h, mi, se, 0, loc)
		sender := strings.TrimSpace(m[8])
		text := m[9]
		if strings.Contains(text, "<Media tidak disertakan>") || strings.Contains(text, "<Media omitted>") {
			continue
		}
		ev := WaEvent{Session: opt.Session, ChatID: opt.ChatJID, IsGroup: opt.IsGroup, SenderName: sender, Text: text,
			Timestamp: ts, FromMe: opt.OwnName != "" && strings.EqualFold(sender, opt.OwnName), IsHistory: true, Transport: "export"}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Wamid = ContentWamid(out[i].ChatID, out[i].SenderName, out[i].Timestamp, out[i].Text)
	}
	return out, nil
}

// ContentWamid derives a stable id from chat, sender, minute and text so the
// same message from an export and from the live bridge collapse into one.
func ContentWamid(chat, sender string, ts time.Time, text string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{chat, strings.ToLower(strings.TrimSpace(sender)), ts.UTC().Format("200601021504"), strings.TrimSpace(text)}, "|")))
	return "c-" + hex.EncodeToString(sum[:])[:24]
}
