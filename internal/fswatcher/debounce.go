package fswatcher

import (
	"sync"
	"time"
)

// Debouncer collapses rapid file events into a single callback per file.
type Debouncer struct {
	window time.Duration
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewDebouncer(window time.Duration) *Debouncer {
	return &Debouncer{
		window: window,
		timers: make(map[string]*time.Timer),
	}
}

func (d *Debouncer) Trigger(path string, cb func(string)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if t, ok := d.timers[path]; ok {
		t.Stop()
	}

	d.timers[path] = time.AfterFunc(d.window, func() {
		cb(path)
		d.mu.Lock()
		delete(d.timers, path)
		d.mu.Unlock()
	})
}

func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.timers {
		t.Stop()
	}
	d.timers = make(map[string]*time.Timer)
}
