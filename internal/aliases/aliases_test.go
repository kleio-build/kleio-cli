package aliases

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

type fakeProvider struct {
	available    bool
	completeFn   func(ctx context.Context, prompt string) (string, error)
	completeCalls int
}

func (f *fakeProvider) Available() bool { return f.available }
func (f *fakeProvider) Complete(ctx context.Context, prompt string) (string, error) {
	f.completeCalls++
	if f.completeFn != nil {
		return f.completeFn(ctx, prompt)
	}
	return "", nil
}
func (f *fakeProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	return nil, errors.New("not used")
}

func writeAliasFile(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "aliases.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExpand_StaticAliasesUnion(t *testing.T) {
	dir := t.TempDir()
	path := writeAliasFile(t, dir, `aliases:
  og: [opengraph, og-image, "og:image"]
  auth: [authentication, jwt]
`)

	exp, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := exp.Expand(context.Background(), "og")
	sort.Strings(got)
	want := []string{"og", "og-image", "og:image", "opengraph"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Expand(og) = %v, want %v", got, want)
	}
}

func TestExpand_CaseInsensitiveAnchorLookup(t *testing.T) {
	dir := t.TempDir()
	path := writeAliasFile(t, dir, `aliases:
  AUTH: [jwt, login]
`)
	exp, _ := New(path, nil)
	got := exp.Expand(context.Background(), "auth")
	if !contains(got, "jwt") || !contains(got, "login") {
		t.Fatalf("expected case-insensitive lookup, got %v", got)
	}
}

func TestExpand_AnchorAlwaysIncluded(t *testing.T) {
	exp, _ := New("", nil)
	got := exp.Expand(context.Background(), "noaliases")
	if len(got) != 1 || got[0] != "noaliases" {
		t.Fatalf("expected anchor preserved, got %v", got)
	}
}

func TestExpand_NoAliasFileIsOK(t *testing.T) {
	exp, err := New(filepath.Join(t.TempDir(), "missing.yaml"), nil)
	if err != nil {
		t.Fatalf("missing alias file should be soft error, got %v", err)
	}
	got := exp.Expand(context.Background(), "anything")
	if len(got) != 1 || got[0] != "anything" {
		t.Fatalf("got %v", got)
	}
}

func TestExpand_LLMUnionWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	path := writeAliasFile(t, dir, `aliases:
  og: [opengraph]
`)
	prov := &fakeProvider{
		available: true,
		completeFn: func(ctx context.Context, prompt string) (string, error) {
			return "og-image\nog:image\ntwitter:card", nil
		},
	}
	exp, _ := New(path, prov)
	exp.llmCachePath = filepath.Join(dir, "cache.json")

	got := exp.Expand(context.Background(), "og")
	for _, want := range []string{"og", "opengraph", "og-image", "og:image", "twitter:card"} {
		if !contains(got, want) {
			t.Errorf("expected %q in expansion, got %v", want, got)
		}
	}
	if prov.completeCalls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", prov.completeCalls)
	}
}

func TestExpand_LLMSkippedWhenProviderUnavailable(t *testing.T) {
	dir := t.TempDir()
	path := writeAliasFile(t, dir, `aliases:
  og: [opengraph]
`)
	prov := &fakeProvider{available: false}
	exp, _ := New(path, prov)
	exp.llmCachePath = filepath.Join(dir, "cache.json")

	exp.Expand(context.Background(), "og")
	if prov.completeCalls != 0 {
		t.Fatalf("expected 0 LLM calls when provider unavailable, got %d", prov.completeCalls)
	}
}

func TestExpand_LLMResponseCachedAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	prov := &fakeProvider{
		available: true,
		completeFn: func(ctx context.Context, prompt string) (string, error) {
			return "alpha\nbeta\ngamma", nil
		},
	}
	exp, _ := New("", prov)
	exp.llmCachePath = filepath.Join(dir, "cache.json")
	exp.now = func() time.Time { return time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC) }

	exp.Expand(context.Background(), "thing")
	exp.Expand(context.Background(), "thing")
	if prov.completeCalls != 1 {
		t.Fatalf("second call should hit cache, got %d completions", prov.completeCalls)
	}

	// New expander shares cache file
	exp2, _ := New("", prov)
	exp2.llmCachePath = exp.llmCachePath
	exp2.now = exp.now
	exp2.Expand(context.Background(), "thing")
	if prov.completeCalls != 1 {
		t.Fatalf("disk cache should survive expander reload, got %d completions", prov.completeCalls)
	}
}

func TestExpand_LLMCacheExpires(t *testing.T) {
	dir := t.TempDir()
	prov := &fakeProvider{
		available: true,
		completeFn: func(ctx context.Context, prompt string) (string, error) {
			return "alpha\nbeta", nil
		},
	}
	exp, _ := New("", prov)
	exp.llmCachePath = filepath.Join(dir, "cache.json")
	exp.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	exp.Expand(context.Background(), "thing")

	exp.now = func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }
	exp.Expand(context.Background(), "thing")

	if prov.completeCalls != 2 {
		t.Fatalf("expected stale cache to refresh, got %d completions", prov.completeCalls)
	}
}

func TestParseExpansionResponse_StripsBulletsAndQuotes(t *testing.T) {
	got := parseExpansionResponse("- alpha\n* beta\n1. \"gamma\"\n2) 'delta'\n")
	want := []string{"alpha", "beta", "gamma", "delta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseExpansionResponse_CapsAtFiveTerms(t *testing.T) {
	got := parseExpansionResponse("a\nb\nc\nd\ne\nf\ng")
	if len(got) != 5 {
		t.Fatalf("expected 5 terms, got %d (%v)", len(got), got)
	}
}

func TestParseExpansionResponse_DropsLongTerms(t *testing.T) {
	long := "this term is much longer than sixty characters and should be ignored entirely"
	got := parseExpansionResponse("ok\n" + long)
	if !contains(got, "ok") {
		t.Fatalf("expected short term retained, got %v", got)
	}
	if contains(got, long) {
		t.Fatalf("expected long term dropped, got %v", got)
	}
}

func TestParseExpansionResponse_EmptyResponse(t *testing.T) {
	if got := parseExpansionResponse(""); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := parseExpansionResponse("\n\n  \n"); got != nil {
		t.Fatalf("expected nil for whitespace-only, got %v", got)
	}
}

func TestExpand_NilExpanderIsSafe(t *testing.T) {
	var exp *Expander
	got := exp.Expand(context.Background(), "anchor")
	if len(got) != 1 || got[0] != "anchor" {
		t.Fatalf("nil expander should still echo anchor, got %v", got)
	}
}

func contains(s []string, want string) bool {
	for _, x := range s {
		if x == want {
			return true
		}
	}
	return false
}
