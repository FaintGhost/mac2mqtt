package system

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNoKeyAccess is UnlockScreen's error while the app may not post key
	// events: macOS lets it once Accessibility allows it.
	ErrNoKeyAccess = errors.New("system: Mac2MQTT is not allowed to type (Accessibility)")
	// ErrNotOnConsole is UnlockScreen's error while another user is at the
	// Mac: what it typed would go to their session.
	ErrNotOnConsole = errors.New("system: another user is using the Mac")
	// ErrStillLocked is UnlockScreen's error when the screen stayed locked
	// after it typed the password, as a wrong one leaves it.
	ErrStillLocked = errors.New("system: the screen stayed locked, check the password")
	// ErrUnlockedMeanwhile is UnlockScreen's error when the screen was
	// unlocked as it typed, which it then stopped.
	ErrUnlockedMeanwhile = errors.New("system: the screen was unlocked meanwhile")
)

// screenState is this session's lock state, as the window server tells it.
type screenState struct {
	Locked    bool
	OnConsole bool
}

// keyTyper types into the frontmost thing of the Mac, the lock screen's
// password field while the screen is locked.
type keyTyper interface {
	// Allowed reports whether the app may post key events.
	Allowed() bool
	// Type types one character, and Return types the Return key.
	Type(r rune)
	Return()
	// State reads this session's lock state.
	State() (screenState, error)
}

// Native typing, and the pause between steps; variables for tests.
var (
	typer      keyTyper = nativeTyper{}
	unlockWait          = func(ctx context.Context, d time.Duration) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
			return true
		}
	}
)

const (
	// unlockWake is how long the lock screen takes to show its password
	// field once the display wakes.
	unlockWake = 1500 * time.Millisecond
	// unlockKeyGap spaces the keys as fast typing would.
	unlockKeyGap = 20 * time.Millisecond
	// unlockCheck and unlockChecks bound the wait for the screen to unlock.
	unlockCheck  = 250 * time.Millisecond
	unlockChecks = 24
)

// KeyAccess reports whether the app may type, which UnlockScreen needs.
func KeyAccess() bool { return typer.Allowed() }

// RequestKeyAccess asks macOS to let the app type, which shows the
// Accessibility settings with the app in them. It reports whether it may.
func RequestKeyAccess() bool { return requestKeyAccess() }

// UnlockScreen wakes the display and types password into the lock
// screen, then waits for the screen to unlock. It types once: retrying a
// wrong password would get the account locked out. A screen not locked is
// left alone, without typing.
func (c *Controller) UnlockScreen(ctx context.Context, password string) error {
	if c.goos != "darwin" {
		return ErrUnsupported
	}
	if !typer.Allowed() {
		return ErrNoKeyAccess
	}
	// Typed only into a locked screen of this session: anywhere else the
	// password would go into the app in front, or another user's session.
	locked := func() (bool, error) {
		s, err := typer.State()
		switch {
		case err != nil:
			return false, err
		case !s.OnConsole:
			return false, ErrNotOnConsole
		}
		return s.Locked, nil
	}
	stillLocked := func() error {
		ok, err := locked()
		if err == nil && !ok {
			err = ErrUnlockedMeanwhile
		}
		return err
	}
	if ok, err := locked(); err != nil || !ok {
		return err
	}
	// Declaring the user active wakes the display, as a key press does.
	if _, err := runCommand(ctx, c.runner, "/usr/bin/caffeinate", "-u", "-t", "1"); err != nil {
		return err
	}
	if !unlockWait(ctx, unlockWake) {
		return ctx.Err()
	}
	for _, r := range password {
		if err := stillLocked(); err != nil {
			return err
		}
		typer.Type(r)
		unlockWait(ctx, unlockKeyGap)
	}
	if err := stillLocked(); err != nil {
		return err
	}
	typer.Return()
	for range unlockChecks {
		if !unlockWait(ctx, unlockCheck) {
			return ctx.Err()
		}
		if s, err := typer.State(); err == nil && !s.Locked {
			return nil
		}
	}
	return ErrStillLocked
}
