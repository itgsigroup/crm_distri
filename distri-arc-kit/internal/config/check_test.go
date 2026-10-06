package config

import "testing"

func levels(c Config) map[string]string {
	out := map[string]string{}
	for _, i := range c.Check() {
		if out[i.Key] != "fail" {
			out[i.Key] = i.Level
		}
	}
	return out
}

func TestCheckProduction(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "x")
	t.Setenv("ARC_DEMO_PASSWORD", "") // make exports .env
	t.Setenv("WA_SEND_HOURS", "off")
	bad := levels(Config{Env: "prod", DatabaseURL: "postgres://arc@db/arc", SessionSecret: "short", Now: "2026-10-05T06:45+07", WATransport: "fake",
		OdooMode: "rpc", OdooURL: "http://odoo", LLMProvider: "anthropic", PublicURL: "http://x"})
	for _, k := range []string{"SESSION_SECRET", "ARC_NOW", "WA_SEND_HOURS", "WA_TRANSPORT", "ODOO_API_KEY", "ODOO_URL", "ANTHROPIC_API_KEY", "PUBLIC_URL"} {
		if bad[k] != "fail" {
			t.Errorf("%s: %q, want fail", k, bad[k])
		}
	}
	t.Setenv("WA_SEND_HOURS", "08-18")
	good := Config{Env: "prod", DatabaseURL: "postgres://arc@db/arc", SessionSecret: "0123456789abcdef0123456789abcdef", WATransport: "whatsmeow",
		OdooMode: "rpc", OdooURL: "https://odoo.gsi.co.id", OdooDB: "gsi", OdooUser: "u", OdooAPIKey: "k", LLMProvider: "anthropic", AnthropicKey: "k",
		PublicURL: "https://distri.gsi.co.id", MetricsToken: "m"}
	for _, i := range good.Check() {
		if i.Level == "fail" {
			t.Errorf("good config: %s %s", i.Key, i.Msg)
		}
	}
	if dev := levels(Config{Env: "dev", DatabaseURL: "postgres://x/y", WATransport: "fake", OdooMode: "fake", LLMProvider: "fake"}); dev["WA_TRANSPORT"] != "warn" || dev["SESSION_SECRET"] != "warn" {
		t.Errorf("dev must only warn: %v", dev)
	}
}
