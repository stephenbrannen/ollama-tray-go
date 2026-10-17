package icon_test

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stephenbrannen/ollama-tray-go/internal/icon"
	"github.com/stephenbrannen/ollama-tray-go/internal/status"
)

var states = []status.Status{status.Loaded, status.Idle, status.Unreachable}

func TestPNGIsValidAndColored(t *testing.T) {
	t.Parallel()

	for _, s := range states {
		t.Run(s.String(), func(t *testing.T) {
			t.Parallel()
			img, err := png.Decode(bytes.NewReader(icon.PNG(s)))
			require.NoError(t, err)

			b := img.Bounds()
			_, _, _, centerAlpha := img.At(b.Dx()/2, b.Dy()/2).RGBA()
			_, _, _, cornerAlpha := img.At(0, 0).RGBA()
			assert.NotZero(t, centerAlpha, "circle is filled at the center")
			assert.Zero(t, cornerAlpha, "corners are transparent")
		})
	}
}

func TestPNGDiffersPerState(t *testing.T) {
	t.Parallel()

	seen := map[string]status.Status{}
	for _, s := range states {
		key := string(icon.PNG(s))
		if prev, dup := seen[key]; dup {
			t.Errorf("%s and %s share the same icon", prev, s)
		}
		seen[key] = s
	}
}

func TestPNGUnknownStateFallsBackToUnreachable(t *testing.T) {
	t.Parallel()
	assert.Equal(t, icon.PNG(status.Unreachable), icon.PNG(status.Status(99)))
}
