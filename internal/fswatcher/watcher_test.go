package fswatcher

// Coverage contract:
//
// TailFollower:
// - reads new lines appended to a JSONL file from last offset
// - returns empty on first call with no content
// - tracks offset across calls (incremental)
// - handles file truncation gracefully (reset offset)
// - handles non-existent file
//
// OffsetStore:
// - saves and loads offset state to/from disk
// - returns zero for unknown files
// - persists across store instances
//
// Debouncer:
// - collapses multiple rapid events into one callback
// - fires after quiet period
// - resets timer on new event during debounce window
//
// Watcher:
// - detects new files in watched directory
// - detects modifications to existing files
// - fires callback with file path
// - can be stopped cleanly

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TailFollower tests ---

func TestTailFollower_ReadsNewLines(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("line1\nline2\n"), 0644))

	tf := NewTailFollower()

	// Act
	lines, err := tf.ReadNewLines(path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2"}, lines)
}

func TestTailFollower_IncrementalReads(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("line1\n"), 0644))

	tf := NewTailFollower()

	// First read
	lines1, err := tf.ReadNewLines(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"line1"}, lines1)

	// Append more data
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString("line2\nline3\n")
	require.NoError(t, err)
	f.Close()

	// Act — second read should only return new lines
	lines2, err := tf.ReadNewLines(path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"line2", "line3"}, lines2)
}

func TestTailFollower_HandlesFileTruncation(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("original line that is quite long\n"), 0644))

	tf := NewTailFollower()
	_, err := tf.ReadNewLines(path)
	require.NoError(t, err)

	// Truncate and rewrite with shorter content
	require.NoError(t, os.WriteFile(path, []byte("new\n"), 0644))

	// Act
	lines, err := tf.ReadNewLines(path)

	// Assert — should reset and read from beginning
	require.NoError(t, err)
	assert.Equal(t, []string{"new"}, lines)
}

func TestTailFollower_NonExistentFile(t *testing.T) {
	// Arrange
	tf := NewTailFollower()

	// Act
	lines, err := tf.ReadNewLines("/nonexistent/path.jsonl")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, lines)
}

// --- OffsetStore tests ---

func TestOffsetStore_SaveAndLoad(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	storePath := filepath.Join(dir, "offsets.json")

	store1 := NewOffsetStore(storePath)
	store1.Set("/path/to/file.jsonl", 42)
	require.NoError(t, store1.Save())

	// Act
	store2 := NewOffsetStore(storePath)
	require.NoError(t, store2.Load())
	offset := store2.Get("/path/to/file.jsonl")

	// Assert
	assert.Equal(t, int64(42), offset)
}

func TestOffsetStore_ReturnsZeroForUnknown(t *testing.T) {
	// Arrange
	store := NewOffsetStore("")

	// Act
	offset := store.Get("/unknown/file")

	// Assert
	assert.Equal(t, int64(0), offset)
}

// --- Debouncer tests ---

func TestDebouncer_CollapseRapidEvents(t *testing.T) {
	// Arrange
	var callCount atomic.Int32
	d := NewDebouncer(100 * time.Millisecond)

	// Act — fire 5 rapid events
	for i := 0; i < 5; i++ {
		d.Trigger("file.jsonl", func(path string) {
			callCount.Add(1)
		})
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)

	// Assert — should have fired only once
	assert.Equal(t, int32(1), callCount.Load())
}

func TestDebouncer_SeparateFilesFireIndependently(t *testing.T) {
	// Arrange
	var mu sync.Mutex
	fired := make(map[string]int)
	d := NewDebouncer(50 * time.Millisecond)

	cb := func(path string) {
		mu.Lock()
		fired[path]++
		mu.Unlock()
	}

	// Act
	d.Trigger("file1.jsonl", cb)
	d.Trigger("file2.jsonl", cb)
	time.Sleep(150 * time.Millisecond)

	// Assert
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, fired["file1.jsonl"])
	assert.Equal(t, 1, fired["file2.jsonl"])
}

// --- Watcher tests ---

func TestWatcher_DetectsNewFile(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	var mu sync.Mutex
	var events []string

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := NewWatcher([]string{dir}, func(path string) {
		mu.Lock()
		events = append(events, path)
		mu.Unlock()
	})
	require.NoError(t, err)

	go w.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	// Act
	testFile := filepath.Join(dir, "new.jsonl")
	require.NoError(t, os.WriteFile(testFile, []byte("test\n"), 0644))
	time.Sleep(300 * time.Millisecond)

	// Assert
	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, events)
}

func TestWatcher_DetectsModification(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	testFile := filepath.Join(dir, "existing.jsonl")
	require.NoError(t, os.WriteFile(testFile, []byte("initial\n"), 0644))

	var mu sync.Mutex
	var events []string

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := NewWatcher([]string{dir}, func(path string) {
		mu.Lock()
		events = append(events, path)
		mu.Unlock()
	})
	require.NoError(t, err)

	go w.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	// Act — modify existing file
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, _ = f.WriteString("appended\n")
	f.Close()
	time.Sleep(300 * time.Millisecond)

	// Assert
	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, events)
}

func TestWatcher_StopsCleanly(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	w, err := NewWatcher([]string{dir}, func(path string) {})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// Act
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Assert — should return within reasonable time
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop within timeout")
	}
}
