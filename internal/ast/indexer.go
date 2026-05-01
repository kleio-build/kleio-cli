// Package ast provides AST-based symbol extraction for the Kleio pipeline.
// Currently supports Go via go/ast; multi-language support via Tree-sitter
// is deferred to a later phase.
package ast

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	kleio "github.com/kleio-build/kleio-core"
	goparser "github.com/kleio-build/kleio-cli/internal/ast/goparser"
	"github.com/kleio-build/kleio-cli/internal/entity"
	"github.com/kleio-build/kleio-cli/internal/localdb"
	"github.com/google/uuid"
)

// SymbolIndexer extracts symbols from changed Go files and persists them
// as code_symbols + entity graph entries.
type SymbolIndexer struct {
	store *localdb.Store
}

func NewSymbolIndexer(store *localdb.Store) *SymbolIndexer {
	return &SymbolIndexer{store: store}
}

// IndexSignals processes RawSignals from the git ingester, extracts
// symbols from changed .go files, and persists them. Returns the number
// of symbols indexed.
func (si *SymbolIndexer) IndexSignals(ctx context.Context, signals []kleio.RawSignal) (int, error) {
	indexed := 0
	for _, sig := range signals {
		if sig.SourceType != kleio.SourceTypeLocalGit {
			continue
		}
		sha, _ := sig.Metadata["sha"].(string)
		repoPath, _ := sig.Metadata["repo_path"].(string)
		filesRaw, _ := sig.Metadata["files"].([]string)
		if sha == "" || repoPath == "" || len(filesRaw) == 0 {
			continue
		}

		for _, filePath := range filesRaw {
			if !goparser.IsGoFile(filePath) {
				continue
			}
			absPath := filepath.Join(repoPath, filePath)
			if _, err := os.Stat(absPath); err != nil {
				continue
			}
			symbols, err := goparser.ExtractSymbols(absPath)
			if err != nil {
				continue
			}
			for _, sym := range symbols {
				n, err := si.persistSymbol(ctx, sig.RepoName, filePath, sha, sym)
				if err != nil {
					continue
				}
				indexed += n
			}
		}
	}
	return indexed, nil
}

func (si *SymbolIndexer) persistSymbol(ctx context.Context, repoName, filePath, commitSHA string, sym goparser.Symbol) (int, error) {
	existing, err := si.store.FindCodeSymbol(ctx, repoName, filePath, sym.Name)
	if err != nil {
		return 0, err
	}

	var symbolID string
	if existing != nil {
		symbolID = existing.ID
		cs := &localdb.CodeSymbol{
			ID:            existing.ID,
			RepoName:      repoName,
			FilePath:      filePath,
			SymbolName:    sym.Name,
			SymbolKind:    sym.Kind,
			StartLine:     sym.StartLine,
			EndLine:       sym.EndLine,
			Language:      "go",
			LastCommitSHA: commitSHA,
		}
		if err := si.store.UpsertCodeSymbol(ctx, cs); err != nil {
			return 0, err
		}
	} else {
		symbolID = uuid.NewString()
		cs := &localdb.CodeSymbol{
			ID:            symbolID,
			RepoName:      repoName,
			FilePath:      filePath,
			SymbolName:    sym.Name,
			SymbolKind:    sym.Kind,
			StartLine:     sym.StartLine,
			EndLine:       sym.EndLine,
			Language:      "go",
			LastCommitSHA: commitSHA,
		}
		if err := si.store.UpsertCodeSymbol(ctx, cs); err != nil {
			return 0, err
		}
	}

	// Record the commit<->symbol relationship.
	_ = si.store.RecordCommitSymbolChange(ctx, &localdb.CommitSymbolChange{
		CommitSHA:  commitSHA,
		SymbolID:   symbolID,
		ChangeKind: "modified",
	})

	// Register symbol as an entity for the entity graph.
	normalizedLabel := entity.NormalizeLabel(kleio.EntityKindSymbol, sym.Name)
	ent := &kleio.Entity{
		ID:              fmt.Sprintf("sym:%s:%s:%s", repoName, filePath, sym.Name),
		Kind:            kleio.EntityKindSymbol,
		Label:           sym.Name,
		NormalizedLabel: normalizedLabel,
		RepoName:        repoName,
	}
	_ = si.store.CreateEntity(ctx, ent)

	// Link the symbol entity to the commit evidence.
	_ = si.store.CreateEntityMention(ctx, &kleio.EntityMention{
		EntityID:     ent.ID,
		EvidenceType: kleio.EvidenceTypeCommit,
		EvidenceID:   fmt.Sprintf("git:%s:%s", repoName, commitSHA),
		Context:      fmt.Sprintf("%s %s in %s", sym.Kind, sym.Name, filePath),
		Confidence:   0.95,
	})

	return 1, nil
}
