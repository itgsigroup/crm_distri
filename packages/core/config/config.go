// Package config loads ARC settings from the environment (and an optional .env file).
package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting used across stages. Empty credentials mean the
// corresponding connector runs with its mock implementation.
type Config struct {
	Env         string
	Addr        string
	PublicURL   string
	DatabaseURL string
	WebDist     string
	ClockAnchor time.Time
	EncryptKey  string

	AnthropicKey     string
	OpenAIKey        string
	OllamaURL        string
	LLMProvider      string // anthropic | openai | ollama | fake (auto: anthropic if key, else fake)
	ModelLight       string
	ModelHeavy       string
	ModelInteractive string
	ModelFallback    string

	BridgeURL          string
	BridgeSecret       string
	WACloudToken       string
	WACloudPhoneID     string
	WACloudAppSecret   string
	WACloudVerifyToken string

	TruecallerKey string
	WebSearchKey  string

	OdooURL       string
	OdooDB        string
	OdooUser      string
	OdooAPIKey    string
	OdooCompanies string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	BasecampToken     string
	BasecampAccountID string
	BasecampProjectID string

	SMTPHost        string
	SMTPPort        int
	SMTPUser        string
	SMTPPass        string
	SMTPFrom        string
	BriefRecipients string

	SchedulerEnabled  bool
	DailyCostAlertIDR float64
}

// Load reads .env (if present, searching upward from the working directory)
// then the process environment, which wins.
func Load() Config {
	loadDotEnv()
	c := Config{
		Env:         get("ARC_ENV", "development"),
		Addr:        get("ARC_ADDR", ":8000"),
		PublicURL:   get("ARC_PUBLIC_URL", "http://localhost:8000"),
		DatabaseURL: get("DATABASE_URL", "postgres://localhost:5433/arc?sslmode=disable"),
		WebDist:     get("ARC_WEB_DIST", "apps/web/dist"),
		EncryptKey:  get("ARC_ENCRYPTION_KEY", "dev-encryption-key-change-me-32b"),

		AnthropicKey:     os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:        os.Getenv("OPENAI_API_KEY"),
		OllamaURL:        get("OLLAMA_URL", ""),
		LLMProvider:      get("ARC_LLM_PROVIDER", "auto"),
		ModelLight:       get("ARC_MODEL_LIGHT", "claude-haiku-4-5"),
		ModelHeavy:       get("ARC_MODEL_HEAVY", "claude-sonnet-5"),
		ModelInteractive: get("ARC_MODEL_INTERACTIVE", "claude-sonnet-5"),
		ModelFallback:    get("ARC_MODEL_FALLBACK", "openai"),

		BridgeURL:          get("BRIDGE_URL", "http://localhost:3001"),
		BridgeSecret:       get("BRIDGE_SECRET", "dev-bridge-secret"),
		WACloudToken:       os.Getenv("WA_CLOUD_TOKEN"),
		WACloudPhoneID:     os.Getenv("WA_CLOUD_PHONE_NUMBER_ID"),
		WACloudAppSecret:   os.Getenv("WA_CLOUD_APP_SECRET"),
		WACloudVerifyToken: get("WA_CLOUD_VERIFY_TOKEN", "arc-verify"),

		TruecallerKey: os.Getenv("TRUECALLER_API_KEY"),
		WebSearchKey:  os.Getenv("WEB_SEARCH_API_KEY"),

		OdooURL:       os.Getenv("ODOO_URL"),
		OdooDB:        os.Getenv("ODOO_DB"),
		OdooUser:      os.Getenv("ODOO_USERNAME"),
		OdooAPIKey:    os.Getenv("ODOO_API_KEY"),
		OdooCompanies: os.Getenv("ODOO_COMPANY_IDS"),

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  get("GOOGLE_REDIRECT_URL", "http://localhost:8000/api/google/callback"),

		BasecampToken:     os.Getenv("BASECAMP_TOKEN"),
		BasecampAccountID: os.Getenv("BASECAMP_ACCOUNT_ID"),
		BasecampProjectID: os.Getenv("BASECAMP_PROJECT_ID"),

		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        getInt("SMTP_PORT", 587),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPass:        os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        get("SMTP_FROM", "ARC <arc@gsi.co.id>"),
		BriefRecipients: get("BRIEF_RECIPIENTS", "ceo"),

		SchedulerEnabled:  get("ARC_SCHEDULER", "on") == "on",
		DailyCostAlertIDR: float64(getInt("ARC_DAILY_COST_ALERT_IDR", 150000)),
	}
	if a := os.Getenv("ARC_CLOCK_ANCHOR"); a != "" {
		if t, err := time.Parse(time.RFC3339, a); err == nil {
			c.ClockAnchor = t
		}
	}
	return c
}

// Validate refuses to start production with development secrets or a demo clock.
func (c Config) Validate() error {
	if c.Env != "production" {
		return nil
	}
	switch {
	case c.EncryptKey == "dev-encryption-key-change-me-32b" || len(c.EncryptKey) < 32:
		return errors.New("ARC_ENCRYPTION_KEY wajib diisi (≥ 32 karakter) di production")
	case c.BridgeSecret == "dev-bridge-secret" || len(c.BridgeSecret) < 16:
		return errors.New("BRIDGE_SECRET wajib diisi (≥ 16 karakter) di production")
	case !c.ClockAnchor.IsZero():
		return errors.New("ARC_CLOCK_ANCHOR harus kosong di production")
	case !strings.HasPrefix(c.PublicURL, "https://"):
		return errors.New("ARC_PUBLIC_URL harus https di production")
	}
	return nil
}

func get(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func getInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// RepoRoot walks up from the working directory to the folder containing go.mod.
func RepoRoot() string {
	dir, _ := os.Getwd()
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

func loadDotEnv() {
	f, err := os.Open(filepath.Join(RepoRoot(), ".env"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
}
