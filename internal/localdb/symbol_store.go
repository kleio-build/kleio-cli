package localdb

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// CodeSymbol represents a symbol extracted from source code.
type CodeSymbol struct {
	ID            string `json:"id"`
	RepoName      string `json:"repo_name"`
	FilePath      string `json:"file_path"`
	SymbolName    string `json:"symbol_name"`
	SymbolKind    string `json:"symbol_kind"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Language      string `json:"language"`
	LastCommitSHA string `json:"last_commit_sha"`
}

// CommitSymbolChange records that a commit changed a symbol.
type CommitSymbolChange struct {
	CommitSHA    string `json:"commit_sha"`
	SymbolID     string `json:"symbol_id"`
	ChangeKind   string `json:"change_kind"`
	LinesAdded   int    `json:"lines_added"`
	LinesDeleted int    `json:"lines_deleted"`
}

func (s *Store) UpsertCodeSymbol(ctx context.Context, sym *CodeSymbol) error {
	if sym.ID == "" {
		sym.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO code_symbols (id, repo_name, file_path, symbol_name, symbol_kind,
		    start_line, end_line, language, last_commit_sha)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		    start_line = excluded.start_line,
		    end_line = excluded.end_line,
		    last_commit_sha = excluded.last_commit_sha`,
		sym.ID, sym.RepoName, sym.FilePath, sym.SymbolName, sym.SymbolKind,
		sym.StartLine, sym.EndLine, sym.Language, sym.LastCommitSHA,
	)
	return err
}

func (s *Store) FindCodeSymbol(ctx context.Context, repoName, filePath, symbolName string) (*CodeSymbol, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_name, file_path, symbol_name, symbol_kind,
		    start_line, end_line, language, last_commit_sha
		 FROM code_symbols
		 WHERE repo_name = ? AND file_path = ? AND symbol_name = ?`,
		repoName, filePath, symbolName,
	)
	var sym CodeSymbol
	var sha sql.NullString
	err := row.Scan(&sym.ID, &sym.RepoName, &sym.FilePath, &sym.SymbolName, &sym.SymbolKind,
		&sym.StartLine, &sym.EndLine, &sym.Language, &sha)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sym.LastCommitSHA = sha.String
	return &sym, nil
}

func (s *Store) RecordCommitSymbolChange(ctx context.Context, change *CommitSymbolChange) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO commit_symbol_changes (commit_sha, symbol_id, change_kind, lines_added, lines_deleted)
		 VALUES (?, ?, ?, ?, ?)`,
		change.CommitSHA, change.SymbolID, change.ChangeKind, change.LinesAdded, change.LinesDeleted,
	)
	return err
}

// ListSymbolsByCommit returns all symbols changed in a given commit.
func (s *Store) ListSymbolsByCommit(ctx context.Context, commitSHA string) ([]CodeSymbol, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cs.id, cs.repo_name, cs.file_path, cs.symbol_name, cs.symbol_kind,
		    cs.start_line, cs.end_line, cs.language, cs.last_commit_sha
		 FROM code_symbols cs
		 JOIN commit_symbol_changes csc ON csc.symbol_id = cs.id
		 WHERE csc.commit_sha = ?`,
		commitSHA,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CodeSymbol
	for rows.Next() {
		var sym CodeSymbol
		var sha sql.NullString
		if err := rows.Scan(&sym.ID, &sym.RepoName, &sym.FilePath, &sym.SymbolName, &sym.SymbolKind,
			&sym.StartLine, &sym.EndLine, &sym.Language, &sha); err != nil {
			return nil, err
		}
		sym.LastCommitSHA = sha.String
		out = append(out, sym)
	}
	return out, rows.Err()
}
