package system

import (
	"bytes"
	"context"
	"encoding/json"
)

// nowPlayingScript reads the system-wide Now Playing item from MediaRemote,
// the private framework behind Control Center, and prints it as JSON.
//
// Since macOS 15.4 mediaremoted only answers Apple-signed processes, so
// calling MediaRemote from this app returns nothing. osascript is signed by
// Apple, and JXA's Objective-C bridge reaches MRNowPlayingRequest's class
// methods without a helper binary.
const nowPlayingScript = `ObjC.import('Foundation');
function run() {
  const out = {};
  if (!$.NSBundle.bundleWithPath('/System/Library/PrivateFrameworks/MediaRemote.framework').load) return JSON.stringify(out);
  const R = $.NSClassFromString('MRNowPlayingRequest');
  if (R.isNil()) return JSON.stringify(out);
  const item = R.localNowPlayingItem;
  const info = item.isNil() ? $() : item.nowPlayingInfo;
  if (info.isNil()) return JSON.stringify(out);
  const value = k => info.objectForKey('kMRMediaRemoteNowPlayingInfo' + k);
  const str = k => { const v = value(k); return v.isNil() ? '' : ObjC.unwrap(v.description); };
  const num = k => { const v = value(k); return v.isNil() ? 0 : v.doubleValue; };
  out.title = str('Title');
  out.artist = str('Artist');
  out.album = str('Album');
  out.duration = num('Duration');
  out.rate = num('PlaybackRate');
  out.elapsed = num('ElapsedTime');
  const ts = value('Timestamp');
  if (!ts.isNil() && out.rate > 0) out.elapsed += -ts.timeIntervalSinceNow * out.rate;
  out.playing = R.localIsPlaying === true;
  const path = R.localNowPlayingPlayerPath;
  if (!path.isNil() && !path.client.isNil()) {
    out.bundle_id = ObjC.unwrap(path.client.bundleIdentifier) || '';
    out.app = ObjC.unwrap(path.client.displayName) || '';
  }
  if (R.respondsToSelector('localSupportedCommands')) {
    const commands = R.localSupportedCommands;
    out.commands = [];
    for (let i = 0; i < commands.count; i++) {
      const c = commands.objectAtIndex(i);
      if (!c.respondsToSelector('isEnabled') || c.isEnabled) out.commands.push(c.command);
    }
  }
  return JSON.stringify(out);
}`

// nowPlayingOutput is what nowPlayingScript prints.
type nowPlayingOutput struct {
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float64 `json:"duration"`
	Elapsed  float64 `json:"elapsed"`
	Rate     float64 `json:"rate"`
	Playing  bool    `json:"playing"`
	BundleID string  `json:"bundle_id"`
	App      string  `json:"app"`
	// Commands are the MediaRemote commands the player takes, nil when
	// macOS does not tell.
	Commands []int `json:"commands"`
}

// MediaRemote's commands for the next and previous track.
const (
	commandNextTrack     = 4
	commandPreviousTrack = 5
)

// readMedia asks MediaRemote, through osascript, for the Now Playing item.
func readMedia(ctx context.Context, runner commandRunner) *MediaInfo {
	output, err := runCommand(ctx, runner, "/usr/bin/osascript", "-l", "JavaScript", "-e", nowPlayingScript)
	if err != nil {
		return nil
	}
	return parseNowPlaying(output)
}

// parseNowPlaying turns the script's output into a MediaInfo. It returns nil
// when nothing has registered as Now Playing.
func parseNowPlaying(output []byte) *MediaInfo {
	// The runner combines stdout and stderr, and osascript may warn on
	// stderr; the script's JSON is its last line.
	output = bytes.TrimSpace(output)
	if i := bytes.LastIndexByte(output, '\n'); i >= 0 {
		output = output[i+1:]
	}
	var out nowPlayingOutput
	if json.Unmarshal(output, &out) != nil || out.Title == "" {
		return nil
	}
	media := &MediaInfo{
		State:           "paused",
		Title:           out.Title,
		Artist:          out.Artist,
		Album:           out.Album,
		AppName:         out.App,
		DurationSeconds: out.Duration,
		PositionSeconds: out.Elapsed,
		PlaybackRate:    out.Rate,
	}
	if media.AppName == "" {
		media.AppName = out.BundleID
	}
	if out.Playing || out.Rate > 0 {
		media.State = "playing"
	}
	// A player takes next and previous only when it has them, as a video
	// without a playlist does not. When macOS does not say, try them.
	media.CanNext, media.CanPrevious = out.Commands == nil, out.Commands == nil
	for _, c := range out.Commands {
		switch c {
		case commandNextTrack:
			media.CanNext = true
		case commandPreviousTrack:
			media.CanPrevious = true
		}
	}
	return media
}
