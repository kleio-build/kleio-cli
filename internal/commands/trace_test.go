package commands

import (
	"testing"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/stretchr/testify/assert"
)

func TestKindIcon(t *testing.T) {
	assert.Equal(t, "[commit]", kindIcon(kleio.SignalTypeGitCommit))
	assert.Equal(t, "[decision]", kindIcon(kleio.SignalTypeDecision))
	assert.Equal(t, "[work_item]", kindIcon(kleio.SignalTypeWorkItem))
	assert.Equal(t, "[checkpoint]", kindIcon(kleio.SignalTypeCheckpoint))
	assert.Equal(t, "[other]", kindIcon("other"))
}

func TestIsInteractive(t *testing.T) {
	result := isInteractive()
	assert.IsType(t, true, result)
}
