package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mac2mqtt/locales"
)

// TestTilesShareColumns checks that the tiles of This Mac are as wide as
// each other, a last one alone on its row included: each left tile's card
// ends where the others' do.
func TestTilesShareColumns(t *testing.T) {
	for _, width := range []int{720, 820, 1100} {
		a := connectedTestApp()
		tt := ui.NewTester(a.view, width, 900)
		tt.Frame()
		img := tt.Image()
		canvas := img.RGBAAt(width-4, 2) // the window's canvas, beside the cards
		var ends []int
		for _, label := range []string{locales.MetricNetwork(), locales.MetricVolume(), locales.MetricDisplay()} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("no %q tile", label)
			}
			y := int(r.Y + r.H/2)
			x := int(r.X + r.W)
			for x < width && img.RGBAAt(x, y) != canvas {
				x++
			}
			ends = append(ends, x)
		}
		if ends[0] != ends[1] || ends[0] != ends[2] {
			t.Errorf("window %d wide: the left tiles end at %v, not as one column", width, ends)
		}
	}
}
