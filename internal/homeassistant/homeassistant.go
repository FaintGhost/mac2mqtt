// Package homeassistant builds Home Assistant MQTT discovery payloads for the
// Mac2MQTT device. It deliberately describes entities only; command handling
// belongs to the service layer so new controls can be added without coupling
// them to discovery generation.
package homeassistant

import (
	"fmt"
	"sort"
	"strings"
)

// Device identifies the physical device in Home Assistant's device registry.
type Device struct {
	ID           string
	Name         string
	Manufacturer string
	Model        string
	ModelID      string // the hardware's identifier, such as "Mac16,10"
	SWVersion    string
}

// Entity is one Home Assistant MQTT discovery entity.
type Entity struct {
	Component string // "sensor", "binary_sensor", "number", "switch", "button"
	ObjectID  string
	// Group is the group the entity is published with, "" for always.
	Group  string
	Config map[string]any
}

// DiscoveryTopic returns the conventional Home Assistant MQTT discovery topic
// for this entity: <prefix>/<component>/<node_id>/<object_id>/config.
func (e Entity) DiscoveryTopic(prefix, nodeID string) string {
	parts := make([]string, 0, 5)
	for _, part := range []string{prefix, e.Component, nodeID, e.ObjectID, "config"} {
		part = strings.Trim(part, "/")
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "/")
}

// DevicePayload returns the value used for the discovery config's "device"
// key. The identifiers list is what links all entities to one HA device.
func DevicePayload(d Device) map[string]any {
	payload := map[string]any{
		"identifiers":  []string{d.ID},
		"name":         d.Name,
		"manufacturer": d.Manufacturer,
		"model":        d.Model,
		"sw_version":   d.SWVersion,
	}
	if d.ModelID != "" {
		payload["model_id"] = d.ModelID
	}
	return payload
}

// Options contains the topics and device metadata shared by all generated
// entities. StateTopic and AvailabilityTopic are derived from BaseTopic when
// they are empty.
type Options struct {
	BaseTopic         string
	StateTopic        string
	AvailabilityTopic string
	Device            Device
	// HasBattery adds the battery level and charging entities. Desktops
	// such as a Mac mini or Mac Studio do not have them.
	HasBattery bool
	// Disabled names the groups the user does not publish.
	Disabled map[string]bool
	// Names are the entities' names by object ID, in the language Home
	// Assistant should show; missing ones are English.
	Names map[string]string
	// HasCamera and HasMicrophone add the entities telling whether they
	// are in use: a Mac without them has nothing to tell.
	HasCamera     bool
	HasMicrophone bool
	// Shortcuts are the names of the user's shortcuts, which the Run
	// Shortcut select offers; with none it is left out.
	Shortcuts []string
}

const (
	payloadAvailable   = "online"
	payloadUnavailable = "offline"

	statePowerBattery = "{{ value_json.power.battery_percent | default('unknown') }}"
	statePowerSource  = "{{ value_json.power.source | default('unknown') }}"
	stateLastSeen     = "{{ value_json.last_seen | default('unknown') }}"
	stateUptime       = "{{ value_json.app.uptime_seconds | default('unknown') }}"
	stateAppVersion   = "{{ value_json.app.version | default('unknown') }}"
	stateHostname     = "{{ value_json.device.hostname | default('unknown') }}"
	stateIdleTime     = "{{ value_json.session.idle_seconds | default('unknown') }}"
	stateVolume       = "{{ value_json.audio.volume | default('unknown') }}"
	// A sensor's state is at most 255 characters.
	stateNowPlaying = "{{ value_json.media_title | default('') | truncate(255, true, '') }}"

	stateNetworkOnline = "{% if value_json.network is defined and value_json.network.online is defined and value_json.network.online is not none %}{{ 'ON' if value_json.network.online else 'OFF' }}{% else %}unknown{% endif %}"
	stateCharging      = "{% if value_json.power is defined and value_json.power.charging is defined and value_json.power.charging is not none %}{{ 'ON' if value_json.power.charging else 'OFF' }}{% else %}unknown{% endif %}"
	stateLocked        = "{% if value_json.session is defined and value_json.session.locked is defined and value_json.session.locked is not none %}{{ 'ON' if value_json.session.locked else 'OFF' }}{% else %}unknown{% endif %}"
	stateCamera        = "{% if value_json.activity is defined %}{{ 'ON' if value_json.activity.camera_in_use else 'OFF' }}{% else %}unknown{% endif %}"
	stateMicrophone    = "{% if value_json.activity is defined %}{{ 'ON' if value_json.activity.microphone_in_use else 'OFF' }}{% else %}unknown{% endif %}"
	stateDisplay       = "{% if value_json.display is defined %}{{ 'ON' if value_json.display.awake else 'OFF' }}{% else %}unknown{% endif %}"
	stateMuted         = "{% if value_json.audio is defined and value_json.audio.muted is defined and value_json.audio.muted is not none %}{{ 'ON' if value_json.audio.muted else 'OFF' }}{% else %}unknown{% endif %}"
)

