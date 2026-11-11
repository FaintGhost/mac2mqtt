package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

// mapSecrets is a Keychain in memory.
type mapSecrets map[string][]byte

func (m mapSecrets) Get(key string) ([]byte, error) {
	if v, ok := m[key]; ok {
		return v, nil
	}
	return nil, mygo.ErrSecretNotFound
}
func (m mapSecrets) Set(key string, value []byte) error { m[key] = value; return nil }
func (m mapSecrets) Delete(key string) error            { delete(m, key); return nil }

func newTestUnlocker(now *time.Time) (*unlocker, *[]string) {
	var typed []string
	u := &unlocker{
		secrets: mapSecrets{},
		unlock: func(_ context.Context, password string) error {
			typed = append(typed, password)
			return nil
		},
		now: func() time.Time { return *now },
	}
	return u, &typed
}

func TestUnlockerTypesThePasswordForThePIN(t *testing.T) {
	now := time.Unix(0, 0)
	u, typed := newTestUnlocker(&now)
	if u.ready() {
		t.Fatal("ready with nothing saved")
	}
	if err := u.Unlock(context.Background(), "1234"); !errors.Is(err, errNoUnlock) {
		t.Fatalf("err = %v, want errNoUnlock", err)
	}
	if err := u.save("hunter2", "12"); !errors.Is(err, errInvalidPIN) {
		t.Fatalf("saved a two-digit PIN: %v", err)
	}
	if err := u.save("hunter2", "2468"); err != nil || !u.ready() {
		t.Fatalf("save: %v, ready %v", err, u.ready())
	}
	if err := u.Unlock(context.Background(), "1357"); !errors.Is(err, errWrongPIN) {
		t.Fatalf("err = %v, want errWrongPIN", err)
	}
	if err := u.Unlock(context.Background(), "2468"); err != nil {
		t.Fatal(err)
	}
	// The PIN changes alone, keeping the password.
	if err := u.save("", "97531"); err != nil {
		t.Fatal(err)
	}
	if err := u.Unlock(context.Background(), "97531"); err != nil {
		t.Fatal(err)
	}
	if len(*typed) != 2 || (*typed)[0] != "hunter2" || (*typed)[1] != "hunter2" {
		t.Errorf("typed %q", *typed)
	}
	if err := u.forget(); err != nil || u.ready() {
		t.Errorf("forget: %v, ready %v", err, u.ready())
	}
}

func TestUnlockerPausesAfterWrongPINs(t *testing.T) {
	now := time.Unix(0, 0)
	u, typed := newTestUnlocker(&now)
	if err := u.save("hunter2", "2468"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < maxPINFailures; i++ {
		if err := u.Unlock(context.Background(), "0000"); !errors.Is(err, errWrongPIN) {
			t.Fatalf("try %d: %v", i, err)
		}
	}
	if err := u.Unlock(context.Background(), "0000"); !errors.Is(err, errPINLockedOut) {
		t.Fatalf("err = %v, want errPINLockedOut", err)
	}
	// Paused, even the right PIN does not unlock.
	now = now.Add(pinLockout - time.Second)
	if err := u.Unlock(context.Background(), "2468"); !errors.Is(err, errPINLockedOut) {
		t.Fatalf("err = %v, want errPINLockedOut", err)
	}
	now = now.Add(time.Second)
	if err := u.Unlock(context.Background(), "2468"); err != nil {
		t.Fatal(err)
	}
	if len(*typed) != 1 {
		t.Errorf("typed %d times", len(*typed))
	}
}
