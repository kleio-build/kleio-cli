package gitreader

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type WalkOptions struct {
	RepoPath string
	Since    time.Time
	Author   string
	Branch   string
}

// Walk opens the git repository at the given path and returns commits
// matching the provided filter options.
func Walk(opts WalkOptions) ([]Commit, error) {
	repo, err := git.PlainOpenWithOptions(opts.RepoPath, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	logOpts := &git.LogOptions{
		Order: git.LogOrderCommitterTime,
	}

	if !opts.Since.IsZero() {
		logOpts.Since = &opts.Since
	}

	if opts.Branch != "" {
		ref, err := repo.Reference(plumbing.NewBranchReferenceName(opts.Branch), true)
		if err != nil {
			return nil, fmt.Errorf("resolve branch %q: %w", opts.Branch, err)
		}
		logOpts.From = ref.Hash()
	}

	iter, err := repo.Log(logOpts)
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	headRef, _ := repo.Head()
	branchMap := buildBranchMap(repo)

	var commits []Commit
	err = iter.ForEach(func(c *object.Commit) error {
		if opts.Author != "" && !strings.EqualFold(c.Author.Email, opts.Author) {
			return nil
		}

		commit := Commit{
			Hash:      c.Hash.String(),
			Message:   c.Message,
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Timestamp: c.Author.When,
			IsMerge:   c.NumParents() > 1,
			Branch:    resolveBranch(c.Hash, branchMap, headRef),
		}

		stats, err := c.Stats()
		if err == nil {
			for _, s := range stats {
				commit.Files = append(commit.Files, s.Name)
			}
		}

		commits = append(commits, commit)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk commits: %w", err)
	}

	return commits, nil
}

func buildBranchMap(repo *git.Repository) map[plumbing.Hash]string {
	m := make(map[plumbing.Hash]string)
	branches, err := repo.Branches()
	if err != nil {
		return m
	}
	_ = branches.ForEach(func(ref *plumbing.Reference) error {
		m[ref.Hash()] = ref.Name().Short()
		return nil
	})
	return m
}

func resolveBranch(hash plumbing.Hash, branchMap map[plumbing.Hash]string, head *plumbing.Reference) string {
	if name, ok := branchMap[hash]; ok {
		return name
	}
	if head != nil {
		return head.Name().Short()
	}
	return "unknown"
}
