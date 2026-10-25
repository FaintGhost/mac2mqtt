package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/config"
	"mac2mqtt/internal/service"
	"mac2mqtt/locales"
)

func testApp() *app {
	a := newApp()
	a.applyConfig(config.Default())
	a.configError = "MQTT broker is not configured yet."
	a.settingsError = "Enter your MQTT broker host, then save."
	return a
}

func TestViewShowsStatusAndAbout(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 540, 700)
	if tt.HasText("Mac2MQTT") == false {
		t.Fatalf("missing app title; texts: %q", tt.Texts())
	}
	if tt.HasText("Connect your Mac to Home Assistant") == false {
		t.Fatalf("missing onboarding hero; texts: %q", tt.Texts())
	}
	if err := tt.Click("About"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Topics") == false {
		t.Fatalf("about tab did not open; texts: %q", tt.Texts())
	}
}

func TestSettingsTabShowsBrokerFields(t *testing.T) {
	a := testApp()
	a.tab = tabSettings
	tt := ui.NewTester(a.view, 540, 700)
	for _, want := range []string{"MQTT broker", "Host", "Port", "TLS", "Discovery", "Base topic", "Save & Reconnect"} {
		if tt.HasText(want) == false {
			t.Errorf("settings is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestSaveSettingsPersistsWithoutStartingService(t *testing.T) {
	a := testApp()
	a.cfgPath = filepath.Join(t.TempDir(), "config.json")
	a.draft.Broker.Host = "broker.example"
	a.draft.Broker.Port = 1883
	a.draft.Broker.ClientID = "mac2mqtt-test"
	a.draft.HomeAssistant.DeviceName = "Test Mac"
	a.draft.HomeAssistant.BaseTopic = "mac2mqtt/test"
	a.draft.Reporting.Enabled = false
	a.editPort = 1883
	a.editInterval = 30
	a.saveSettings()
	if a.settingsError != "" {
		t.Fatalf("save failed: %s", a.settingsError)
	}
	cfg, exists, err := config.Load(a.cfgPath)
	if err != nil || exists == false {
		t.Fatalf("load saved config: %v, exists=%v", err, exists)
	}
	if cfg.Broker.Host != "broker.example" || cfg.Reporting.Enabled {
		t.Errorf("saved config = %+v", cfg)
	}
}

func TestSaveDisabledConfigWithoutBrokerHost(t *testing.T) {
	a := testApp()
	a.cfgPath = filepath.Join(t.TempDir(), "config.json")
	a.draft.Reporting.Enabled = false
	a.draft.Broker.Host = ""
	a.editPort = 1883
	a.editInterval = 30
	a.saveSettings()
	if a.settingsError != "" {
		t.Fatalf("save failed: %s", a.settingsError)
	}
	cfg, exists, err := config.Load(a.cfgPath)
	if err != nil || exists == false {
		t.Fatalf("load saved config: %v, exists=%v", err, exists)
	}
	if cfg.Reporting.Enabled || cfg.Broker.Host != "" {
		t.Errorf("saved config = %+v", cfg)
	}
}

func TestCleanupTopics(t *testing.T) {
	old := testApp().cfg
	old.Broker.Host = "broker.example"
	old.HomeAssistant.DiscoveryPrefix = "homeassistant"
	old.HomeAssistant.BaseTopic = "mac2mqtt/old"
	old.Broker.ClientID = "mac2mqtt-old"
	old.Reporting.Retain = true
	next := old
	next.HomeAssistant.BaseTopic = "mac2mqtt/new"
	next.Broker.ClientID = "mac2mqtt-new"
	topics := cleanupTopics(old, next)
	if len(topics) < 14 {
		t.Errorf("cleanup topics = %v", topics)
	}
	want := map[string]bool{
		"mac2mqtt/old/state":        true,
		"mac2mqtt/old/availability": true,
	}
	for _, topic := range topics {
		delete(want, topic)
	}
	if len(want) != 0 {
		t.Errorf("missing cleanup topics: %v", want)
	}
	next.HomeAssistant.BaseTopic = old.HomeAssistant.BaseTopic
	next.Broker.ClientID = old.Broker.ClientID
	next.Reporting.Retain = false
	topics = cleanupTopics(old, next)
	if len(topics) != 1 || topics[0] != "mac2mqtt/old/state" {
		t.Errorf("retain cleanup = %v", topics)
	}
}

func TestStatus(t *testing.T) {
	a := testApp()
	if st := a.status(); st.label != locales.StatusNotConfigured() {
		t.Errorf("no host = %q", st.label)
	}
	a = connectedTestApp()
	if st := a.status(); st.label != locales.StatusConnected() || st.phase != service.PhaseOnline {
		t.Errorf("online = %+v", st)
	}
	a.mqttStatus.Phase, a.mqttStatus.LastError = service.PhaseError, "refused"
	if st := a.status(); st.label != locales.StatusConnectionError() || st.detail != "refused" {
		t.Errorf("error = %+v", st)
	}
	a.cfg.Reporting.Enabled = false
	if st := a.status(); st.label != locales.StatusPaused() || st.phase != service.PhaseIdle {
		t.Errorf("paused = %+v", st)
	}
}
