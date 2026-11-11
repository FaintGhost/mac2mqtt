package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"mac2mqtt/internal/homeassistant"
	"mac2mqtt/internal/system"
	"mac2mqtt/locales"
)

// The Keychain's keys of what remote unlocking needs.
const (
	secretLoginPassword = "unlock-login-password"
	secretUnlockPIN     = "unlock-pin"
)

const (
	// maxPINFailures wrong PINs in a row stop unlocking for pinLockout,
	// so that a short PIN cannot be guessed by trying them all.
	maxPINFailures = 5
	pinLockout     = 15 * time.Minute
)

var (
	errWrongPIN     = errors.New("the PIN is wrong")
	errPINLockedOut = fmt.Errorf("unlocking is paused after %d wrong PINs", maxPINFailures)
	errNoUnlock     = errors.New("remote unlocking is not set up")
	errInvalidPIN   = errors.New("the PIN must be four digits or more")

	validPIN = regexp.MustCompile(homeassistant.UnlockPINFormat)
)

// secretStore is the Keychain: mygo.Secrets, or a map in tests.
type secretStore interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}

// unlocker unlocks the screen for Home Assistant: given the user's PIN, it
// types the login password, both kept in the Keychain.
type unlocker struct {
	secrets secretStore
	// unlock types the password into the lock screen.
	unlock func(ctx context.Context, password string) error
	now    func() time.Time

	mu       sync.Mutex
	failures int
	until    time.Time
}

// ready reports whether the PIN and the login password are stored.
func (u *unlocker) ready() bool {
	for _, key := range []string{secretUnlockPIN, secretLoginPassword} {
		if v, err := u.secrets.Get(key); err != nil || len(v) == 0 {
			return false
		}
	}
	return true
}

// Unlock unlocks the screen when code is the PIN.
func (u *unlocker) Unlock(ctx context.Context, code string) error {
	if err := u.checkPIN(code); err != nil {
		return err
	}
	password, err := u.secrets.Get(secretLoginPassword)
	if err != nil || len(password) == 0 {
		return errNoUnlock
	}
	return u.unlock(ctx, string(password))
}

// checkPIN compares code with the PIN in constant time, counting the
// wrong ones.
func (u *unlocker) checkPIN(code string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := u.now()
	if now.Before(u.until) {
		return errPINLockedOut
	}
	pin, err := u.secrets.Get(secretUnlockPIN)
	if err != nil || len(pin) == 0 {
		return errNoUnlock
	}
	if subtle.ConstantTimeCompare(pin, []byte(code)) == 1 {
		u.failures = 0
		return nil
	}
	u.failures++
	if u.failures >= maxPINFailures {
		u.failures, u.until = 0, now.Add(pinLockout)
		return errPINLockedOut
	}
	return errWrongPIN
}

// save stores what the user typed: the login password and the PIN, each
// only when given, so either can change alone.
func (u *unlocker) save(password, pin string) error {
	if pin != "" && !validPIN.MatchString(pin) {
		return errInvalidPIN
	}
	if password != "" {
		if err := u.secrets.Set(secretLoginPassword, []byte(password)); err != nil {
			return err
		}
	}
	if pin != "" {
		return u.secrets.Set(secretUnlockPIN, []byte(pin))
	}
	return nil
}

// forget removes them from the Keychain.
func (u *unlocker) forget() error {
	return errors.Join(u.secrets.Delete(secretLoginPassword), u.secrets.Delete(secretUnlockPIN))
}

// loadUnlock reads whether remote unlocking is set up, off the main
// thread: the Keychain may ask the user first.
func (a *app) loadUnlock() {
	go func() {
		ready := a.unlocker.ready()
		a.ui(func() { a.unlockReady = ready })
	}()
}

// saveUnlock stores the login password and the PIN as typed, and
// publishes the lock when they make unlocking ready.
func (a *app) saveUnlock() {
	password, pin := a.unlockPassword, a.unlockPIN
	if pin != "" && !validPIN.MatchString(pin) {
		a.unlockNote, a.unlockFailed = locales.UnlockInvalidPIN(), true
		return
	}
	a.unlockSaving = true
	go func() {
		err := a.unlocker.save(password, pin)
		ready := a.unlocker.ready()
		a.ui(func() {
			a.unlockSaving = false
			if err != nil {
				a.unlockNote, a.unlockFailed = locales.UnlockSaveFailed(err.Error()), true
				return
			}
			a.unlockPassword, a.unlockPIN = "", ""
			a.unlockNote, a.unlockFailed = locales.UnlockSaved(), false
			if ready != a.unlockReady {
				a.unlockReady = ready
				a.restartService()
			}
		})
	}()
}

// forgetUnlock removes them from the Keychain, and the lock from Home
// Assistant.
func (a *app) forgetUnlock() {
	go func() {
		err := a.unlocker.forget()
		a.ui(func() {
			a.unlockPassword, a.unlockPIN = "", ""
			a.unlockNote, a.unlockFailed = "", false
			if err != nil {
				a.unlockNote, a.unlockFailed = locales.UnlockSaveFailed(err.Error()), true
			}
			if a.unlockReady {
				a.unlockReady = false
				a.restartService()
			}
		})
	}()
}

// requestKeyAccess asks macOS to let the app type, and shows where the
// user allows it.
func requestKeyAccess() {
	if !system.RequestKeyAccess() {
		mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
	}
}
