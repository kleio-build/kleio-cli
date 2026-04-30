package localdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/google/uuid"
)

// Store implements kleio.Store backed by a local SQLite database.
type Store struct {
	db *sql.DB
}

// New creates a Store from an already-opened *sql.DB.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Mode() kleio.StoreMode { return kleio.StoreModeLocal }

func (s *Store) Close() error { return s.db.Close() }

// --- Events ---

func (s *Store) CreateEvent(ctx context.Context, e *kleio.Event) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if e.AuthorType == "" {
		e.AuthorType = kleio.AuthorTypeHuman
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, signal_type, content, source_type, created_at,
		    repo_name, branch_name, file_path, freeform_context, structured_data,
		    author_type, author_label, synced)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		e.ID, e.SignalType, e.Content, e.SourceType, e.CreatedAt,
		nullStr(e.RepoName), nullStr(e.BranchName), nullStr(e.FilePath),
		nullStr(e.FreeformContext), nullStr(e.StructuredData),
		e.AuthorType, nullStr(e.AuthorLabel),
	)
	return err
}

func (s *Store) GetEvent(ctx context.Context, id string) (*kleio.Event, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, signal_type, content, source_type, created_at,
		    COALESCE(repo_name,''), COALESCE(branch_name,''), COALESCE(file_path,''),
		    COALESCE(freeform_context,''), COALESCE(structured_data,''),
		    COALESCE(author_type,'human'), COALESCE(author_label,''), synced
		 FROM events WHERE id = ?`, id)
	var e kleio.Event
	var synced int
	err := row.Scan(&e.ID, &e.SignalType, &e.Content, &e.SourceType, &e.CreatedAt,
		&e.RepoName, &e.BranchName, &e.FilePath,
		&e.FreeformContext, &e.StructuredData,
		&e.AuthorType, &e.AuthorLabel, &synced)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	e.Synced = synced != 0
	return &e, nil
}

func (s *Store) ListEvents(ctx context.Context, f kleio.EventFilter) ([]kleio.Event, error) {
	var where []string
	var args []interface{}

	if f.SignalType != "" {
		where = append(where, "signal_type = ?")
		args = append(args, f.SignalType)
	}
	if f.SourceType != "" {
		where = append(where, "source_type = ?")
		args = append(args, f.SourceType)
	}
	if f.RepoName != "" {
		where = append(where, "repo_name = ?")
		args = append(args, f.RepoName)
	}
	if f.AuthorType != "" {
		where = append(where, "author_type = ?")
		args = append(args, f.AuthorType)
	}
	if f.CreatedAfter != "" {
		where = append(where, "created_at > ?")
		args = append(args, f.CreatedAfter)
	}
	if f.CreatedBefore != "" {
		where = append(where, "created_at < ?")
		args = append(args, f.CreatedBefore)
	}

	q := "SELECT id, signal_type, content, source_type, created_at, " +
		"COALESCE(repo_name,''), COALESCE(branch_name,''), COALESCE(file_path,''), " +
		"COALESCE(freeform_context,''), COALESCE(structured_data,''), " +
		"COALESCE(author_type,'human'), COALESCE(author_label,''), synced " +
		"FROM events"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kleio.Event
	for rows.Next() {
		var e kleio.Event
		var synced int
		if err := rows.Scan(&e.ID, &e.SignalType, &e.Content, &e.SourceType, &e.CreatedAt,
			&e.RepoName, &e.BranchName, &e.FilePath,
			&e.FreeformContext, &e.StructuredData,
			&e.AuthorType, &e.AuthorLabel, &synced); err != nil {
			return nil, err
		}
		e.Synced = synced != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- Backlog Items ---

func (s *Store) CreateBacklogItem(ctx context.Context, item *kleio.BacklogItem) error {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	if item.UpdatedAt == "" {
		item.UpdatedAt = now
	}
	if item.Status == "" {
		item.Status = kleio.StatusOpen
	}
	if item.Category == "" {
		item.Category = kleio.CategoryTask
	}
	if item.Urgency == "" {
		item.Urgency = kleio.UrgencyMedium
	}
	if item.Importance == "" {
		item.Importance = kleio.ImportanceMedium
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO backlog_items (id, short_id, title, summary, body,
		    status, category, urgency, importance, repo_name,
		    created_at, updated_at, synced)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		item.ID, nullInt(item.ShortID), item.Title, nullStr(item.Summary), nullStr(item.Body),
		item.Status, item.Category, item.Urgency, item.Importance, nullStr(item.RepoName),
		item.CreatedAt, item.UpdatedAt,
	)
	return err
}

func (s *Store) GetBacklogItem(ctx context.Context, id string) (*kleio.BacklogItem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(short_id,0), title, COALESCE(summary,''), COALESCE(body,''),
		    status, category, urgency, importance, COALESCE(repo_name,''),
		    created_at, updated_at, synced
		 FROM backlog_items WHERE id = ?`, id)
	var item kleio.BacklogItem
	var synced int
	err := row.Scan(&item.ID, &item.ShortID, &item.Title, &item.Summary, &item.Body,
		&item.Status, &item.Category, &item.Urgency, &item.Importance, &item.RepoName,
		&item.CreatedAt, &item.UpdatedAt, &synced)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backlog item %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	item.Synced = synced != 0
	return &item, nil
}

