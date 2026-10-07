package importer

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ServiceAccount is the JSON key of a Google service account (role BigQuery Data Viewer + Job User).
type ServiceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// ParseServiceAccount validates a key file.
func ParseServiceAccount(raw []byte) (ServiceAccount, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return sa, errors.New("bukan file JSON service account Google")
	}
	if sa.Type != "service_account" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return sa, errors.New("file JSON harus kunci service account (type, client_email, private_key)")
	}
	if _, err := sa.key(); err != nil {
		return sa, err
	}
	return sa, nil
}

func (sa ServiceAccount) key() (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("private_key service account tidak terbaca")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private_key: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private_key bukan RSA")
	}
	return rk, nil
}

// BigQuery runs the configured read-only queries through the BigQuery REST API (jobs.query + getQueryResults).
type BigQuery struct {
	SA       ServiceAccount
	Project  string // billing project for the query jobs (default: the key's project)
	Location string // e.g. asia-southeast2 (optional)
	BaseURL  string // tests; default https://bigquery.googleapis.com
	HTTP     *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
}

const bqScope = "https://www.googleapis.com/auth/bigquery.readonly"

func (b *BigQuery) client() *http.Client {
	if b.HTTP != nil {
		return b.HTTP
	}
	return &http.Client{Timeout: 120 * time.Second}
}

// accessToken signs a JWT with the service account and exchanges it (cached until it expires).
func (b *BigQuery) accessToken(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.token != "" && time.Until(b.exp) > time.Minute {
		return b.token, nil
	}
	key, err := b.SA.key()
	if err != nil {
		return "", err
	}
	aud := b.SA.TokenURI
	if aud == "" {
		aud = "https://oauth2.googleapis.com/token"
	}
	now := time.Now()
	enc := func(v any) string {
		j, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(j)
	}
	unsigned := enc(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + enc(map[string]any{
		"iss": b.SA.ClientEmail, "scope": bqScope, "aud": aud, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, aud, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := b.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("token Google: %w", err)
	}
	defer res.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error_description"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != 200 || out.AccessToken == "" {
		return "", fmt.Errorf("token Google ditolak (%d): %s", res.StatusCode, out.Error)
	}
	b.token, b.exp = out.AccessToken, now.Add(time.Duration(out.ExpiresIn)*time.Second)
	return b.token, nil
}

type bqField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type bqResult struct {
	JobComplete bool `json:"jobComplete"`
	JobRef      struct {
		JobID    string `json:"jobId"`
		Location string `json:"location"`
	} `json:"jobReference"`
	Schema struct {
		Fields []bqField `json:"fields"`
	} `json:"schema"`
	Rows []struct {
		F []struct {
			V any `json:"v"`
		} `json:"f"`
	} `json:"rows"`
	PageToken string `json:"pageToken"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (b *BigQuery) call(ctx context.Context, method, path string, body any, out any) error {
	tok, err := b.accessToken(ctx)
	if err != nil {
		return err
	}
	base := b.BaseURL
	if base == "" {
		base = "https://bigquery.googleapis.com"
	}
	var rd io.Reader
	if body != nil {
		j, _ := json.Marshal(body)
		rd = bytes.NewReader(j)
	}
	req, _ := http.NewRequestWithContext(ctx, method, base+path, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := b.client().Do(req)
	if err != nil {
		return fmt.Errorf("BigQuery: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 256<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("BigQuery %d: %s", res.StatusCode, e.Error.Message)
	}
	return json.Unmarshal(raw, out)
}

func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		j, _ := json.Marshal(x)
		return string(j)
	}
}

// Query runs one standard-SQL query and returns its rows keyed by column name (lower case).
func (b *BigQuery) Query(ctx context.Context, sql string) ([]Row, error) {
	project := b.Project
	if project == "" {
		project = b.SA.ProjectID
	}
	if project == "" {
		return nil, errors.New("project BigQuery belum diisi")
	}
	req := map[string]any{"query": sql, "useLegacySql": false, "timeoutMs": 60000, "maxResults": 10000}
	if b.Location != "" {
		req["location"] = b.Location
	}
	var res bqResult
	if err := b.call(ctx, http.MethodPost, "/bigquery/v2/projects/"+url.PathEscape(project)+"/queries", req, &res); err != nil {
		return nil, err
	}
	var out []Row
	loc := b.Location
	for {
		if res.JobRef.Location != "" {
			loc = res.JobRef.Location
		}
		if res.Error != nil {
			return nil, fmt.Errorf("BigQuery: %s", res.Error.Message)
		}
		if res.JobComplete {
			names := make([]string, len(res.Schema.Fields))
			for i, f := range res.Schema.Fields {
				names[i] = strings.ToLower(f.Name)
			}
			for _, r := range res.Rows {
				row := Row{}
				for i, c := range r.F {
					if i < len(names) {
						row[names[i]] = cell(c.V)
					}
				}
				out = append(out, row)
			}
			if res.PageToken == "" {
				return out, nil
			}
		} else {
			time.Sleep(time.Second)
		}
		q := url.Values{"maxResults": {"10000"}, "timeoutMs": {"60000"}}
		if res.PageToken != "" {
			q.Set("pageToken", res.PageToken)
		}
		if loc != "" {
			q.Set("location", loc)
		}
		job := res.JobRef.JobID
		res = bqResult{}
		if err := b.call(ctx, http.MethodGet, "/bigquery/v2/projects/"+url.PathEscape(project)+"/queries/"+url.PathEscape(job)+"?"+q.Encode(), nil, &res); err != nil {
			return nil, err
		}
		res.JobRef.JobID = job
	}
}
