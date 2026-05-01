package goparser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSymbols_SelfParse(t *testing.T) {
	// Parse this package's own parser.go as a known fixture.
	symbols, err := ExtractSymbols("parser.go")
	require.NoError(t, err)
	require.NotEmpty(t, symbols)

	names := map[string]string{}
	for _, s := range symbols {
		names[s.Name] = s.Kind
	}
	assert.Equal(t, "type", names["Symbol"])
	assert.Equal(t, "function", names["ExtractSymbols"])
	assert.Equal(t, "function", names["SymbolsForHunk"])
	assert.Equal(t, "function", names["IsGoFile"])
	assert.Equal(t, "function", names["receiverTypeName"])
}

func TestExtractSymbols_MethodReceiver(t *testing.T) {
	src := `package example
type Foo struct{}
func (f *Foo) Bar() {}
func (f Foo) Baz() {}
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "example.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	symbols, err := ExtractSymbols(path)
	require.NoError(t, err)

	var methods []Symbol
	for _, s := range symbols {
		if s.Kind == "method" {
			methods = append(methods, s)
		}
	}
	require.Len(t, methods, 2)
	assert.Equal(t, "Foo", methods[0].Receiver)
	assert.Equal(t, "Bar", methods[0].Name)
	assert.Equal(t, "Foo", methods[1].Receiver)
	assert.Equal(t, "Baz", methods[1].Name)
}

func TestExtractSymbols_Interface(t *testing.T) {
	src := `package example
type Doer interface {
	Do()
}
type Config struct {
	Name string
}
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "types.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	symbols, err := ExtractSymbols(path)
	require.NoError(t, err)

	names := map[string]string{}
	for _, s := range symbols {
		names[s.Name] = s.Kind
	}
	assert.Equal(t, "interface", names["Doer"])
	assert.Equal(t, "type", names["Config"])
}

func TestSymbolsForHunk(t *testing.T) {
	symbols := []Symbol{
		{Name: "Foo", Kind: "function", StartLine: 10, EndLine: 20},
		{Name: "Bar", Kind: "function", StartLine: 25, EndLine: 35},
		{Name: "Baz", Kind: "function", StartLine: 40, EndLine: 50},
	}

	matched := SymbolsForHunk(symbols, 18, 10) // lines 18-27
	require.Len(t, matched, 2)
	assert.Equal(t, "Foo", matched[0].Name)
	assert.Equal(t, "Bar", matched[1].Name)
}

func TestSymbolsForHunk_NoOverlap(t *testing.T) {
	symbols := []Symbol{
		{Name: "Foo", Kind: "function", StartLine: 10, EndLine: 20},
	}
	matched := SymbolsForHunk(symbols, 25, 5)
	assert.Empty(t, matched)
}

func TestIsGoFile(t *testing.T) {
	assert.True(t, IsGoFile("main.go"))
	assert.True(t, IsGoFile("internal/auth/token.go"))
	assert.False(t, IsGoFile("main_test.go"))
	assert.False(t, IsGoFile("readme.md"))
	assert.False(t, IsGoFile("script.py"))
}
