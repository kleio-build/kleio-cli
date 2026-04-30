package localdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) a SQLite database at path, runs schema migrations,
// and enables WAL mode for concurrent read performance.
func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return db, nil
}

// OpenInMemory opens an in-memory SQLite database for testing. The returned DB
// uses a shared cache with a unique name so each call produces an isolated
// store.
func OpenInMemory() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// RunMigrations re-runs schema migrations and backfills. Exported for testing.
func RunMigrations(db *sql.DB) error {
	return migrate(db)
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return err
	}
	if err := backfillCommitsToEvents(db); err != nil {
		return fmt.Errorf("backfill commits to events: %w", err)
	}
	return backfillFTS(db)
}

func backfillCommitsToEvents(db *sql.DB) error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO events (id, signal_type, content, source_type,
		    created_at, repo_name, branch_name, structured_data, author_type)
		SELECT 'git:' || sha, 'git_commit', message, 'local_git',
		    committed_at, repo_name, branch,
		    json_object('sha', sha, 'files_changed', files_changed,
		        'author_name', author_name, 'author_email', author_email),
		    'human'
		FROM commits
		WHERE NOT EXISTS (
		    SELECT 1 FROM events WHERE id = 'git:' || commits.sha
		)
	`)
	return err
}

func backfillFTS(db *sql.DB) error {
	_, err := db.Exec(`
		INSERT INTO events_fts (event_id, content, freeform_context)
		SELECT id, content, COALESCE(freeform_context, '')
		FROM events
		WHERE id NOT IN (SELECT event_id FROM events_fts)
	`)
	return err
}
