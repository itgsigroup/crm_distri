package chatbackup

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/clock"
)

// A chat reads exactly like WhatsApp's own Indonesian Android export (no media): the time in WIB, "Nama: pesan",
// "<Media tidak disertakan>", deleted and edited messages, multi-line messages continuing on the next lines.
func TestWriterWhatsAppFormat(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 23, h, m, 0, 0, clock.WIB).UTC() }
	var b bytes.Buffer
	x := NewWriter(&b, Conv{Kind: "dealer", Title: "Mbak Rina · Toko Sinar", Account: "Andi"}, at(2, 15))
	for _, m := range []Msg{
		{At: at(9, 1), Direction: "in", FromName: "Rina", Body: "Pagi pak, stok NVR 16ch ada?"},
		{At: at(9, 3), Direction: "out", Account: "Andi", Body: "Ada bu.\nKirim Senin ya"},
		{At: at(9, 4), Direction: "in", Media: "image", Body: "ini fotonya"},
		{At: at(9, 5), Direction: "in", Media: "document", FileName: "PO-0812.pdf"},
		{At: at(9, 6), Direction: "in", Media: "sticker"},
		{At: at(9, 7), Direction: "in", Body: "salah kirim", Deleted: true},
		{At: at(9, 8), Direction: "out", Account: "Andi", Body: "Harga 2,1 jt", Edited: true},
		{At: at(9, 9), Direction: "in", Body: "password: Rahasia123"},
	} {
		x.Write(m)
	}
	if err := x.Flush(); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"23/10/26 02.15 - Salinan chat dari GSI Orbit, format ekspor WhatsApp (media tidak disertakan). Chat dengan Mbak Rina · Toko Sinar lewat nomor GSI Andi.",
		"23/10/26 09.01 - Mbak Rina · Toko Sinar: Pagi pak, stok NVR 16ch ada?",
		"23/10/26 09.03 - Andi: Ada bu.",
		"Kirim Senin ya",
		"23/10/26 09.04 - Mbak Rina · Toko Sinar: <Media tidak disertakan>",
		"ini fotonya",
		"23/10/26 09.05 - Mbak Rina · Toko Sinar: PO-0812.pdf <Media tidak disertakan>",
		"23/10/26 09.06 - Mbak Rina · Toko Sinar: <Media tidak disertakan>",
		"23/10/26 09.07 - Mbak Rina · Toko Sinar: Pesan ini telah dihapus",
		"23/10/26 09.08 - Andi: Harga 2,1 jt <Pesan ini diedit>",
		"23/10/26 09.09 - Mbak Rina · Toko Sinar: password: ••••••",
		"",
	}, "\n")
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
	if x.Count != 8 {
		t.Fatalf("count %d", x.Count)
	}
	// every message line parses as a WhatsApp export line ("dd/mm/yy hh.mm - name: text")
	line := regexp.MustCompile(`^\d{2}/\d{2}/\d{2} \d{2}\.\d{2} - [^:]+: `)
	count := 0
	for _, l := range strings.Split(b.String(), "\n") {
		if line.MatchString(l) {
			count++
		}
	}
	if count != 8 {
		t.Fatalf("%d message lines parse, want 8", count)
	}
}

// In a group the member's name is written, else their number; what a GSI number sent carries that number's name.
func TestWriterGroupSenders(t *testing.T) {
	var b bytes.Buffer
	x := NewWriter(&b, Conv{Kind: "group", Title: "Installer Semarang", Account: "Andi"}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	at := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC) // 10.00 WIB
	x.Write(Msg{At: at, Direction: "in", FromName: "Budi Teknik", FromNum: "62811"})
	x.Write(Msg{At: at, Direction: "in", FromNum: "62812", Body: "siap"})
	x.Write(Msg{At: at, Direction: "out", Account: "Dewi", Body: "ok"})
	_ = x.Flush()
	got := strings.Split(b.String(), "\n")[1:4]
	want := []string{"01/10/26 10.00 - Budi Teknik: ", "01/10/26 10.00 - +62812: siap", "01/10/26 10.00 - Dewi: ok"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: %q want %q", i, got[i], want[i])
		}
	}
}

func TestNames(t *testing.T) {
	for _, c := range []struct{ title, nick, kind, jid, want string }{
		{"Toko Sinar", "Mbak Rina", "dealer", "", "Mbak Rina - Toko Sinar"},
		{"Mbak Rina · Toko Sinar", "rina", "dealer", "", "Mbak Rina · Toko Sinar"},
		{"", "", "new", "628123@s.whatsapp.net", "+628123"},
		{"", "", "group", "1203@g.us", "Grup"},
	} {
		if got := displayTitle(c.title, c.nick, c.kind, c.jid); got != c.want {
			t.Errorf("displayTitle(%q, %q) = %q, want %q", c.title, c.nick, got, c.want)
		}
	}
	if got := FileName(Conv{Title: `PT A/B: "Proyek" <1>`}); got != "Chat WhatsApp dengan PT A B Proyek 1.txt" {
		t.Errorf("FileName %q", got)
	}
	used := map[string]bool{}
	a, b := entryName(Conv{Title: "Rina", Account: "Andi"}, used), entryName(Conv{Title: "rina", Account: "Andi"}, used)
	if a != "Andi/Chat WhatsApp dengan Rina.txt" || b != "Andi/Chat WhatsApp dengan rina (2).txt" {
		t.Errorf("entry names %q %q", a, b)
	}
	if DownloadName("chat-2026-10.zip") != "Cadangan chat GSI Orbit 2026-10.zip" {
		t.Error(DownloadName("chat-2026-10.zip"))
	}
	if _, ok := Path(t.TempDir(), "../etc/passwd"); ok {
		t.Error("path outside the backups")
	}
}
