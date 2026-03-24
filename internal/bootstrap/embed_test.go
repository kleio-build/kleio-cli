package bootstrap

import (
	"io/fs"
	"testing"
)

func TestTemplateFS_hasAGENTS(t *testing.T) {
	fsys, err := TemplateFS()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(fsys, "AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 20 {
		t.Fatalf("AGENTS.md too short: %d", len(b))
	}
}

func TestTemplateFS_cursorRuleEmbedded(t *testing.T) {
	fsys, err := TemplateFS()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(fsys, "cursor/rules/kleio-mcp.mdc")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 10 {
		t.Fatalf("cursor/rules/kleio-mcp.mdc too short: %d", len(b))
	}
}
