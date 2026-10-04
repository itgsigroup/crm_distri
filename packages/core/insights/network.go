package insights

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"arc/packages/core/domain"
)

// NetSales is a sales WhatsApp number node.
type NetSales struct {
	ID, UserID, Name, Branch, No string
}

// NetContact is an external contact node.
type NetContact struct {
	ID, Name, Role, Account, AccountName string
	Health                               *int
	Decision                             bool
	Tag                                  string
}

// NetworkGraph is the WhatsApp connection graph for a period of k months (30·k days).
type NetworkGraph struct {
	Months   []string
	Period   int
	Sales    []NetSales
	Contacts []NetContact
	Edges    [][3]any // salesID, contactID, messages
	Monthly  map[string][]int
	Pairs    []NetPair
	Insights []NetInsight
	Count    [2]int
}

// NetPair is one of the strongest pairs.
type NetPair struct {
	A, B, Account string
	W             int
}

// NetInsight is a deterministic pattern read from the graph.
type NetInsight struct {
	K, Icon, T, S string
}

// PeriodLabel renders "30 hari" etc.
func PeriodLabel(k int) string { return fmt.Sprintf("%d hari", k*30) }

// Network builds the graph. salesFilter narrows pairs/insights/count only;
// account restricts contacts to one account (mini map on the account page).
// Internal numbers are always excluded.
func (s *Service) Network(ctx context.Context, period int, salesFilter, account string) (NetworkGraph, error) {
	if period != 1 && period != 2 && period != 3 && period != 6 {
		period = 1
	}
	now := domain.Now()
	g := NetworkGraph{Period: period, Monthly: map[string][]int{}}
	for i := 5; i >= 0; i-- {
		g.Months = append(g.Months, domain.MonthName(now.AddDate(0, -i, 0).Month()))
	}
	rows, err := s.DB.Pool.Query(ctx, `SELECT 's-'||u.id, u.id, u.name, u.branch, COALESCE(NULLIF(w.phone,''), COALESCE(u.wa_numbers[1],''))
		FROM users u LEFT JOIN wa_sessions w ON w.user_id=u.id WHERE u.role='sales'
		ORDER BY CASE u.branch WHEN 'Semarang' THEN 1 WHEN 'Yogyakarta' THEN 2 WHEN 'Surabaya' THEN 3 ELSE 4 END`)
	if err != nil {
		return g, err
	}
	salesByUser := map[string]string{}
	for rows.Next() {
		var sv NetSales
		if err := rows.Scan(&sv.ID, &sv.UserID, &sv.Name, &sv.Branch, &sv.No); err != nil {
			rows.Close()
			return g, err
		}
		sv.No = maskPhone(sv.No)
		salesByUser[sv.UserID] = sv.ID
		g.Sales = append(g.Sales, sv)
	}
	rows.Close()

	// Per (user, person, month-bucket) message counts over the last 180 days.
	type key struct{ u, p string }
	edgeAll := map[key][6]int{}
	rows, err = s.DB.Pool.Query(ctx, `SELECT u, p, LEAST(5, FLOOR(EXTRACT(EPOCH FROM ($1 - i.occurred_at))/86400/30))::int AS b, sum(i.message_count)
		FROM interactions i, unnest(i.user_ids) u, unnest(i.person_ids) p
		WHERE i.channel IN ('wa_aggregate','wa_message','wa_group_message') AND i.occurred_at >= $1 - interval '180 days' AND i.occurred_at <= $1 + interval '1 day'
		GROUP BY 1,2,3`, now)
	if err != nil {
		return g, err
	}
	for rows.Next() {
		var u, p string
		var b, n int
		if err := rows.Scan(&u, &p, &b, &n); err != nil {
			rows.Close()
			return g, err
		}
		if b < 0 {
			b = 0
		}
		k := key{u, p}
		arr := edgeAll[k]
		arr[5-b] += n
		edgeAll[k] = arr
	}
	rows.Close()

	persons := map[string]bool{}
	for k := range edgeAll {
		persons[k.p] = true
	}
	cond := "TRUE"
	args := []any{}
	if account != "" {
		cond = "p.account_id = $1"
		args = append(args, account)
	}
	rows, err = s.DB.Pool.Query(ctx, `SELECT p.id, p.name, p.role, COALESCE(p.account_id,''), COALESCE(a.name,''), p.is_decision, p.stakeholder_tag, cardinality(p.phones) > 0,
		(SELECT o.health FROM opportunities o WHERE o.account_id=p.account_id AND o.status='open' AND NOT o.historical AND o.health IS NOT NULL ORDER BY o.expected_revenue DESC LIMIT 1)
		FROM people p LEFT JOIN accounts a ON a.id=p.account_id WHERE NOT p.is_internal AND `+cond+` ORDER BY p.created_at`, args...)
	if err != nil {
		return g, err
	}
	cids := map[string]bool{}
	for rows.Next() {
		var c NetContact
		var hasPhone bool
		if err := rows.Scan(&c.ID, &c.Name, &c.Role, &c.Account, &c.AccountName, &c.Decision, &c.Tag, &hasPhone, &c.Health); err != nil {
			rows.Close()
			return g, err
		}
		if !persons[c.ID] && !(c.Decision && hasPhone && c.Health != nil) {
			continue
		}
		cids[c.ID] = true
		g.Contacts = append(g.Contacts, c)
	}
	rows.Close()

	contactByID := map[string]NetContact{}
	for _, c := range g.Contacts {
		contactByID[c.ID] = c
		g.Monthly[c.ID] = make([]int, 6)
	}
	var keys []key
	for k := range edgeAll {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].u != keys[j].u {
			return keys[i].u < keys[j].u
		}
		return keys[i].p < keys[j].p
	})
	for _, k := range keys {
		sid, ok := salesByUser[k.u]
		if !ok || !cids[k.p] {
			continue
		}
		arr := edgeAll[k]
		w := 0
		for i := 6 - period; i < 6; i++ {
			w += arr[i]
		}
		for i := 0; i < 6; i++ {
			g.Monthly[k.p][i] += arr[i]
		}
		g.Edges = append(g.Edges, [3]any{sid, k.p, w})
	}

	name := func(id string) string {
		for _, sv := range g.Sales {
			if sv.ID == id {
				return sv.Name
			}
		}
		if c, ok := contactByID[id]; ok {
			return c.Name
		}
		return id
	}
	inScope := func(sid string) bool { return salesFilter == "" || salesFilter == "all" || sid == salesFilter }
	var scope [][3]any
	for _, e := range g.Edges {
		if inScope(e[0].(string)) && e[2].(int) > 0 {
			scope = append(scope, e)
		}
	}
	total := 0
	for _, e := range scope {
		total += e[2].(int)
	}
	g.Count = [2]int{len(scope), total}
	top := append([][3]any{}, scope...)
	sort.SliceStable(top, func(i, j int) bool { return top[i][2].(int) > top[j][2].(int) })
	if len(top) > 6 {
		top = top[:6]
	}
	for _, e := range top {
		c := contactByID[e[1].(string)]
		label := c.AccountName
		if label == "" {
			label = c.Role
		}
		g.Pairs = append(g.Pairs, NetPair{A: e[0].(string), B: e[1].(string), Account: label, W: e[2].(int)})
	}

	// Patterns (ported from the mockup's renderNetPanels).
	single := s.PolicyFloat(ctx, "single_thread_share", 80) / 100
	accTot := map[string]int{}
	accTop := map[string][3]any{}
	for _, e := range scope {
		c := contactByID[e[1].(string)]
		if c.Account == "" || c.Health == nil {
			continue
		}
		accTot[c.Account] += e[2].(int)
		if t, ok := accTop[c.Account]; !ok || e[2].(int) > t[2].(int) {
			accTop[c.Account] = e
		}
	}
	accOrder := []string{}
	seen := map[string]bool{}
	for _, c := range g.Contacts {
		if c.Account != "" && !seen[c.Account] {
			seen[c.Account] = true
			accOrder = append(accOrder, c.Account)
		}
	}
	for _, acc := range accOrder {
		tot := accTot[acc]
		if tot < 20 {
			continue
		}
		te := accTop[acc]
		share := float64(te[2].(int)) / float64(tot)
		if share >= single {
			c := contactByID[te[1].(string)]
			extra := ""
			if c.Tag == "ghost" {
				extra = " — dan orang itu sedang mutasi"
			}
			g.Insights = append(g.Insights, NetInsight{"warn", "i-people", fmt.Sprintf("%s: %d%% pesan lewat %s", c.AccountName, int(math.Round(share*100)), c.Name),
				fmt.Sprintf("Hubungan bertumpu satu orang%s. Pola ini kalah 3,1× lebih sering.", extra)})
		}
	}
	salesTouchesAccount := func(acc string) bool {
		if salesFilter == "" || salesFilter == "all" {
			return true
		}
		for _, e := range g.Edges {
			if e[0] == salesFilter && contactByID[e[1].(string)].Account == acc {
				return true
			}
		}
		return false
	}
	for _, c := range g.Contacts {
		if !c.Decision || c.Account == "" || c.Health == nil {
			continue
		}
		w := 0
		for _, e := range scope {
			if e[1] == c.ID {
				w += e[2].(int)
			}
		}
		if w < 5 && salesTouchesAccount(c.Account) {
			path := "tanpa jalur"
			if w > 0 {
				path = fmt.Sprintf("%d pesan", w)
			}
			g.Insights = append(g.Insights, NetInsight{"bad", "i-user-x", fmt.Sprintf("%s (%s) — %s · %s", c.Name, c.Role, path, c.AccountName),
				"Pengambil keputusan tidak berada di jaringan WA siapa pun. Butuh perkenalan lewat kontak yang ada."})
		}
	}
	multi := map[string][][2]any{}
	var multiOrder []string
	for _, e := range g.Edges {
		if e[2].(int) == 0 {
			continue
		}
		b := e[1].(string)
		if _, ok := multi[b]; !ok {
			multiOrder = append(multiOrder, b)
		}
		multi[b] = append(multi[b], [2]any{e[0], e[2]})
	}
	for _, b := range multiOrder {
		v := multi[b]
		if len(v) < 2 {
			continue
		}
		touched := salesFilter == "" || salesFilter == "all"
		parts := []string{}
		for _, x := range v {
			if x[0] == salesFilter {
				touched = true
			}
			parts = append(parts, fmt.Sprintf("%s (%d)", name(x[0].(string)), x[1].(int)))
		}
		if touched {
			g.Insights = append(g.Insights, NetInsight{"accent", "i-net", fmt.Sprintf("%s dihubungi %s", name(b), strings.Join(parts, " dan ")),
				"Dua sales menyentuh kontak yang sama — pastikan satu pesan, satu pemilik."})
		}
	}
	for _, sv := range g.Sales {
		if !inScope(sv.ID) {
			continue
		}
		var names []string
		tot := 0
		topName, topW := "", 0
		for _, e := range g.Edges {
			c := contactByID[e[1].(string)]
			if e[0] != sv.ID || c.Health != nil || e[2].(int) == 0 {
				continue
			}
			names = append(names, c.Name)
			tot += e[2].(int)
			if e[2].(int) > topW {
				topName, topW = c.Name, e[2].(int)
			}
		}
		if len(names) >= 2 {
			g.Insights = append(g.Insights, NetInsight{"neutral", "i-flag", fmt.Sprintf("%d kontak %s tanpa opportunity di Odoo", len(names), sv.Name),
				fmt.Sprintf("%s — %d pesan %s. ARC menyarankan membuat lead untuk %s (%d pesan).", strings.Join(names, ", "), tot, PeriodLabel(period), topName, topW)})
		}
	}
	if len(g.Insights) > 6 {
		g.Insights = g.Insights[:6]
	}
	return g, nil
}

func maskPhone(p string) string {
	n := domain.NormalizePhone(p)
	if len(n) < 9 {
		return p
	}
	return fmt.Sprintf("+62 %s-••••-%s", n[2:5], n[len(n)-4:])
}

// MaskPhone is exported for API views.
func MaskPhone(p string) string { return maskPhone(p) }
