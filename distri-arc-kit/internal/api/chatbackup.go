package api

import (
	"mime"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/chatbackup"
	"distri-arc/internal/httpx"
	"distri-arc/internal/wa"
)

// Cadangan chat (Sam 23 Oct, ADR 0061): like WhatsApp — "Ekspor chat" saves one chat as WhatsApp's own text export,
// and every night the month's chats are kept as one zip of those texts, which outlives the 90-day chat retention.

func (s *Server) chatBackupRoutes(r chi.Router) {
	r.Get("/chat/threads/{id}/export", s.chatExport)
	r.Get("/chat/backups", s.chatBackups)
	r.Post("/chat/backups", s.chatBackupNow)
	r.Get("/chat/backups/{name}", s.chatBackupFile)
}

// chatExport: GET /chat/threads/{id}/export — "Ekspor chat": every stored message of the chat as
// "Chat WhatsApp dengan <nama>.txt", streamed (a long group never sits in memory). Anyone who reads the chat may
// export it, as on the phone; each export is audited.
func (s *Server) chatExport(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	c, err := chatbackup.LoadConv(r.Context(), s.st.Pool, t.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	now := s.clock.Now()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": chatbackup.FileName(c)}))
	w.Header().Set("Cache-Control", "no-store")
	n, err := chatbackup.WriteConv(r.Context(), s.st.Pool, w, c, time.Time{}, now.Add(24*time.Hour), now)
	if err != nil {
		s.log.Warn("chat export", "thread", t.ID, "err", err) // headers are out: the file simply ends
		return
	}
	s.audit(r, "chat.export", "chat_thread", &t.ID, map[string]any{"messages": n, "number": wa.MaskNumber(phoneOfJID(t.WaJid))})
}

// chatBackups: GET /chat/backups — the months kept, like WhatsApp's "Cadangan chat" screen (CEO/admin).
func (s *Server) chatBackups(w http.ResponseWriter, r *http.Request) {
	if !s.backupAdmin(w, r) {
		return
	}
	files, err := chatbackup.List(s.cfg.ChatBackupDir)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": files, "keep_months": s.cfg.ChatBackupKeepMonths, "schedule": "Setiap malam 02.15 WIB", "on": s.cfg.ChatBackupDir != ""})
}

// backupNow keeps "Cadangkan" from running twice at once in this process (the files are swapped atomically anyway).
var backupNow sync.Mutex

// chatBackupNow: POST /chat/backups — "Cadangkan": this month now (and any stored month without a file yet).
func (s *Server) chatBackupNow(w http.ResponseWriter, r *http.Request) {
	if !s.backupAdmin(w, r) {
		return
	}
	if !backupNow.TryLock() {
		httpx.Fail(w, http.StatusConflict, "busy", "Pencadangan sedang berjalan — tunggu sebentar")
		return
	}
	defer backupNow.Unlock()
	start := time.Now()
	files, err := chatbackup.Run(r.Context(), s.st.Pool, s.cfg.ChatBackupDir, s.clock.Now(), s.cfg.ChatBackupKeepMonths)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Cadangan gagal: "+err.Error())
		return
	}
	chats, msgs := 0, 0
	for _, f := range files {
		chats, msgs = chats+f.Chats, msgs+f.Messages
	}
	s.audit(r, "chat.backup", "chat_backup", nil, map[string]any{"months": len(files), "messages": msgs})
	httpx.JSON(w, http.StatusOK, map[string]any{"written": files, "chats": chats, "messages": msgs, "ms": time.Since(start).Milliseconds()})
}

// chatBackupFile: GET /chat/backups/{name} — download one month's zip (CEO/admin, audited).
func (s *Server) chatBackupFile(w http.ResponseWriter, r *http.Request) {
	if !s.backupAdmin(w, r) {
		return
	}
	name := chi.URLParam(r, "name")
	p, ok := chatbackup.Path(s.cfg.ChatBackupDir, name)
	if !ok {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Cadangan tidak ditemukan")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Cadangan tidak ditemukan")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "chat.backup.download", "chat_backup", nil, map[string]any{"file": name})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": chatbackup.DownloadName(name)}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

// backupAdmin: the whole company's chats are for the CEO/admin only (pages Pengguna or Pengaturan).
func (s *Server) backupAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Cadangan chat hanya untuk CEO/admin")
		return false
	}
	if s.cfg.ChatBackupDir == "" {
		httpx.Fail(w, http.StatusConflict, "off", "Cadangan chat mati (CHAT_BACKUP_DIR kosong)")
		return false
	}
	return true
}

// forgetInBackups removes a chat from Cadangan chat (marked "Obrolan pribadi"): no copy is left there either.
func (s *Server) forgetInBackups(jid string) {
	if s.cfg.ChatBackupDir == "" {
		return
	}
	if n, err := chatbackup.Forget(s.cfg.ChatBackupDir, jid); err != nil {
		s.log.Warn("chat backup forget", "err", err)
	} else if n > 0 {
		s.log.Info("chat backup forget", "chats", n)
	}
}
