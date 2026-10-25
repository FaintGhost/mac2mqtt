package system

import (
	"context"
	"testing"
)

const consoleUnlocked = `+-o Root  <class IORegistryEntry, id 0x100000100, retain 28>
    {
      "IOConsoleUsers" = ({"kCGSSessionOnConsoleKey"=Yes,"kCGSSessionUserNameKey"="me","kCGSSessionUserIDKey"=501,"kCGSessionLoginDoneKey"=Yes})
    }
`

const consoleLocked = `+-o Root  <class IORegistryEntry, id 0x100000100, retain 28>
    {
      "IOConsoleUsers" = ({"kCGSSessionOnConsoleKey"=No,"kCGSSessionUserIDKey"=502},{"CGSSessionScreenIsLocked"=Yes,"kCGSSessionOnConsoleKey"=Yes,"kCGSSessionUserIDKey"=501})
    }
`

func TestParseScreenLocked(t *testing.T) {
	for _, c := range []struct {
		name   string
		output string
		uid    int
		locked bool
	}{
		{"unlocked", consoleUnlocked, 501, false},
		{"locked", consoleLocked, 501, true},
		{"switched away", consoleLocked, 502, true},
	} {
		locked, err := parseScreenLocked(c.output, c.uid)
		if err != nil || locked != c.locked {
			t.Errorf("%s: locked = %v, %v; want %v", c.name, locked, err, c.locked)
		}
	}
	if _, err := parseScreenLocked(consoleUnlocked, 503); err == nil {
		t.Error("a missing session reads as unlocked")
	}
	if _, err := parseScreenLocked("", 501); err == nil {
		t.Error("empty output reads as unlocked")
	}
}

func TestReadScreenLockedRunsIOReg(t *testing.T) {
	runner := &fakeRunner{runFn: func(context.Context, string, ...string) ([]byte, error) {
		return []byte(consoleLocked), nil
	}}
	locked, err := readScreenLocked(context.Background(), runner, 501)
	if err != nil || locked == false {
		t.Fatalf("locked = %v, %v", locked, err)
	}
	if calls := runner.Calls(); len(calls) != 1 || calls[0].name != "/usr/sbin/ioreg" {
		t.Fatalf("calls = %+v", calls)
	}
}
