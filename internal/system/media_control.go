package system

import (
	"context"
	"fmt"
	"strconv"
)

// MediaCommand is a command for the Now Playing app, as the media keys and
// Control Center send them.
type MediaCommand string

const (
	MediaPlay      MediaCommand = "PLAY"
	MediaPause     MediaCommand = "PAUSE"
	MediaPlayPause MediaCommand = "PLAY_PAUSE"
	MediaStop      MediaCommand = "STOP"
	MediaNext      MediaCommand = "NEXT"
	MediaPrevious  MediaCommand = "PREVIOUS"
)

// mediaRemoteCommands are MediaRemote's MRMediaRemoteCommand values.
var mediaRemoteCommands = map[MediaCommand]int{
	MediaPlay:      0,
	MediaPause:     1,
	MediaPlayPause: 2,
	MediaStop:      3,
	MediaNext:      4,
	MediaPrevious:  5,
}

// ParseMediaCommand reads a command payload such as "PLAY_PAUSE".
func ParseMediaCommand(s string) (MediaCommand, bool) {
	c := MediaCommand(s)
	_, ok := mediaRemoteCommands[c]
	return c, ok
}

// mediaControlScript sends a command, or with "seek" sets the elapsed time,
// through MediaRemote from osascript, which macOS lets use it, as
// nowPlayingScript does. MediaRemote delivers it asynchronously: the run
// loop runs a moment so that osascript does not exit before it does.
const mediaControlScript = `ObjC.import('Foundation');
function run(argv) {
  $.NSBundle.bundleWithPath('/System/Library/PrivateFrameworks/MediaRemote.framework').load;
  if (argv[0] === 'seek') {
    ObjC.bindFunction('MRMediaRemoteSetElapsedTime', ['void', ['double']]);
    $.MRMediaRemoteSetElapsedTime(parseFloat(argv[1]));
  } else {
    ObjC.bindFunction('MRMediaRemoteSendCommand', ['bool', ['int', 'id']]);
    $.MRMediaRemoteSendCommand(parseInt(argv[0]), $());
  }
  $.NSRunLoop.currentRunLoop.runUntilDate($.NSDate.dateWithTimeIntervalSinceNow(0.4));
  return 'ok';
}`

// SendMediaCommand sends a command to the Now Playing app.
func (c *Controller) SendMediaCommand(ctx context.Context, command MediaCommand) error {
	code, ok := mediaRemoteCommands[command]
	if !ok {
		return fmt.Errorf("system: unknown media command %q", command)
	}
	if c.goos != "darwin" {
		return ErrUnsupported
	}
	if _, err := runCommand(ctx, c.runner, "/usr/bin/osascript", "-l", "JavaScript", "-e", mediaControlScript, strconv.Itoa(code)); err != nil {
		return fmt.Errorf("system: media command: %w", err)
	}
	return nil
}

// SeekMedia moves the Now Playing item to seconds from its start.
func (c *Controller) SeekMedia(ctx context.Context, seconds float64) error {
	if seconds < 0 {
		return fmt.Errorf("system: seek to %v seconds", seconds)
	}
	if c.goos != "darwin" {
		return ErrUnsupported
	}
	if _, err := runCommand(ctx, c.runner, "/usr/bin/osascript", "-l", "JavaScript", "-e", mediaControlScript, "seek", strconv.FormatFloat(seconds, 'f', 3, 64)); err != nil {
		return fmt.Errorf("system: seek: %w", err)
	}
	return nil
}
