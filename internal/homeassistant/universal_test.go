package homeassistant

import (
	"strings"
	"testing"
)

func TestUniversalMediaPlayerYAML(t *testing.T) {
	opts := testOptions()
	opts.Device.Name = "mac-mini"
	yaml := UniversalMediaPlayerYAML(opts)
	for _, want := range []string{
		"platform: universal",
		`name: "mac-mini"`,
		"media_title: sensor.mac_mini_now_playing\n",
		"volume_level: sensor.mac_mini_now_playing|volume_level",
		"states('sensor.mac_mini_media_state')",
		"media_next_track:\n        action: mqtt.publish\n        data:\n          topic: mac2mqtt/studio/media/command\n          payload: NEXT",
		"topic: mac2mqtt/studio/media/volume/set",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("missing %q in\n%s", want, yaml)
		}
	}
	if strings.Contains(yaml, "\t") {
		t.Error("YAML holds tabs")
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"mac-mini": "mac_mini", "Studio Mac": "studio_mac", "张的Mac": "mac", "": "mac2mqtt"} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
