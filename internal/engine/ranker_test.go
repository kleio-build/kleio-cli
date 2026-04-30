package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecencyScore_Recent(t *testing.T) {
	ts := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	s := recencyScore(ts)
	assert.Greater(t, s, 0.9)
}

func TestRecencyScore_Old(t *testing.T) {
	ts := time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	s := recencyScore(ts)
	assert.Less(t, s, 0.15)
}

func TestRecencyScore_InvalidTimestamp(t *testing.T) {
	assert.Equal(t, 0.0, recencyScore("not-a-date"))
}

func TestKeywordScore_AllMatch(t *testing.T) {
	s := keywordScore("feat: add auth module login", "auth login")
	assert.Equal(t, 1.0, s)
}

func TestKeywordScore_PartialMatch(t *testing.T) {
	s := keywordScore("feat: add auth module", "auth login database")
	assert.InDelta(t, 1.0/3.0, s, 0.01)
}

func TestKeywordScore_NoMatch(t *testing.T) {
	s := keywordScore("nothing relevant here", "auth login")
	assert.Equal(t, 0.0, s)
}

func TestKeywordScore_EmptyQuery(t *testing.T) {
	s := keywordScore("some text", "")
	assert.Equal(t, 0.0, s)
}

func TestFileOverlapScore_Full(t *testing.T) {
	s := fileOverlapScore([]string{"a.go", "b.go"}, []string{"a.go", "b.go"})
	assert.Equal(t, 1.0, s)
}

func TestFileOverlapScore_Partial(t *testing.T) {
	s := fileOverlapScore([]string{"a.go", "b.go"}, []string{"b.go", "c.go"})
	assert.InDelta(t, 1.0/3.0, s, 0.01)
}

func TestFileOverlapScore_None(t *testing.T) {
	s := fileOverlapScore([]string{"a.go"}, []string{"b.go"})
	assert.Equal(t, 0.0, s)
}

func TestFileOverlapScore_Empty(t *testing.T) {
	assert.Equal(t, 0.0, fileOverlapScore(nil, []string{"a.go"}))
	assert.Equal(t, 0.0, fileOverlapScore([]string{"a.go"}, nil))
}

func TestSortByScore(t *testing.T) {
	items := []ScoredItem[string]{
		{Item: "low", Score: 0.1},
		{Item: "high", Score: 0.9},
		{Item: "mid", Score: 0.5},
	}
	SortByScore(items)
	assert.Equal(t, "high", items[0].Item)
	assert.Equal(t, "mid", items[1].Item)
	assert.Equal(t, "low", items[2].Item)
}
