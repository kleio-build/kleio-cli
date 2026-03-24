package bootstrap

import (
	"embed"
	"io/fs"
)

//go:embed templates
var templatesRoot embed.FS

// TemplateFS returns the embedded prepackage tree (AGENTS.md, .cursor/..., etc.).
func TemplateFS() (fs.FS, error) {
	return fs.Sub(templatesRoot, "templates")
}
