// Package config reads the process configuration from the environment (.env in development).
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the runtime configuration shared by api, worker and ctl.
type Config struct {
	Env             string // dev | prod
	DatabaseURL     string
	DatabaseURLTest string
	APIAddr         string
	Now             string // ARC_NOW: pinned start time (RFC 3339), dev only
	LogLevel        string
	WebOrigin       string

	WATransport    string // fake | whatsmeow | cloudapi
	WASendGap      string // "20s-90s"
	WAReplyDelay   string // "2s-6s"
	WABackfillDays int
	WACloudNumber  string
	WACloudPhoneID string
	WACloudToken   string
	WACloudVerify  string
	WACloudSecret  string

	OdooMode   string // fake | rpc | off
	OdooURL    string
	OdooDB     string
	OdooUser   string
	OdooAPIKey string
	OdooWrite  bool

	LLMProvider  string // fake | anthropic
	LLMModel     string // overrides policy llm.routing.model
	AnthropicKey string
	OpenAIKey    string
	LLMIDRPerUSD float64 // cost estimate exchange rate (0 = built-in default)

	PublicURL string // public base URL (MCP endpoint shown in Pengaturan), e.g. https://distri.gsi.co.id
}

// Load reads .env (if present, without overriding real env vars) and returns the configuration.
func Load() Config {
	loadDotEnv(".env")
	return Config{
		Env:             get("APP_ENV", "dev"),
		DatabaseURL:     get("DATABASE_URL", "postgres://localhost:5432/distri_arc?sslmode=disable"),
		DatabaseURLTest: get("DATABASE_URL_TEST", ""),
		APIAddr:         get("API_ADDR", ":8080"),
		Now:             get("ARC_NOW", ""),
		LogLevel:        get("LOG_LEVEL", "info"),
		WebOrigin:       get("WEB_ORIGIN", "http://localhost:5173"),
		WATransport:     get("WA_TRANSPORT", "fake"),
		WASendGap:       get("WA_SEND_GAP", "20s-90s"),
		WAReplyDelay:    get("WA_REPLY_DELAY", "2s-6s"),
		WABackfillDays:  atoi(get("WA_BACKFILL_DAYS", "30")),
		WACloudNumber:   get("WA_CLOUD_NUMBER", ""),
		WACloudPhoneID:  get("WA_CLOUD_PHONE_ID", ""),
		WACloudToken:    get("WA_CLOUD_TOKEN", ""),
		WACloudVerify:   get("WA_CLOUD_VERIFY_TOKEN", ""),
		WACloudSecret:   get("WA_CLOUD_APP_SECRET", ""),
		OdooMode:        get("ODOO_MODE", "fake"),
		OdooURL:         get("ODOO_URL", ""),
		OdooDB:          get("ODOO_DB", ""),
		OdooUser:        get("ODOO_USER", ""),
		OdooAPIKey:      get("ODOO_API_KEY", ""),
		OdooWrite:       get("ODOO_WRITE", "false") == "true",
		LLMProvider:     get("LLM_PROVIDER", "fake"),
		LLMModel:        get("LLM_MODEL", ""),
		AnthropicKey:    get("ANTHROPIC_API_KEY", ""),
		OpenAIKey:       get("OPENAI_API_KEY", ""),
		LLMIDRPerUSD:    parseFloat(get("LLM_IDR_PER_USD", "")),
		PublicURL:       get("PUBLIC_URL", ""),
	}
}

// IsDev reports whether development-only shortcuts (X-Dev-User auth, pinned clock) are allowed.
func (c Config) IsDev() bool { return c.Env == "dev" }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Range parses "20s-90s" into two durations.
func Range(s string, lo, hi time.Duration) (time.Duration, time.Duration) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		b = a
	}
	x, err1 := time.ParseDuration(strings.TrimSpace(a))
	y, err2 := time.ParseDuration(strings.TrimSpace(b))
	if err1 != nil || err2 != nil {
		return lo, hi
	}
	return x, y
}

func get(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
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
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
