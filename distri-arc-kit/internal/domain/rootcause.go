package domain

import "strings"

// Root-cause keys of a dealer that drifts past its cycle ("akar terduga"); the UI maps them to copy.
const (
	RootProjectUnpaid     = "project_unpaid"     // proyek belum cair (minta tempo)
	RootMarketplaceModule = "marketplace_module" // harga modul vs marketplace
	RootMarketplace       = "marketplace"        // harga vs marketplace
	RootWholesaler        = "wholesaler"         // beralih ke grosir lokal · WA tak dibalas
	RootSmallShare        = "small_share"        // share of wallet kecil sejak awal
)

// RootCause reads a drifting dealer's WA signals and memo (05-agents › AI Follow-up): a tempo request means
// the project has not been paid, a marketplace mention means price, a local wholesaler or unanswered WA means
// it is switching; otherwise the share was small all along.
func RootCause(texts []string) string {
	all := strings.ToLower(strings.Join(texts, " \n "))
	switch {
	case strings.Contains(all, "belum cair") || strings.Contains(all, "minta tempo") || strings.Contains(all, "tambah tempo"):
		return RootProjectUnpaid
	case strings.Contains(all, "marketplace") && (strings.Contains(all, "modul") || strings.Contains(all, "led")):
		return RootMarketplaceModule
	case strings.Contains(all, "marketplace"):
		return RootMarketplace
	case strings.Contains(all, "grosir"):
		return RootWholesaler
	default:
		return RootSmallShare
	}
}