// Groups of entities, which the user publishes or not as a whole.
const (
	GroupNetwork     = "network"
	GroupPower       = "power"
	GroupSession     = "session"
	GroupActivity    = "activity"
	GroupNotify      = "notifications"
	GroupShortcuts   = "shortcuts"
	GroupAudio       = "audio"
	GroupMedia       = "media"
	GroupDiagnostics = "diagnostics"
)

// Groups lists the groups in the order the app shows them. The Refresh
// button belongs to none: it is always published.
var Groups = []string{GroupNetwork, GroupPower, GroupSession, GroupActivity, GroupAudio, GroupMedia, GroupNotify, GroupShortcuts, GroupDiagnostics}

// SystemEntities returns the standard status and control entities exposed by
// Mac2MQTT, leaving out the groups opts.Disabled names. The order is stable
// so discovery publishing is deterministic.
func SystemEntities(opts Options) []Entity {
	all := AllEntities(opts)
	entities := all[:0]
	for _, entity := range all {
		if entity.Group != "" && opts.Disabled[entity.Group] {
			continue
		}
		if opts.HasBattery == false && (entity.ObjectID == "battery" || entity.ObjectID == "charging") {
			continue
		}
		if entity.ObjectID == "run_shortcut" && len(opts.Shortcuts) == 0 ||
			entity.ObjectID == "camera_in_use" && !opts.HasCamera ||
			entity.ObjectID == "microphone_in_use" && !opts.HasMicrophone {
			continue
		}
		entities = append(entities, entity)
	}
	return entities
}

// AllEntities returns every entity Mac2MQTT can publish, whatever the
// options leave out: those whose retained discovery documents the service
// clears when they are not published.
func AllEntities(opts Options) []Entity {
	entities := allEntities(opts)
	// The entity IDs are English, whatever language names the entities,
	// as automations and the universal media player's YAML name them:
	// sensor.mac_mini_now_playing. Home Assistant takes them only when
	// it adds an entity, so renaming one later keeps them.
	device := slugify(opts.Device.Name)
	for _, e := range entities {
		if e.Component != "media_player" {
			e.Config["default_entity_id"] = e.Component + "." + device + "_" + e.ObjectID
		}
	}
	return entities
}

