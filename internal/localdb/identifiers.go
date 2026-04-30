package localdb

import (
	"context"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/google/uuid"
)

// CreateIdentifier inserts an identifier if it doesn't already exist (by
// kind+value unique constraint). Returns nil on conflict.
func (s *Store) CreateIdentifier(ctx context.Context, id *kleio.Identifier) error {
	if id.ID == "" {
		id.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO identifiers (id, kind, value, provider, url, first_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id.ID, id.Kind, id.Value, nullStr(id.Provider), nullStr(id.URL), id.FirstSeenAt)
	return err
}
