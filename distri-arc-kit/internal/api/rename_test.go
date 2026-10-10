package api_test

import (
	"context"
	"net/http"
	"testing"
)

// Renaming a user (Pengguna) renames their own sales profile and their Claude connections too.
func TestRenameUserCarriesToProfileAndClaude(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var id string
	_ = st.Pool.QueryRow(ctx, "select id from users where email = 'sam@gsi.co.id'").Scan(&id)
	if _, err := st.Pool.Exec(ctx, "insert into mcp_clients (name, kind, scopes, user_id) values ('Claude · Sam Setiadi', 'oauth', '{read}', $1)", id); err != nil {
		t.Fatal(err)
	}
	if code, out := put(t, srv.URL+"/api/users/"+id, "sam@gsi.co.id", map[string]any{"name": "Admin"}); code != http.StatusNoContent {
		t.Fatalf("rename: %d %v", code, out)
	}
	var user, profile, client string
	_ = st.Pool.QueryRow(ctx, "select u.name, coalesce(s.name, '') from users u left join sales_users s on s.id = u.sales_user_id where u.id = $1", id).Scan(&user, &profile)
	_ = st.Pool.QueryRow(ctx, "select name from mcp_clients where user_id = $1", id).Scan(&client)
	if user != "Admin" || profile != "Admin" || client != "Claude · Admin" {
		t.Fatalf("after rename: user %q, profile %q, Claude %q", user, profile, client)
	}
}
