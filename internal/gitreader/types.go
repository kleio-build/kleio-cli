package gitreader

import "time"

type Commit struct {
	Hash      string
	Message   string
	Author    string
	Email     string
	Timestamp time.Time
	Branch    string
	IsMerge   bool
	Files     []string
}

type Task struct {
	Summary  string
	Branch   string
	Commits  []Commit
	Tickets  []string
	Effort   time.Duration
	StartAt  time.Time
	EndAt    time.Time
}

type ScanResult struct {
	Tasks   []Task
	Commits []Commit
	Since   time.Time
	Until   time.Time
	Author  string
	Branch  string
}