func (s *Store) ListBacklogItems(ctx context.Context, f kleio.BacklogFilter) ([]kleio.BacklogItem, error) {
	var where []string
	var args []interface{}

	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Category != "" {
		where = append(where, "category = ?")
		args = append(args, f.Category)
	}
	if f.Urgency != "" {
		where = append(where, "urgency = ?")
		args = append(args, f.Urgency)
	}
	if f.Importance != "" {
		where = append(where, "importance = ?")
		args = append(args, f.Importance)
	}
	if f.RepoName != "" {
		where = append(where, "repo_name = ?")
		args = append(args, f.RepoName)
	}
	if f.Search != "" {
		where = append(where, "(title LIKE ? OR summary LIKE ?)")
		pattern := "%" + f.Search + "%"
		args = append(args, pattern, pattern)
	}

	q := "SELECT id, COALESCE(short_id,0), title, COALESCE(summary,''), COALESCE(body,''), " +
		"status, category, urgency, importance, COALESCE(repo_name,''), " +
		"created_at, updated_at, synced FROM backlog_items"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kleio.BacklogItem
	for rows.Next() {
		var item kleio.BacklogItem
		var synced int
		if err := rows.Scan(&item.ID, &item.ShortID, &item.Title, &item.Summary, &item.Body,
			&item.Status, &item.Category, &item.Urgency, &item.Importance, &item.RepoName,
			&item.CreatedAt, &item.UpdatedAt, &synced); err != nil {
			return nil, err
		}
		item.Synced = synced != 0
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpdateBacklogItem(ctx context.Context, id string, update *kleio.BacklogItem) error {
	var sets []string
	var args []interface{}

	if update.Title != "" {
		sets = append(sets, "title = ?")
		args = append(args, update.Title)
	}
	if update.Summary != "" {
		sets = append(sets, "summary = ?")
		args = append(args, update.Summary)
	}
	if update.Status != "" {
		sets = append(sets, "status = ?")
		args = append(args, update.Status)
	}
	if update.Category != "" {
		sets = append(sets, "category = ?")
		args = append(args, update.Category)
	}
	if update.Urgency != "" {
		sets = append(sets, "urgency = ?")
		args = append(args, update.Urgency)
	}
	if update.Importance != "" {
		sets = append(sets, "importance = ?")
		args = append(args, update.Importance)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339))
	args = append(args, id)

	q := "UPDATE backlog_items SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backlog item %q not found", id)
	}
	return nil
}

// --- Git Index ---

