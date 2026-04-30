package engine

import (
	"context"
	"sort"

	kleio "github.com/kleio-build/kleio-core"
)

// FileReport summarizes the history and ownership of a single file.
type FileReport struct {
	FilePath      string
	TotalChanges  int
	TopAuthors    []AuthorCount
	CoChangedWith []string
	LastChangeSHA string
}

// AuthorCount tracks how often an author touched a file.
type AuthorCount struct {
	Name  string
	Count int
}

// AnalyzeFile builds a report for a single file path.
func (e *Engine) AnalyzeFile(ctx context.Context, path string) (*FileReport, error) {
	history, err := e.store.FileHistory(ctx, path)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return &FileReport{FilePath: path}, nil
	}

	report := &FileReport{
		FilePath:     path,
		TotalChanges: len(history),
	}

	authorMap := map[string]int{}
	coFileMap := map[string]int{}
	for i, fc := range history {
		if i == 0 {
			report.LastChangeSHA = fc.CommitSHA
		}

		commits, _ := e.store.QueryCommits(ctx, kleio.CommitFilter{Limit: 1})
		for _, c := range commits {
			if c.SHA == fc.CommitSHA {
				authorMap[c.AuthorName]++
			}
		}

		siblings, _ := e.store.FileHistory(ctx, path)
		_ = siblings
		coFiles, _ := e.coFilesForCommit(ctx, fc.CommitSHA, path)
		for _, cf := range coFiles {
			coFileMap[cf]++
		}
	}

	report.TopAuthors = topN(authorMap, 5)
	report.CoChangedWith = topCoFiles(coFileMap, 5)

	return report, nil
}

func (e *Engine) coFilesForCommit(ctx context.Context, sha, exclude string) ([]string, error) {
	commits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{Limit: 500})
	if err != nil {
		return nil, err
	}
	for _, c := range commits {
		if c.SHA != sha {
			continue
		}
		history, _ := e.store.FileHistory(ctx, exclude)
		_ = history
	}
	return nil, nil
}

func topN(m map[string]int, n int) []AuthorCount {
	items := make([]AuthorCount, 0, len(m))
	for k, v := range m {
		items = append(items, AuthorCount{Name: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > n {
		items = items[:n]
	}
	return items
}

func topCoFiles(m map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	items := make([]kv, 0, len(m))
	for k, v := range m {
		items = append(items, kv{k, v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
	out := make([]string, 0, n)
	for i, item := range items {
		if i >= n {
			break
		}
		out = append(out, item.k)
	}
	return out
}
