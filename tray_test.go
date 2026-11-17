package main

import (
	"testing"

	"github.com/egoist/mygo"

	"mac2mqtt/internal/service"
	"mac2mqtt/locales"
)

func TestOpenTrayMenuFollowsConnectionAndReportingChanges(t *testing.T) {
	a := testApp()
	a.cfg.Broker.Host = "broker.example"
	a.cfg.Reporting.Enabled = true
	a.mqttStatus.Phase = service.PhaseOnline
	// Only the live items are needed here. Building the full tray menu also
	// asks the updater for its locale, which requires a running app loop.
	a.openTrayMenu = mygo.NewMenu([]*mygo.MenuItem{
		{ID: "status"}, {ID: "publish"}, {ID: "reconnect"},
		{ID: "reporting"}, {ID: "login", Type: mygo.MenuItemCheckbox},
	})
	a.updateTray()
	menu := a.openTrayMenu
	status := menu.ItemByID("status")
	publish := menu.ItemByID("publish")
	reconnect := menu.ItemByID("reconnect")
	reporting := menu.ItemByID("reporting")
	login := menu.ItemByID("login")
	if status.Label != locales.StatusConnected() || publish.Disabled || reconnect.Disabled {
		t.Fatal("connected menu does not offer publishing and reconnecting")
	}

	// Keep the same native items while a pause overrides a still-online
	// service snapshot. The worker reports its idle state asynchronously.
	a.cfg.Reporting.Enabled = false
	a.updateTray()
	if a.openTrayMenu != menu || menu.ItemByID("status") != status {
		t.Fatal("updating the open menu replaced its items")
	}
	if status.Label != locales.StatusPaused() || !publish.Disabled || !reconnect.Disabled || reporting.Label != locales.MenuResumeReporting() {
		t.Fatal("paused menu retained connected actions")
	}

	a.cfg.Reporting.Enabled = true
	a.mqttStatus.Phase = service.PhaseConnecting
	a.openAtLogin = true
	a.updateTray()
	if status.Label != locales.StatusConnecting() || !publish.Disabled || reconnect.Disabled || reporting.Label != locales.MenuPauseReporting() || !login.Checked {
		t.Fatal("reconnecting menu did not refresh its controls")
	}

	a.mqttStatus.Phase = service.PhaseOnline
	a.updateTray()
	onlineIcon := status.Icon
	a.mqttStatus.LastError = "publish failed"
	a.updateTray()
	if status.Icon == onlineIcon || status.Label != locales.StatusConnected() {
		t.Fatal("a publish failure did not change the connected dot's color")
	}

	a.cfg.Broker.Host = ""
	a.updateTray()
	if status.Label != locales.StatusNotConfigured() || status.Subtitle != "" || !publish.Disabled || !reconnect.Disabled || !reporting.Disabled {
		t.Fatal("unconfigured menu retained broker actions")
	}
}

func TestTruncateCountsWideCharactersTwice(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"short", "short"},
		{"abcdefghij", "abcdefghij"},
		{"abcdefghijk", "abcdefghi…"},
		{"我的体育老", "我的体育老"},
		{"我的！体育老师TV版", "我的！体…"},
	} {
		if got := truncate(c.in, 10); got != c.want {
			t.Errorf("truncate(%q, 10) = %q, want %q", c.in, got, c.want)
		}
	}
}
