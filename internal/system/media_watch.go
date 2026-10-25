package system

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
	"time"
)

// mediaWatcher keeps one osascript watching Now Playing, rather than
// starting one for every read: it prints a line only when something but
// the position changes. It starts on the first read and stops when no one
// has read it for a while, as when Now Playing is not published.
type mediaWatcher struct {
	// start runs the watching script, returning its output; tests replace
	// it.
	start func(ctx context.Context) (io.ReadCloser, func() error, error)
	now   func() time.Time

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	latest   *nowPlayingOutput
	heard    time.Time // when the script last printed
	read     time.Time // when Current was last called
	failures int
	retry    time.Time
}

// Silence longer than this means the script is stuck: its heartbeat is
// every ten seconds.
const watcherSilence = 25 * time.Second

func newMediaWatcher() *mediaWatcher {
	return &mediaWatcher{
		start: func(ctx context.Context) (io.ReadCloser, func() error, error) {
			cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", nowPlayingScript, "watch")
			out, err := cmd.StdoutPipe()
			if err != nil {
				return nil, nil, err
			}
			if err := cmd.Start(); err != nil {
				return nil, nil, err
			}
			return out, cmd.Wait, nil
		},
		now: time.Now,
	}
}

// Current returns what plays, with its position run on to now, and false
// when the watcher has nothing to tell yet, or cannot run.
func (w *mediaWatcher) Current() (*MediaInfo, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	w.read = now
	if !w.running && now.After(w.retry) {
		w.launch()
	}
	if !w.running || w.latest == nil || now.Sub(w.heard) > watcherSilence {
		return nil, false
	}
	out := *w.latest
	media := mediaFromOutput(&out)
	if media != nil && media.State == "playing" && out.At > 0 {
		at := time.Unix(0, int64(out.At*float64(time.Second)))
		media.PositionSeconds += now.Sub(at).Seconds() * max(media.PlaybackRate, 0)
		if media.DurationSeconds > 0 {
			media.PositionSeconds = min(media.PositionSeconds, media.DurationSeconds)
		}
	}
	return media, true
}

// launch starts the script and reads its lines; w.mu is held.
func (w *mediaWatcher) launch() {
	ctx, cancel := context.WithCancel(context.Background())
	out, wait, err := w.start(ctx)
	if err != nil {
		cancel()
		w.failed()
		return
	}
	w.running, w.cancel, w.latest = true, cancel, nil
	go w.readLines(ctx, out, wait)
	go w.stopWhenIdle(ctx)
}

func (w *mediaWatcher) readLines(ctx context.Context, out io.ReadCloser, wait func() error) {
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var line nowPlayingOutput
		if !decodeNowPlaying(scanner.Bytes(), &line) {
			continue
		}
		w.mu.Lock()
		w.latest, w.heard, w.failures = &line, w.now(), 0
		w.mu.Unlock()
	}
	out.Close()
	_ = wait()
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx.Err() == nil {
		w.failed() // ended by itself
	}
	w.running, w.latest = false, nil
}

// failed backs off before the next launch: 5 s, doubling to 5 minutes.
func (w *mediaWatcher) failed() {
	w.failures++
	d := 5 * time.Second << min(w.failures-1, 6)
	w.retry = w.now().Add(min(d, 5*time.Minute))
}

// stopWhenIdle stops the script once no one read it for a minute.
func (w *mediaWatcher) stopWhenIdle(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			idle := w.now().Sub(w.read) > time.Minute
			w.mu.Unlock()
			if idle {
				w.Stop()
				return
			}
		}
	}
}

// Stop ends the script, as when the app quits.
func (w *mediaWatcher) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	w.cancel = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
