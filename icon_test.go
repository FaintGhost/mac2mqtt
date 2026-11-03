package main

import (
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

// iconMarks is resources/icon.svg without its body: the house and the
// signal, which ui draws, over a body drawn as a gradient, which its SVG
// reader does not.
const iconMarks = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
  <path d="M512 300 L736 486 L736 740 L288 740 L288 486 Z" fill="none" stroke="#ffffff" stroke-width="48" stroke-linejoin="round"/>
  <circle cx="512" cy="636" r="30" fill="#2dd4bf"/>
  <path d="M440 572 A102 102 0 0 1 584 572" fill="none" stroke="#2dd4bf" stroke-width="42" stroke-linecap="round"/>
</svg>`

// TestIconPNG writes resources/icon.png from resources/icon.svg's design
// when MAC2MQTT_ICON is set: go test -run TestIconPNG with MAC2MQTT_ICON=1.
func TestIconPNG(t *testing.T) {
	if os.Getenv("MAC2MQTT_ICON") == "" {
		t.Skip("set MAC2MQTT_ICON to write resources/icon.png")
	}
	img := renderIcon(1024)
	f, err := os.Create("resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// renderIcon draws the app icon size pixels square.
func renderIcon(size int) *image.RGBA {
	marks := ui.MustParseSVG([]byte(iconMarks))
	return ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Absolute().Left(100).Top(100).Size(824, 824).Radius(186).
			Gradient(ui.Hex("#22262d"), ui.Hex("#0d0f12"), 180).Border(2, ui.RGBA(255, 255, 255, 0.12))
		ui.Image(c, marks).Absolute().Left(0).Top(0).Size(1024, 1024)
	}, 1024, 1024, float32(size)/1024)
}
