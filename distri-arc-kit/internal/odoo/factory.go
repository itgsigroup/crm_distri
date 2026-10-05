package odoo

import (
	"fmt"
	"io/fs"
)

// Config selects the Odoo source (ODOO_MODE).
type Config struct {
	Mode, URL, DB, User, APIKey string
	Write                       bool
	SeedFS                      fs.FS // db.Seed, for the fake
}

// New returns the configured source, or nil when ODOO_MODE=off.
func New(c Config) (Source, error) {
	switch c.Mode {
	case "rpc":
		if c.URL == "" || c.DB == "" || c.User == "" || c.APIKey == "" {
			return nil, fmt.Errorf("ODOO_MODE=rpc needs ODOO_URL, ODOO_DB, ODOO_USER and ODOO_API_KEY")
		}
		return NewRPC(c.URL, c.DB, c.User, c.APIKey, c.Write), nil
	case "off":
		return nil, nil
	default:
		f, err := NewFake(c.SeedFS, "seed/odoo")
		if err != nil {
			return nil, err
		}
		f.Write = c.Write
		return f, nil
	}
}
