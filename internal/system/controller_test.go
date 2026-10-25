package system

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestControllerSetVolumeUsesExpectedScript(t *testing.T) {
	runner := &fakeRunner{}
	controller := newController("darwin", runner)

	for _, volume := range []int{0, 42, 100} {
		if err := controller.SetVolume(context.Background(), volume); err != nil {
			t.Fatalf("SetVolume(%d) error = %v", volume, err)
		}
	}

	wantScripts := []string{
		"set volume output volume 0",
		"set volume output volume 42",
		"set volume output volume 100",
	}
	calls := runner.Calls()
	requireEqual(t, "call count", len(calls), len(wantScripts))
	for index, script := range wantScripts {
		requireEqual(t, "command name", calls[index].name, "/usr/bin/osascript")
		wantArgs := []string{"-e", script}
		requireEqual(t, "command args", reflect.DeepEqual(calls[index].args, wantArgs), true)
	}
}

func TestControllerSetMutedUsesExpectedScript(t *testing.T) {
	runner := &fakeRunner{}
	controller := newController("darwin", runner)

	if err := controller.SetMuted(context.Background(), true); err != nil {
		t.Fatalf("SetMuted(true) error = %v", err)
	}
	if err := controller.SetMuted(context.Background(), false); err != nil {
		t.Fatalf("SetMuted(false) error = %v", err)
	}

	calls := runner.Calls()
	requireEqual(t, "call count", len(calls), 2)
	wantArgs := []string{
		"-e", "set volume with output muted",
		"-e", "set volume without output muted",
	}
	requireEqual(t, "first args", reflect.DeepEqual(calls[0].args, wantArgs[:2]), true)
	requireEqual(t, "second args", reflect.DeepEqual(calls[1].args, wantArgs[2:]), true)
}

func TestControllerRejectsOutOfRangeVolume(t *testing.T) {
	runner := &fakeRunner{}
	controller := newController("darwin", runner)

	for _, volume := range []int{-1, 101} {
		err := controller.SetVolume(context.Background(), volume)
		if err == nil {
			t.Fatalf("SetVolume(%d) error = nil, want validation error", volume)
		}
	}
	requireEqual(t, "command count", len(runner.Calls()), 0)
}

func TestControllerUnsupportedPlatform(t *testing.T) {
	runner := &fakeRunner{}
	controller := newController("linux", runner)

	if !errors.Is(controller.SetVolume(context.Background(), 50), ErrUnsupported) {
		t.Fatal("SetVolume() did not return ErrUnsupported")
	}
	if !errors.Is(controller.SetMuted(context.Background(), true), ErrUnsupported) {
		t.Fatal("SetMuted() did not return ErrUnsupported")
	}
	requireEqual(t, "command count", len(runner.Calls()), 0)
}

func TestControllerPropagatesErrorsAndUsesDeadline(t *testing.T) {
	sentinel := errors.New("osascript failure")
	deadlineSeen := false
	runner := &fakeRunner{runFn: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		_, deadlineSeen = ctx.Deadline()
		return nil, sentinel
	}}
	controller := newController("darwin", runner)

	err := controller.SetVolume(context.Background(), 50)
	if !errors.Is(err, sentinel) {
		t.Fatalf("SetVolume() error = %v, want wrapped sentinel", err)
	}
	requireEqual(t, "command deadline", deadlineSeen, true)
}
