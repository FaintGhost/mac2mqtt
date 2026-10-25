package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultNormalizeAndValidate(t *testing.T) {
	c := Default()
	c.Broker.Host = "localhost"
	c.Normalize()
	if err := c.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if c.Broker.Port != 1883 {
		t.Errorf("port = %d, want 1883", c.Broker.Port)
	}
	if c.Broker.ClientID == "" || c.NodeID() == "" {
		t.Error("client ID or node ID empty")
	}
	if strings.HasPrefix(c.HomeAssistant.BaseTopic, "mac2mqtt/") == false {
		t.Errorf("base topic = %q", c.HomeAssistant.BaseTopic)
	}
	if c.UI.Language != "" {
		t.Errorf("default language = %q, want follow system", c.UI.Language)
	}
	if c.Reporting.IntervalSeconds != 30 || c.Reporting.Enabled == false || c.Reporting.Retain == false {
		t.Errorf("unexpected reporting defaults: %+v", c.Reporting)
	}
}

func TestNormalizeFillsDerivedValues(t *testing.T) {
	c := Config{
		Broker:        Broker{Host: "  mqtt.local ", TLS: true},
		HomeAssistant: HomeAssistant{DeviceName: "Studio Mac", DiscoveryPrefix: "/homeassistant/"},
	}
	c.Normalize()
	if c.Broker.Port != 8883 {
		t.Errorf("TLS port = %d, want 8883", c.Broker.Port)
	}
	if c.Broker.Host != "mqtt.local" {
		t.Errorf("host = %q", c.Broker.Host)
	}
	if c.HomeAssistant.BaseTopic != "mac2mqtt/studio-mac" {
		t.Errorf("base topic = %q", c.HomeAssistant.BaseTopic)
	}
	if c.HomeAssistant.DiscoveryPrefix != "homeassistant" {
		t.Errorf("discovery prefix = %q", c.HomeAssistant.DiscoveryPrefix)
	}
	if c.Reporting.IntervalSeconds != 30 {
		t.Errorf("interval = %d", c.Reporting.IntervalSeconds)
	}
}

func TestValidateErrors(t *testing.T) {
	c := Default()
	c.Broker.Host = "mqtt://broker"
	c.Broker.Port = 70000
	c.Broker.ClientID = ""
	c.HomeAssistant.BaseTopic = "bad/#"
	c.Reporting.IntervalSeconds = 1
	err := c.Validate()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"host", "port", "client ID", "wildcards", "interval"} {
		if strings.Contains(err.Error(), want) == false {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestLoadMissingUsesDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, exists, err := Load(path)
	if err != nil || exists {
		t.Fatalf("Load = %+v, %v, %v", c, exists, err)
	}
	if c.Broker.Port != 1883 {
		t.Errorf("default port = %d", c.Broker.Port)
	}
}

func TestSaveLoadRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	c := Default()
	c.Broker.Host = "broker.example"
	c.Broker.Port = 8883
	c.Broker.TLS = true
	c.Broker.Username = "ada"
	c.Broker.Password = "secret"
	c.HomeAssistant.DeviceName = "Desk Mac"
	c.HomeAssistant.BaseTopic = "home/desk"
	c.Reporting.IntervalSeconds = 15
	c.Reporting.Retain = false
	c.UI.Language = "zh-Hans"
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, exists, err := Load(path)
	if err != nil || exists == false {
		t.Fatalf("Load: %v exists=%v", err, exists)
	}
	if got.Broker != c.Broker || !reflect.DeepEqual(got.HomeAssistant, c.HomeAssistant) || got.Reporting != c.Reporting || got.UI != c.UI {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, c)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
}

func TestValidateAllowsDisabledWithoutHost(t *testing.T) {
	c := Default()
	c.Reporting.Enabled = false
	c.Broker.Host = ""
	c.Normalize()
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled config with no host rejected: %v", err)
	}
}

func TestNormalizeTrimsTopics(t *testing.T) {
	c := Default()
	c.HomeAssistant.DiscoveryPrefix = "  homeassistant/  "
	c.HomeAssistant.BaseTopic = "  mac2mqtt/test/  "
	c.Normalize()
	if c.HomeAssistant.DiscoveryPrefix != "homeassistant" || c.HomeAssistant.BaseTopic != "mac2mqtt/test" {
		t.Errorf("topics = %q, %q", c.HomeAssistant.DiscoveryPrefix, c.HomeAssistant.BaseTopic)
	}
}

func TestSanitizeID(t *testing.T) {
	cases := map[string]string{
		"Mac mini (Studio)": "mac-mini-studio",
		"  MQTT_Device  ":   "mqtt_device",
		"":                  "mac2mqtt",
		":::":               "mac2mqtt",
		"UPPER.example.com": "upper-example-com",
	}
	for in, want := range cases {
		if got := SanitizeID(in); got != want {
			t.Errorf("SanitizeID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPublishedGroups(t *testing.T) {
	var h HomeAssistant
	if h.Publishes("media") == false {
		t.Fatal("groups are published by default")
	}
	h.SetPublishes("media", false)
	h.SetPublishes("audio", false)
	h.SetPublishes("media", false)
	if h.Publishes("media") || h.Publishes("audio") || h.Publishes("network") == false {
		t.Fatalf("disabled = %v", h.DisabledGroups)
	}
	if len(h.DisabledGroups) != 2 || h.DisabledGroups[0] != "audio" {
		t.Fatalf("disabled = %v, want sorted without duplicates", h.DisabledGroups)
	}
	h.SetPublishes("media", true)
	if h.Publishes("media") == false || len(h.DisabledGroups) != 1 {
		t.Fatalf("disabled = %v", h.DisabledGroups)
	}
}

func TestOptInGroups(t *testing.T) {
	var h HomeAssistant
	if h.Publishes("shortcuts") || !h.Disabled()["shortcuts"] {
		t.Fatal("shortcuts are published by default")
	}
	h.SetPublishes("shortcuts", true)
	if !h.Publishes("shortcuts") || h.Disabled()["shortcuts"] || len(h.DisabledGroups) != 0 {
		t.Fatalf("enabled = %v, disabled = %v", h.EnabledGroups, h.DisabledGroups)
	}
	h.SetPublishes("shortcuts", false)
	if h.Publishes("shortcuts") || len(h.EnabledGroups) != 0 {
		t.Fatalf("enabled = %v", h.EnabledGroups)
	}
}
