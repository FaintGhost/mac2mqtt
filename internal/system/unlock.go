package system

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrNoKeyAccess means Accessibility does not allow lock-screen input.
	ErrNoKeyAccess = errors.New("system: Mac2MQTT is not allowed to type (Accessibility)")
	// ErrNotOnConsole means another user's session is on the console.
	ErrNotOnConsole = errors.New("system: another user is using the Mac")
	// ErrStillLocked means the one password submission did not unlock the screen.
	ErrStillLocked = errors.New("system: the screen stayed locked, check the password")
	// ErrUnlockedMeanwhile means input stopped because the screen unlocked.
	ErrUnlockedMeanwhile = errors.New("system: the screen was unlocked meanwhile")
	// ErrNoPasswordField means the login window has no focused secure text field.
	ErrNoPasswordField = errors.New("system: the lock screen password field is not ready")
	// ErrPasswordInput means loginwindow did not confirm the expected input length.
	ErrPasswordInput    = errors.New("system: the password field did not accept the complete input")
	ErrUnlockInProgress = errors.New("system: an unlock is already in progress")
)

// screenState is this session's lock state, as the window server tells it.
type screenState struct {
	Locked    bool
	OnConsole bool
}

// passwordField belongs to loginwindow. Its identity is retained for one
// attempt, so a focus change cannot redirect password input to another field.
type passwordField interface {
	Focused() (bool, error)
	SetValue(string) error
	ValueHasLength(string) (bool, error)
	Close()
}

type keyTyper interface {
	Allowed() bool
	State() (screenState, error)
	PasswordField() (passwordField, error)
	Type(rune) error
	Key(code uint16, flags uint64) error
}

// Native typing, and the pause between steps; variables for tests.
var (
	typer keyTyper = nativeTyper{}
	// The keyboard is shared even between different Controller instances.
	unlockMu   sync.Mutex
	unlockWait = func(ctx context.Context, d time.Duration) bool {
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
	keyA        = 0
	keyReturn   = 36
	keyDelete   = 51
	keyShift    = 56
	flagCommand = 1 << 20

	unlockKeyGap      = 20 * time.Millisecond
	unlockReadyCheck  = 100 * time.Millisecond
	unlockReadyChecks = 30
	unlockValueCheck  = 50 * time.Millisecond
	unlockClearChecks = 20
	unlockValueChecks = 40
	unlockCheck       = 250 * time.Millisecond
	unlockChecks      = 28
)

func KeyAccess() bool { return typer.Allowed() }

// RequestKeyAccess asks macOS to allow input through Accessibility.
func RequestKeyAccess() bool { return requestKeyAccess() }

// UnlockScreen waits for loginwindow's password field, replaces its contents,
// and submits once. It never retries authentication. A missing field, changed
// focus, switched session or canceled request stops input before submission.
func (c *Controller) UnlockScreen(ctx context.Context, password string) error {
	if c.goos != "darwin" {
		return ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !unlockMu.TryLock() {
		return ErrUnlockInProgress
	}
	defer unlockMu.Unlock()
	if !typer.Allowed() {
		return ErrNoKeyAccess
	}
	locked := func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
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
	if _, err := runCommand(ctx, c.runner, "/usr/bin/caffeinate", "-u", "-t", "1"); err != nil {
		return err
	}
	if err := stillLocked(); err != nil {
		return err
	}
	// Reveal the password prompt without submitting any text left in it.
	if err := typer.Key(keyShift, 0); err != nil {
		return err
	}
	field, err := waitPasswordField(ctx, stillLocked)
	if err != nil {
		return err
	}
	defer field.Close()
	focused := func() error {
		if err := stillLocked(); err != nil {
			return err
		}
		ok, err := field.Focused()
		if err != nil {
			return err
		}
		if !ok {
			return ErrNoPasswordField
		}
		// AX calls can wait on loginwindow; recheck after they return.
		return stillLocked()
	}
	if err := focused(); err != nil {
		return err
	}
	if err := replacePassword(ctx, field, password, focused); err != nil {
		return err
	}
	if err := focused(); err != nil {
		return err
	}
	if err := typer.Key(keyReturn, 0); err != nil {
		return err
	}
	for range unlockChecks {
		if !unlockWait(ctx, unlockCheck) {
			return ctx.Err()
		}
		if ok, err := locked(); err != nil || !ok {
			return err
		}
	}
	return ErrStillLocked
}

func waitPasswordField(ctx context.Context, guard func() error) (passwordField, error) {
	for range unlockReadyChecks {
		if err := guard(); err != nil {
			return nil, err
		}
		field, err := typer.PasswordField()
		if err == nil {
			return field, nil
		}
		if !errors.Is(err, ErrNoPasswordField) {
			return nil, err
		}
		if !unlockWait(ctx, unlockReadyCheck) {
			return nil, ctx.Err()
		}
	}
	return nil, ErrNoPasswordField
}

func replacePassword(ctx context.Context, field passwordField, password string, guard func() error) error {
	// Clear and confirm before inserting: loginwindow may still be updating
	// the field after waking, and an AX success only acknowledges the call.
	if err := field.SetValue(""); err != nil {
		if errors.Is(err, ErrNoKeyAccess) {
			return err
		}
		if err := clearPassword(ctx, guard); err != nil {
			return err
		}
	}
	if err := waitPasswordValue(ctx, field, "", unlockClearChecks, guard); err != nil {
		return err
	}
	if err := guard(); err != nil {
		return err
	}
	if err := field.SetValue(password); err != nil {
		if errors.Is(err, ErrNoKeyAccess) {
			return err
		}
		// A rejected AX write could have changed part of the value. Clear it
		// again before the Unicode fallback, without submitting either value.
		if err := clearPassword(ctx, guard); err != nil {
			return err
		}
		if err := waitPasswordValue(ctx, field, "", unlockClearChecks, guard); err != nil {
			return err
		}
		if err := typePassword(ctx, password, guard); err != nil {
			return err
		}
	}
	return waitPasswordValue(ctx, field, password, unlockValueChecks, guard)
}

func waitPasswordValue(ctx context.Context, field passwordField, password string, checks int, guard func() error) error {
	for range checks {
		if err := guard(); err != nil {
			return err
		}
		ok, err := field.ValueHasLength(password)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrPasswordInput, err)
		}
		if ok {
			return nil
		}
		if !unlockWait(ctx, unlockValueCheck) {
			return ctx.Err()
		}
	}
	return ErrPasswordInput
}

func clearPassword(ctx context.Context, guard func() error) error {
	// Select all and delete, including a partial prior input. Neither key
	// submits a password. Each event pair releases its modifiers.
	for _, key := range []struct {
		code  uint16
		flags uint64
	}{{keyA, flagCommand}, {keyDelete, 0}} {
		if err := guard(); err != nil {
			return err
		}
		if err := typer.Key(key.code, key.flags); err != nil {
			return err
		}
		if !unlockWait(ctx, unlockKeyGap) {
			return ctx.Err()
		}
	}
	return nil
}

func typePassword(ctx context.Context, password string, guard func() error) error {
	for _, r := range password {
		if err := guard(); err != nil {
			return err
		}
		if err := typer.Type(r); err != nil {
			return err
		}
		if !unlockWait(ctx, unlockKeyGap) {
			return ctx.Err()
		}
	}
	return nil
}
