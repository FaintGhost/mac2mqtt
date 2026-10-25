package system

import (
	"context"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Collector gathers a Snapshot. Its mutable state is safe for concurrent use.
type Collector struct {
	version string
	start   time.Time
	locked  atomic.Bool

	goos   string
	runner commandRunner
	// readActivity reads the activity; tests replace it.
	readActivity func() Activity
	// nativeAudio reads the volume without a process; nil, as in tests,
	// reads it with osascript.
	nativeAudio func() (*AudioState, error)
	// nativeLocked reads the lock state without a process; nil, as in
	// tests, reads it with ioreg.
	nativeLocked func() (bool, error)
	// media watches Now Playing with one script; nil, as in tests, reads
	// it with a script each time.
	media *mediaWatcher

	osVersionOnce sync.Once
	osVersion     string
	hardwareOnce  sync.Once
	hardware      Hardware
}

// NewCollector creates a Collector using the real macOS commands. The version
// is the application version reported in each snapshot.
func NewCollector(version string) *Collector {
	c := newCollector(version, runtime.GOOS, execCommandRunner{})
	c.nativeAudio = readNativeAudio
	c.nativeLocked = readNativeScreenLocked
	c.media = newMediaWatcher()
	return c
}

// newCollector is the testable constructor: platform and command execution are
// injected so tests never touch the host's hardware or audio settings.
func newCollector(version, goos string, runner commandRunner) *Collector {
	return &Collector{
		version: version,
		start:   time.Now(),
		goos:    goos,
		runner:  runner,

		readActivity: readActivity,
	}
}

// SetLocked updates the session lock state. It is safe to call from any
// goroutine.
func (c *Collector) SetLocked(locked bool) {
	c.locked.Store(locked)
}

// Snapshot collects the current system state. Commands are only executed on
// macOS; runtime-only state is supplied by the caller.
func (c *Collector) Snapshot(ctx context.Context, rt Runtime) Snapshot {
	now := time.Now()
	hostname, _ := os.Hostname()

	var audio *AudioState
	if c.goos == "darwin" {
		if c.nativeAudio != nil {
			audio, _ = c.nativeAudio()
		}
		if audio == nil {
			if state, err := readAudio(ctx, c.runner); err == nil {
				audio = state
			}
		}
	}

	activity := c.Activity()
	var media *MediaInfo
	if rt.SkipMedia == false {
		media = c.Media(ctx)
	}
	return Snapshot{
		LastSeen: now,
		App: AppInfo{
			Version:       c.version,
			UptimeSeconds: nonNegativeSeconds(time.Since(c.start)),
		},
		Device: DeviceInfo{
			Hostname:  hostname,
			OSVersion: c.detectOSVersion(ctx),
			Arch:      runtime.GOARCH,
			Model:     c.detectHardware(ctx).Name,
			ModelID:   c.detectHardware(ctx).ID,
		},
		Network: NetworkInfo{
			Online:  rt.NetworkOnline,
			Metered: rt.NetworkMetered,
		},
		Power: c.collectPower(ctx, rt),
		Session: SessionInfo{
			Locked:      c.screenLocked(ctx),
			IdleSeconds: nonNegativeSeconds(rt.Idle),
		},
		Activity: ActivityInfo{
			CameraInUse:     activity.CameraInUse,
			MicrophoneInUse: activity.MicrophoneInUse,
			HasCamera:       activity.HasCamera,
			HasMicrophone:   activity.HasMicrophone,
		},
		Display: DisplayInfo{Awake: !activity.DisplayAsleep},
		Audio:   audio,
		Media:   media,
	}
}

// screenLocked reads whether the screen is locked, falling back to what
// the last lock or unlock notification said.
func (c *Collector) screenLocked(ctx context.Context) bool {
	if c.goos == "darwin" {
		if c.nativeLocked != nil {
			if locked, err := c.nativeLocked(); err == nil {
				c.locked.Store(locked)
				return locked
			}
		}
		if locked, err := readScreenLocked(ctx, c.runner, os.Getuid()); err == nil {
			c.locked.Store(locked)
			return locked
		}
	}
	return c.locked.Load()
}

func nonNegativeSeconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return int64(duration / time.Second)
}

// detectOSVersion caches the result of sw_vers so periodic snapshots do not
// spawn a process every time. Failures fall back to the Go platform name.
func (c *Collector) detectOSVersion(ctx context.Context) string {
	if c.goos != "darwin" {
		return c.goos
	}

	c.osVersionOnce.Do(func() {
		output, err := runCommand(ctx, c.runner, "/usr/bin/sw_vers", "-productVersion")
		if err == nil {
			if version := strings.TrimSpace(string(output)); version != "" {
				c.osVersion = version
				return
			}
		}
		c.osVersion = c.goos
	})
	return c.osVersion
}

// Activity reads whether a camera or a microphone is in use, and whether
// the display sleeps. It is cheap: no process starts.
func (c *Collector) Activity() Activity {
	if c.goos != "darwin" {
		return Activity{}
	}
	return c.readActivity()
}

// detectHardware reads the model of this Mac once: it does not change.
func (c *Collector) detectHardware(ctx context.Context) Hardware {
	if c.goos != "darwin" {
		return Hardware{}
	}
	c.hardwareOnce.Do(func() { c.hardware = readHardware(ctx, c.runner) })
	return c.hardware
}

// Media reads the system-wide Now Playing item. It returns nil when nothing
// is playing or the system does not expose MediaRemote.
func (c *Collector) Media(ctx context.Context) *MediaInfo {
	if c.goos != "darwin" {
		return nil
	}
	if c.media != nil {
		if media, ok := c.media.Current(); ok {
			return media
		}
	}
	return readMedia(ctx, c.runner)
}

// Close stops what the collector runs in the background.
func (c *Collector) Close() {
	if c.media != nil {
		c.media.Stop()
	}
}
