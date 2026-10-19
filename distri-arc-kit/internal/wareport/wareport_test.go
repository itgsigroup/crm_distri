package wareport_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/waanalysis"
	"distri-arc/internal/wareport"
)

// Monday 19 Oct 2026, working hours 08–17 WIB (Mon–Sat).
func at(h, m int) time.Time { return time.Date(2026, 10, 19, h, m, 0, 0, clock.WIB) }

func msg(in bool, t time.Time, text string) waanalysis.Message {
	return waanalysis.Message{ID: uuid.New(), In: in, At: t, Text: text}
}

func hold(t time.Time, text string) waanalysis.Message {
	m := msg(false, t, text)
	m.Holding = true
	return m
}

func cfg() wareport.Config {
	return wareport.Config{Now: at(16, 0), Since: at(0, 0).AddDate(0, 0, -6), Days: 7, Hours: waanalysis.DefaultWorkHours(), SLAMin: 30}
}

// ADR 0050: per sales — first-response time and SLA, hanging chats and broken promises, new numbers, order requests won
// — scored and ranked, with feedback from the numbers.
func TestBuild(t *testing.T) {
	mitra := uuid.New()
	andi1 := waanalysis.Thread{ID: uuid.New(), Kind: "dealer", Title: "Mitra", DealerID: &mitra, DealerName: "CV Mitra Jaya", SalesName: "Andi", Messages: []waanalysis.Message{
		msg(true, at(9, 0), "Pak mau order NVR 8 channel 5 unit"),
		msg(false, at(9, 10), "Siap Pak, saya buatkan SO ya"),
		msg(true, at(10, 0), "Stok kamera 2MP ready?"),
		msg(false, at(10, 5), "Ready Pak, 40 unit"),
	}}
	andiNew := waanalysis.Thread{ID: uuid.New(), Kind: "new", Title: "+62 812-0000-1111", SalesName: "Andi", Messages: []waanalysis.Message{
		msg(true, at(11, 0), "Halo, harga DVR 4 channel berapa?"),
		msg(false, at(11, 15), "Halo Kak, Rp 650 rb"),
	}}
	dewi1 := waanalysis.Thread{ID: uuid.New(), Kind: "dealer", Title: "Graha", DealerName: "PT Graha Sentosa", SalesName: "Dewi", Messages: []waanalysis.Message{
		msg(true, at(8, 30), "Bu, invoice bulan lalu tolong kirim ulang"),
		msg(false, at(10, 30), "Baik Pak"),
		msg(true, at(11, 0), "Harga khusus untuk 20 unit bisa?"),
		hold(at(11, 5), "Saya cek dulu ya Pak"),
	}}
	dewi2 := waanalysis.Thread{ID: uuid.New(), Kind: "dealer", Title: "Nusa", DealerName: "Nusa Teknik", SalesName: "Dewi", Messages: []waanalysis.Message{
		msg(true, at(12, 0), "Barang kemarin rusak, mau retur"),
	}}
	orders := []wareport.Order{{DealerID: mitra, At: at(13, 0), Total: 12_500_000}}

	rep := wareport.Build([]waanalysis.Thread{andi1, andiNew, dewi1, dewi2}, orders, cfg())

	if len(rep.Sales) != 2 || rep.Sales[0].Name != "Andi" || rep.Sales[0].Rank != 1 || rep.Sales[1].Rank != 2 {
		t.Fatalf("ranking: %+v", rep.Sales)
	}
	andi, dewi := rep.Sales[0], rep.Sales[1]
	if andi.Requests != 3 || andi.Answered != 3 || andi.WithinSLA != 3 || andi.New != 1 || andi.NewReply == nil || *andi.NewReply != 15 {
		t.Fatalf("andi: %+v", andi)
	}
	if andi.OrderAsks != 1 || andi.OrderWon != 1 || andi.OrderValue != 12_500_000 {
		t.Fatalf("andi orders: %+v", andi)
	}
	if andi.Score <= dewi.Score || andi.Score < 90 {
		t.Fatalf("scores: andi %d dewi %d", andi.Score, dewi.Score)
	}
	// Dewi: one reply after 2 h (outside SLA), one promise kept open since 11.05, one retur waiting since 12.00
	if dewi.Requests != 3 || dewi.Answered != 1 || dewi.WithinSLA != 0 || dewi.Promises != 1 || dewi.Hanging != 1 || dewi.Waiting != 2 {
		t.Fatalf("dewi: %+v", dewi)
	}
	if len(rep.Hanging) != 2 || rep.Hanging[0].Why != "janji" || rep.Hanging[0].Sales != "Dewi" || rep.Hanging[1].Why != "menunggu" {
		t.Fatalf("hanging: %+v", rep.Hanging)
	}
	if !has(dewi.Bad, "janji") || !has(dewi.Bad, "menunggu") || !has(andi.Good, "jadi SO") || !has(andi.Good, "Tidak ada chat gantung") {
		t.Fatalf("feedback: andi %v / dewi %v", andi.Good, dewi.Bad)
	}
	if len(rep.NewNumbers) != 1 || rep.NewNumbers[0].FirstReply == nil {
		t.Fatalf("new numbers: %+v", rep.NewNumbers)
	}
	if rep.Team.Requests != 6 || rep.Team.Inbound != 6 || rep.Heat[time.Monday][9] != 1 || len(rep.Daily) != 7 {
		t.Fatalf("team %+v heat %v days %d", rep.Team, rep.Heat[time.Monday], len(rep.Daily))
	}
	if !has(rep.Insights, "Sales terbaik: Andi") {
		t.Fatalf("insights: %v", rep.Insights)
	}
}

func TestBuildEmpty(t *testing.T) {
	rep := wareport.Build(nil, nil, cfg())
	if len(rep.Sales) != 0 || rep.Team.Requests != 0 || len(rep.Insights) != 1 {
		t.Fatalf("%+v", rep)
	}
}

func has(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}
