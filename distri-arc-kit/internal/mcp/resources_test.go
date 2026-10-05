package mcp

import (
	"os"
	"testing"
)

// The embedded glossary must stay identical to docs/design/01-glossary.md.
func TestGlossaryInSync(t *testing.T) {
	b, err := os.ReadFile("../../docs/design/01-glossary.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != Glossary {
		t.Fatal("internal/mcp/resources/glossary.md differs from docs/design/01-glossary.md — copy it again")
	}
}
