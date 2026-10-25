package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/service"
	"mac2mqtt/internal/system"
)

// TestSnapshots writes PNGs of each page, light and dark, into the directory
// MAC2MQTT_SNAPSHOTS names, to look at the design without opening a window.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("MAC2MQTT_SNAPSHOTS")
	if dir == "" {
		t.Skip("set MAC2MQTT_SNAPSHOTS to a directory to write snapshots")
	}
	for _, page := range []struct {
		name string
		tab  int
		app  func() *app
	}{
		{"onboarding", 0, testApp},
		{"overview", 0, connectedTestApp},
		{"entities", tabEntities, connectedTestApp},
		{"settings", tabSettings, connectedTestApp},
		{"about", tabAbout, connectedTestApp},
		{"overview-zh", tabOverview, connectedTestApp},
		{"entities-zh", tabEntities, connectedTestApp},
	} {
		for _, dark := range []bool{false, true} {
			a := page.app()
			a.tab = page.tab
			tt := ui.NewTester(a.view, 820, 660)
			tt.SetScale(2)
			tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 88})
			tt.SetDark(dark)
			if strings.HasSuffix(page.name, "-zh") {
				i18n.SetLocaleFunc(func() string { return "zh-Hans" })
			} else {
				i18n.SetLocaleFunc(func() string { return "en" })
			}
			tt.Frame()
			name := page.name
			if dark {
				name += "-dark"
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}

func connectedTestApp() *app {
	a := testApp()
	a.configError, a.settingsError = "", ""
	a.cfg.Broker.Host = "192.168.1.10"
	a.cfg.HomeAssistant.BaseTopic = "mac2mqtt/mac-mini"
	a.cfg.HomeAssistant.DeviceName = "mac-mini"
	a.draft = a.cfg
	a.mqttStatus = service.Status{Phase: service.PhaseOnline, Published: 128, LastPublish: time.Date(2026, 10, 25, 19, 30, 15, 0, time.Local)}
	a.latest = system.Snapshot{
		LastSeen: time.Date(2026, 10, 25, 19, 30, 15, 0, time.Local),
		Device:   system.DeviceInfo{Hostname: "mac-mini", OSVersion: "27.0.1", Arch: "arm64"},
		Network:  system.NetworkInfo{Online: true},
		Power:    system.PowerInfo{Source: "AC"},
		Session:  system.SessionInfo{IdleSeconds: 42},
		Audio:    &system.AudioState{Volume: 35},
		Media: &system.MediaInfo{
			State: "playing", Title: "“超微距”下的瑞士长什么样？", Artist: "Mediastorm影视飓风",
			AppName: "Google Chrome", DurationSeconds: 1368.961, PositionSeconds: 412.5, PlaybackRate: 1,
		},
	}
	return a
}
