package memory_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/memory"
)

func sig(kind, text, concl string) agents.Signal {
	return agents.Signal{ID: uuid.New(), Kind: kind, At: time.Now(), Text: text, Conclusion: concl}
}

func TestValidateRejectsSentenceWithoutSource(t *testing.T) {
	a := uuid.New()
	ok := memory.Memo{{Text: "Bayar 41 hari.", SignalIDs: []uuid.UUID{a}}}
	if err := memory.Validate(ok, []uuid.UUID{a}); err != nil {
		t.Fatal(err)
	}
	bad := append(ok, memory.Sentence{Text: "Akan order besar bulan depan."})
	if err := memory.Validate(bad, []uuid.UUID{a}); !errors.Is(err, memory.ErrNoSource) {
		t.Fatalf("no source accepted: %v", err)
	}
	if err := memory.Validate(memory.Memo{{Text: "x", SignalIDs: []uuid.UUID{uuid.New()}}}, []uuid.UUID{a}); !errors.Is(err, memory.ErrNoSource) {
		t.Fatalf("foreign source accepted: %v", err)
	}
	long := memory.Memo{{Text: strings.Repeat("kata ", 130), SignalIDs: []uuid.UUID{a}}}
	if err := memory.Validate(long, []uuid.UUID{a}); !errors.Is(err, memory.ErrTooLong) {
		t.Fatalf("long memo: %v", err)
	}
}

// The Mitra Jaya sample memo: every kept sentence gets a source of the right kind.
func TestAttributeMitra(t *testing.T) {
	so := sig("so", "Order terakhir Rp 116 jt. Lewat jadwal sejak 27 Sep (1,2× siklus order)", "")
	inv := sig("invoice", "Invoice INV/0889 Rp 46 jt · jatuh tempo 2026-09-24", "")
	tempo := sig("wa", "\"Bu, invoice yang 46 jt minggu depan ya, proyek belum cair.\"", "Sinyal: minta tempo tanpa tanggal · pola berulang 2×")
	sigs := []agents.Signal{so, inv, tempo, sig("wa", "Ada Pak Agus, siap kirim. Saya buatkan SO-nya ya.", "")}
	text := `Installer kecil dengan proyek musiman; dua bulan terakhir order turun dan satu invoice Rp 46 jt lewat 11 hari. Pak Agus minta tambah tempo "minggu depan" tanpa tanggal. Over limit / overdue (exposure 162 / 150 jt) dan lewat jadwal (30 hari dari siklus order 21). Dua masalah satu akar: proyek belum cair. Yang berhasil di dealer serupa: cicilan 2× + order kecil cash.`
	m, dropped := memory.FromText(text, sigs)
	if len(m) < 4 {
		t.Fatalf("kept %d sentences, dropped %v", len(m), dropped)
	}
	for _, s := range m {
		if len(s.SignalIDs) == 0 {
			t.Fatalf("no source: %q", s.Text)
		}
	}
	if !strings.Contains(m.Text(), "tempo") || !strings.Contains(m.Text(), "lewat jadwal") || !strings.Contains(m.Text(), "Over limit") {
		t.Fatalf("memo lost the essentials: %s", m.Text())
	}
}

func TestSplitKeepsNumbers(t *testing.T) {
	got := memory.Split("Piutang Rp 1,28 M. Siklus 1.284 selesai. Terima kasih!")
	if len(got) != 3 || got[1] != "Siklus 1.284 selesai." {
		t.Fatalf("%q", got)
	}
}
