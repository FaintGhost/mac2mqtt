package system

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestRunSystemCommand(t *testing.T) {
	runner := &fakeRunner{}
	c := newController("darwin", runner)
	locked := false
	lockScreen = func() error { locked = true; return nil }
	defer func() { lockScreen = lockScreenNow }()
	for _, command := range []SystemCommand{CommandLock, CommandDisplaySleep, CommandDisplayWake, CommandSleep} {
		if err := c.RunSystemCommand(context.Background(), command); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if !locked {
		t.Error("LOCK did not lock")
	}
	var got []string
	for _, call := range runner.Calls() {
		got = append(got, call.name+" "+strings.Join(call.args, " "))
	}
	want := []string{"/usr/bin/pmset displaysleepnow", "/usr/bin/caffeinate -u -t 1", "/usr/bin/pmset sleepnow"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("ran %q, want %q", got, want)
	}
	if _, ok := ParseSystemCommand("reboot"); ok {
		t.Error("parsed an unknown command")
	}
	if c, ok := ParseSystemCommand(" lock "); !ok || c != CommandLock {
		t.Errorf("lock = %v, %v", c, ok)
	}
}

func TestShortcuts(t *testing.T) {
	var mu sync.Mutex
	ran := make(chan []string, 1)
	runner := &fakeRunner{runFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "list" {
			return []byte("Morning\nFocus On\n\n"), nil
		}
		mu.Lock()
		defer mu.Unlock()
		input := ""
		if len(args) == 4 {
			b, _ := os.ReadFile(args[3])
			input = string(b)
		}
		ran <- append(append([]string{}, args...), input)
		return nil, nil
	}}
	c := newController("darwin", runner)
	names, err := c.Shortcuts(context.Background())
	if err != nil || strings.Join(names, ",") != "Morning,Focus On" {
		t.Fatalf("shortcuts = %q, %v", names, err)
	}
	if err := c.RunShortcut(context.Background(), "Focus On", "hello"); err != nil {
		t.Fatal(err)
	}
	args := <-ran
	if args[0] != "run" || args[1] != "Focus On" || args[2] != "--input-path" || args[4] != "hello" {
		t.Errorf("ran %q", args)
	}
	if err := c.RunShortcut(context.Background(), " ", ""); err == nil {
		t.Error("ran a shortcut without a name")
	}
}
