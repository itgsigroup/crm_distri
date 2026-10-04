// Package identity holds the inbound-number identification sources that are
// external services: Truecaller Business API and public web search. Getcontact
// has no official API — ARC only supports manual import (pasted tag text).
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// CallerInfo is a Truecaller result.
type CallerInfo struct {
	Name      string `json:"name"`
	SpamScore int    `json:"spam_score"`
	SpamCount int    `json:"spam_count"`
}

// Truecaller looks up a number.
type Truecaller interface {
	Lookup(ctx context.Context, phone string) (CallerInfo, error)
}

// WebResult is one public web hit.
type WebResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// WebSearch searches public sources (company sites, news).
type WebSearch interface {
	Search(ctx context.Context, query string) ([]WebResult, error)
}

// ErrNotFound is returned when a source has no data.
var ErrNotFound = errors.New("tidak ditemukan")

// TruecallerAPI calls the Truecaller Business API.
type TruecallerAPI struct {
	Key  string
	HTTP *http.Client
}

// Lookup queries the API.
func (t *TruecallerAPI) Lookup(ctx context.Context, phone string) (CallerInfo, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api4.truecaller.com/v1/search?type=4&q="+url.QueryEscape(phone), nil)
	req.Header.Set("Authorization", "Bearer "+t.Key)
	c := t.HTTP
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return CallerInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return CallerInfo{}, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return CallerInfo{}, fmt.Errorf("truecaller: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Name     string `json:"name"`
			SpamInfo struct {
				SpamScore int `json:"spamScore"`
				SpamStats struct {
					NumReports int `json:"numReports"`
				} `json:"spamStats"`
			} `json:"spamInfo"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return CallerInfo{}, err
	}
	if len(out.Data) == 0 {
		return CallerInfo{}, ErrNotFound
	}
	d := out.Data[0]
	return CallerInfo{Name: d.Name, SpamScore: d.SpamInfo.SpamScore, SpamCount: d.SpamInfo.SpamStats.NumReports}, nil
}

// SearchAPI is a generic web search (Brave Search compatible endpoint).
type SearchAPI struct {
	Key  string
	HTTP *http.Client
}

// Search runs the query.
func (s *SearchAPI) Search(ctx context.Context, q string) ([]WebResult, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.search.brave.com/res/v1/web/search?count=5&q="+url.QueryEscape(q), nil)
	req.Header.Set("X-Subscription-Token", s.Key)
	req.Header.Set("Accept", "application/json")
	c := s.HTTP
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var res []WebResult
	for _, r := range out.Web.Results {
		res = append(res, WebResult{r.Title, r.URL, r.Description})
	}
	return res, nil
}

// FakeSources serves both interfaces from a JSON fixture keyed by normalized phone / query.
type FakeSources struct {
	Callers map[string]CallerInfo  `json:"truecaller"`
	Web     map[string][]WebResult `json:"web"`
}

// LoadFake reads tests/fixtures/identity.json (missing file = empty fake).
func LoadFake(path string) *FakeSources {
	f := &FakeSources{Callers: map[string]CallerInfo{}, Web: map[string][]WebResult{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, f)
	}
	return f
}

// Lookup returns the canned caller info.
func (f *FakeSources) Lookup(_ context.Context, phone string) (CallerInfo, error) {
	if c, ok := f.Callers[phone]; ok {
		return c, nil
	}
	return CallerInfo{}, ErrNotFound
}

// Search returns canned results for queries containing a key.
func (f *FakeSources) Search(_ context.Context, q string) ([]WebResult, error) {
	for k, v := range f.Web {
		if strings.Contains(strings.ToLower(q), strings.ToLower(k)) {
			return v, nil
		}
	}
	return nil, ErrNotFound
}
