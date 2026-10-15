package analyst

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"distri-arc/internal/llm"
)

// The model list of Pengaturan → API & model AI: the models GSI Orbit knows (Models, OpenAIModels) plus every model
// the provider's Models API returns for the saved key, each with its price per million tokens in USD when known.

// ModelEntry is one model of a provider.
type ModelEntry struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	InUSD  *float64 `json:"in_usd"`  // per million input tokens; nil: price unknown
	OutUSD *float64 `json:"out_usd"` // per million output tokens
	Live   bool     `json:"live"`    // returned by the provider for this key
}

// liveTTL: how long a provider's model list is reused (the page refreshes every 30 s); liveRetry: how long a failed
// fetch waits before the next try.
const (
	liveTTL   = time.Hour
	liveRetry = 5 * time.Minute
)

type liveEntry struct {
	at     time.Time
	models []ModelInfo
}

var (
	liveMu    sync.Mutex
	liveCache = map[string]liveEntry{}
)

// ListLive replaces the provider's Models API (tests); nil: the real one.
var ListLive func(ctx context.Context, provider, key string) ([]ModelInfo, error)

// ModelList is a provider's models: the known ones first (in their order), then the others the key can use; dated
// snapshots are left out when the model itself is listed. Without a key, or when the provider cannot be reached,
// only the known models.
func ModelList(ctx context.Context, provider, key string) []ModelEntry {
	known := Models
	if provider == FallbackOpenAI {
		known = OpenAIModels
	}
	live := cachedLive(ctx, provider, key)
	liveIDs := map[string]bool{}
	for _, m := range live {
		liveIDs[m.ID] = true
	}
	out := make([]ModelEntry, 0, len(known)+len(live))
	seen := map[string]bool{}
	for _, m := range known {
		out = append(out, entry(m, liveIDs[m.ID]))
		seen[m.ID] = true
	}
	var extra []ModelEntry
	for _, m := range live {
		base := datedSuffix.ReplaceAllString(m.ID, "")
		if seen[m.ID] || (base != m.ID && (seen[base] || liveIDs[base])) {
			continue
		}
		seen[m.ID] = true
		extra = append(extra, entry(m, true))
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].ID > extra[j].ID }) // newer versions first
	return append(out, extra...)
}

var datedSuffix = regexp.MustCompile(`-(\d{4}-\d{2}-\d{2}|\d{8})$`)

func entry(m ModelInfo, live bool) ModelEntry {
	e := ModelEntry{ID: m.ID, Label: m.Label, Live: live}
	if p, ok := llm.PriceOf(m.ID); ok {
		in, out := p.In, p.Out
		e.InUSD, e.OutUSD = &in, &out
	}
	return e
}

func cachedLive(ctx context.Context, provider, key string) []ModelInfo {
	if key == "" {
		return nil
	}
	h := sha256.Sum256([]byte(provider + ":" + key))
	k := hex.EncodeToString(h[:8])
	liveMu.Lock()
	if e, ok := liveCache[k]; ok && time.Since(e.at) < liveTTL {
		liveMu.Unlock()
		return e.models
	}
	liveMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	list := ListLive
	if list == nil {
		list = listLive
	}
	models, err := list(ctx, provider, key)
	if err != nil {
		// the known models still show; an unreachable provider is asked again after liveRetry, not on every refresh
		liveMu.Lock()
		liveCache[k] = liveEntry{at: time.Now().Add(liveRetry - liveTTL)}
		liveMu.Unlock()
		return nil
	}
	liveMu.Lock()
	liveCache[k] = liveEntry{at: time.Now(), models: models}
	liveMu.Unlock()
	return models
}

func listLive(ctx context.Context, provider, key string) ([]ModelInfo, error) {
	if provider == FallbackOpenAI {
		return listOpenAI(ctx, key)
	}
	c := anthropic.NewClient(option.WithAPIKey(key), option.WithRequestTimeout(10*time.Second), option.WithMaxRetries(1))
	pager := c.Models.ListAutoPaging(ctx, anthropic.ModelListParams{Limit: anthropic.Int(100)})
	var out []ModelInfo
	for pager.Next() {
		m := pager.Current()
		if claudeID.MatchString(m.ID) {
			out = append(out, ModelInfo{ID: m.ID, Label: m.DisplayName})
		}
	}
	return out, pager.Err()
}

// notChat: OpenAI models that are not chat models with tool calling (audio, images, embeddings, search, …).
var notChat = []string{"audio", "realtime", "transcribe", "tts", "image", "embed", "moderation", "search", "instruct",
	"codex", "computer-use", "deep-research", "o1-mini", "o1-preview", "dall-e", "whisper", "babbage", "davinci"}

func listOpenAI(ctx context.Context, key string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAIBase+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAI menjawab %s", res.Status)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	var out []ModelInfo
next:
	for _, m := range body.Data {
		if !openaiID.MatchString(m.ID) {
			continue
		}
		for _, w := range notChat {
			if strings.Contains(m.ID, w) {
				continue next
			}
		}
		out = append(out, ModelInfo{ID: m.ID, Label: openAILabel(m.ID)})
	}
	return out, nil
}

// openAILabel: "gpt-5-mini" → "GPT-5 mini", "o4-mini" stays.
func openAILabel(id string) string {
	for _, m := range OpenAIModels {
		if m.ID == id {
			return m.Label
		}
	}
	if rest, ok := strings.CutPrefix(id, "gpt-"); ok {
		parts := strings.SplitN(rest, "-", 2)
		if len(parts) == 2 {
			return "GPT-" + parts[0] + " " + strings.ReplaceAll(parts[1], "-", " ")
		}
		return "GPT-" + rest
	}
	return id
}
