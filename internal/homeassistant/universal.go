package homeassistant

import (
	"fmt"
	"regexp"
	"strings"
)

// UniversalMediaPlayerYAML returns configuration for Home Assistant's
// universal media player, a media_player made of the sensors Mac2MQTT
// publishes and commands sent to its topics, for those without the MQTT
// Media Player integration. The entity IDs are those Home Assistant gives
// the sensors unless the user renamed them.
func UniversalMediaPlayerYAML(opts Options) string {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseTopic), "/")
	device := slugify(opts.Device.Name)
	nowPlaying := "sensor." + device + "_now_playing"
	publish := func(topic, payload string) string {
		return fmt.Sprintf("\n        action: mqtt.publish\n        data:\n          topic: %s/%s\n          payload: %s", base, topic, payload)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "media_player:\n  - platform: universal\n    name: %q\n    unique_id: %s\n", opts.Device.Name, uniqueID(opts.Device.ID, "universal_media_player"))
	fmt.Fprintf(&b, "    state_template: >\n      {{ states('sensor.%s_media_state') if states('sensor.%s_media_state') in ['playing', 'paused'] else 'idle' }}\n", device, device)
	b.WriteString("    attributes:\n")
	fmt.Fprintf(&b, "      media_title: %s\n", nowPlaying)
	for _, attr := range []struct{ name, from string }{
		{"media_artist", "media_artist"},
		{"media_album_name", "media_album_name"},
		{"media_duration", "media_duration"},
		{"media_position", "media_position"},
		{"media_position_updated_at", "media_position_updated_at"},
		{"volume_level", "volume_level"},
		{"is_volume_muted", "is_volume_muted"},
		{"source", "app_name"},
	} {
		fmt.Fprintf(&b, "      %s: %s|%s\n", attr.name, nowPlaying, attr.from)
	}
	b.WriteString("    commands:\n")
	for _, cmd := range []struct{ name, payload string }{
		{"media_play", "PLAY"},
		{"media_pause", "PAUSE"},
		{"media_play_pause", "PLAY_PAUSE"},
		{"media_stop", "STOP"},
		{"media_next_track", "NEXT"},
		{"media_previous_track", "PREVIOUS"},
	} {
		fmt.Fprintf(&b, "      %s:%s\n", cmd.name, publish("media/command", cmd.payload))
	}
	fmt.Fprintf(&b, "      volume_set:%s\n", publish("media/volume/set", `"{{ volume_level }}"`))
	fmt.Fprintf(&b, "      volume_mute:%s\n", publish("media/mute/set", `"{{ 'mute' if is_volume_muted else 'unmute' }}"`))
	fmt.Fprintf(&b, "      media_seek:%s\n", publish("media/seek", `"{{ seek_position }}"`))
	return b.String()
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify makes an entity ID's part of a name, as Home Assistant does for
// ASCII names.
func slugify(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if s == "" {
		return "mac2mqtt"
	}
	return s
}
