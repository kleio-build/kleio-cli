package gitowner

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectOwner walks up from dir to find a .git directory, parses its config for
// the "origin" remote URL, and returns the GitHub owner segment (e.g.
// "kleio-build" from "github.com/kleio-build/kleio-app"). Returns "" if the
// directory is not inside a git repo or the origin remote is not on GitHub.
func DetectOwner(dir string) string {
	gitDir := findGitDir(dir)
	if gitDir == "" {
		return ""
	}
	url := originURL(gitDir)
	if url == "" {
		return ""
	}
	return ownerFromURL(url)
}

// findGitDir walks up from dir looking for a .git entry. If .git is a file
// (worktree/submodule), it reads the gitdir pointer; otherwise returns the
// .git directory path.
func findGitDir(dir string) string {
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate
			}
			// .git file (worktree/submodule): "gitdir: <path>"
			data, err := os.ReadFile(candidate)
			if err == nil {
				line := strings.TrimSpace(string(data))
				if after, ok := strings.CutPrefix(line, "gitdir: "); ok {
					target := strings.TrimSpace(after)
					if !filepath.IsAbs(target) {
						target = filepath.Join(dir, target)
					}
					return target
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// originURL reads the git config file and returns the URL for [remote "origin"].
func originURL(gitDir string) string {
	f, err := os.Open(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	defer f.Close()

	inOrigin := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = strings.EqualFold(line, `[remote "origin"]`)
			continue
		}
		if inOrigin {
			key, val, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(key) == "url" {
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}

var (
	sshColonRe = regexp.MustCompile(`^git@github\.com:([^/]+)/`)
	httpsRe    = regexp.MustCompile(`^https?://github\.com/([^/]+)/`)
	sshSlashRe = regexp.MustCompile(`^ssh://[^@]*@github\.com/([^/]+)/`)
)

// ownerFromURL extracts the GitHub owner from a remote URL.
func ownerFromURL(rawURL string) string {
	if m := sshColonRe.FindStringSubmatch(rawURL); len(m) > 1 {
		return m[1]
	}
	if m := httpsRe.FindStringSubmatch(rawURL); len(m) > 1 {
		return m[1]
	}
	if m := sshSlashRe.FindStringSubmatch(rawURL); len(m) > 1 {
		return m[1]
	}
	return ""
}