func allEntities(opts Options) []Entity {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseTopic), "/")
	stateTopic := strings.TrimSpace(opts.StateTopic)
	if stateTopic == "" {
		stateTopic = joinTopic(base, "state")
	}
	availabilityTopic := strings.TrimSpace(opts.AvailabilityTopic)
	if availabilityTopic == "" {
		availabilityTopic = joinTopic(base, "availability")
	}
	state := func(group, component, objectID, name string, extra map[string]any) Entity {
		e := newStateEntity(opts, stateTopic, availabilityTopic, component, objectID, name, extra)
		e.Group = group
		return e
	}
	media := func(topic, objectID, name string, extra map[string]any) Entity {
		e := newStateEntity(opts, joinTopic(base, topic), availabilityTopic, "sensor", objectID, name, extra)
		e.Group = GroupMedia
		return e
	}
	return []Entity{
		state(GroupNetwork, "binary_sensor", "network_connectivity", "Network Connectivity", map[string]any{
			"value_template": stateNetworkOnline,
			"device_class":   "connectivity",
		}),
		state(GroupPower, "sensor", "power_source", "Power Source", map[string]any{
			"value_template": statePowerSource,
			"icon":           "mdi:power-plug-outline",
		}),
		state(GroupPower, "binary_sensor", "charging", "Charging", map[string]any{
			"value_template": stateCharging,
			"device_class":   "battery_charging",
		}),
		state(GroupPower, "sensor", "battery", "Battery", map[string]any{
			"value_template":      statePowerBattery,
			"device_class":        "battery",
			"unit_of_measurement": "%",
			"state_class":         "measurement",
		}),
		// Not the lock device class: its ON means unlocked.
		state(GroupSession, "binary_sensor", "screen_locked", "Screen Locked", map[string]any{
			"value_template": stateLocked,
			"icon":           "mdi:monitor-lock",
		}),
		state(GroupSession, "sensor", "idle_time", "Idle Time", map[string]any{
			"value_template":      stateIdleTime,
			"device_class":        "duration",
			"unit_of_measurement": "s",
			"enabled_by_default":  false,
		}),
		state(GroupSession, "binary_sensor", "display", "Display", map[string]any{
			"value_template": stateDisplay,
			"icon":           "mdi:monitor",
		}),
		commandButton(opts, availabilityTopic, base, GroupSession, "system/command", "lock_screen", "Lock Screen", "LOCK", "mdi:monitor-lock"),
		commandButton(opts, availabilityTopic, base, GroupSession, "system/command", "display_sleep", "Sleep Display", "DISPLAY_SLEEP", "mdi:monitor-off"),
		commandButton(opts, availabilityTopic, base, GroupSession, "system/command", "display_wake", "Wake Display", "DISPLAY_WAKE", "mdi:monitor-eye"),
		commandButton(opts, availabilityTopic, base, GroupSession, "system/command", "sleep", "Sleep", "SLEEP", "mdi:power-sleep"),
		state(GroupActivity, "binary_sensor", "camera_in_use", "Camera In Use", map[string]any{
			"value_template": stateCamera,
			"icon":           "mdi:webcam",
		}),
		state(GroupActivity, "binary_sensor", "microphone_in_use", "Microphone In Use", map[string]any{
			"value_template": stateMicrophone,
			"icon":           "mdi:microphone",
		}),
		state(GroupAudio, "number", "volume", "Volume", map[string]any{
			"value_template":      stateVolume,
			"command_topic":       joinTopic(base, "volume/set"),
			"min":                 0,
			"max":                 100,
			"step":                1,
			"mode":                "slider",
			"unit_of_measurement": "%",
			"icon":                "mdi:volume-high",
		}),
		state(GroupAudio, "switch", "mute", "Mute", map[string]any{
			"value_template": stateMuted,
			"command_topic":  joinTopic(base, "mute/set"),
			"payload_on":     "ON",
			"payload_off":    "OFF",
			"icon":           "mdi:volume-mute",
		}),
		// MQTT discovery has no media_player platform, so Now Playing is
		// read-only sensors: the playback state, and the title with the
		// rest of the item as attributes.
		media("media/state", "media_state", "Media State", map[string]any{
			"device_class": "enum",
			"options":      []string{"playing", "paused", "idle"},
			"icon":         "mdi:play-pause",
		}),
		media("media/attributes", "now_playing", "Now Playing", map[string]any{
			"value_template":        stateNowPlaying,
			"json_attributes_topic": joinTopic(base, "media/attributes"),
			"icon":                  "mdi:music",
		}),
		commandButton(opts, availabilityTopic, base, GroupMedia, "media/command", "media_play_pause", "Play/Pause", "PLAY_PAUSE", "mdi:play-pause"),
		availableWhen(commandButton(opts, availabilityTopic, base, GroupMedia, "media/command", "media_next", "Next Track", "NEXT", "mdi:skip-next"),
			availabilityTopic, joinTopic(base, "media/next/available")),
		availableWhen(commandButton(opts, availabilityTopic, base, GroupMedia, "media/command", "media_previous", "Previous Track", "PREVIOUS", "mdi:skip-previous"),
			availabilityTopic, joinTopic(base, "media/previous/available")),
		mediaPlayer(opts, availabilityTopic, base),
		{
			Component: "notify",
			ObjectID:  "notify",
			Group:     GroupNotify,
			Config: withCommonConfig(opts, availabilityTopic, "notify", "Notification", map[string]any{
				"command_topic": joinTopic(base, "notify"),
				"icon":          "mdi:message-badge",
			}),
		},
		{
			Component: "notify",
			ObjectID:  "say",
			Group:     GroupNotify,
			Config: withCommonConfig(opts, availabilityTopic, "say", "Speak", map[string]any{
				"command_topic": joinTopic(base, "say"),
				"icon":          "mdi:account-voice",
			}),
		},
		{
			Component: "select",
			ObjectID:  "run_shortcut",
			Group:     GroupShortcuts,
			Config: withCommonConfig(opts, availabilityTopic, "run_shortcut", "Run Shortcut", map[string]any{
				"command_topic": joinTopic(base, "shortcut/run"),
				"options":       append([]string(nil), opts.Shortcuts...),
				"optimistic":    true,
				"icon":          "mdi:apple",
			}),
		},
		state(GroupDiagnostics, "sensor", "last_update", "Last Update", map[string]any{
			"value_template": stateLastSeen,
			"device_class":   "timestamp",
		}),
		state(GroupDiagnostics, "sensor", "app_uptime", "App Uptime", map[string]any{
			"value_template":      stateUptime,
			"device_class":        "duration",
			"unit_of_measurement": "s",
		}),
		state(GroupDiagnostics, "sensor", "app_version", "App Version", map[string]any{
			"value_template":  stateAppVersion,
			"entity_category": "diagnostic",
		}),
		state(GroupDiagnostics, "sensor", "hostname", "Hostname", map[string]any{
			"value_template":     stateHostname,
			"entity_category":    "diagnostic",
			"enabled_by_default": false,
		}),
		{
			Component: "button",
			ObjectID:  "refresh",
			Config: withCommonConfig(opts, availabilityTopic, "refresh", "Refresh", map[string]any{
				"command_topic": joinTopic(base, "refresh"),
				"payload_press": "PRESS",
			}),
		},
	}
}

