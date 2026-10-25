package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"mac2mqtt/internal/broker"
	"mac2mqtt/internal/config"
	"mac2mqtt/internal/system"
)

type publishedMessage struct {
	topic  string
	body   []byte
	retain bool
}

type fakeClient struct {
	mu          sync.Mutex
	published   []publishedMessage
	subscribed  map[string]func(broker.Message)
	lost        chan struct{}
	lostOnce    sync.Once
	disconnects int
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		subscribed: make(map[string]func(broker.Message)),
		lost:       make(chan struct{}),
	}
}

func (c *fakeClient) Connect(ctx context.Context) error { return ctx.Err() }
func (c *fakeClient) Publish(ctx context.Context, topic string, payload []byte, retain bool, qos byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, publishedMessage{topic: topic, body: append([]byte(nil), payload...), retain: retain})
	return nil
}
func (c *fakeClient) Subscribe(ctx context.Context, filter string, qos byte, handler func(broker.Message)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscribed[filter] = handler
	return nil
}
func (c *fakeClient) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	c.disconnects++
	c.mu.Unlock()
	return nil
}
func (c *fakeClient) Lost() <-chan struct{} { return c.lost }
func (c *fakeClient) markLost() {
	c.lostOnce.Do(func() { close(c.lost) })
}
func (c *fakeClient) disconnectCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnects
}

func (c *fakeClient) messages() []publishedMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]publishedMessage(nil), c.published...)
}
func (c *fakeClient) handler(topic string) func(broker.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.subscribed[topic]
}

func testConfig() config.Config {
	c := config.Default()
	c.Broker.Host = "broker.test"
	c.Broker.ClientID = "mac2mqtt-test"
	c.HomeAssistant.BaseTopic = "mac2mqtt/test"
	c.HomeAssistant.DiscoveryPrefix = "homeassistant"
	c.HomeAssistant.DeviceName = "Test Mac"
	c.Reporting.Enabled = true
	c.Reporting.IntervalSeconds = 5
	c.Reporting.Retain = true
	c.Normalize()
	return c
}

func testSnapshot() system.Snapshot {
	level := 87.0
	charging := false
	return system.Snapshot{
		LastSeen: time.Date(2026, 10, 25, 16, 20, 0, 0, time.UTC),
		App:      system.AppInfo{Version: "0.1.0", UptimeSeconds: 42},
		Device:   system.DeviceInfo{Hostname: "test-mac", OSVersion: "15.1", Arch: "arm64", Model: "Mac mini", ModelID: "Mac16,10"},
		Network:  system.NetworkInfo{Online: true},
		Power:    system.PowerInfo{OnBattery: false, Source: "AC", BatteryPercent: &level, Charging: &charging},
		Session:  system.SessionInfo{IdleSeconds: 12},
		Audio:    &system.AudioState{Volume: 35},
	}
}

func TestManagerPublishesDiscoveryStateAndAvailability(t *testing.T) {
	fake := newFakeClient()
	m := New(Options{
		Config:   testConfig(),
		Version:  "0.1.0",
		Snapshot: func(context.Context) system.Snapshot { return testSnapshot() },
		NewClient: func(broker.Config) broker.Client {
			return fake
		},
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool { return len(fake.messages()) >= 22 })
	msgs := fake.messages()
	// Clearing retained documents comes first: legacy discovery has no body.
	if msgs[0].topic != "homeassistant/media_player/mac2mqtt-test/now_playing/config" || len(msgs[0].body) != 0 || msgs[0].retain == false {
		t.Errorf("first publish = %+v, want the legacy media_player cleared", msgs[0])
	}
	var state *publishedMessage
	discovery := 0
	online := false
	for i := range msgs {
		if msgs[i].topic == "mac2mqtt/test/state" {
			state = &msgs[i]
		}
		if msgs[i].topic == "mac2mqtt/test/availability" && string(msgs[i].body) == "online" && msgs[i].retain {
			if discovery > 0 {
				t.Error("availability was published after discovery")
			}
			online = true
		}
		if strings.HasPrefix(msgs[i].topic, "homeassistant/") && len(msgs[i].body) > 0 {
			discovery++
		}
	}
	if online == false {
		t.Error("availability was not published")
	}
	if state == nil {
		t.Fatal("state document was not published")
	}
	for _, msg := range msgs {
		if msg.topic != "homeassistant/sensor/mac2mqtt-test/power_source/config" {
			continue
		}
		var cfg struct {
			Device map[string]any `json:"device"`
		}
		if err := json.Unmarshal(msg.body, &cfg); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"manufacturer": "Apple", "model": "Mac mini", "model_id": "Mac16,10", "sw_version": "macOS 15.1"}
		for k, v := range want {
			if cfg.Device[k] != v {
				t.Errorf("device %s = %v, want %v", k, cfg.Device[k], v)
			}
		}
	}
	if discovery != 19 {
		t.Errorf("discovery entities = %d, want 19", discovery)
	}
	var got system.Snapshot
	if err := json.Unmarshal(state.body, &got); err != nil {
		t.Fatalf("state JSON: %v", err)
	}
	if got.Device.Hostname != "test-mac" || got.Audio == nil || got.Audio.Volume != 35 {
		t.Errorf("unexpected state: %+v", got)
	}
	if m.Status().Phase != PhaseOnline || m.Status().Published == 0 {
		t.Errorf("status = %+v", m.Status())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop")
	}
	msgs = fake.messages()
	last := msgs[len(msgs)-1]
	if last.topic != "mac2mqtt/test/availability" || string(last.body) != "offline" {
		t.Errorf("last publish = %+v", last)
	}
	if fake.disconnectCount() == 0 {
		t.Error("client was not disconnected")
	}
}

