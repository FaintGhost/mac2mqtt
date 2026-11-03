package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/system"
)

// TestSiteAssets writes the website's screenshots and icons into site/assets
// when MAC2MQTT_SITE is set: MAC2MQTT_SITE=1 go test -run TestSiteAssets .
func TestSiteAssets(t *testing.T) {
	if os.Getenv("MAC2MQTT_SITE") == "" {
		t.Skip("set MAC2MQTT_SITE to write site/assets")
	}
	defer i18n.SetLocaleFunc(func() string { return "en" })
	for _, shot := range []struct {
		name string
		tab  int
	}{{"overview", tabOverview}, {"entities", tabEntities}} {
		for _, lang := range []string{"en", "zh"} {
			for _, dark := range []bool{false, true} {
				locale := map[string]string{"en": "en", "zh": "zh-Hans"}[lang]
				i18n.SetLocaleFunc(func() string { return locale })
				a := siteApp()
				a.tab = shot.tab
				tt := ui.NewTester(func(c *ui.Context) {
					a.view(c)
					trafficLights(c)
				}, 820, 600)
				tt.SetScale(2)
				tt.SetTitleBar(ui.TitleBar{Height: 52, Left: 88})
				tt.SetDark(dark)
				tt.Frame()
				tt.Frame()
				theme := "light"
				if dark {
					theme = "dark"
				}
				writePNG(t, filepath.Join("site", "assets", shot.name+"-"+lang+"-"+theme+".png"), windowCorners(tt.Image(), 2*windowRadius))
			}
		}
	}
	// The icons, from resources/icon.png's design.
	for name, size := range map[string]int{"icon.png": 1024, "apple-touch-icon.png": 180, "favicon.png": 64} {
		writePNG(t, filepath.Join("site", "assets", name), renderIcon(size))
	}
	icon, err := os.ReadFile("resources/icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("site", "assets", "icon.svg"), icon, 0o644); err != nil {
		t.Fatal(err)
	}
}

// siteApp is a Mac as the website shows it: a broker on the local network,
// music playing, a camera and a microphone.
func siteApp() *app {
	a := connectedTestApp()
	a.cfg.Broker.Host = "homeassistant.local"
	a.cfg.HomeAssistant.BaseTopic = "mac2mqtt/studio"
	a.cfg.HomeAssistant.DeviceName = "Studio"
	a.applyConfig(a.cfg)
	when := time.Date(2026, 11, 3, 21, 42, 8, 0, time.Local)
	a.mqttStatus.Published, a.mqttStatus.LastPublish = 1284, when
	a.latest.LastSeen = when
	a.latest.Session.IdleSeconds = 8
	a.latest.Audio = &system.AudioState{Volume: 42}
	a.latest.Activity = system.ActivityInfo{HasCamera: true, HasMicrophone: true}
	a.latest.Media = &system.MediaInfo{
		State: "playing", Title: "Come Away With Me", Artist: "Norah Jones", Album: "Come Away With Me",
		AppName: "Music", DurationSeconds: 198, PositionSeconds: 106, PlaybackRate: 1,
		CanPrevious: true, CanNext: true,
	}
	return a
}

// trafficLights draws the window's close, minimize and zoom buttons, which
// macOS draws over the window and a test does not.
func trafficLights(c *ui.Context) {
	for i, color := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		ui.Box(c).Absolute().Left(float32(20+i*20)).Top(20).Size(12, 12).Radius(6).
			Background(ui.Hex(color)).Border(0.5, ui.RGBA(0, 0, 0, 0.12)).PassThrough()
	}
}

// windowCorners rounds an image's corners to a window's radius, in pixels,
// leaving them transparent.
func windowCorners(img *image.RGBA, r int) *image.RGBA {
	b := img.Bounds()
	for y := 0; y < r; y++ {
		for x := 0; x < r; x++ {
			dx, dy := float64(r-x)-0.5, float64(r-y)-0.5
			if math.Hypot(dx, dy) <= float64(r) {
				continue
			}
			for _, p := range [][2]int{{x, y}, {b.Max.X - 1 - x, y}, {x, b.Max.Y - 1 - y}, {b.Max.X - 1 - x, b.Max.Y - 1 - y}} {
				img.SetRGBA(p[0], p[1], color.RGBA{})
			}
		}
	}
	return img
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
