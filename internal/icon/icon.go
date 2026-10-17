// Package icon draws the tray icons at runtime so the repo holds no binary assets.
package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/stephenbrannen/ollama-tray-go/internal/status"
)

// size is large enough to stay sharp on high-DPI panels; the tray scales it down.
const size = 64

var (
	green = color.RGBA{R: 0x2e, G: 0xa0, B: 0x43, A: 0xff}
	grey  = color.RGBA{R: 0x8b, G: 0x94, B: 0x9e, A: 0xff}
	red   = color.RGBA{R: 0xd7, G: 0x3a, B: 0x49, A: 0xff}

	once  sync.Once
	cache map[status.Status][]byte
)

// PNG returns the icon for s as PNG bytes. The result is shared; callers must not modify it.
func PNG(s status.Status) []byte {
	once.Do(func() {
		cache = map[status.Status][]byte{
			status.Loaded:      draw(green),
			status.Idle:        draw(grey),
			status.Checking:    draw(grey),
			status.Unreachable: draw(red),
		}
	})
	if b, ok := cache[s]; ok {
		return b
	}
	return cache[status.Unreachable]
}

// draw renders a filled circle on a transparent background.
func draw(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	const (
		center = float64(size) / 2
		radius = float64(size)/2 - 4
	)
	for y := range size {
		for x := range size {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	// Encoding an in-memory RGBA image cannot fail.
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
