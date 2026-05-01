package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

func setupRepoWithCommits(t *testing.T, msgs ...string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")

	for i, m := range msgs {
		f := filepath.Join(dir, "f"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(f, []byte(m), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-m", m)
	}
	return dir
}

func TestIngester_NameIsStable(t *testing.T) {
	if got := New().Name(); got != "git" {
		t.Errorf("Name()=%q want git", got)
	}
}

func TestIngester_EmptyRepoListReturnsNoSignals(t *testing.T) {
	signals, err := New().Ingest(context.Background(), kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Fatalf("want 0 signals, got %d", len(signals))
	}
}

func TestIngester_EmitsOneSignalPerCommit(t *testing.T) {
	repo := setupRepoWithCommits(t,
		"initial commit",
		"feat: add KL-12 work",
		"fix: something else",
	)

	ing := New(repo)
	signals, err := ing.Ingest(context.Background(), kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 3 {
		t.Fatalf("want 3 signals, got %d: %#v", len(signals), signals)
	}
	for _, s := range signals {
		if s.SourceType != kleio.SourceTypeLocalGit {
			t.Errorf("SourceType=%q want %q", s.SourceType, kleio.SourceTypeLocalGit)
		}
		if s.Kind != kleio.SignalTypeGitCommit {
			t.Errorf("Kind=%q want %q", s.Kind, kleio.SignalTypeGitCommit)
		}
		if !strings.HasPrefix(s.SourceOffset, "commit:") {
			t.Errorf("SourceOffset=%q want commit:* prefix", s.SourceOffset)
		}
		if s.Metadata["sha"] == nil || s.Metadata["sha"].(string) == "" {
			t.Errorf("metadata.sha empty: %#v", s.Metadata)
		}
		if s.Author == "" {
			t.Errorf("Author empty: %#v", s)
		}
		if s.RepoName == "" {
			t.Errorf("RepoName empty: %#v", s)
		}
	}
}

func TestIngester_RespectsScopeSince(t *testing.T) {
	repo := setupRepoWithCommits(t, "old commit")
	future := time.Now().Add(24 * time.Hour)
	signals, err := New(repo).Ingest(context.Background(), kleio.IngestScope{Since: future})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Fatalf("want 0 signals (since in future), got %d", len(signals))
	}
}

func TestIngester_DiscoveryFnTakesPrecedence(t *testing.T) {
	repoA := setupRepoWithCommits(t, "a commit")
	repoB := setupRepoWithCommits(t, "b commit")
	ing := New(repoA)
	ing.RepoDiscoveryFn = func(_ kleio.IngestScope) ([]string, error) {
		return []string{repoB}, nil
	}
	signals, err := ing.Ingest(context.Background(), kleio.IngestScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 2 {
		t.Fatalf("want 2 signals (repoA + repoB), got %d", len(signals))
	}
}

func TestRepoNameFromRemote(t *testing.T) {
	cases := map[string]string{
		"https://github.com/kleio-build/kleio-cli.git":     "kleio-cli",
		"https://github.com/kleio-build/kleio-cli":         "kleio-cli",
		"git@github.com:kleio-build/kleio-cli.git":         "kleio-cli",
		"git@gitlab.com:owner/group/sub-repo.git":          "sub-repo",
		"":                                                  "",
		"foo": "",
	}
	for in, want := range cases {
		if got := repoNameFromRemote(in); got != want {
			t.Errorf("repoNameFromRemote(%q)=%q want %q", in, got, want)
		}
	}
}

func TestUniqueAbsDeduplicates(t *testing.T) {
	dir := t.TempDir()
	out := uniqueAbs([]string{dir, dir, dir})
	if len(out) != 1 {
		t.Fatalf("want 1 unique entry, got %d: %v", len(out), out)
	}
}