// commandButton is a button sending a command to one of the app's
// command topics.
func commandButton(opts Options, availabilityTopic, base, group, topic, objectID, name, command, icon string) Entity {
	return Entity{
		Component: "button",
		ObjectID:  objectID,
		Group:     group,
		Config: withCommonConfig(opts, availabilityTopic, objectID, name, map[string]any{
			"command_topic": joinTopic(base, topic),
			"payload_press": command,
			"icon":          icon,
		}),
	}
}

// availableWhen makes an entity available only while the Mac is and
// another topic says online too, as the next track button while the
// player has a next track.
func availableWhen(e Entity, availabilityTopic, topic string) Entity {
	delete(e.Config, "availability_topic")
	delete(e.Config, "payload_available")
	delete(e.Config, "payload_not_available")
	e.Config["availability"] = []map[string]any{
		{"topic": availabilityTopic, "payload_available": payloadAvailable, "payload_not_available": payloadUnavailable},
		{"topic": topic, "payload_available": payloadAvailable, "payload_not_available": payloadUnavailable},
	}
	e.Config["availability_mode"] = "all"
	return e
}

// MediaPlayerObjectID names the media player in unique IDs; its discovery
// topic has no object ID, as the integration reading it expects.
const MediaPlayerObjectID = "media_player"

