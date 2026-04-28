package fswatcher

import (
	"context"
	"fmt"

	"github.com/fsnotify/fsnotify"
)

type EventCallback func(path string)

// Watcher monitors directories for file changes using fsnotify.
type Watcher struct {
	fsw      *fsnotify.Watcher
	callback EventCallback
}

func NewWatcher(dirs []string, callback EventCallback) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}

	for _, dir := range dirs {
		if err := fsw.Add(dir); err != nil {
			fsw.Close()
			return nil, fmt.Errorf("watch %q: %w", dir, err)
		}
	}

	return &Watcher{
		fsw:      fsw,
		callback: callback,
	}, nil
}

// Run blocks until ctx is cancelled, dispatching write/create events.
func (w *Watcher) Run(ctx context.Context) {
	defer w.fsw.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				w.callback(event.Name)
			}
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
		}
	}
}

// AddDir adds a new directory to watch at runtime.
func (w *Watcher) AddDir(dir string) error {
	return w.fsw.Add(dir)
}
