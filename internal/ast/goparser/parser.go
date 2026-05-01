// Package goparser extracts top-level Go symbols from .go files using
// the standard library's go/ast package. No external dependencies.
package goparser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// Symbol represents a top-level declaration extracted from a Go file.
type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // function, method, type, interface
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Receiver  string `json:"receiver,omitempty"` // method receiver type
}

// ExtractSymbols parses a Go source file and returns its top-level symbols.
func ExtractSymbols(filePath string) ([]Symbol, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var symbols []Symbol
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := Symbol{
				Name:      d.Name.Name,
				StartLine: fset.Position(d.Pos()).Line,
				EndLine:   fset.Position(d.End()).Line,
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = "method"
				sym.Receiver = receiverTypeName(d.Recv.List[0].Type)
			} else {
				sym.Kind = "function"
			}
			symbols = append(symbols, sym)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				sym := Symbol{
					Name:      ts.Name.Name,
					StartLine: fset.Position(ts.Pos()).Line,
					EndLine:   fset.Position(ts.End()).Line,
				}
				if _, isIface := ts.Type.(*ast.InterfaceType); isIface {
					sym.Kind = "interface"
				} else {
					sym.Kind = "type"
				}
				symbols = append(symbols, sym)
			}
		}
	}
	return symbols, nil
}

// SymbolsForHunk returns the symbols that overlap with a diff hunk
// defined by startLine and lineCount.
func SymbolsForHunk(symbols []Symbol, startLine, lineCount int) []Symbol {
	endLine := startLine + lineCount - 1
	var matched []Symbol
	for _, s := range symbols {
		if s.StartLine <= endLine && s.EndLine >= startLine {
			matched = append(matched, s)
		}
	}
	return matched
}

// IsGoFile returns true if the path ends with .go and is not a test file.
func IsGoFile(path string) bool {
	return filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go")
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return ""
	}
}
