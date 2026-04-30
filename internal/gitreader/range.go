package gitreader

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// CommitRange returns commits reachable from target but not from source,
// effectively computing the diff between two refs (branches, tags, SHAs,
// or relative refs like HEAD~5).
func CommitRange(repoPath, source, target string) ([]Commit, error) {
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	sourceHash, err := resolveRef(repo, source)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", source, err)
	}

	targetHash, err := resolveRef(repo, target)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", target, err)
	}

	sourceAncestors := make(map[plumbing.Hash]bool)
	sourceCommit, err := repo.CommitObject(sourceHash)
	if err != nil {
		return nil, fmt.Errorf("get source commit: %w", err)
	}
	iter := object.NewCommitPreorderIter(sourceCommit, nil, nil)
	_ = iter.ForEach(func(c *object.Commit) error {
		sourceAncestors[c.Hash] = true
		return nil
	})

	targetCommit, err := repo.CommitObject(targetHash)
	if err != nil {
		return nil, fmt.Errorf("get target commit: %w", err)
	}

	headRef, _ := repo.Head()
	branchMap := buildBranchMap(repo)

	var commits []Commit
	targetIter := object.NewCommitPreorderIter(targetCommit, nil, nil)
	_ = targetIter.ForEach(func(c *object.Commit) error {
		if sourceAncestors[c.Hash] {
			return stopIter
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

	return commits, nil
}

var stopIter = fmt.Errorf("stop")

func resolveRef(repo *git.Repository, ref string) (plumbing.Hash, error) {
	if h, err := repo.ResolveRevision(plumbing.Revision(ref)); err == nil {
		return *h, nil
	}

	if ref, err := repo.Reference(plumbing.NewBranchReferenceName(ref), true); err == nil {
		return ref.Hash(), nil
	}

	if strings.HasPrefix(ref, "v") {
		if ref, err := repo.Reference(plumbing.NewTagReferenceName(ref), true); err == nil {
			return ref.Hash(), nil
		}
	}

	return plumbing.Hash{}, fmt.Errorf("cannot resolve %q", ref)
}
