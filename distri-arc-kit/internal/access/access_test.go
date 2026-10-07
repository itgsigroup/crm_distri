package access

import (
	"slices"
	"testing"
)

func TestResolve(t *testing.T) {
	r := Resolve("gudang-medan", "Gudang Medan", "warehouse", []string{"stock", "ar", "chat"}, []string{"transfer", "credit_limit"}, false)
	if !slices.Equal(r.Screens, []string{"chat", "stock"}) || !slices.Equal(r.Decide, []string{"transfer"}) || r.WAAllowed {
		t.Fatalf("%+v", r)
	}
	if c := Resolve("ceo", "CEO", "ceo", []string{"today"}, []string{}, false); len(c.Screens) != len(BaseScreens("ceo")) || len(c.Decide) != len(BaseKinds("ceo")) || !c.WAAllowed {
		t.Fatalf("CEO narrowed: %+v", c)
	}
	if o := Resolve("", "", "sales", nil, nil, true); o.Key != "sales" || o.Name != "Sales" || !slices.Equal(o.Screens, BaseScreens("sales")) {
		t.Fatalf("no role row: %+v", o)
	}
	if ScreenForPath("/api/chat/threads") != "chat" || ScreenForPath("/api/stock/push") != "" || ScreenForPath("/api/credit/overview") != "ar" {
		t.Fatal("ScreenForPath")
	}
	if !ValidKey("sales-tele") || ValidKey("Sales Tele") || ValidKey("x") {
		t.Fatal("ValidKey")
	}
}
