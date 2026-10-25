package system

// Activity is what the Mac is doing that changes from one moment to the
// next: whether an app uses a camera or a microphone, as the indicators in
// the menu bar show, and whether the display sleeps. It is cheap to read,
// so the service reads it every few seconds and publishes changes at once.
type Activity struct {
	CameraInUse     bool
	MicrophoneInUse bool
	DisplayAsleep   bool
}
