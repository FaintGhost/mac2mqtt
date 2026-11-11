package system

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeTyper is a lock screen: Return unlocks it when the typed text is
// the password.
type fakeTyper struct {
	allowed  bool
	state    screenState
	password string
	typed    string
	returns  int
	// onType runs after each character, as a user unlocking meanwhile.
	onType func(f *fakeTyper)
}

func (f *fakeTyper) Allowed() bool { return f.allowed }
func (f *fakeTyper) Type(r rune) {
	f.typed += string(r)
	if f.onType != nil {
		f.onType(f)
	}
}
func (f *fakeTyper) Return() {
	f.returns++
	if f.typed == f.password {
		f.state.Locked = false
	}
}
func (f *fakeTyper) State() (screenState, error) { return f.state, nil }

func useTyper(t *testing.T, f *fakeTyper) {
	t.Helper()
	oldTyper, oldWait := typer, unlockWait
	typer = f
	unlockWait = func(ctx context.Context, _ time.Duration) bool { return ctx.Err() == nil }
	t.Cleanup(func() { typer, unlockWait = oldTyper, oldWait })
}

func TestUnlockScreenTypesThePasswordIntoTheLockScreen(t *testing.T) {
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, password: "pässwörd 1"}
	useTyper(t, f)
	runner := &fakeRunner{}
	if err := newController("darwin", runner).UnlockScreen(context.Background(), "pässwörd 1"); err != nil {
		t.Fatal(err)
	}
	if f.typed != "pässwörd 1" || f.returns != 1 {
		t.Errorf("typed %q and %d returns", f.typed, f.returns)
	}
	if calls := runner.Calls(); len(calls) != 1 || calls[0].name != "/usr/bin/caffeinate" {
		t.Errorf("ran %v, want the display woken", calls)
	}
}

func TestUnlockScreenTypesOnceWhenThePasswordIsWrong(t *testing.T) {
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, password: "right"}
	useTyper(t, f)
	err := newController("darwin", &fakeRunner{}).UnlockScreen(context.Background(), "wrong")
	if !errors.Is(err, ErrStillLocked) {
		t.Fatalf("err = %v, want ErrStillLocked", err)
	}
	if f.typed != "wrong" || f.returns != 1 {
		t.Errorf("typed %q and %d returns, want one attempt", f.typed, f.returns)
	}
}

func TestUnlockScreenTypesNothingOutsideALockedScreen(t *testing.T) {
	for name, tc := range map[string]struct {
		typer *fakeTyper
		want  error
	}{
		"unlocked":     {&fakeTyper{allowed: true, state: screenState{OnConsole: true}}, nil},
		"another user": {&fakeTyper{allowed: true, state: screenState{Locked: true}}, ErrNotOnConsole},
		"not allowed":  {&fakeTyper{state: screenState{Locked: true, OnConsole: true}}, ErrNoKeyAccess},
		"unlocked as it types": {&fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, onType: func(f *fakeTyper) {
			f.state.Locked = false
		}}, ErrUnlockedMeanwhile},
	} {
		t.Run(name, func(t *testing.T) {
			useTyper(t, tc.typer)
			err := newController("darwin", &fakeRunner{}).UnlockScreen(context.Background(), "secret")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(tc.typer.typed) > 1 || tc.typer.returns != 0 {
				t.Errorf("typed %q and %d returns", tc.typer.typed, tc.typer.returns)
			}
		})
	}
}
