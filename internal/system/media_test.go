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
	want := MediaInfo{State: "playing", Title: "Song", Artist: "Artist", AppName: "Google Chrome", DurationSeconds: 200.5, PositionSeconds: 12.25, PlaybackRate: 1}
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
