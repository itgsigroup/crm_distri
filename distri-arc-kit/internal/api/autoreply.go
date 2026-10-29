package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/autoreply"
	"distri-arc/internal/clock"
	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// Pengaturan → Balas otomatis (ADR 0066): while a sales is on a break, fact questions get the AI Trigger answer and
// the rest the approved template. Anyone with Pengaturan reads it; only the CEO turns it on and approves the template.
func (s *Server) autoReplyRoutes(r chi.Router) {
	r.Get("/auto-reply", s.autoReplyInfo)
	r.Put("/auto-reply", s.autoReplySave)
}

type autoReplyNumber struct {
	WANumber string `json:"wa_number"`
	Label    string `json:"label"`
	Holder   string `json:"holder"`
	Team     string `json:"team"`
	State    string `json:"state"`
}

func (s *Server) autoReplyInfo(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !can(u, "conn") {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Balas otomatis untuk pemegang menu Pengaturan")
		return
	}
	ctx := r.Context()
	set := autoreply.Load(ctx, s.st.Q)
	nums := []autoReplyNumber{}
	rows, err := s.st.Pool.Query(ctx, `select n.wa_number, coalesce(n.label, ''), coalesce(u.name, ''), n.team, n.state
		from wa_numbers n left join users u on u.id = n.user_id order by n.team, coalesce(u.name, n.label, n.wa_number)`)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var n autoReplyNumber
		if err := rows.Scan(&n.WANumber, &n.Label, &n.Holder, &n.Team, &n.State); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		nums = append(nums, n)
	}
	recent, err := s.st.Q.RecentAutoReplies(ctx, 30)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	now := s.clock.Now()
	every := set
	every.Numbers = nil
	_, end, active := every.Active(now, "")
	res := map[string]any{"settings": set, "numbers": nums, "recent": nonNil(recent), "ai_ready": s.replyAI.HasModel(ctx), "lyra_ready": s.hermes().LoadConfig(ctx).Configured,
		"active_now": active, "can_edit": deref(u.Role) == "ceo", "send_from": autoreply.SendFrom, "send_to": autoreply.SendTo,
		"max_facts": autoreply.MaxFactsPer, "human_quiet_min": int(autoreply.HumanQuiet.Minutes()),
		"preview": autoreply.Fill(set.Template, "Toko Sinar Jaya", "13.00", "siang")}
	if active {
		res["active_until"] = end.In(clock.WIB).Format("15.04")
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (s *Server) autoReplySave(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var in autoreply.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	in.Template = strings.TrimSpace(in.Template)
	if err := autoreply.Validate(r.Context(), s.st.Q, in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	u, ok = s.personOf(w, r) // the approver's profile is what every automatic message is decided by
	if !ok {
		return
	}
	now := s.clock.Now()
	in.ApprovedBy, in.ApprovedProfile, in.ApprovedName, in.ApprovedAt = &u.ID, u.SalesUserID, deref(u.Name), &now
	b, _ := json.Marshal(in)
	if _, err := s.st.Q.SetPolicy(r.Context(), gen.SetPolicyParams{Key: autoreply.PolicyKey, Value: b, UpdatedBy: u.SalesUserID, UpdatedAt: now}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "auto_reply.save", "policy:"+autoreply.PolicyKey, map[string]any{"enabled": in.Enabled, "lyra": in.Lyra, "sales_style": in.SalesStyle, "breaks": in.Breaks,
		"numbers": in.Numbers, "facts": in.Facts, "template": in.Template, "by": deref(u.Email)})
	s.autoReplyInfo(w, r)
}
