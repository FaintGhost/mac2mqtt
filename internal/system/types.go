// Package system collects macOS system status and controls system audio.
package system

import "time"

// AudioState is the current output volume and mute state.
type AudioState struct {
	Volume int  `json:"volume"`
	Muted  bool `json:"muted"`
}

// AppInfo describes this application instance.
type AppInfo struct {
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// DeviceInfo describes the host running the application.
type DeviceInfo struct {
	Hostname  string `json:"hostname"`
	OSVersion string `json:"os_version"`
	Arch      string `json:"arch"`
	Model     string `json:"model,omitempty"`    // "Mac mini"
	ModelID   string `json:"model_id,omitempty"` // "Mac16,10"
	// UptimeSeconds is how long ago the Mac started, sleep included.
	UptimeSeconds int64 `json:"uptime_seconds,omitempty"`
}

// NetworkInfo describes the network state supplied by the app layer.
type NetworkInfo struct {
	Online  bool `json:"online"`
	Metered bool `json:"metered"`
}

// PowerInfo describes the current power source and, when present, the battery.
type PowerInfo struct {
	OnBattery      bool     `json:"on_battery"`
	Source         string   `json:"source"`
	BatteryPercent *float64 `json:"battery_percent,omitempty"`
	Charging       *bool    `json:"charging,omitempty"`
}

// SessionInfo describes the current user session.
type SessionInfo struct {
	Locked      bool  `json:"locked"`
	IdleSeconds int64 `json:"idle_seconds"`
}

// MediaInfo describes the system-wide Now Playing item.
type MediaInfo struct {
	State           string  `json:"state"` // playing, paused or idle
	Title           string  `json:"title,omitempty"`
	Artist          string  `json:"artist,omitempty"`
	Album           string  `json:"album,omitempty"`
	AppName         string  `json:"app_name,omitempty"`
	BundleID        string  `json:"bundle_id,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	PositionSeconds float64 `json:"position_seconds,omitempty"`
	PlaybackRate    float64 `json:"playback_rate,omitempty"`
	// CanNext and CanPrevious tell whether the player takes the next and
	// previous track commands.
	CanNext     bool `json:"can_next"`
	CanPrevious bool `json:"can_previous"`
}

// ActivityInfo tells whether an app uses a camera or a microphone.
type ActivityInfo struct {
	CameraInUse     bool `json:"camera_in_use"`
	MicrophoneInUse bool `json:"microphone_in_use"`
	HasCamera       bool `json:"has_camera"`
	HasMicrophone   bool `json:"has_microphone"`
}

// DisplayInfo tells whether the display is awake.
type DisplayInfo struct {
	Awake bool `json:"awake"`
}

// Snapshot is one point-in-time report of system status.
type Snapshot struct {
	LastSeen time.Time    `json:"last_seen"`
	App      AppInfo      `json:"app"`
	Device   DeviceInfo   `json:"device"`
	Network  NetworkInfo  `json:"network"`
	Power    PowerInfo    `json:"power"`
	Session  SessionInfo  `json:"session"`
	Activity ActivityInfo `json:"activity"`
	Display  DisplayInfo  `json:"display"`
	Audio    *AudioState  `json:"audio,omitempty"`
	Media    *MediaInfo   `json:"media,omitempty"`
}

// Runtime contains values that only the MyGo application layer can supply.
type Runtime struct {
	NetworkOnline  bool
	NetworkMetered bool
	OnBattery      bool
	Idle           time.Duration
	// SkipMedia leaves Now Playing out, when it is not published.
	SkipMedia bool
}
