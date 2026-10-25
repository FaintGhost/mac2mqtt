package system

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

// fakeScript stands in for the watching osascript: lines written to it are
// what it prints.
type fakeScript struct {
	mu     sync.Mutex
	w      *io.PipeWriter
	starts int
}

func (f *fakeScript) start(ctx context.Context) (io.ReadCloser, func() error, error) {
	r, w := io.Pipe()
	f.mu.Lock()
	f.w = w
	f.starts++
	f.mu.Unlock()
	go func() {
		<-ctx.Done()
		w.Close()
	}()
	return r, func() error { return nil }, nil
}

func (f *fakeScript) print(line string) {
	f.mu.Lock()
	w := f.w
	f.mu.Unlock()
	w.Write([]byte(line + "\n"))
}

func TestMediaWatcherRunsThePositionOn(t *testing.T) {
	now := time.Unix(1000, 0)
	var clock sync.Mutex
	f := &fakeScript{}
	w := &mediaWatcher{start: f.start, now: func() time.Time { clock.Lock(); defer clock.Unlock(); return now }}
	defer w.Stop()

	if _, ok := w.Current(); ok {
		t.Fatal("the watcher told something before the script printed")
	}
	f.print(`{"title":"Song","playing":true,"rate":1,"elapsed":10,"duration":200,"at":1000}`)
	waitFor(t, func() bool { _, ok := w.Current(); return ok })
	clock.Lock()
	now = now.Add(5 * time.Second)
	clock.Unlock()
	media, _ := w.Current()
	if media.Title != "Song" || media.PositionSeconds != 15 {
		t.Fatalf("media = %+v, want Song at 15 s", media)
	}
	// Silent past its heartbeat, the script is not to be trusted.
	clock.Lock()
	now = now.Add(watcherSilence + time.Second)
	clock.Unlock()
	if _, ok := w.Current(); ok {
		t.Error("a silent script was trusted")
	}
	if f.starts != 1 {
		t.Errorf("started %d scripts, want 1", f.starts)
	}
}

// waitFor waits for a condition, failing after a minute: it never decides
// on time alone.
func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met")
		}
		time.Sleep(time.Millisecond)
	}
}
