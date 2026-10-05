// Package config reads the process configuration from the environment (.env in development).
package config

import (
	"bufio"
	"os"
	"strings"
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
	}
}

// IsDev reports whether development-only shortcuts (X-Dev-User auth, pinned clock) are allowed.
func (c Config) IsDev() bool { return c.Env == "dev" }

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
