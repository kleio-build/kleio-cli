package engine

import (
	"context"

	kleio "github.com/kleio-build/kleio-core"
)

// GraphNode represents a node in the identifier hierarchy: project ->
// milestone -> ticket -> PR -> commit -> file.
type GraphNode struct {
	ID       string
	Kind     string // "commit", "event", "identifier", "file"
	Label    string
	Children []GraphNode
}

// ExpandFromAnchor builds a local subgraph starting from an anchor ID (a
// commit SHA, event ID, or identifier value). It traverses links outward
// to the configured depth.
func (e *Engine) ExpandFromAnchor(ctx context.Context, anchorID string, depth int) (*GraphNode, error) {
	if depth <= 0 {
		depth = 2
	}
	root := &GraphNode{ID: anchorID, Kind: "anchor", Label: anchorID}
	visited := map[string]bool{anchorID: true}
	if err := e.expand(ctx, root, visited, depth); err != nil {
		return nil, err
	}
	return root, nil
}

func (e *Engine) expand(ctx context.Context, node *GraphNode, visited map[string]bool, remaining int) error {
	if remaining <= 0 {
		return nil
	}

	links, err := e.store.QueryLinks(ctx, kleio.LinkFilter{SourceID: node.ID, Limit: 50})
	if err != nil {
		return err
	}
	reverse, err := e.store.QueryLinks(ctx, kleio.LinkFilter{TargetID: node.ID, Limit: 50})
	if err != nil {
		return err
	}
	links = append(links, reverse...)

	for _, l := range links {
		peerID := l.TargetID
		if peerID == node.ID {
			peerID = l.SourceID
		}
		if visited[peerID] {
			continue
		}
		visited[peerID] = true

		child := GraphNode{
			ID:    peerID,
			Kind:  l.LinkType,
			Label: peerID,
		}

		if c, _ := e.store.GetEvent(ctx, peerID); c != nil {
			child.Kind = "event"
			child.Label = firstLine(c.Content)
		}

		if err := e.expand(ctx, &child, visited, remaining-1); err != nil {
			return err
		}
		node.Children = append(node.Children, child)
	}

	return nil
}
