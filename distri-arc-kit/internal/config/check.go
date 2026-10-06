package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Issue is one finding of arc ctl check-env.
type Issue struct {
	Level string // ok | warn | fail
	Key   string
	Msg   string
}

// Check validates the configuration for its environment (09-policies-security: secrets only from env). Production
// fails on a missing or weak secret, a dev shortcut left on, or an adapter switched on without its credentials.
func (c Config) Check() []Issue {
	var out []Issue
	add := func(level, key, msg string, args ...any) {
		out = append(out, Issue{level, key, fmt.Sprintf(msg, args...)})
	}
	prod := !c.IsDev()
	failProd := "warn"
	if prod {
		failProd = "fail"
	}
	if c.Env != "dev" && c.Env != "prod" {
		add("fail", "APP_ENV", "harus dev atau prod, bukan %q", c.Env)
	} else {
		add("ok", "APP_ENV", "%s", c.Env)
	}
	if u, err := url.Parse(c.DatabaseURL); err != nil || u.Scheme == "" {
		add("fail", "DATABASE_URL", "tidak valid")
	} else {
		add("ok", "DATABASE_URL", "%s@%s%s", u.User.Username(), u.Host, u.Path)
	}
	switch {
	case len(c.SessionSecret) >= 32:
		add("ok", "SESSION_SECRET", "%d karakter", len(c.SessionSecret))
	case c.SessionSecret == "":
		add(failProd, "SESSION_SECRET", "kosong (wajib ≥ 32 karakter di produksi: openssl rand -hex 32)")
	default:
		add(failProd, "SESSION_SECRET", "terlalu pendek (%d < 32)", len(c.SessionSecret))
	}
	if prod {
		if c.Now != "" {
			add("fail", "ARC_NOW", "jam tetap hanya untuk dev — kosongkan")
		}
		if os.Getenv("ARC_DEMO_PASSWORD") != "" {
			add("fail", "ARC_DEMO_PASSWORD", "akun contoh hanya untuk dev — kosongkan")
		}
		if strings.EqualFold(os.Getenv("WA_SEND_HOURS"), "off") {
			add("fail", "WA_SEND_HOURS", "jendela kirim 08–18 WIB wajib di produksi")
		}
		if u, err := url.Parse(c.PublicURL); err != nil || u.Scheme != "https" {
			add("fail", "PUBLIC_URL", "wajib https://… (Caddy + TLS)")
		} else {
			add("ok", "PUBLIC_URL", "%s", c.PublicURL)
		}
		if c.MetricsToken == "" {
			add("warn", "METRICS_TOKEN", "kosong: /metrics hanya dari localhost")
		}
		if os.Getenv("BACKUP_PASSPHRASE") == "" && os.Getenv("BACKUP_AGE_RECIPIENT") == "" {
			add("fail", "BACKUP_PASSPHRASE", "backup harian harus terenkripsi (BACKUP_PASSPHRASE atau BACKUP_AGE_RECIPIENT)")
		}
	}
	switch c.WATransport {
	case "fake":
		add(failProd, "WA_TRANSPORT", "fake: tidak ada WhatsApp sungguhan")
	case "whatsmeow":
		add("ok", "WA_TRANSPORT", "whatsmeow (linked device, pasangkan QR di Pengaturan)")
	case "cloudapi":
		for k, v := range map[string]string{"WA_CLOUD_NUMBER": c.WACloudNumber, "WA_CLOUD_PHONE_ID": c.WACloudPhoneID, "WA_CLOUD_TOKEN": c.WACloudToken,
			"WA_CLOUD_VERIFY_TOKEN": c.WACloudVerify, "WA_CLOUD_APP_SECRET": c.WACloudSecret} {
			if v == "" {
				add("fail", k, "wajib untuk WA_TRANSPORT=cloudapi")
			}
		}
		add("ok", "WA_TRANSPORT", "cloudapi")
	default:
		add("fail", "WA_TRANSPORT", "tidak dikenal: %q", c.WATransport)
	}
	switch c.OdooMode {
	case "fake":
		add(failProd, "ODOO_MODE", "fake: data dari db/seed/odoo")
	case "off":
		add("warn", "ODOO_MODE", "off: tidak ada sinkron Odoo")
	case "rpc":
		for k, v := range map[string]string{"ODOO_URL": c.OdooURL, "ODOO_DB": c.OdooDB, "ODOO_USER": c.OdooUser, "ODOO_API_KEY": c.OdooAPIKey} {
			if v == "" {
				add("fail", k, "wajib untuk ODOO_MODE=rpc")
			}
		}
		if !strings.HasPrefix(c.OdooURL, "https://") && prod {
			add("fail", "ODOO_URL", "harus https di produksi")
		}
		add("ok", "ODOO_MODE", "rpc · tulis %v", c.OdooWrite)
	default:
		add("fail", "ODOO_MODE", "tidak dikenal: %q", c.OdooMode)
	}
	switch c.LLMProvider {
	case "anthropic":
		if c.AnthropicKey == "" {
			add("fail", "ANTHROPIC_API_KEY", "wajib untuk LLM_PROVIDER=anthropic")
		} else {
			add("ok", "LLM_PROVIDER", "anthropic")
		}
		if c.OpenAIKey == "" {
			add("warn", "OPENAI_API_KEY", "kosong: tanpa cadangan saat Claude API gagal")
		}
	case "fake":
		add("warn", "LLM_PROVIDER", "fake: alasan & draft dari template agen")
	default:
		add("fail", "LLM_PROVIDER", "tidak dikenal: %q", c.LLMProvider)
	}
	return out
}
