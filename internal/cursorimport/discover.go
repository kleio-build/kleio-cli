package cursorimport

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DiscoverTranscripts finds all Cursor agent-transcript JSONL files
// under the user's ~/.cursor/projects/ directory.
func DiscoverTranscripts() ([]string, error) {
	base := cursorProjectsDir()
	if base == "" {
		return nil, fmt.Errorf("could not determine Cursor projects directory")
	}

	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil, nil
	}

	var transcripts []string
	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".jsonl") && strings.Contains(path, "agent-transcripts") {
			transcripts = append(transcripts, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk cursor projects: %w", err)
	}

	return transcripts, nil
}

// DiscoverTranscriptsForProject finds transcripts for a specific project slug.
func DiscoverTranscriptsForProject(projectSlug string) ([]string, error) {
	base := cursorProjectsDir()
	if base == "" {
		return nil, fmt.Errorf("could not determine Cursor projects directory")
	}

	transcriptDir := filepath.Join(base, projectSlug, "agent-transcripts")
	if _, err := os.Stat(transcriptDir); os.IsNotExist(err) {
		return nil, nil
	}

	var transcripts []string
	err := filepath.Walk(transcriptDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
			transcripts = append(transcripts, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk transcripts: %w", err)
	}

	return transcripts, nil
}

func cursorProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(home, ".cursor", "projects")
	}
	return filepath.Join(home, ".cursor", "projects")
}
