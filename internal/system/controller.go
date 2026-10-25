package system

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

// ErrUnsupported is returned when audio controls are used on a non-macOS
// platform.
var ErrUnsupported = errors.New("system: audio control is unsupported on this platform")

// Controller changes the host's output volume and mute state.
type Controller struct {
	goos   string
	runner commandRunner
	// nativeVolume and nativeMuted set the volume without a process; nil,
	// as in tests, sets it with osascript.
	nativeVolume func(int) error
	nativeMuted  func(bool) error
}

// NewController creates a Controller that uses the real system audio commands.
func NewController() *Controller {
	c := newController(runtime.GOOS, execCommandRunner{})
	c.nativeVolume, c.nativeMuted = setNativeVolume, setNativeMuted
	return c
}

func newController(goos string, runner commandRunner) *Controller {
	return &Controller{goos: goos, runner: runner}
}

// SetVolume sets the output volume. Values outside 0..100 are rejected rather
// than silently clamped so callers can surface invalid input.
func (c *Controller) SetVolume(ctx context.Context, volume int) error {
	if volume < 0 || volume > 100 {
		return fmt.Errorf("system: volume %d is outside 0..100", volume)
	}
	if c.goos != "darwin" {
		return ErrUnsupported
	}

	if c.nativeVolume != nil && c.nativeVolume(volume) == nil {
		return nil
	}
	script := fmt.Sprintf("set volume output volume %d", volume)
	if _, err := runCommand(ctx, c.runner, "/usr/bin/osascript", "-e", script); err != nil {
		return fmt.Errorf("system: set volume: %w", err)
	}
	return nil
}

// SetMuted mutes or unmutes the output.
func (c *Controller) SetMuted(ctx context.Context, muted bool) error {
	if c.goos != "darwin" {
		return ErrUnsupported
	}

	if c.nativeMuted != nil && c.nativeMuted(muted) == nil {
		return nil
	}
	script := "set volume without output muted"
	if muted {
		script = "set volume with output muted"
	}
	if _, err := runCommand(ctx, c.runner, "/usr/bin/osascript", "-e", script); err != nil {
		return fmt.Errorf("system: set muted: %w", err)
	}
	return nil
}
