package homeassistant

import (
	"encoding/json"
	"strings"
	"testing"
)

func testOptions() Options {
	return Options{
		BaseTopic:         "mac2mqtt/studio",
		StateTopic:        "mac2mqtt/studio/state",
		AvailabilityTopic: "mac2mqtt/studio/availability",
		Device: Device{
			ID:           "studio",
			Name:         "Studio Mac",
			Manufacturer: "Mac2MQTT",
			Model:        "macOS",
			SWVersion:    "0.1.0",
		},
		HasBattery:    true,
		HasCamera:     true,
		HasMicrophone: true,
	}
}

func TestSystemEntitiesHaveStableUniqueTopics(t *testing.T) {
	entities := SystemEntities(testOptions())
	if len(entities) != 28 {
		t.Fatalf("entities = %d, want 28", len(entities))
	}
	topics := make(map[string]bool)
	uniqueIDs := make(map[string]bool)
	for _, entity := range entities {
		if entity.Component == "" || entity.ObjectID == "" && entity.Component != "media_player" {
			t.Errorf("incomplete entity: %+v", entity)
		}
		topic := entity.DiscoveryTopic("homeassistant", "studio")
		want := "homeassistant/" + entity.Component + "/studio/" + entity.ObjectID + "/config"
		if entity.ObjectID == "" {
			want = "homeassistant/" + entity.Component + "/studio/config"
		}
		if topic != want {
			t.Errorf("discovery topic = %q", topic)
		}
		if topics[topic] {
			t.Errorf("duplicate discovery topic %q", topic)
		}
		topics[topic] = true
		unique, _ := entity.Config["unique_id"].(string)
		if unique == "" || uniqueIDs[unique] {
			t.Errorf("duplicate or empty unique_id %q", unique)
		}
		uniqueIDs[unique] = true
		payload, err := json.Marshal(entity.Config)
		if err != nil {
			t.Errorf("%s does not marshal: %v", entity.ObjectID, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Errorf("%s JSON invalid: %v", entity.ObjectID, err)
		}
		if decoded["availability"] != nil {
			continue // checked by TestNextTrackAvailability
		}
		if decoded["availability_topic"] != "mac2mqtt/studio/availability" {
			t.Errorf("%s availability = %v", entity.ObjectID, decoded["availability_topic"])
		}
		if decoded["payload_available"] != "online" || decoded["payload_not_available"] != "offline" {
			t.Errorf("%s availability payloads wrong", entity.ObjectID)
		}
	}
}

func TestSystemEntitiesExpectedEntities(t *testing.T) {
	entities := SystemEntities(testOptions())
	byID := make(map[string]Entity, len(entities))
	for _, entity := range entities {
		byID[entity.ObjectID] = entity
	}
	for _, id := range []string{"network_connectivity", "charging", "battery", "power_source", "last_update", "app_uptime", "app_version", "hostname", "idle_time", "volume", "mute", "refresh", "media_state", "now_playing", "screen_locked"} {
		if _, ok := byID[id]; ok == false {
			t.Errorf("missing entity %q", id)
		}
	}
	if byID["volume"].Component != "number" || byID["volume"].Config["command_topic"] != "mac2mqtt/studio/volume/set" {
		t.Errorf("volume entity = %+v", byID["volume"])
	}
	if byID["mute"].Component != "switch" || byID["mute"].Config["command_topic"] != "mac2mqtt/studio/mute/set" {
		t.Errorf("mute entity = %+v", byID["mute"])
	}
	if byID["refresh"].Component != "button" || byID["refresh"].Config["command_topic"] != "mac2mqtt/studio/refresh" {
		t.Errorf("refresh entity = %+v", byID["refresh"])
	}
	if byID["media_state"].Component != "sensor" || byID["media_state"].Config["state_topic"] != "mac2mqtt/studio/media/state" || byID["media_state"].Config["device_class"] != "enum" {
		t.Errorf("media state entity = %+v", byID["media_state"])
	}
	if byID["now_playing"].Component != "sensor" || byID["now_playing"].Config["state_topic"] != "mac2mqtt/studio/media/attributes" || byID["now_playing"].Config["json_attributes_topic"] != "mac2mqtt/studio/media/attributes" {
		t.Errorf("now playing entity = %+v", byID["now_playing"])
	}
	for _, entity := range entities {
		// Only the MQTT Media Player integration's: Home Assistant's own
		// MQTT discovery has no media_player.
		if entity.Component == "media_player" && entity.Config["state_state_topic"] == nil {
			t.Errorf("%s is a media_player no integration reads", entity.ObjectID)
		}
	}
	if byID["last_update"].Config["device_class"] != "timestamp" {
		t.Errorf("last update = %+v", byID["last_update"])
	}
}

func TestDesktopHasNoBatteryEntities(t *testing.T) {
	opts := testOptions()
	opts.HasBattery = false
	entities := SystemEntities(opts)
	if len(entities) != 26 {
		t.Fatalf("desktop entities = %d, want 26", len(entities))
	}
	for _, entity := range entities {
		if entity.ObjectID == "battery" || entity.ObjectID == "charging" {
			t.Errorf("desktop unexpectedly has %s entity", entity.ObjectID)
		}
	}
}

func TestSystemEntitiesDeriveTopics(t *testing.T) {
	opts := testOptions()
	opts.StateTopic = ""
	opts.AvailabilityTopic = ""
	entities := SystemEntities(opts)
	for _, entity := range entities {
		if entity.Config["state_topic"] == nil || entity.Group == GroupMedia {
			continue
		}
		if entity.Config["state_topic"] != "mac2mqtt/studio/state" {
			t.Errorf("%s state topic = %v", entity.ObjectID, entity.Config["state_topic"])
		}
		if entity.Config["availability_topic"] != "mac2mqtt/studio/availability" {
			t.Errorf("%s availability topic = %v", entity.ObjectID, entity.Config["availability_topic"])
		}
	}
}

func TestDevicePayload(t *testing.T) {
	payload := DevicePayload(Device{ID: "studio", Name: "Studio Mac", Manufacturer: "Mac2MQTT", Model: "macOS", SWVersion: "0.1.0"})
	identifiers, ok := payload["identifiers"].([]string)
	if ok == false || len(identifiers) != 1 || identifiers[0] != "studio" {
		t.Errorf("identifiers = %#v", payload["identifiers"])
	}
	if payload["name"] != "Studio Mac" || payload["manufacturer"] != "Mac2MQTT" {
		t.Errorf("device payload = %#v", payload)
	}
}

func TestDisabledGroupsAreLeftOut(t *testing.T) {
	opts := testOptions()
	opts.Disabled = map[string]bool{GroupMedia: true, GroupAudio: true}
	for _, entity := range SystemEntities(opts) {
		if entity.Group == GroupMedia || entity.Group == GroupAudio {
			t.Errorf("disabled group published %s", entity.ObjectID)
		}
	}
	if n := len(AllEntities(opts)); n != 29 {
		t.Errorf("all entities = %d, want 29 whatever is disabled", n)
	}
	for _, entity := range AllEntities(opts) {
		if entity.Group == "" && entity.ObjectID != "refresh" {
			t.Errorf("%s has no group", entity.ObjectID)
		}
	}
}

func TestScreenLockedTemplate(t *testing.T) {
	for _, entity := range SystemEntities(testOptions()) {
		if entity.ObjectID == "screen_locked" {
			if entity.Component != "binary_sensor" || entity.Config["value_template"] != stateLocked {
				t.Errorf("screen locked = %+v", entity)
			}
			return
		}
	}
	t.Error("no screen_locked entity")
}

func TestMediaPlayerForMQTTMediaPlayer(t *testing.T) {
	var player Entity
	for _, entity := range SystemEntities(testOptions()) {
		if entity.Component == "media_player" {
			player = entity
		}
	}
	if player.Group != GroupMedia {
		t.Fatalf("media player = %+v", player)
	}
	if topic := player.DiscoveryTopic("homeassistant", "studio"); topic != "homeassistant/media_player/studio/config" {
		t.Errorf("discovery topic = %q", topic)
	}
	for key, want := range map[string]string{
		"state_state_topic":       "mac2mqtt/studio/media/state",
		"state_volume_topic":      "mac2mqtt/studio/media/volume",
		"command_playpause_topic": "mac2mqtt/studio/media/command",
		"command_next_payload":    "NEXT",
		"command_seek_topic":      "mac2mqtt/studio/media/seek",
		"name":                    "Studio Mac",
	} {
		if player.Config[key] != want {
			t.Errorf("%s = %v, want %s", key, player.Config[key], want)
		}
	}
	want := []string{
		"mac2mqtt/studio/media/command", "mac2mqtt/studio/media/command", "mac2mqtt/studio/media/command",
		"mac2mqtt/studio/media/command", "mac2mqtt/studio/media/command",
		"mac2mqtt/studio/media/mute/set", "mac2mqtt/studio/media/seek", "mac2mqtt/studio/media/volume/set",
	}
	if got := player.CommandTopics(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("command topics = %q", got)
	}
}

func TestShortcutSelectListsShortcuts(t *testing.T) {
	opts := testOptions()
	opts.Shortcuts = []string{"Morning", "Focus On"}
	for _, entity := range SystemEntities(opts) {
		if entity.ObjectID == "run_shortcut" {
			options, _ := entity.Config["options"].([]string)
			if entity.Component != "select" || strings.Join(options, ",") != "Morning,Focus On" || entity.Config["command_topic"] != "mac2mqtt/studio/shortcut/run" {
				t.Errorf("run shortcut = %+v", entity)
			}
			return
		}
	}
	t.Error("no run_shortcut select")
}

func TestSystemButtons(t *testing.T) {
	want := map[string]string{"lock_screen": "LOCK", "display_sleep": "DISPLAY_SLEEP", "display_wake": "DISPLAY_WAKE", "sleep": "SLEEP"}
	for _, entity := range SystemEntities(testOptions()) {
		if payload, ok := want[entity.ObjectID]; ok {
			if entity.Component != "button" || entity.Config["payload_press"] != payload || entity.Config["command_topic"] != "mac2mqtt/studio/system/command" {
				t.Errorf("%s = %+v", entity.ObjectID, entity)
			}
			delete(want, entity.ObjectID)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing %v", want)
	}
}

func TestNoCameraOrMicrophoneNoActivityEntities(t *testing.T) {
	opts := testOptions()
	opts.HasCamera, opts.HasMicrophone = false, false
	for _, entity := range SystemEntities(opts) {
		if entity.ObjectID == "camera_in_use" || entity.ObjectID == "microphone_in_use" {
			t.Errorf("a Mac without them publishes %s", entity.ObjectID)
		}
	}
	opts.HasMicrophone = true
	mic := false
	for _, entity := range SystemEntities(opts) {
		switch entity.ObjectID {
		case "camera_in_use":
			t.Error("a Mac without a camera publishes camera_in_use")
		case "microphone_in_use":
			mic = true
		}
	}
	if !mic {
		t.Error("a Mac with a microphone lacks microphone_in_use")
	}
}

func TestNextTrackAvailability(t *testing.T) {
	for _, entity := range SystemEntities(testOptions()) {
		if entity.ObjectID != "media_next" && entity.ObjectID != "media_previous" {
			continue
		}
		list, _ := entity.Config["availability"].([]map[string]any)
		if len(list) != 2 || list[0]["topic"] != "mac2mqtt/studio/availability" || entity.Config["availability_mode"] != "all" || entity.Config["availability_topic"] != nil {
			t.Errorf("%s availability = %+v", entity.ObjectID, entity.Config)
		}
	}
}

func TestNamesAndEntityIDs(t *testing.T) {
	opts := testOptions()
	opts.Device.Name = "Mac mini"
	opts.Names = map[string]string{"now_playing": "正在播放"}
	for _, entity := range SystemEntities(opts) {
		switch entity.ObjectID {
		case "now_playing":
			if entity.Config["name"] != "正在播放" || entity.Config["default_entity_id"] != "sensor.mac_mini_now_playing" {
				t.Errorf("now playing = %v, %v", entity.Config["name"], entity.Config["default_entity_id"])
			}
		case "lock_screen":
			if entity.Config["name"] != "Lock Screen" || entity.Config["default_entity_id"] != "button.mac_mini_lock_screen" {
				t.Errorf("lock screen = %v, %v", entity.Config["name"], entity.Config["default_entity_id"])
			}
		}
		if entity.Component == "media_player" && entity.Config["default_entity_id"] != nil {
			t.Error("the media player, another integration's, has a default entity ID")
		}
	}
}
