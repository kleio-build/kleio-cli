package localdb

import (
	"context"
	"fmt"

	kleio "github.com/kleio-build/kleio-core"
)

// ListUnsyncedEvents returns all events with synced=0.
func (s *Store) ListUnsyncedEvents(ctx context.Context) ([]kleio.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, signal_type, content, source_type, created_at,
		    COALESCE(repo_name,''), COALESCE(branch_name,''), COALESCE(file_path,''),
		    COALESCE(freeform_context,''), COALESCE(structured_data,''),
		    COALESCE(author_type,'human'), COALESCE(author_label,''), synced
		 FROM events WHERE synced = 0 ORDER BY created_at ASC`)
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

// ListUnsyncedBacklogItems returns all backlog items with synced=0.
func (s *Store) ListUnsyncedBacklogItems(ctx context.Context) ([]kleio.BacklogItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(short_id,0), title, COALESCE(summary,''), COALESCE(body,''),
		    status, category, urgency, importance, COALESCE(repo_name,''),
		    created_at, updated_at, synced
		 FROM backlog_items WHERE synced = 0 ORDER BY created_at ASC`)
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

// MarkEventSynced sets synced=1 for the given event ID.
func (s *Store) MarkEventSynced(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE events SET synced = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("event %q not found", id)
	}
	return nil
}

// MarkBacklogItemSynced sets synced=1 for the given backlog item ID.
func (s *Store) MarkBacklogItemSynced(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE backlog_items SET synced = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backlog item %q not found", id)
	}
	return nil
}
