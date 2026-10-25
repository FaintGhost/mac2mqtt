package system

import (
	"context"
	"testing"
)

func TestSendMediaCommand(t *testing.T) {
	runner := &fakeRunner{}
	c := newController("darwin", runner)
	for command, code := range map[MediaCommand]string{MediaPlayPause: "2", MediaNext: "4", MediaPrevious: "5"} {
		if err := c.SendMediaCommand(context.Background(), command); err != nil {
			t.Fatal(err)
		}
		calls := runner.Calls()
		last := calls[len(calls)-1]
		if last.name != "/usr/bin/osascript" || last.args[len(last.args)-1] != code {
			t.Errorf("%s ran %+v", command, last)
		}
	}
	if err := c.SendMediaCommand(context.Background(), "EJECT"); err == nil {
		t.Error("an unknown command was sent")
	}
	if err := newController("linux", runner).SendMediaCommand(context.Background(), MediaPlay); err != ErrUnsupported {
		t.Errorf("linux = %v", err)
	}
}

func TestSeekMedia(t *testing.T) {
	runner := &fakeRunner{}
	c := newController("darwin", runner)
	if err := c.SeekMedia(context.Background(), 90.5); err != nil {
		t.Fatal(err)
	}
	args := runner.Calls()[0].args
	if args[len(args)-2] != "seek" || args[len(args)-1] != "90.500" {
		t.Errorf("args = %q", args)
	}
	if err := c.SeekMedia(context.Background(), -1); err == nil {
		t.Error("seeked before the start")
	}
}

func TestParseMediaCommand(t *testing.T) {
	if c, ok := ParseMediaCommand("NEXT"); !ok || c != MediaNext {
		t.Errorf("NEXT = %v, %v", c, ok)
	}
	if _, ok := ParseMediaCommand("next"); ok {
		t.Error("commands are upper case")
	}
}
