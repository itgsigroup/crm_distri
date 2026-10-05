// Package db embeds the goose migrations and the seed data so the single `arc` binary can migrate and seed
// without the repository checked out.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS

//go:embed seed/*.json
var Seed embed.FS
