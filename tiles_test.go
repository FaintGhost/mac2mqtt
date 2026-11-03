package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mac2mqtt/locales"
)

// TestTilesShareColumns checks that the tiles of This Mac are as wide as
// each other: in three columns, each left tile's card ends where the
// others' do.
func TestTilesShareColumns(t *testing.T) {
	for _, width := range []int{720, 820, 1100} {
		a := connectedTestApp()
		tt := ui.NewTester(a.view, width, 900)
		tt.Frame()
		img := tt.Image()
		var ends []int
		for _, label := range []string{locales.MetricNetwork(), locales.MetricScreen()} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("no %q tile", label)
			}
			y := int(r.Y + r.H/2)
			// Past the label, across the card's own face to its edge: the
			// canvas behind, with its light, is no one color to look for.
			x := int(r.X+r.W) + 2
			face := img.RGBAAt(x, y)
			for x < width && img.RGBAAt(x, y) == face {
				x++
			}
			ends = append(ends, x)
		}
		if ends[0] != ends[1] {
			t.Errorf("window %d wide: the left tiles end at %v, not as one column", width, ends)
		}
	}
}