// mediaPlayer is a media_player for the MQTT Media Player integration
// (github.com/bkbilly/mqtt_media_player, from HACS), as Home Assistant's
// own MQTT integration has none. Without it Home Assistant ignores the
// document, and the sensors and buttons above stand in.
func mediaPlayer(opts Options, availabilityTopic, base string) Entity {
	topic := func(suffix string) string { return joinTopic(base, "media/"+suffix) }
	cfg := withCommonConfig(opts, availabilityTopic, MediaPlayerObjectID, opts.Device.Name, map[string]any{
		"availability": map[string]any{
			"topic":                 availabilityTopic,
			"payload_available":     payloadAvailable,
			"payload_not_available": payloadUnavailable,
		},
		"state_state_topic":         topic("state"),
		"state_title_topic":         topic("title"),
		"state_artist_topic":        topic("artist"),
		"state_album_topic":         topic("album"),
		"state_duration_topic":      topic("duration"),
		"state_position_topic":      topic("position"),
		"state_volume_topic":        topic("volume"),
		"state_mute_topic":          topic("mute"),
		"state_mediatype_topic":     topic("mediatype"),
		"command_volume_topic":      topic("volume/set"),
		"command_mute_topic":        topic("mute/set"),
		"command_play_topic":        topic("command"),
		"command_play_payload":      "PLAY",
		"command_pause_topic":       topic("command"),
		"command_pause_payload":     "PAUSE",
		"command_playpause_topic":   topic("command"),
		"command_playpause_payload": "PLAY_PAUSE",
		"command_next_topic":        topic("command"),
		"command_next_payload":      "NEXT",
		"command_previous_topic":    topic("command"),
		"command_previous_payload":  "PREVIOUS",
		"command_seek_topic":        topic("seek"),
	})
	return Entity{Component: "media_player", ObjectID: "", Group: GroupMedia, Config: cfg}
}

// CommandTopics lists the topics an entity takes commands on: its
// command_topic, and a media player's command_*_topic.
func (e Entity) CommandTopics() []string {
	var topics []string
	for key, value := range e.Config {
		topic, ok := value.(string)
		if ok && topic != "" && strings.HasPrefix(key, "command_") && strings.HasSuffix(key, "topic") {
			topics = append(topics, topic)
		}
	}
	sort.Strings(topics)
	return topics
}

// LegacyEntities are entities earlier versions published that no longer
// exist. Their retained discovery documents must be cleared.
func LegacyEntities() []Entity {
	return []Entity{
		// Home Assistant ignored it: MQTT has no media_player platform.
		{Component: "media_player", ObjectID: "now_playing"},
	}
}

func newStateEntity(opts Options, stateTopic, availabilityTopic, component, objectID, name string, extra map[string]any) Entity {
	cfg := make(map[string]any, len(extra)+7)
	cfg["state_topic"] = stateTopic
	for key, value := range extra {
		cfg[key] = value
	}
	return Entity{
		Component: component,
		ObjectID:  objectID,
		Config:    withCommonConfig(opts, availabilityTopic, objectID, name, cfg),
	}
}

func withCommonConfig(opts Options, availabilityTopic, objectID, name string, cfg map[string]any) map[string]any {
	// Copy so callers cannot accidentally share or mutate an entity's config.
	out := make(map[string]any, len(cfg)+7)
	for key, value := range cfg {
		out[key] = value
	}
	if localized := opts.Names[objectID]; localized != "" {
		name = localized
	}
	out["name"] = name
	out["unique_id"] = uniqueID(opts.Device.ID, objectID)
	out["availability_topic"] = availabilityTopic
	out["payload_available"] = payloadAvailable
	out["payload_not_available"] = payloadUnavailable
	out["device"] = DevicePayload(opts.Device)
	return out
}

// uniqueID is stable for a device/entity pair. The object ID is already
// supplied separately at the call site; when the device ID is available it is
// included to avoid collisions between multiple Mac2MQTT installs.
func uniqueID(deviceID, objectID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "mac2mqtt_" + objectID
	}
	return fmt.Sprintf("mac2mqtt_%s_%s", deviceID, objectID)
}

func joinTopic(base, suffix string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	suffix = strings.Trim(strings.TrimSpace(suffix), "/")
	switch {
	case base == "":
		return suffix
	case suffix == "":
		return base
	default:
		return base + "/" + suffix
	}
}
