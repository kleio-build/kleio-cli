package indexer

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/gitreader"
	"github.com/kleio-build/kleio-cli/internal/localdb"
)

const squashThreshold = 50

// GitIndexer walks a git repository's history and indexes commits, file
// changes, and extracted identifiers into the local SQLite store.
type GitIndexer struct {
	store     *localdb.Store
	extractor *IdentifierExtractor
}

// NewGitIndexer creates a new indexer backed by the given local store.
func NewGitIndexer(store *localdb.Store) *GitIndexer {
	return &GitIndexer{
		store:     store,
		extractor: NewIdentifierExtractor(),
	}
}

// IndexResult summarizes what the indexer did.
type IndexResult struct {
	RepoPath       string
	RepoName       string
	CommitsIndexed int
	FilesTracked   int
	Identifiers    int
	Links          int
	Incremental    bool
	Duration       time.Duration
	IsSquashHeavy  bool
}

// Index walks the full (or incremental) git history of the repository at
// repoPath and stores everything in SQLite.
func (g *GitIndexer) Index(ctx context.Context, repoPath string) (*IndexResult, error) {
	start := time.Now()

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	repoName := filepath.Base(absPath)

	repo, err := g.store.GetRepo(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("get repo state: %w", err)
	}

	var since time.Time
	incremental := false
	if repo != nil && repo.LastIndexedSHA != "" {
		incremental = true
		if repo.LastIndexedAt != "" {
			if t, parseErr := time.Parse(time.RFC3339, repo.LastIndexedAt); parseErr == nil {
				since = t.Add(-time.Hour)
			}
		}
	}

	walkOpts := gitreader.WalkOptions{
		RepoPath: absPath,
	}
	if !since.IsZero() {
		walkOpts.Since = since
	}

	rawCommits, err := gitreader.Walk(walkOpts)
	if err != nil {
		return nil, fmt.Errorf("walk git history: %w", err)
	}

	if len(rawCommits) == 0 {
		return &IndexResult{
			RepoPath:    absPath,
			RepoName:    repoName,
			Incremental: incremental,
			Duration:    time.Since(start),
		}, nil
	}

	result := &IndexResult{
		RepoPath:    absPath,
		RepoName:    repoName,
		Incremental: incremental,
	}

	mergeCount := 0
	largeCommitCount := 0

	commits := make([]kleio.Commit, 0, len(rawCommits))
	for _, rc := range rawCommits {
		c := kleio.Commit{
			SHA:          rc.Hash,
			RepoPath:     absPath,
			RepoName:     repoName,
			Branch:       rc.Branch,
			AuthorName:   rc.Author,
			AuthorEmail:  rc.Email,
			Message:      rc.Message,
			CommittedAt:  rc.Timestamp.UTC().Format(time.RFC3339),
			FilesChanged: len(rc.Files),
			IsMerge:      rc.IsMerge,
		}
		commits = append(commits, c)

		if rc.IsMerge {
			mergeCount++
		}
		if len(rc.Files) > squashThreshold {
			largeCommitCount++
		}
	}

	if err := g.store.IndexCommits(ctx, absPath, commits); err != nil {
		return nil, fmt.Errorf("index commits: %w", err)
	}
	result.CommitsIndexed = len(commits)

	for _, rc := range rawCommits {
		for _, f := range rc.Files {
			fc := &kleio.FileChange{
				CommitSHA:  rc.Hash,
				FilePath:   f,
				ChangeType: kleio.ChangeTypeModified,
			}
			if err := g.store.TrackFileChange(ctx, fc); err != nil {
				continue
			}
			result.FilesTracked++
		}
	}

	for _, rc := range rawCommits {
		ids, links := g.extractor.Extract(rc.Hash, rc.Branch, rc.Message, rc.IsMerge)
		for _, id := range ids {
			if err := g.store.CreateIdentifier(ctx, &id); err != nil {
				continue
			}
			result.Identifiers++
		}
		for _, link := range links {
			if err := g.store.CreateLink(ctx, &link); err != nil {
				continue
			}
			result.Links++
		}
	}

	isSquashHeavy := false
	if len(commits) > 20 {
		squashRatio := float64(largeCommitCount) / float64(len(commits))
		mergeRatio := float64(mergeCount) / float64(len(commits))
		isSquashHeavy = squashRatio > 0.2 || mergeRatio > 0.5
	}
	result.IsSquashHeavy = isSquashHeavy

	latestSHA := rawCommits[0].Hash
	if err := g.store.UpsertRepo(ctx, &kleio.Repo{
		Path:           absPath,
		Name:           repoName,
		LastIndexedSHA: latestSHA,
		IsSquashHeavy:  isSquashHeavy,
	}); err != nil {
		return nil, fmt.Errorf("update repo state: %w", err)
	}

	result.Duration = time.Since(start)
	return result, nil
}
