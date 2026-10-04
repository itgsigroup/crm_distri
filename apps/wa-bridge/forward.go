package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// sign returns the hex HMAC-SHA256 of body.
func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func verify(secret string, body []byte, sig string) bool {
	sig = strings.TrimPrefix(sig, "sha256=")
	return hmac.Equal([]byte(sign(secret, body)), []byte(sig))
}

// forwarder delivers payloads to POST {API}/webhooks/wa. When the API is down
// the payload is appended to a JSONL file queue and retried, so no event is
// lost; the API deduplicates by wamid, so retries are idempotent.
type forwarder struct {
	cfg       config
	http      *http.Client
	mu        sync.Mutex
	queuePath string
	lastEvent time.Time
}

func newForwarder(cfg config) *forwarder {
	return &forwarder{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}, queuePath: filepath.Join(cfg.DataDir, "queue.jsonl")}
}

func (f *forwarder) post(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.cfg.APIURL+"/webhooks/wa", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ARC-Signature", sign(f.cfg.Secret, body))
	resp, err := f.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("api HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

// send forwards a payload ({"event":…}, {"events":[…]} or {"status":…}).
func (f *forwarder) send(ctx context.Context, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal payload", "err", err)
		return
	}
	f.mu.Lock()
	f.lastEvent = time.Now()
	f.mu.Unlock()
	if err := f.post(ctx, body); err != nil {
		slog.Warn("forward failed, queued", "err", err)
		f.enqueue(body)
	}
}

func (f *forwarder) enqueue(body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fh, err := os.OpenFile(f.queuePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		slog.Error("queue open", "err", err)
		return
	}
	defer fh.Close()
	_, _ = fh.Write(append(body, '\n'))
}

// queueLen returns the number of pending payloads.
func (f *forwarder) queueLen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := os.ReadFile(f.queuePath)
	if err != nil {
		return 0
	}
	return bytes.Count(data, []byte{'\n'})
}

// flush retries queued payloads in order; anything still failing stays queued.
func (f *forwarder) flush(ctx context.Context) {
	f.mu.Lock()
	data, err := os.ReadFile(f.queuePath)
	if err != nil || len(data) == 0 {
		f.mu.Unlock()
		return
	}
	_ = os.Remove(f.queuePath)
	f.mu.Unlock()

	var failed [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		if len(line) == 0 {
			continue
		}
		if len(failed) > 0 {
			failed = append(failed, line) // keep order once the API fails again
			continue
		}
		if err := f.post(ctx, line); err != nil {
			failed = append(failed, line)
		}
	}
	for _, line := range failed {
		f.enqueue(line)
	}
	if len(failed) == 0 {
		slog.Info("queue flushed")
	}
}

func (f *forwarder) retryLoop(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.flush(ctx)
		}
	}
}

// actionApproved asks the API whether action_id is approved by a human.
func (f *forwarder) actionApproved(ctx context.Context, actionID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.cfg.APIURL+"/bridge/actions/"+actionID, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-ARC-Signature", sign(f.cfg.Secret, []byte(actionID)))
	resp, err := f.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var out struct {
		Approved bool `json:"approved"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Approved, nil
}
