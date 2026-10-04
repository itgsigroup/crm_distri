package llm

import "testing"

// Stage 03: every Indonesian mobile format seen in WhatsApp, Odoo and e-mail is masked.
func TestMaskPhoneFormats(t *testing.T) {
	for _, p := range []string{"+62 812-3456-5521", "+62 812 3456 5521", "+6281234565521", "6281234565521", "62 812-3456-5521",
		"0812-3456-5521", "0812 3456 5521", "081234565521", "+62-812-3456-5521", "62.812.3456.5521"} {
		m := NewMasker()
		out := m.Mask("hubungi " + p + " ya")
		if out != "hubungi [PHONE_1] ya" || ContainsRawPhone(out) {
			t.Errorf("%q → %q", p, out)
		}
		if m.Unmask(out) != "hubungi "+p+" ya" {
			t.Errorf("unmask %q", p)
		}
	}
	// Non-phone numbers are left alone.
	for _, s := range []string{"Rp 2.400.000.000", "PO 25 Sep 2026", "SO-2026-0745"} {
		if out := NewMasker().Mask(s); out != s {
			t.Errorf("%q wrongly masked: %q", s, out)
		}
	}
}
