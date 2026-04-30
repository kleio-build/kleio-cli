package localdb

import (
	"context"
	"database/sql"
	"time"

	kleio "github.com/kleio-build/kleio-core"
)

// GetRepo returns the indexing state for a tracked repository, or nil if not yet tracked.
func (s *Store) GetRepo(ctx context.Context, path string) (*kleio.Repo, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT path, name, COALESCE(last_indexed_sha,''), COALESCE(last_indexed_at,''),
		    is_squash_heavy
		 FROM repos WHERE path = ?`, path)
	var r kleio.Repo
	var isSquash int
	err := row.Scan(&r.Path, &r.Name, &r.LastIndexedSHA, &r.LastIndexedAt, &isSquash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.IsSquashHeavy = isSquash != 0
	return &r, nil
}

// UpsertRepo inserts or updates the indexing state for a tracked repository.
func (s *Store) UpsertRepo(ctx context.Context, r *kleio.Repo) error {
	isSquash := 0
	if r.IsSquashHeavy {
		isSquash = 1
	}
	if r.LastIndexedAt == "" {
		r.LastIndexedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO repos (path, name, last_indexed_sha, last_indexed_at, is_squash_heavy)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
		    name = excluded.name,
		    last_indexed_sha = excluded.last_indexed_sha,
		    last_indexed_at = excluded.last_indexed_at,
		    is_squash_heavy = excluded.is_squash_heavy`,
		r.Path, r.Name, nullStr(r.LastIndexedSHA), r.LastIndexedAt, isSquash)
	return err
}
