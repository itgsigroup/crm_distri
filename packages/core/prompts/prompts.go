// Package prompts embeds the versioned LLM prompts (packages/core/prompts/<agent>/<version>.md).
// Changing a prompt means adding a new version file and re-running `make eval`.
package prompts

import (
	"embed"
	"strings"
)

//go:embed */*.md
var files embed.FS

// Get returns the prompt body for "agent/version" (e.g. "capture/v1").
func Get(name string) string {
	b, err := files.ReadFile(name + ".md")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
