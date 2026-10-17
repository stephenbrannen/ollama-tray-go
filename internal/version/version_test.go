package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringDefaultsToDevBuild(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "ollama-tray dev", String())
}
