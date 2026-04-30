package engine

import (
	"context"
	"strings"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/google/uuid"
)

// AutoLink scans events and commits for unlinked relationships and creates
// links where confidence is high enough. Returns the number of links created.
func (e *Engine) AutoLink(ctx context.Context) (int, error) {
	events, err := e.store.ListEvents(ctx, kleio.EventFilter{Limit: 500})
	if err != nil {
		return 0, err
	}
	commits, err := e.store.QueryCommits(ctx, kleio.CommitFilter{Limit: 500})
	if err != nil {
		return 0, err
	}

	created := 0
	now := time.Now().UTC().Format(time.RFC3339)

	for _, ev := range events {
		for _, c := range commits {
			// SHA mention in event content
			shortSHA := c.SHA
			if len(shortSHA) > 7 {
				shortSHA = shortSHA[:7]
			}
			if strings.Contains(ev.Content, shortSHA) || strings.Contains(ev.Content, c.SHA) {
				if err := e.store.CreateLink(ctx, &kleio.Link{
					ID:         uuid.NewString(),
					SourceID:   ev.ID,
					TargetID:   c.SHA,
					LinkType:   kleio.LinkTypeReferences,
					Confidence: 0.95,
					Reason:     "sha_mention",
					CreatedAt:  now,
				}); err == nil {
					created++
				}
			}

			// File path overlap between event and commit
			if ev.FilePath != "" {
				history, _ := e.store.FileHistory(ctx, ev.FilePath)
				for _, fc := range history {
					if fc.CommitSHA == c.SHA {
						if err := e.store.CreateLink(ctx, &kleio.Link{
							ID:         uuid.NewString(),
							SourceID:   ev.ID,
							TargetID:   c.SHA,
							LinkType:   kleio.LinkTypeTouches,
							Confidence: 0.7,
							Reason:     "shared_file",
							CreatedAt:  now,
						}); err == nil {
							created++
						}
						break
					}
				}
			}
		}
	}

	return created, nil
}