func TestManagerHandlesVolumeMuteAndRefreshCommands(t *testing.T) {
	fake := newFakeClient()
	volumes := make(chan int, 1)
	muted := make(chan bool, 1)
	m := New(Options{
		Config:   testConfig(),
		Version:  "0.1.0",
		Snapshot: func(context.Context) system.Snapshot { return testSnapshot() },
		NewClient: func(broker.Config) broker.Client {
			return fake
		},
		SetVolume:  func(_ context.Context, v int) error { volumes <- v; return nil },
		SetMuted:   func(_ context.Context, v bool) error { muted <- v; return nil },
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool { return fake.handler("mac2mqtt/test/volume/set") != nil && len(fake.messages()) > 0 })
	volumeHandler := fake.handler("mac2mqtt/test/volume/set")
	if volumeHandler == nil {
		t.Fatal("volume subscription missing")
	}
	volumeHandler(broker.Message{Topic: "mac2mqtt/test/volume/set", Payload: []byte("42")})
	select {
	case v := <-volumes:
		if v != 42 {
			t.Errorf("volume = %d", v)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("volume setter not called")
	}
	muteHandler := fake.handler("mac2mqtt/test/mute/set")
	if muteHandler == nil {
		t.Fatal("mute subscription missing")
	}
	muteHandler(broker.Message{Topic: "mac2mqtt/test/mute/set", Payload: []byte("ON")})
	select {
	case v := <-muted:
		if v == false {
			t.Error("mute setter got false")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("mute setter not called")
	}
	refreshHandler := fake.handler("mac2mqtt/test/refresh")
	if refreshHandler == nil {
		t.Fatal("refresh subscription missing")
	}
	before := len(fake.messages())
	refreshHandler(broker.Message{Topic: "mac2mqtt/test/refresh", Payload: []byte("PRESS")})
	waitFor(t, func() bool { return len(fake.messages()) > before })
	cancel()
	<-done
}

func TestManagerPublishesOfflineWhenConnectionIsLost(t *testing.T) {
	fake := newFakeClient()
	m := New(Options{
		Config:   testConfig(),
		Version:  "0.1.0",
		Snapshot: func(context.Context) system.Snapshot { return testSnapshot() },
		NewClient: func(broker.Config) broker.Client {
			return fake
		},
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool { return len(fake.messages()) > 0 })
	fake.markLost()
	waitFor(t, func() bool {
		for _, msg := range fake.messages() {
			if msg.topic == "mac2mqtt/test/availability" && string(msg.body) == "offline" {
				return true
			}
		}
		return false
	})
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop after connection loss")
	}
}

func TestManagerPublishesMediaStateAndAttributes(t *testing.T) {
	fake := newFakeClient()
	media := &system.MediaInfo{
		State:           "playing",
		Title:           "Test Song",
		Artist:          "Test Artist",
		Album:           "Test Album",
		DurationSeconds: 180,
		PositionSeconds: 12,
		PlaybackRate:    1,
	}
	snapshot := testSnapshot()
	snapshot.Media = media
	m := New(Options{
		Config:   testConfig(),
		Version:  "0.1.0",
		Snapshot: func(context.Context) system.Snapshot { return snapshot },
		Media:    func(context.Context) *system.MediaInfo { return media },
		NewClient: func(broker.Config) broker.Client {
			return fake
		},
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool {
		var state, attrs bool
		for _, msg := range fake.messages() {
			if msg.topic == "mac2mqtt/test/media/state" && string(msg.body) == "playing" {
				state = true
			}
			if msg.topic == "mac2mqtt/test/media/attributes" {
				var data map[string]any
				if json.Unmarshal(msg.body, &data) == nil && data["media_title"] == "Test Song" && data["media_artist"] == "Test Artist" {
					attrs = true
				}
			}
		}
		return state && attrs
	})
	cancel()
	<-done
}

func TestManagerOmitsBatteryEntitiesOnDesktop(t *testing.T) {
	fake := newFakeClient()
	desktop := testSnapshot()
	desktop.Power.BatteryPercent = nil
	desktop.Power.Charging = nil
	desktop.Power.Source = "AC"
	m := New(Options{
		Config:   testConfig(),
		Version:  "0.1.0",
		Snapshot: func(context.Context) system.Snapshot { return desktop },
		NewClient: func(broker.Config) broker.Client {
			return fake
		},
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool {
		for _, msg := range fake.messages() {
			if msg.topic == "mac2mqtt/test/state" {
				return true
			}
		}
		return false
	})
	for _, msg := range fake.messages() {
		if strings.Contains(msg.topic, "/battery/") || strings.Contains(msg.topic, "/charging/") {
			if len(msg.body) != 0 {
				t.Errorf("desktop published battery discovery %s: %s", msg.topic, msg.body)
			}
		}
	}
	cancel()
	<-done
}

func TestManagerClearsRetainedTopicsBeforeOnline(t *testing.T) {
	fake := newFakeClient()
	m := New(Options{
		Config:        testConfig(),
		Version:       "0.1.0",
		Snapshot:      func(context.Context) system.Snapshot { return testSnapshot() },
		CleanupTopics: []string{"old/discovery/config", "old/state"},
		NewClient: func(broker.Config) broker.Client {
			return fake
		},
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool { return len(fake.messages()) >= 2 })
	msgs := fake.messages()
	if msgs[0].topic != "old/discovery/config" || msgs[0].retain == false || len(msgs[0].body) != 0 {
		t.Errorf("first cleanup = %+v", msgs[0])
	}
	if msgs[1].topic != "old/state" || msgs[1].retain == false || len(msgs[1].body) != 0 {
		t.Errorf("second cleanup = %+v", msgs[1])
	}
	cancel()
	<-done
}

func TestParseControlPayloads(t *testing.T) {
	for _, value := range []string{"0", "42", " 100 "} {
		if _, err := parseVolume(value); err != nil {
			t.Errorf("parseVolume(%q): %v", value, err)
		}
	}
	for _, value := range []string{"-1", "101", "x", ""} {
		if _, err := parseVolume(value); err == nil {
			t.Errorf("parseVolume(%q) accepted invalid input", value)
		}
	}
	for _, value := range []string{"ON", "off", "true", "0"} {
		if _, err := parseMute(value); err != nil {
			t.Errorf("parseMute(%q): %v", value, err)
		}
	}
	if _, err := parseMute("maybe"); err == nil {
		t.Error("parseMute accepted invalid input")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for ok() == false {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestManagerLeavesOutDisabledGroups(t *testing.T) {
	fake := newFakeClient()
	cfg := testConfig()
	cfg.HomeAssistant.SetPublishes("media", false)
	mediaReads := 0
	var mu sync.Mutex
	m := New(Options{
		Config:   cfg,
		Version:  "0.1.0",
		Snapshot: func(context.Context) system.Snapshot { return testSnapshot() },
		Media: func(context.Context) *system.MediaInfo {
			mu.Lock()
			mediaReads++
			mu.Unlock()
			return &system.MediaInfo{State: "playing", Title: "Song"}
		},
		NewClient:  func(broker.Config) broker.Client { return fake },
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool {
		for _, msg := range fake.messages() {
			if msg.topic == "mac2mqtt/test/state" {
				return true
			}
		}
		return false
	})
	cancel()
	<-done
	cleared := map[string]bool{}
	for _, msg := range fake.messages() {
		if strings.Contains(msg.topic, "now_playing") || strings.Contains(msg.topic, "media_state") || strings.HasPrefix(msg.topic, "mac2mqtt/test/media/") {
			if len(msg.body) != 0 {
				t.Errorf("disabled media published %s: %s", msg.topic, msg.body)
			}
			cleared[msg.topic] = true
		}
	}
	for _, topic := range []string{
		"homeassistant/sensor/mac2mqtt-test/now_playing/config",
		"homeassistant/sensor/mac2mqtt-test/media_state/config",
		"mac2mqtt/test/media/state",
		"mac2mqtt/test/media/attributes",
	} {
		if cleared[topic] == false {
			t.Errorf("%s was not cleared", topic)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if mediaReads != 0 {
		t.Errorf("read Now Playing %d times while media is disabled", mediaReads)
	}
}

func TestManagerControlsMedia(t *testing.T) {
	fake := newFakeClient()
	var mu sync.Mutex
	var commands []system.MediaCommand
	var seeks []float64
	var volumes []int
	var mutes []bool
	m := New(Options{
		Config:   testConfig(),
		Version:  "0.1.0",
		Snapshot: func(context.Context) system.Snapshot { return testSnapshot() },
		Media: func(context.Context) *system.MediaInfo {
			return &system.MediaInfo{State: "playing", Title: "Song", Artist: "Artist", AppName: "Music", DurationSeconds: 200, PositionSeconds: 12}
		},
		MediaCommand: func(_ context.Context, c system.MediaCommand) error {
			mu.Lock()
			defer mu.Unlock()
			commands = append(commands, c)
			return nil
		},
		SeekMedia: func(_ context.Context, s float64) error {
			mu.Lock()
			defer mu.Unlock()
			seeks = append(seeks, s)
			return nil
		},
		SetVolume: func(_ context.Context, v int) error {
			mu.Lock()
			defer mu.Unlock()
			volumes = append(volumes, v)
			return nil
		},
		SetMuted: func(_ context.Context, muted bool) error {
			mu.Lock()
			defer mu.Unlock()
			mutes = append(mutes, muted)
			return nil
		},
		NewClient:  func(broker.Config) broker.Client { return fake },
		RetryDelay: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()
	for _, topic := range []string{"media/command", "media/seek", "media/volume/set", "media/mute/set"} {
		topic := "mac2mqtt/test/" + topic
		waitFor(t, func() bool { return fake.handler(topic) != nil })
	}
	send := func(topic, payload string) {
		fake.handler("mac2mqtt/test/" + topic)(broker.Message{Topic: "mac2mqtt/test/" + topic, Payload: []byte(payload)})
	}
	send("media/command", "NEXT")
	send("media/command", "PLAY_PAUSE")
	send("media/seek", "42.5")
	send("media/volume/set", "0.42")
	send("media/mute/set", "mute")
	send("media/command", "EJECT")
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(commands) == 2 && len(seeks) == 1 && len(volumes) == 1 && len(mutes) == 1
	})
	mu.Lock()
	if commands[0] != system.MediaNext || commands[1] != system.MediaPlayPause || seeks[0] != 42.5 || volumes[0] != 42 || !mutes[0] {
		t.Errorf("commands %v, seeks %v, volumes %v, mutes %v", commands, seeks, volumes, mutes)
	}
	mu.Unlock()
	// The media player's one-field topics hold what plays and the volume.
	want := map[string]string{
		"mac2mqtt/test/media/title":     "Song",
		"mac2mqtt/test/media/artist":    "Artist",
		"mac2mqtt/test/media/duration":  "200",
		"mac2mqtt/test/media/volume":    "0.35",
		"mac2mqtt/test/media/mute":      "unmute",
		"mac2mqtt/test/media/mediatype": "music",
	}
	waitFor(t, func() bool {
		got := map[string]string{}
		for _, msg := range fake.messages() {
			got[msg.topic] = string(msg.body)
		}
		for topic, value := range want {
			if got[topic] != value {
				return false
			}
		}
		return true
	})
}
