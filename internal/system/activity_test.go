package system

import "testing"

func TestPlaceholderDisplay(t *testing.T) {
	// What a Mac mini reports with its monitor's power cut at the plug.
	if !isPlaceholderDisplay(1970170734, 1986622068) {
		t.Error("macOS's placeholder display ('unkn', 'virt') counts as a display")
	}
	// A real monitor, with the vendor and model of its EDID.
	if isPlaceholderDisplay(21746, 65535) {
		t.Error("a monitor counts as the placeholder display")
	}
}
