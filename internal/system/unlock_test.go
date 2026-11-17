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
	onType      func(f *fakeTyper)
	onReturn    func(f *fakeTyper)
	stateErr    error
	axWorks     bool
	ignoreSet   bool
	notReady    int
	fieldChecks int
	fieldLost   bool
	closed      bool
	selectedAll bool
	keys        []uint16
	onSet       func(*fakeTyper)
	typeErr     error
}

func (f *fakeTyper) Allowed() bool { return f.allowed }
func (f *fakeTyper) Type(r rune) error {
	if f.typeErr != nil {
		return f.typeErr
	}
	f.typed += string(r)
	if f.onType != nil {
		f.onType(f)
	}
	return nil
}
func (f *fakeTyper) Return() {
	f.returns++
	if f.typed == f.password {
		f.state.Locked = false
	}
	if f.onReturn != nil {
		f.onReturn(f)
	}
}
func (f *fakeTyper) State() (screenState, error) { return f.state, f.stateErr }
func (f *fakeTyper) Key(code uint16, flags uint64) error {
	f.keys = append(f.keys, code)
	switch code {
	case keyA:
		f.selectedAll = flags == flagCommand
	case keyDelete:
		if f.selectedAll {
			f.typed = ""
		}
	case keyReturn:
		f.Return()
	}
	return nil
}
func (f *fakeTyper) PasswordField() (passwordField, error) {
	f.fieldChecks++
	if f.fieldChecks <= f.notReady {
		return nil, ErrNoPasswordField
	}
	return f, nil
}
func (f *fakeTyper) Focused() (bool, error) { return !f.fieldLost, nil }
func (f *fakeTyper) SetValue(password string) error {
	if !f.axWorks {
		return errors.New("AX value is read-only")
	}
	if f.ignoreSet {
		return nil
	}
	f.typed = password
	if f.onSet != nil && password != "" {
		f.onSet(f)
	}
	return nil
}
func (f *fakeTyper) ValueHasLength(password string) (bool, error) {
	return len([]rune(f.typed)) == len([]rune(password)), nil
}
func (f *fakeTyper) Close() { f.closed = true }

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

func TestUnlockScreenClearsPreviousInput(t *testing.T) {
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, password: "right", typed: "leftover"}
	useTyper(t, f)
	if err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "right"); err != nil {
		t.Fatalf("unlock with existing password-field text: %v", err)
	}
	if f.typed != "right" || f.returns != 1 {
		t.Errorf("got %q and %d submissions, want only the new password once", f.typed, f.returns)
	}
}

func TestUnlockScreenStopsTypingWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, onType: func(*fakeTyper) { cancel() }}
	useTyper(t, f)
	if err := newController("darwin", &fakeRunner{}).UnlockScreen(ctx, "secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if f.typed != "s" || f.returns != 0 {
		t.Errorf("continued after cancellation: typed %q, submitted %d times", f.typed, f.returns)
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

func TestUnlockScreenChecksSessionAfterSubmitting(t *testing.T) {
	stateError := errors.New("session unavailable")
	for name, tc := range map[string]struct {
		change func(*fakeTyper)
		want   error
	}{
		"switched user": {func(f *fakeTyper) { f.state = screenState{} }, ErrNotOnConsole},
		"state failed":  {func(f *fakeTyper) { f.stateErr = stateError }, stateError},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, password: "right", onReturn: tc.change}
			useTyper(t, f)
			if err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "right"); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestUnlockScreenDoesNotSubmitUnconfirmedInput(t *testing.T) {
	for _, existing := range []string{"", "old"} {
		f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, axWorks: true, ignoreSet: true, password: "right", typed: existing}
		useTyper(t, f)
		err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "right")
		if err == nil || f.returns != 0 {
			t.Fatalf("unconfirmed input submitted: err = %v, submissions = %d", err, f.returns)
		}
	}
}

func TestUnlockScreenUsesAXToReplaceTheWholePassword(t *testing.T) {
	password := "páss 密码 🔐\x00end"
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, axWorks: true, password: password, typed: "old text"}
	useTyper(t, f)
	if err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), password); err != nil {
		t.Fatal(err)
	}
	if f.typed != password || f.returns != 1 || !f.closed {
		t.Fatalf("password replaced = %v, submissions = %d, closed = %v", f.typed == password, f.returns, f.closed)
	}
	if len(f.keys) != 2 || f.keys[0] != keyShift || f.keys[1] != keyReturn {
		t.Fatalf("AX path posted extra keys: %v", f.keys)
	}
}

func TestUnlockScreenWaitsForThePasswordField(t *testing.T) {
	for _, notReady := range []int{4, unlockReadyChecks} {
		f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, password: "right", notReady: notReady}
		useTyper(t, f)
		err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "right")
		if notReady < unlockReadyChecks {
			if err != nil || f.fieldChecks != notReady+1 || f.typed != "right" {
				t.Fatalf("late field: err = %v, checks = %d, typed = %q", err, f.fieldChecks, f.typed)
			}
		} else if !errors.Is(err, ErrNoPasswordField) || f.typed != "" || f.returns != 0 {
			t.Fatalf("missing field: err = %v, typed = %q, submissions = %d", err, f.typed, f.returns)
		}
	}
}

func TestUnlockScreenStopsWhenFocusChanges(t *testing.T) {
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, onType: func(f *fakeTyper) { f.fieldLost = true }}
	useTyper(t, f)
	if err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "secret"); !errors.Is(err, ErrNoPasswordField) {
		t.Fatalf("err = %v, want ErrNoPasswordField", err)
	}
	if f.typed != "s" || f.returns != 0 || !f.closed {
		t.Fatalf("typed = %q, submissions = %d, closed = %v", f.typed, f.returns, f.closed)
	}
}

func TestUnlockScreenDoesNotSubmitAfterAXInputIsInterrupted(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		want := ErrUnlockedMeanwhile
		f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, axWorks: true, onSet: func(f *fakeTyper) {
			if cancelRequest {
				cancel()
			} else {
				f.state.Locked = false
			}
		}}
		if cancelRequest {
			want = context.Canceled
		}
		useTyper(t, f)
		if err := newController("darwin", &fakeRunner{}).UnlockScreen(ctx, "secret"); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
		if f.returns != 0 || !f.closed {
			t.Fatalf("submissions = %d, closed = %v", f.returns, f.closed)
		}
	}
}

func TestUnlockScreenDoesNotSubmitAfterInputFailure(t *testing.T) {
	inputError := errors.New("event allocation failed")
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, typeErr: inputError}
	useTyper(t, f)
	if err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "secret"); !errors.Is(err, inputError) {
		t.Fatalf("err = %v, want event allocation failure", err)
	}
	if f.returns != 0 || !f.closed {
		t.Fatalf("submissions = %d, closed = %v", f.returns, f.closed)
	}
}

func TestUnlockScreenRejectsOverlappingAttempts(t *testing.T) {
	f := &fakeTyper{allowed: true, state: screenState{Locked: true, OnConsole: true}, password: "right"}
	f.onType = func(*fakeTyper) {
		if err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "second"); !errors.Is(err, ErrUnlockInProgress) {
			t.Fatalf("overlapping unlock = %v", err)
		}
	}
	useTyper(t, f)
	if err := newController("darwin", &fakeRunner{}).UnlockScreen(t.Context(), "right"); err != nil {
		t.Fatal(err)
	}
	if f.typed != "right" || f.returns != 1 {
		t.Fatalf("attempts interleaved: %q, submissions = %d", f.typed, f.returns)
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
