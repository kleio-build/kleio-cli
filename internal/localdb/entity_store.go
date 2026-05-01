package localdb

import (
	"context"
	"database/sql"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/google/uuid"
)

func (s *Store) CreateEntity(ctx context.Context, e *kleio.Entity) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if e.FirstSeenAt == "" {
		e.FirstSeenAt = now
	}
	if e.LastSeenAt == "" {
		e.LastSeenAt = now
	}
	if e.Confidence == 0 {
		e.Confidence = 0.5
	}
	if e.MentionCount == 0 {
		e.MentionCount = 1
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO entities (id, kind, label, normalized_label, repo_name,
		    first_seen_at, last_seen_at, mention_count, confidence)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Kind, e.Label, e.NormalizedLabel, nullStr(e.RepoName),
		e.FirstSeenAt, e.LastSeenAt, e.MentionCount, e.Confidence,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil // fresh insert, defaults are fine
	}
	// Row already existed -- bump mention_count and recompute confidence.
	_, err = s.db.ExecContext(ctx,
		`UPDATE entities SET
		    mention_count = mention_count + 1,
		    last_seen_at = ?,
		    confidence = MIN(0.95, 0.3 + 0.1 * CAST(mention_count + 1 AS REAL) / 3.0)
		 WHERE id = ?`,
		e.LastSeenAt, e.ID,
	)
	return err
}

func (s *Store) FindEntity(ctx context.Context, kind, normalizedLabel string) (*kleio.Entity, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, kind, label, normalized_label, repo_name,
		    first_seen_at, last_seen_at, mention_count, confidence
		 FROM entities WHERE kind = ? AND normalized_label = ?`,
		kind, normalizedLabel,
	)

	var e kleio.Entity
	var repoName sql.NullString
	err := row.Scan(&e.ID, &e.Kind, &e.Label, &e.NormalizedLabel, &repoName,
		&e.FirstSeenAt, &e.LastSeenAt, &e.MentionCount, &e.Confidence)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.RepoName = repoName.String
	return &e, nil
}

func (s *Store) ListEntities(ctx context.Context, filter kleio.EntityFilter) ([]kleio.Entity, error) {
	query := `SELECT id, kind, label, normalized_label, repo_name,
	    first_seen_at, last_seen_at, mention_count, confidence
	 FROM entities WHERE 1=1`
	var args []any

	if filter.Kind != "" {
		query += ` AND kind = ?`
		args = append(args, filter.Kind)
	}
	if filter.NormalizedLabel != "" {
		query += ` AND normalized_label = ?`
		args = append(args, filter.NormalizedLabel)
	}
	if filter.RepoName != "" {
		query += ` AND repo_name = ?`
		args = append(args, filter.RepoName)
	}
	query += ` ORDER BY mention_count DESC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kleio.Entity
	for rows.Next() {
		var e kleio.Entity
		var repoName sql.NullString
		if err := rows.Scan(&e.ID, &e.Kind, &e.Label, &e.NormalizedLabel, &repoName,
			&e.FirstSeenAt, &e.LastSeenAt, &e.MentionCount, &e.Confidence); err != nil {
			return nil, err
		}
		e.RepoName = repoName.String
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) CreateEntityAlias(ctx context.Context, a *kleio.EntityAlias) error {
	if a.CreatedAt == "" {
		a.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO entity_aliases (entity_id, alias, source, confidence, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		a.EntityID, a.Alias, a.Source, a.Confidence, a.CreatedAt,
	)
	return err
}

func (s *Store) CreateEntityMention(ctx context.Context, m *kleio.EntityMention) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.CreatedAt == "" {
		m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO entity_mentions (id, entity_id, evidence_type, evidence_id,
		    mention_context, confidence, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.EntityID, m.EvidenceType, m.EvidenceID,
		nullStr(m.Context), m.Confidence, m.CreatedAt,
	)
	return err
}

// LearnCoOccurrenceAliases finds entity pairs that co-occur in >= threshold
// evidence IDs and creates alias links between them.
func (s *Store) LearnCoOccurrenceAliases(ctx context.Context, threshold int) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT e1.id, e1.label, e2.id, e2.label, COUNT(DISTINCT m1.evidence_id) AS shared
		 FROM entity_mentions m1
		 JOIN entity_mentions m2 ON m1.evidence_id = m2.evidence_id AND m1.entity_id < m2.entity_id
		 JOIN entities e1 ON e1.id = m1.entity_id
		 JOIN entities e2 ON e2.id = m2.entity_id
		 WHERE e1.kind = e2.kind
		 GROUP BY m1.entity_id, m2.entity_id
		 HAVING shared >= ?`,
		threshold,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	created := 0
	now := time.Now().UTC().Format(time.RFC3339)
	for rows.Next() {
		var e1ID, e1Label, e2ID, e2Label string
		var shared int
		if err := rows.Scan(&e1ID, &e1Label, &e2ID, &e2Label, &shared); err != nil {
			continue
		}
		conf := min(0.9, 0.4+0.1*float64(shared))
		// Create bidirectional aliases.
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO entity_aliases (entity_id, alias, source, confidence, created_at)
			 VALUES (?, ?, 'co_occurrence', ?, ?)`,
			e1ID, e2Label, conf, now); err == nil {
			created++
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO entity_aliases (entity_id, alias, source, confidence, created_at)
			 VALUES (?, ?, 'co_occurrence', ?, ?)`,
			e2ID, e1Label, conf, now); err == nil {
			created++
		}
	}
	return created, rows.Err()
}

func (s *Store) FindEntitiesByEvidence(ctx context.Context, evidenceID string) ([]kleio.Entity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT e.id, e.kind, e.label, e.normalized_label, e.repo_name,
		    e.first_seen_at, e.last_seen_at, e.mention_count, e.confidence
		 FROM entities e
		 JOIN entity_mentions em ON em.entity_id = e.id
		 WHERE em.evidence_id = ?
		 ORDER BY e.confidence DESC`,
		evidenceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kleio.Entity
	for rows.Next() {
		var e kleio.Entity
		var repoName sql.NullString
		if err := rows.Scan(&e.ID, &e.Kind, &e.Label, &e.NormalizedLabel, &repoName,
			&e.FirstSeenAt, &e.LastSeenAt, &e.MentionCount, &e.Confidence); err != nil {
			return nil, err
		}
		e.RepoName = repoName.String
		out = append(out, e)
	}
	return out, rows.Err()
}
