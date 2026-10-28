package main

import "mac2mqtt/locales"

// entityNames are the names of the entities in the app's language, as
// Home Assistant shows them.
func entityNames() map[string]string {
	return map[string]string{
		"network_connectivity": locales.EntityNetworkConnectivity(),
		"power_source":         locales.EntityPowerSource(),
		"charging":             locales.EntityCharging(),
		"battery":              locales.EntityBattery(),
		"screen_locked":        locales.EntityScreenLocked(),
		"idle_time":            locales.EntityIdleTime(),
		"display":              locales.EntityDisplay(),
		"lock_screen":          locales.EntityLockScreen(),
		"display_sleep":        locales.EntityDisplaySleep(),
		"display_wake":         locales.EntityDisplayWake(),
		"sleep":                locales.EntitySleep(),
		"camera_in_use":        locales.EntityCameraInUse(),
		"microphone_in_use":    locales.EntityMicrophoneInUse(),
		"volume":               locales.EntityVolume(),
		"mute":                 locales.EntityMute(),
		"media_state":          locales.EntityMediaState(),
		"now_playing":          locales.EntityNowPlaying(),
		"media_play_pause":     locales.EntityMediaPlayPause(),
		"media_next":           locales.EntityMediaNext(),
		"media_previous":       locales.EntityMediaPrevious(),
		"notify":               locales.EntityNotify(),
		"say":                  locales.EntitySay(),
		"run_shortcut":         locales.EntityRunShortcut(),
		"last_update":          locales.EntityLastUpdate(),
		"uptime":               locales.EntityUptime(),
		"app_uptime":           locales.EntityAppUptime(),
		"app_version":          locales.EntityAppVersion(),
		"hostname":             locales.EntityHostname(),
		"refresh":              locales.EntityRefresh(),
	}
}
