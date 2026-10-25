package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// SystemCommand is a command for the Mac itself.
type SystemCommand string

const (
	CommandLock         SystemCommand = "LOCK"
	CommandDisplaySleep SystemCommand = "DISPLAY_SLEEP"
	CommandDisplayWake  SystemCommand = "DISPLAY_WAKE"
	CommandSleep        SystemCommand = "SLEEP"
)

// ParseSystemCommand reads a command payload such as "LOCK".
func ParseSystemCommand(s string) (SystemCommand, bool) {
	switch c := SystemCommand(strings.ToUpper(strings.TrimSpace(s))); c {
	case CommandLock, CommandDisplaySleep, CommandDisplayWake, CommandSleep:
		return c, true
	}
	return "", false
}

// lockScreen locks the screen; a variable for tests.
var lockScreen = lockScreenNow

// RunSystemCommand locks the screen, sleeps or wakes the display, or puts
// the Mac to sleep.
func (c *Controller) RunSystemCommand(ctx context.Context, command SystemCommand) error {
	if c.goos != "darwin" {
		return ErrUnsupported
	}
	var err error
	switch command {
	case CommandLock:
		err = lockScreen()
	case CommandDisplaySleep:
		_, err = runCommand(ctx, c.runner, "/usr/bin/pmset", "displaysleepnow")
	case CommandDisplayWake:
		// Declaring the user active wakes the display, as a key press does.
		_, err = runCommand(ctx, c.runner, "/usr/bin/caffeinate", "-u", "-t", "1")
	case CommandSleep:
		_, err = runCommand(ctx, c.runner, "/usr/bin/pmset", "sleepnow")
	default:
		return fmt.Errorf("system: unknown command %q", command)
	}
	if err != nil {
		return fmt.Errorf("system: %s: %w", strings.ToLower(string(command)), err)
	}
	return nil
}

// Say speaks text with the system voice, as the say command does. It
// returns once speaking started, without waiting for it to end.
func (c *Controller) Say(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("system: nothing to say")
	}
	if c.goos != "darwin" {
		return ErrUnsupported
	}
	c.background(2*time.Minute, "/usr/bin/say", "--", text)
	return nil
}

// Shortcuts lists the names of the user's shortcuts.
func (c *Controller) Shortcuts(ctx context.Context) ([]string, error) {
	if c.goos != "darwin" {
		return nil, ErrUnsupported
	}
	output, err := runCommand(ctx, c.runner, "/usr/bin/shortcuts", "list")
	if err != nil {
		return nil, fmt.Errorf("system: list shortcuts: %w", err)
	}
	var names []string
	for _, line := range strings.Split(string(output), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// RunShortcut runs a shortcut, with text as its input unless empty. It
// returns once the shortcut started: shortcuts may run for long, and
// commands run one at a time.
func (c *Controller) RunShortcut(ctx context.Context, name, input string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("system: no shortcut named")
	}
	if c.goos != "darwin" {
		return ErrUnsupported
	}
	args := []string{"run", name}
	if input != "" {
		f, err := os.CreateTemp("", "mac2mqtt-shortcut-*.txt")
		if err != nil {
			return err
		}
		_, err = f.WriteString(input)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(f.Name())
			return err
		}
		args = append(args, "--input-path", f.Name())
		defer func() {
			// The shortcut reads its input as it starts.
			time.AfterFunc(time.Minute, func() { os.Remove(f.Name()) })
		}()
	}
	c.background(10*time.Minute, "/usr/bin/shortcuts", args...)
	return nil
}

// background runs a command without waiting for it, for at most d.
func (c *Controller) background(d time.Duration, name string, args ...string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		_, _ = c.runner.Run(ctx, name, args...)
	}()
}
