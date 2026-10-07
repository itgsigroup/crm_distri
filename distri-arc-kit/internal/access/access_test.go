package access

import (
	"slices"
	"testing"
)

func TestNormalizeAndDerive(t *testing.T) {
	// own data: pages with everyone's data and credit release are dropped; base sales
	r := Normalize(Role{Screens: []string{"chat", "ar", "users", "dealer", "nope"}, Decide: []string{"reply", "credit_release", "x"}, Scope: "own"})
	if !slices.Equal(r.Screens, []string{"chat", "dealer"}) || !slices.Equal(r.Decide, []string{"reply"}) || r.Base != "sales" {
		t.Fatalf("own: %+v", r)
	}
	// policy rights: all data, credit release allowed, base ceo
	c := Normalize(Role{Screens: []string{"conn"}, Decide: []string{"credit_release"}, Scope: "own", Policies: true})
	if c.Scope != "all" || c.Base != "ceo" || !slices.Equal(c.Decide, []string{"credit_release"}) {
		t.Fatalf("policies: %+v", c)
	}
	if DeriveBase(Role{Scope: "all", Screens: []string{"users"}}) != "admin" || DeriveBase(Role{Scope: "all", Screens: []string{"ar"}}) != "finance" {
		t.Fatal("derive")
	}
	if o := Resolve("", "", "sales", nil, nil, "", false, false); o.Scope != "own" || !slices.Equal(o.Screens, LegacyScreens("sales")) || !o.WAAllowed {
		t.Fatalf("no role row: %+v", o)
	}
	if ScreenForPath("/api/chat/threads") != "chat" || ScreenForPath("/api/stock/push") != "" || ScreenForPath("/api/users/x") != "users" || ScreenForPath("/api/branches") != "" {
		t.Fatal("ScreenForPath")
	}
	if !ValidKey("sales-tele") || ValidKey("Sales Tele") || ValidKey("x") {
		t.Fatal("ValidKey")
	}
}
