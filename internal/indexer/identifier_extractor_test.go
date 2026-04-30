package indexer

import (
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
)

func TestExtract_JiraTicket_FromMessage(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, links := ext.Extract("sha1", "", "fix: resolve PROJ-123 login bug", false)

	assert.Len(t, ids, 1)
	assert.Equal(t, kleio.IdentifierKindTicket, ids[0].Kind)
	assert.Equal(t, "PROJ-123", ids[0].Value)
	assert.Equal(t, kleio.ProviderJira, ids[0].Provider)
	assert.Len(t, links, 1)
	assert.Equal(t, "sha1", links[0].SourceID)
}

func TestExtract_JiraTicket_FromBranch(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, _ := ext.Extract("sha1", "feat/ENG-456-add-auth", "add auth", false)

	assert.Len(t, ids, 1)
	assert.Equal(t, "ENG-456", ids[0].Value)
}

func TestExtract_GitHubPR_MergeCommit(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, links := ext.Extract("sha1", "", "Merge pull request #42 from user/branch", true)

	var prIDs []kleio.Identifier
	for _, id := range ids {
		if id.Kind == kleio.IdentifierKindPR {
			prIDs = append(prIDs, id)
		}
	}
	assert.GreaterOrEqual(t, len(prIDs), 1)
	assert.Equal(t, "#42", prIDs[0].Value)

	found := false
	for _, l := range links {
		if l.LinkType == kleio.LinkTypeSquashContains {
			found = true
		}
	}
	assert.True(t, found, "merge commit PR should have squash_contains link")
}

func TestExtract_GitHubPR_Suffix(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, _ := ext.Extract("sha1", "", "feat: add auth flow (#7)", false)

	var prIDs []kleio.Identifier
	for _, id := range ids {
		if id.Kind == kleio.IdentifierKindPR {
			prIDs = append(prIDs, id)
		}
	}
	assert.GreaterOrEqual(t, len(prIDs), 1)
	assert.Equal(t, "#7", prIDs[0].Value)
}

func TestExtract_KleioTicket(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, _ := ext.Extract("sha1", "", "closes KL-42 add rate limiter", false)

	assert.GreaterOrEqual(t, len(ids), 1)
	found := false
	for _, id := range ids {
		if id.Value == "KL-42" {
			found = true
			assert.Equal(t, "kleio", id.Provider)
		}
	}
	assert.True(t, found)
}

func TestExtract_VersionTag(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, _ := ext.Extract("sha1", "", "release v1.2.0", false)

	var tags []kleio.Identifier
	for _, id := range ids {
		if id.Kind == kleio.IdentifierKindTag {
			tags = append(tags, id)
		}
	}
	assert.Len(t, tags, 1)
	assert.Equal(t, "v1.2.0", tags[0].Value)
}

func TestExtract_Deduplicates(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, _ := ext.Extract("sha1", "feat/PROJ-100-login", "fix PROJ-100: resolve login PROJ-100 again", false)

	count := 0
	for _, id := range ids {
		if id.Value == "PROJ-100" {
			count++
		}
	}
	assert.Equal(t, 1, count, "should deduplicate same ticket from branch + message")
}

func TestExtract_NoIdentifiers(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, links := ext.Extract("sha1", "main", "fix: typo in readme", false)
	assert.Empty(t, ids)
	assert.Empty(t, links)
}

func TestExtract_MultipleTypes(t *testing.T) {
	ext := NewIdentifierExtractor()
	ids, _ := ext.Extract("sha1", "feat/ENG-123-login",
		"Merge pull request #5 from user/feat/ENG-123-login\n\nresolves ENG-123, see v2.0.0", true)

	kinds := map[string]int{}
	for _, id := range ids {
		kinds[id.Kind]++
	}
	assert.Greater(t, kinds[kleio.IdentifierKindTicket], 0)
	assert.Greater(t, kinds[kleio.IdentifierKindPR], 0)
	assert.Greater(t, kinds[kleio.IdentifierKindTag], 0)
}
