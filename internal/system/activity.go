package system

// Activity is what the Mac is doing that changes from one moment to the
// next: whether an app uses a camera or a microphone, as the indicators in
// the menu bar show, and whether the display sleeps. It is cheap to read,
// so the service reads it every few seconds and publishes changes at once.
type Activity struct {
	CameraInUse     bool
	MicrophoneInUse bool
	DisplayAsleep   bool
	// Displays is how many displays are connected, built in or not;
	// macOS's placeholder for none does not count.
	Displays int
	// HasCamera and HasMicrophone tell whether the Mac has any camera and
	// any audio input at all, built in or plugged in: a Mac mini has
	// neither until one is connected.
	HasCamera     bool
	HasMicrophone bool
}

// isPlaceholderDisplay tells whether a display is the one macOS makes up
// when no screen answers, as when a monitor's power is cut at the plug: its
// vendor is 'unkn' and its model 'virt'. The Mac has no display to show on
// then, so the display counts as off, as when it sleeps.
func isPlaceholderDisplay(vendor, model uint32) bool {
	return vendor == 0x756e6b6e && model == 0x76697274 // 'unkn', 'virt'
}