func (s *Store) IndexCommits(ctx context.Context, repoPath string, commits []kleio.Commit) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	commitStmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO commits (sha, repo_path, repo_name, branch,
		    author_name, author_email, message, committed_at,
		    files_changed, insertions, deletions, is_merge, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer commitStmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range commits {
		isMerge := 0
		if c.IsMerge {
			isMerge = 1
		}
		indexedAt := c.IndexedAt
		if indexedAt == "" {
			indexedAt = now
		}
		_, err := commitStmt.ExecContext(ctx,
			c.SHA, repoPath, nullStr(c.RepoName), nullStr(c.Branch),
			nullStr(c.AuthorName), nullStr(c.AuthorEmail), c.Message, c.CommittedAt,
			c.FilesChanged, c.Insertions, c.Deletions, isMerge, indexedAt)
		if err != nil {
			return fmt.Errorf("index commit %s: %w", c.SHA, err)
		}
	}

	return tx.Commit()
}

func (s *Store) QueryCommits(ctx context.Context, f kleio.CommitFilter) ([]kleio.Commit, error) {
	var where []string
	var args []interface{}

	if f.RepoPath != "" {
		where = append(where, "c.repo_path = ?")
		args = append(args, f.RepoPath)
	}
	if f.Branch != "" {
		where = append(where, "c.branch = ?")
		args = append(args, f.Branch)
	}
	if f.AuthorEmail != "" {
		where = append(where, "c.author_email = ?")
		args = append(args, f.AuthorEmail)
	}
	if f.AuthorName != "" {
		where = append(where, "c.author_name = ?")
		args = append(args, f.AuthorName)
	}
	if f.MessageSearch != "" {
		where = append(where, "c.message LIKE ?")
		args = append(args, "%"+f.MessageSearch+"%")
	}
	if f.Since != "" {
		where = append(where, "c.committed_at >= ?")
		args = append(args, f.Since)
	}
	if f.Until != "" {
		where = append(where, "c.committed_at <= ?")
		args = append(args, f.Until)
	}
	if f.IsMerge != nil {
		v := 0
		if *f.IsMerge {
			v = 1
		}
		where = append(where, "c.is_merge = ?")
		args = append(args, v)
	}
	if f.FilePath != "" {
		where = append(where, "EXISTS (SELECT 1 FROM commit_files cf WHERE cf.commit_sha = c.sha AND cf.file_path = ?)")
		args = append(args, f.FilePath)
	}

	q := "SELECT c.sha, c.repo_path, COALESCE(c.repo_name,''), COALESCE(c.branch,''), " +
		"COALESCE(c.author_name,''), COALESCE(c.author_email,''), c.message, c.committed_at, " +
		"COALESCE(c.files_changed,0), COALESCE(c.insertions,0), COALESCE(c.deletions,0), " +
		"c.is_merge, c.indexed_at FROM commits c"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY c.committed_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kleio.Commit
	for rows.Next() {
		var c kleio.Commit
		var isMerge int
		if err := rows.Scan(&c.SHA, &c.RepoPath, &c.RepoName, &c.Branch,
			&c.AuthorName, &c.AuthorEmail, &c.Message, &c.CommittedAt,
			&c.FilesChanged, &c.Insertions, &c.Deletions, &isMerge, &c.IndexedAt); err != nil {
			return nil, err
		}
		c.IsMerge = isMerge != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- Links ---

func (s *Store) CreateLink(ctx context.Context, l *kleio.Link) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	if l.CreatedAt == "" {
		l.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if l.Confidence == 0 {
		l.Confidence = 1.0
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO links (id, source_id, target_id, link_type, confidence, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.SourceID, l.TargetID, l.LinkType, l.Confidence, nullStr(l.Reason), l.CreatedAt)
	return err
}

func (s *Store) QueryLinks(ctx context.Context, f kleio.LinkFilter) ([]kleio.Link, error) {
	var where []string
	var args []interface{}

	if f.SourceID != "" {
		where = append(where, "source_id = ?")
		args = append(args, f.SourceID)
	}
	if f.TargetID != "" {
		where = append(where, "target_id = ?")
		args = append(args, f.TargetID)
	}
	if f.LinkType != "" {
		where = append(where, "link_type = ?")
		args = append(args, f.LinkType)
	}

	q := "SELECT id, source_id, target_id, link_type, confidence, COALESCE(reason,''), created_at FROM links"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kleio.Link
	for rows.Next() {
		var l kleio.Link
		if err := rows.Scan(&l.ID, &l.SourceID, &l.TargetID, &l.LinkType,
			&l.Confidence, &l.Reason, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// --- File History ---

func (s *Store) TrackFileChange(ctx context.Context, fc *kleio.FileChange) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO commit_files (commit_sha, file_path, change_type, old_path)
		 VALUES (?, ?, ?, ?)`,
		fc.CommitSHA, fc.FilePath, nullStr(fc.ChangeType), nullStr(fc.OldPath))
	return err
}

func (s *Store) FileHistory(ctx context.Context, path string) ([]kleio.FileChange, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cf.commit_sha, cf.file_path, COALESCE(cf.change_type,''), COALESCE(cf.old_path,'')
		 FROM commit_files cf
		 JOIN commits c ON c.sha = cf.commit_sha
		 WHERE cf.file_path = ?
		 ORDER BY c.committed_at DESC`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kleio.FileChange
	for rows.Next() {
		var fc kleio.FileChange
		if err := rows.Scan(&fc.CommitSHA, &fc.FilePath, &fc.ChangeType, &fc.OldPath); err != nil {
			return nil, err
		}
		out = append(out, fc)
	}
	return out, rows.Err()
}

// --- Search ---

func (s *Store) Search(ctx context.Context, query string, opts kleio.SearchOpts) ([]kleio.SearchResult, error) {
	pattern := "%" + query + "%"
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	var results []kleio.SearchResult

	// Search events
	eventArgs := []interface{}{pattern, pattern}
	eventWhere := "(content LIKE ? OR freeform_context LIKE ?)"
	if opts.RepoName != "" {
		eventWhere += " AND repo_name = ?"
		eventArgs = append(eventArgs, opts.RepoName)
	}
	if opts.SignalType != "" {
		eventWhere += " AND signal_type = ?"
		eventArgs = append(eventArgs, opts.SignalType)
	}
	eventArgs = append(eventArgs, limit)

	eRows, err := s.db.QueryContext(ctx,
		"SELECT id, content, COALESCE(repo_name,''), COALESCE(file_path,''), signal_type, created_at "+
			"FROM events WHERE "+eventWhere+" ORDER BY created_at DESC LIMIT ?", eventArgs...)
	if err != nil {
		return nil, err
	}
	for eRows.Next() {
		var r kleio.SearchResult
		if err := eRows.Scan(&r.ID, &r.Content, &r.RepoName, &r.FilePath, &r.SignalType, &r.CreatedAt); err != nil {
			eRows.Close()
			return nil, err
		}
		r.Kind = "event"
		r.Score = 1.0
		results = append(results, r)
	}
	eRows.Close()

	// Search commits
	cRows, err := s.db.QueryContext(ctx,
		"SELECT sha, message, COALESCE(repo_name,''), committed_at "+
			"FROM commits WHERE message LIKE ? ORDER BY committed_at DESC LIMIT ?",
		pattern, limit)
	if err != nil {
		return nil, err
	}
	for cRows.Next() {
		var r kleio.SearchResult
		if err := cRows.Scan(&r.ID, &r.Content, &r.RepoName, &r.CreatedAt); err != nil {
			cRows.Close()
			return nil, err
		}
		r.Kind = "commit"
		r.Score = 0.8
		results = append(results, r)
	}
	cRows.Close()

	// Search backlog items
	bRows, err := s.db.QueryContext(ctx,
		"SELECT id, title || ': ' || COALESCE(summary,''), COALESCE(repo_name,''), created_at "+
			"FROM backlog_items WHERE title LIKE ? OR summary LIKE ? ORDER BY created_at DESC LIMIT ?",
		pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	for bRows.Next() {
		var r kleio.SearchResult
		if err := bRows.Scan(&r.ID, &r.Content, &r.RepoName, &r.CreatedAt); err != nil {
			bRows.Close()
			return nil, err
		}
		r.Kind = "backlog_item"
		r.Score = 0.9
		results = append(results, r)
	}
	bRows.Close()

	return results, nil
}

// --- helpers ---

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
