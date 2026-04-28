package fswatcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kleio-build/kleio-cli/internal/cursorimport"
	"github.com/kleio-build/kleio-cli/internal/privacy"
)

const defaultDebounceWindow = 5 * time.Second

// SignalSink receives extracted signals for delivery to the API or local queue.
type SignalSink func(signals []cursorimport.Signal) error

// WatchEngine ties together the fsnotify watcher, JSONL tail-follower,
// transcript parser, and signal delivery into a single background process.
type WatchEngine struct {
	tailFollower *TailFollower
	debouncer    *Debouncer
	parser       *cursorimport.TranscriptParser
	privacyF     *privacy.Filter
	sink         SignalSink
	offsetStore  *OffsetStore
	seenHashes   map[string]bool
}

func NewWatchEngine(sink SignalSink) *WatchEngine {
	home, _ := os.UserHomeDir()
	offsetPath := filepath.Join(home, ".kleio", "watch-state.json")

	store := NewOffsetStore(offsetPath)
	_ = store.Load()

	return &WatchEngine{
		tailFollower: NewTailFollower(),
		debouncer:    NewDebouncer(defaultDebounceWindow),
		parser:       cursorimport.NewTranscriptParser(),
		privacyF:     privacy.NewFilter(privacy.DefaultRules()),
		sink:         sink,
		offsetStore:  store,
		seenHashes:   make(map[string]bool),
	}
}

// Run starts watching Cursor transcript directories. Blocks until ctx is cancelled.
func (e *WatchEngine) Run(ctx context.Context) error {
	dirs := e.resolveCursorDirs()
	if len(dirs) == 0 {
		return fmt.Errorf("no Cursor transcript directories found")
	}

	watcher, err := NewWatcher(dirs, func(path string) {
		if !strings.HasSuffix(path, ".jsonl") {
			return
		}
		e.debouncer.Trigger(path, func(p string) {
			e.processFile(p)
		})
	})
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	fmt.Fprintf(os.Stderr, "kleio watch: monitoring %d directories for Cursor transcripts\n", len(dirs))
	watcher.Run(ctx)

	e.debouncer.Stop()
	_ = e.offsetStore.Save()
	return nil
}

func (e *WatchEngine) processFile(path string) {
	offset := e.offsetStore.Get(path)
	e.tailFollower.SetOffset(path, offset)

	lines, err := e.tailFollower.ReadNewLines(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kleio watch: error reading %s: %v\n", filepath.Base(path), err)
		return
	}

	if len(lines) == 0 {
		return
	}

	result, err := e.parser.ParseLines(lines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kleio watch: error parsing %s: %v\n", filepath.Base(path), err)
		return
	}

	var newSignals []cursorimport.Signal
	for _, sig := range result.Signals {
		if sig.AlreadyCaptured {
			continue
		}
		hash := sig.Hash()
		if e.seenHashes[hash] {
			continue
		}
		e.seenHashes[hash] = true
		sig.Content = e.privacyF.Redact(sig.Content)
		newSignals = append(newSignals, sig)
	}

	if len(newSignals) > 0 && e.sink != nil {
		if err := e.sink(newSignals); err != nil {
			fmt.Fprintf(os.Stderr, "kleio watch: delivery error: %v\n", err)
		}
	}

	newOffset := e.tailFollower.GetOffset(path)
	e.offsetStore.Set(path, newOffset)
	_ = e.offsetStore.Save()
}

func (e *WatchEngine) resolveCursorDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	projectsDir := filepath.Join(home, ".cursor", "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil
	}

	var dirs []string
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		transcriptDir := filepath.Join(projectsDir, entry.Name(), "agent-transcripts")
		if _, err := os.Stat(transcriptDir); err == nil {
			dirs = append(dirs, transcriptDir)
		}
	}

	return dirs
}
