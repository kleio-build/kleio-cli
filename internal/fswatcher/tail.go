package fswatcher

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

// TailFollower reads new lines from files starting from the last known offset.
type TailFollower struct {
	mu      sync.Mutex
	offsets map[string]int64
}

func NewTailFollower() *TailFollower {
	return &TailFollower{
		offsets: make(map[string]int64),
	}
}

// SetOffset sets the known offset for a file (used when restoring from persistent state).
func (tf *TailFollower) SetOffset(path string, offset int64) {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	tf.offsets[path] = offset
}

// GetOffset returns the current offset for a file.
func (tf *TailFollower) GetOffset(path string) int64 {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	return tf.offsets[path]
}

// ReadNewLines reads lines appended since the last read.
// Returns empty slice and nil error for non-existent files.
func (tf *TailFollower) ReadNewLines(path string) ([]string, error) {
	tf.mu.Lock()
	offset := tf.offsets[path]
	tf.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// File was truncated — reset offset
	if info.Size() < offset {
		offset = 0
	}

	if info.Size() == offset {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, err
		}
	}

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return lines, err
	}

	newOffset, _ := f.Seek(0, 1)
	tf.mu.Lock()
	tf.offsets[path] = newOffset
	tf.mu.Unlock()

	return lines, nil
}
