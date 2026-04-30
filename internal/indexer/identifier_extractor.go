package indexer

import (
	"regexp"
	"time"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/google/uuid"
)

var (
	jiraPattern   = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)
	githubPR      = regexp.MustCompile(`(?:^|[\s(])#(\d+)\b`)
	mergeCommitPR = regexp.MustCompile(`Merge pull request #(\d+)`)
	prSuffix      = regexp.MustCompile(`\(#(\d+)\)$`)
	kleioPattern  = regexp.MustCompile(`\b(KL-\d+)\b`)
	versionTag    = regexp.MustCompile(`\bv(\d+\.\d+\.\d+(?:-[\w.]+)?)\b`)
)

// IdentifierExtractor parses commit messages and branch names to find
// references to external entities: tickets, PRs, milestones, and tags.
type IdentifierExtractor struct{}

// NewIdentifierExtractor creates a new extractor.
func NewIdentifierExtractor() *IdentifierExtractor {
	return &IdentifierExtractor{}
}

// Extract parses a single commit's branch name and message, returning any
// discovered identifiers and links from the commit to those identifiers.
func (e *IdentifierExtractor) Extract(commitSHA, branch, message string, isMerge bool) ([]kleio.Identifier, []kleio.Link) {
	seen := map[string]bool{}
	var identifiers []kleio.Identifier
	var links []kleio.Link
	now := time.Now().UTC().Format(time.RFC3339)

	addID := func(kind, value, provider, linkType string) {
		key := kind + ":" + value
		if seen[key] {
			return
		}
		seen[key] = true

		id := kleio.Identifier{
			ID:          uuid.NewString(),
			Kind:        kind,
			Value:       value,
			Provider:    provider,
			FirstSeenAt: now,
		}
		identifiers = append(identifiers, id)

		links = append(links, kleio.Link{
			ID:         uuid.NewString(),
			SourceID:   commitSHA,
			TargetID:   id.ID,
			LinkType:   linkType,
			Confidence: 1.0,
			CreatedAt:  now,
		})
	}

	// Kleio internal tickets (checked before Jira so KL-N isn't misclassified)
	for _, m := range kleioPattern.FindAllStringSubmatch(message, -1) {
		addID(kleio.IdentifierKindTicket, m[1], "kleio", kleio.LinkTypeReferences)
	}

	// Jira-style tickets from branch and message
	for _, m := range jiraPattern.FindAllStringSubmatch(branch, -1) {
		addID(kleio.IdentifierKindTicket, m[1], kleio.ProviderJira, kleio.LinkTypeReferences)
	}
	for _, m := range jiraPattern.FindAllStringSubmatch(message, -1) {
		addID(kleio.IdentifierKindTicket, m[1], kleio.ProviderJira, kleio.LinkTypeReferences)
	}

	// GitHub PR references
	if isMerge {
		if m := mergeCommitPR.FindStringSubmatch(message); len(m) > 1 {
			addID(kleio.IdentifierKindPR, "#"+m[1], kleio.ProviderGitHub, kleio.LinkTypeSquashContains)
		}
	}
	if m := prSuffix.FindStringSubmatch(message); len(m) > 1 {
		addID(kleio.IdentifierKindPR, "#"+m[1], kleio.ProviderGitHub, kleio.LinkTypeReferences)
	}
	for _, m := range githubPR.FindAllStringSubmatch(message, -1) {
		addID(kleio.IdentifierKindPR, "#"+m[1], kleio.ProviderGitHub, kleio.LinkTypeReferences)
	}

	// Version tags
	for _, m := range versionTag.FindAllStringSubmatch(message, -1) {
		addID(kleio.IdentifierKindTag, "v"+m[1], kleio.ProviderGitTag, kleio.LinkTypeReferences)
	}

	return identifiers, links
}
