package system

import (
	"context"
	"errors"
	"testing"
)

func TestParseNowPlaying(t *testing.T) {
	media := parseNowPlaying([]byte(`{"title":"Song","artist":"Artist","album":"","duration":200.5,"rate":1,"elapsed":12.25,"playing":true,"bundle_id":"com.google.Chrome","app":"Google Chrome"}` + "\n"))
	if media == nil {
		t.Fatal("media = nil")
	}
	want := MediaInfo{State: "playing", Title: "Song", Artist: "Artist", AppName: "Google Chrome", BundleID: "com.google.Chrome", DurationSeconds: 200.5, PositionSeconds: 12.25, PlaybackRate: 1, CanNext: true, CanPrevious: true}
	if *media != want {
		t.Fatalf("media = %+v, want %+v", *media, want)
	}
}

func TestParseNowPlayingPausedFallsBackToBundleID(t *testing.T) {
	media := parseNowPlaying([]byte("osascript: some warning\n" + `{"title":"Video","rate":0,"playing":false,"bundle_id":"com.apple.Safari"}`))
	if media == nil {
		t.Fatal("media = nil")
	}
	requireEqual(t, "state", media.State, "paused")
	requireEqual(t, "app", media.AppName, "com.apple.Safari")
}

func TestParseNowPlayingNothing(t *testing.T) {
	for _, output := range []string{"", "{}", `{"title":""}`, "execution error"} {
		if media := parseNowPlaying([]byte(output)); media != nil {
			t.Errorf("parseNowPlaying(%q) = %+v, want nil", output, media)
		}
	}
}

func TestReadMediaRunsJXA(t *testing.T) {
	runner := &fakeRunner{runFn: func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"title":"Song","playing":true}`), nil
	}}
	media := readMedia(context.Background(), runner)
	if media == nil || media.Title != "Song" || media.State != "playing" {
		t.Fatalf("media = %+v", media)
	}
	calls := runner.Calls()
	if len(calls) != 1 || calls[0].name != "/usr/bin/osascript" || calls[0].args[0] != "-l" || calls[0].args[1] != "JavaScript" {
		t.Fatalf("calls = %+v", calls)
	}

	runner.runFn = func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("boom") }
	if media := readMedia(context.Background(), runner); media != nil {
		t.Fatalf("media after error = %+v", media)
	}
}

func TestParseNowPlayingSupportedCommands(t *testing.T) {
	// A video without a playlist: play, pause, toggle, stop and seek.
	media := parseNowPlaying([]byte(`{"title":"Video","playing":true,"commands":[24,3,2,1,0]}`))
	if media.CanNext || media.CanPrevious {
		t.Errorf("media = %+v, want neither next nor previous", media)
	}
	media = parseNowPlaying([]byte(`{"title":"Album","playing":true,"commands":[0,1,2,4,5]}`))
	if !media.CanNext || !media.CanPrevious {
		t.Errorf("media = %+v, want next and previous", media)
	}
}

func TestAppIcon(t *testing.T) {
	runner := &fakeRunner{runFn: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[len(args)-1] != "com.google.Chrome" {
			t.Errorf("args = %q", args)
		}
		return []byte("iVBORw0KGgo=\n"), nil
	}}
	c := newCollector("1", "darwin", runner)
	data, err := c.AppIcon(context.Background(), "com.google.Chrome")
	if err != nil || string(data[1:4]) != "PNG" {
		t.Fatalf("icon = %q, %v", data, err)
	}
	if _, err := c.AppIcon(context.Background(), ""); err == nil {
		t.Error("an icon without a bundle ID")
	}
}
