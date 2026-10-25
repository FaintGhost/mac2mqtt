// Package service owns the MQTT connection, Home Assistant discovery and the
// periodic publishing of Mac2MQTT's state document.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"mac2mqtt/internal/broker"
	"mac2mqtt/internal/config"
	"mac2mqtt/internal/homeassistant"
	"mac2mqtt/internal/system"
)

// Phase is the connection state shown in the app.
type Phase string

const (
	PhaseIdle       Phase = "idle"
	PhaseConnecting Phase = "connecting"
	PhaseOnline     Phase = "online"
	PhaseError      Phase = "error"
)

// Status is a snapshot of the MQTT service for the UI.
type Status struct {
	Phase       Phase
	Broker      string
	LastError   string
	LastPublish time.Time
	Published   uint64
}

// Options configures a Manager.
type Options struct {
	Config    config.Config
	Version   string
	Snapshot  func(context.Context) system.Snapshot
	OnStatus  func(Status)
	OnState   func(system.Snapshot)
	SetVolume func(context.Context, int) error
	SetMuted  func(context.Context, bool) error
	Media     func(context.Context) *system.MediaInfo
	// MediaCommand and SeekMedia control the Now Playing app.
	MediaCommand func(context.Context, system.MediaCommand) error
	SeekMedia    func(context.Context, float64) error
	// Activity reads whether a camera or microphone is in use and whether
	// the display sleeps, which the service watches every few seconds.
	Activity func() system.Activity
	// SystemCommand locks the screen, sleeps or wakes the display, or puts
	// the Mac to sleep.
	SystemCommand func(context.Context, system.SystemCommand) error
	// Notify shows a notification, and Say speaks text.
	Notify func(ctx context.Context, title, message string) error
	Say    func(context.Context, string) error
	// Shortcuts lists the user's shortcuts, and RunShortcut runs one.
	Shortcuts   func(context.Context) []string
	RunShortcut func(ctx context.Context, name, input string) error
	// CleanupTopics are retained topics of an earlier configuration whose
	// payloads the service clears with empty retained publishes after connect.
	CleanupTopics []string

	// NewClient is for tests; nil uses broker.Dial.
	NewClient func(broker.Config) broker.Client
	// RetryDelay is the first reconnect delay; it doubles up to one minute.
	RetryDelay time.Duration
	// rediscoverHoldoff is how long after publishing discovery a birth of
	// Home Assistant does not publish it again; tests shorten it.
	rediscoverHoldoff time.Duration
}

// Manager runs one MQTT lifecycle at a time. Run blocks until ctx is canceled
// or the manager gives up after an unrecoverable local error.
type Manager struct {
	opts Options

	mu       sync.Mutex
	status   Status
	publish  chan struct{}
	commands chan commandRequest
	// mediaRefresh asks the connection's loop, which owns lastMedia, to
	// read and publish Now Playing again, after a command changed it.
	mediaRefresh chan struct{}

	publishMu sync.Mutex

	lastMedia *system.MediaInfo
	mediaSet  bool
	// lastAudio is the audio state of the last state published, which the
	// media player's volume and mute topics repeat.
	lastAudio *system.AudioState

	lastActivity system.Activity
	shortcuts    []string

	// retry cuts short the wait before the next connection attempt, as
	// when the network changes or the Mac wakes.
	retry chan struct{}
	// rediscover asks the connection's loop to publish discovery again, as
	// Home Assistant asks when it starts.
	rediscover chan struct{}
	// discovered is a digest of the discovery last published, which a
	// reconnect does not publish again: the broker retains it. Owned by the
	// goroutine running Run.
	discovered   string
	discoveredAt time.Time
	lastProbe    system.Snapshot
}

type commandRequest struct {
	client broker.Client
	msg    broker.Message
}

// New creates a Manager. Call Run to connect and publish.
func New(opts Options) *Manager {
	if opts.NewClient == nil {
		opts.NewClient = broker.Dial
	}
	if opts.rediscoverHoldoff == 0 {
		opts.rediscoverHoldoff = 10 * time.Second
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = 2 * time.Second
	}
	m := &Manager{
		opts:         opts,
		publish:      make(chan struct{}, 1),
		commands:     make(chan commandRequest, 32),
		mediaRefresh: make(chan struct{}, 1),
		retry:        make(chan struct{}, 1),
		rediscover:   make(chan struct{}, 1),
	}
	m.status = Status{Phase: PhaseIdle, Broker: opts.Config.BrokerAddress()}
	return m
}

// Status returns the latest connection status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Retry connects at once if the service is waiting to reconnect, as when
// the network changed or the Mac woke, rather than after its backoff.
func (m *Manager) Retry() {
	select {
	case m.retry <- struct{}{}:
	default:
	}
}

// PublishNow asks the running service to publish a fresh state document.
func (m *Manager) PublishNow() {
	select {
	case m.publish <- struct{}{}:
	default:
	}
}

// Run connects, publishes discovery and state, and reconnects with exponential
// backoff until ctx is canceled. It is safe to run in a goroutine.
func (m *Manager) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		m.commandWorker(ctx)
	}()
	defer func() { <-workerDone }()

	delay := m.opts.RetryDelay
	for {
		if ctx.Err() != nil {
			m.setPhase(PhaseIdle, "")
			return
		}
		m.setPhase(PhaseConnecting, "")
		client := m.opts.NewClient(m.brokerConfig())
		err := client.Connect(ctx)
		if err == nil {
			if err = m.afterConnect(ctx, client); err != nil {
				m.publishOffline(client)
			}
		}
		if err != nil {
			_ = disconnect(client)
			if ctx.Err() != nil {
				m.setPhase(PhaseIdle, "")
				return
			}
			m.setPhase(PhaseError, friendlyError(err))
			if !m.wait(ctx, delay) {
				m.setPhase(PhaseIdle, "")
				return
			}
			delay = nextDelay(delay)
			continue
		}
		sessionStart := time.Now()
		reconnect := m.runConnected(ctx, client)
		m.publishOffline(client)
		_ = disconnect(client)
		if !reconnect || ctx.Err() != nil {
			m.setPhase(PhaseIdle, "")
			return
		}
		// A connection that lasted resets the backoff: the next attempt
		// comes soon. One dropped again at once backs off further.
		if time.Since(sessionStart) >= time.Minute {
			delay = m.opts.RetryDelay
		}
		if !m.wait(ctx, delay) {
			m.setPhase(PhaseIdle, "")
			return
		}
		delay = nextDelay(delay)
	}
}

// wait waits about d before the next connection attempt, less if Retry is
// called, and reports false if ctx ended. The wait varies by a fifth, so
// that clients a broker restart dropped do not all return at once.
func (m *Manager) wait(ctx context.Context, d time.Duration) bool {
	d = time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-m.retry:
		return true
	case <-timer.C:
		return true
	}
}

// runConnected returns false when Run should stop. Only a lost connection
// ends it: a publish that fails or times out on a slow link is reported,
// and the next one tries again, while the connection lives on.
func (m *Manager) runConnected(ctx context.Context, client broker.Client) bool {
	interval := time.Duration(m.opts.Config.Reporting.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var activityC <-chan time.Time
	if m.opts.Activity != nil {
		activityTicker := time.NewTicker(2 * time.Second)
		defer activityTicker.Stop()
		activityC = activityTicker.C
	}
	var mediaC <-chan time.Time
	if m.mediaEnabled() {
		mediaTicker := time.NewTicker(3 * time.Second)
		defer mediaTicker.Stop()
		mediaC = mediaTicker.C
	}
	report := func(err error) {
		if err != nil && ctx.Err() == nil {
			m.setLastError(friendlyError(err))
		}
	}
	readMedia := func() {
		media := m.opts.Media(ctx)
		m.lastMedia, m.mediaSet = media, true
		report(m.publishMedia(ctx, client, media))
	}
	for {
		select {
		case <-ctx.Done():
			return false
		case <-client.Lost():
			m.setPhase(PhaseConnecting, "connection lost; reconnecting")
			return true
		case <-ticker.C:
			// Discovery that failed to publish is tried again.
			if m.discovered == "" {
				report(m.publishDiscovery(ctx, client, false))
			}
			report(m.publishState(ctx, client))
			if m.mediaEnabled() {
				readMedia()
			}
		case <-m.publish:
			report(m.publishState(ctx, client))
		case <-m.rediscover:
			// Home Assistant started, and lost what it knew; once is enough
			// for the burst of births a restart may send.
			if time.Since(m.discoveredAt) < m.opts.rediscoverHoldoff {
				continue
			}
			report(m.publishDiscovery(ctx, client, true))
			report(m.publishState(ctx, client))
			if m.mediaEnabled() {
				readMedia()
			}
		case <-m.mediaRefresh:
			if m.mediaEnabled() {
				readMedia()
			}
		case <-activityC:
			// A camera or microphone in use, or the display waking, is
			// published at once, for automations to follow.
			activity := m.opts.Activity()
			if activity == m.lastActivity {
				continue
			}
			devices := activity.HasCamera != m.lastActivity.HasCamera || activity.HasMicrophone != m.lastActivity.HasMicrophone
			m.lastActivity = activity
			if devices {
				// A camera or microphone was connected or removed: its
				// entity comes or goes.
				report(m.publishDiscovery(ctx, client, false))
			}
			report(m.publishState(ctx, client))
		case <-mediaC:
			media := m.opts.Media(ctx)
			if m.mediaSet && sameMedia(m.lastMedia, media) {
				continue
			}
			m.lastMedia, m.mediaSet = media, true
			report(m.publishMedia(ctx, client, media))
			report(m.publishState(ctx, client))
		}
	}
}

func (m *Manager) afterConnect(ctx context.Context, client broker.Client) error {
	setupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	probe := system.Snapshot{}
	if m.opts.Snapshot != nil {
		probe = m.opts.Snapshot(setupCtx)
	}
	m.shortcuts = nil
	if m.opts.Shortcuts != nil && m.opts.Config.HomeAssistant.Publishes(homeassistant.GroupShortcuts) {
		m.shortcuts = m.opts.Shortcuts(setupCtx)
	}
	if m.opts.Activity != nil {
		m.lastActivity = m.opts.Activity()
	}
	entities := m.entities(probe)
	var topics []string
	subscribed := map[string]bool{}
	for _, entity := range entities {
		for _, topic := range entity.CommandTopics() {
			if !subscribed[topic] {
				subscribed[topic] = true
				topics = append(topics, topic)
			}
		}
	}
	// Home Assistant announces itself on its status topic as it starts:
	// discovery is then published again.
	topics = append(topics, m.haStatusTopic())
	if err := client.SubscribeAll(setupCtx, topics, 1, func(msg broker.Message) {
		m.handleCommand(client, msg)
	}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	m.lastProbe = probe
	if err := client.Publish(setupCtx, m.opts.Config.AvailabilityTopic(), []byte("online"), true, 1); err != nil {
		return fmt.Errorf("publish availability: %w", err)
	}
	// Home Assistant shows the device available from here; what fails to
	// publish after is reported without leaving this state.
	m.setPhase(PhaseOnline, "")
	// Discovery that does not publish now is tried again with the next
	// report: the connection is up, and commands already work.
	if err := m.publishDiscovery(setupCtx, client, false); err != nil {
		m.setLastError(friendlyError(err))
	}
	// Like discovery, a state that does not publish now is published with
	// the next report, on the same connection.
	if err := m.publishState(setupCtx, client); err != nil {
		m.setLastError(friendlyError(err))
	}
	if m.mediaEnabled() {
		m.lastMedia = probe.Media
		m.mediaSet = true
		if err := m.publishMedia(setupCtx, client, probe.Media); err != nil {
			m.setLastError(friendlyError(err))
		}
	}
	return nil
}

func (m *Manager) haStatusTopic() string {
	return m.opts.Config.HomeAssistant.DiscoveryPrefix + "/status"
}

// publishDiscovery publishes the discovery documents, and clears those of
// what is not published, in one batch: over a slow link, each waiting for
// its acknowledgment could take longer than a connection's setup may. The
// broker retains them, so a reconnect publishes them again only when they
// changed, or force asks, as when Home Assistant starts.
func (m *Manager) publishDiscovery(ctx context.Context, client broker.Client, force bool) error {
	entities := m.entities(m.lastProbe)
	var batch []broker.Publication
	clear := func(topic string) {
		if topic != "" {
			batch = append(batch, broker.Publication{Topic: topic, Retain: true, QoS: 1})
		}
	}
	for _, topic := range m.opts.CleanupTopics {
		clear(topic)
	}
	// Clear the discovery of every entity not published: groups the user
	// turned off, a battery this Mac lacks, and entities of old versions.
	published := make(map[string]bool, len(entities))
	for _, entity := range entities {
		published[entity.Component+"/"+entity.ObjectID] = true
	}
	for _, entity := range append(homeassistant.AllEntities(m.entityOptions(m.lastProbe)), homeassistant.LegacyEntities()...) {
		if !published[entity.Component+"/"+entity.ObjectID] {
			clear(entity.DiscoveryTopic(m.opts.Config.HomeAssistant.DiscoveryPrefix, m.opts.Config.NodeID()))
		}
	}
	if !m.mediaEnabled() {
		stateTopic, attributesTopic := m.mediaTopics()
		clear(stateTopic)
		clear(attributesTopic)
		for _, field := range mediaFields {
			clear(m.opts.Config.HomeAssistant.BaseTopic + "/media/" + field)
		}
	}
	for _, entity := range entities {
		payload, err := json.Marshal(entity.Config)
		if err != nil {
			return fmt.Errorf("encode discovery %s: %w", entity.ObjectID, err)
		}
		topic := entity.DiscoveryTopic(m.opts.Config.HomeAssistant.DiscoveryPrefix, m.opts.Config.NodeID())
		batch = append(batch, broker.Publication{Topic: topic, Payload: payload, Retain: true, QoS: 1})
	}
	digest := sha256.New()
	for _, p := range batch {
		fmt.Fprintf(digest, "%s\x00%s\x00", p.Topic, p.Payload)
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	if !force && sum == m.discovered {
		return nil
	}
	m.discovered = ""
	publishCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.PublishAll(publishCtx, batch); err != nil {
		return fmt.Errorf("publish discovery: %w", err)
	}
	m.discovered, m.discoveredAt = sum, time.Now()
	return nil
}

func (m *Manager) handleCommand(client broker.Client, msg broker.Message) {
	select {
	case m.commands <- commandRequest{client: client, msg: msg}:
	default:
		m.setLastError("command queue is full")
	}
}

// commandWorker executes commands one at a time for the lifetime of Run.
// Serial execution keeps rapid volume changes in order and makes shutdown
// bounded by the context timeout.
func (m *Manager) commandWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-m.commands:
			m.executeCommand(ctx, req)
		}
	}
}

func (m *Manager) executeCommand(ctx context.Context, req commandRequest) {
	commandCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	payload := strings.TrimSpace(string(req.msg.Payload))
	base := m.opts.Config.HomeAssistant.BaseTopic
	var err error
	switch req.msg.Topic {
	case base + "/volume/set":
		volume, parseErr := parseVolume(payload)
		if parseErr != nil {
			err = parseErr
			break
		}
		if m.opts.SetVolume == nil {
			err = fmt.Errorf("volume control is unavailable")
			break
		}
		err = m.opts.SetVolume(commandCtx, volume)
	case base + "/mute/set":
		muted, parseErr := parseMute(payload)
		if parseErr != nil {
			err = parseErr
			break
		}
		if m.opts.SetMuted == nil {
			err = fmt.Errorf("mute control is unavailable")
			break
		}
		err = m.opts.SetMuted(commandCtx, muted)
	case base + "/media/volume/set":
		// The media player's volume, from 0 to 1.
		level, parseErr := strconv.ParseFloat(payload, 64)
		if parseErr != nil || level < 0 || level > 1 {
			err = fmt.Errorf("invalid media volume %q", payload)
			break
		}
		if m.opts.SetVolume == nil {
			err = fmt.Errorf("volume control is unavailable")
			break
		}
		err = m.opts.SetVolume(commandCtx, int(level*100+0.5))
	case base + "/media/mute/set":
		if m.opts.SetMuted == nil {
			err = fmt.Errorf("mute control is unavailable")
			break
		}
		switch strings.ToLower(payload) {
		case "mute":
			err = m.opts.SetMuted(commandCtx, true)
		case "unmute":
			err = m.opts.SetMuted(commandCtx, false)
		default:
			err = fmt.Errorf("invalid media mute %q", payload)
		}
	case base + "/media/command", base + "/media/seek":
		if err = m.mediaControl(commandCtx, req.msg.Topic == base+"/media/seek", payload); err == nil {
			// The app takes a moment to change; then publish what plays.
			if sleep(commandCtx, 300*time.Millisecond) {
				m.refreshMedia()
			}
		}
	case base + "/system/command":
		command, ok := system.ParseSystemCommand(payload)
		switch {
		case !ok:
			err = fmt.Errorf("invalid system command %q", payload)
		case m.opts.SystemCommand == nil:
			err = fmt.Errorf("system control is unavailable")
		default:
			err = m.opts.SystemCommand(commandCtx, command)
		}
	case base + "/notify":
		title, message := parseNotification(payload)
		switch {
		case message == "":
			err = fmt.Errorf("empty notification")
		case m.opts.Notify == nil:
			err = fmt.Errorf("notifications are unavailable")
		default:
			err = m.opts.Notify(commandCtx, title, message)
		}
	case base + "/say":
		if m.opts.Say == nil {
			err = fmt.Errorf("speech is unavailable")
			break
		}
		_, message := parseNotification(payload)
		err = m.opts.Say(commandCtx, message)
	case base + "/shortcut/run":
		name, input := parseShortcut(payload)
		if m.opts.RunShortcut == nil {
			err = fmt.Errorf("shortcuts are unavailable")
			break
		}
		err = m.opts.RunShortcut(commandCtx, name, input)
	case m.haStatusTopic():
		if strings.EqualFold(payload, "online") {
			select {
			case m.rediscover <- struct{}{}:
			default:
			}
		}
		return
	case base + "/refresh":
	// A button command only asks for a fresh report.
	default:
		return
	}
	if err != nil {
		m.setLastError(friendlyError(err))
		return
	}
	if pubErr := m.publishState(commandCtx, req.client); pubErr != nil {
		m.setLastError(friendlyError(pubErr))
		return
	}
	// The media player repeats the volume and mute.
	if strings.Contains(req.msg.Topic, "/volume/") || strings.Contains(req.msg.Topic, "/mute/") {
		m.refreshMedia()
	}
}

// refreshMedia asks the connection's loop to publish Now Playing again.
func (m *Manager) refreshMedia() {
	select {
	case m.mediaRefresh <- struct{}{}:
	default:
	}
}

// parseNotification reads a notification: its text, or JSON with a title
// and a message, as an automation's mqtt.publish may send.
func parseNotification(payload string) (title, message string) {
	var n struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	if strings.HasPrefix(payload, "{") && json.Unmarshal([]byte(payload), &n) == nil {
		return strings.TrimSpace(n.Title), strings.TrimSpace(n.Message)
	}
	return "", strings.TrimSpace(payload)
}

// parseShortcut reads which shortcut to run: its name, or JSON with a name
// and text for its input.
func parseShortcut(payload string) (name, input string) {
	var s struct {
		Name  string `json:"name"`
		Input string `json:"input"`
	}
	if strings.HasPrefix(payload, "{") && json.Unmarshal([]byte(payload), &s) == nil {
		return strings.TrimSpace(s.Name), s.Input
	}
	return strings.TrimSpace(payload), ""
}

// mediaControl sends a command, or a seek, to the Now Playing app.
func (m *Manager) mediaControl(ctx context.Context, seek bool, payload string) error {
	if seek {
		seconds, err := strconv.ParseFloat(payload, 64)
		if err != nil || seconds < 0 {
			return fmt.Errorf("invalid seek %q", payload)
		}
		if m.opts.SeekMedia == nil {
			return fmt.Errorf("media control is unavailable")
		}
		return m.opts.SeekMedia(ctx, seconds)
	}
	command, ok := system.ParseMediaCommand(strings.ToUpper(payload))
	if !ok {
		return fmt.Errorf("invalid media command %q", payload)
	}
	if m.opts.MediaCommand == nil {
		return fmt.Errorf("media control is unavailable")
	}
	return m.opts.MediaCommand(ctx, command)
}

func (m *Manager) mediaTopics() (stateTopic, attributesTopic string) {
	base := m.opts.Config.HomeAssistant.BaseTopic
	return base + "/media/state", base + "/media/attributes"
}

func (m *Manager) publishMedia(ctx context.Context, client broker.Client, media *system.MediaInfo) error {
	stateTopic, attributesTopic := m.mediaTopics()
	state := "idle"
	attributes := map[string]any{}
	// The media player's topics, each holding one field, as the MQTT
	// Media Player integration reads them.
	base := m.opts.Config.HomeAssistant.BaseTopic + "/media/"
	fields := map[string]string{
		"title": "", "artist": "", "album": "", "duration": "0", "position": "0", "mediatype": "music",
	}
	if media != nil {
		if media.State != "" {
			state = media.State
		}
		if media.Title != "" {
			attributes["media_title"] = media.Title
		}
		if media.Artist != "" {
			attributes["media_artist"] = media.Artist
		}
		if media.Album != "" {
			attributes["media_album_name"] = media.Album
		}
		if media.AppName != "" {
			attributes["app_name"] = media.AppName
			attributes["source"] = media.AppName
		}
		if media.DurationSeconds > 0 {
			attributes["media_duration"] = media.DurationSeconds
		}
		if media.PositionSeconds >= 0 {
			attributes["media_position"] = media.PositionSeconds
			attributes["media_position_updated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if media.PlaybackRate != 0 {
			attributes["playback_rate"] = media.PlaybackRate
		}
		fields["title"], fields["artist"], fields["album"] = media.Title, media.Artist, media.Album
		fields["duration"] = strconv.Itoa(int(media.DurationSeconds))
		fields["position"] = strconv.Itoa(int(media.PositionSeconds))
		if isVideoApp(media.AppName) {
			fields["mediatype"] = "video"
		}
	}
	m.publishMu.Lock()
	audio := m.lastAudio
	m.publishMu.Unlock()
	if audio != nil {
		attributes["volume_level"] = float64(audio.Volume) / 100
		attributes["is_volume_muted"] = audio.Muted
		fields["volume"] = strconv.FormatFloat(float64(audio.Volume)/100, 'f', 2, 64)
		fields["mute"] = "unmute"
		if audio.Muted {
			fields["mute"] = "mute"
		}
	}
	payload, err := json.Marshal(attributes)
	if err != nil {
		return fmt.Errorf("encode media attributes: %w", err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	m.publishMu.Lock()
	defer m.publishMu.Unlock()
	retain := m.opts.Config.Reporting.Retain
	batch := []broker.Publication{
		{Topic: stateTopic, Payload: []byte(state), Retain: retain, QoS: 1},
		{Topic: attributesTopic, Payload: payload, Retain: retain, QoS: 1},
	}
	for _, field := range mediaFields {
		if value, ok := fields[field]; ok {
			batch = append(batch, broker.Publication{Topic: base + field, Payload: []byte(value), Retain: retain, QoS: 1})
		}
	}
	if err := client.PublishAll(publishCtx, batch); err != nil {
		return fmt.Errorf("publish media: %w", err)
	}
	return nil
}

// mediaFields are the media player's one-field topics, in publishing order.
var mediaFields = []string{"title", "artist", "album", "duration", "position", "volume", "mute", "mediatype"}

// isVideoApp reports whether what an app plays is most likely video, for
// the media player's media type.
func isVideoApp(app string) bool {
	switch app {
	case "Google Chrome", "Safari", "Arc", "Firefox", "Microsoft Edge", "IINA", "VLC", "QuickTime Player", "TV", "Infuse":
		return true
	}
	return false
}

func sameMedia(a, b *system.MediaInfo) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// A live stream's duration grows as it plays: that alone is no change
	// worth a publish every few seconds.
	return a.State == b.State && a.Title == b.Title && a.Artist == b.Artist &&
		a.Album == b.Album && a.AppName == b.AppName && math.Abs(a.DurationSeconds-b.DurationSeconds) < 5
}

func (m *Manager) publishState(ctx context.Context, client broker.Client) error {
	if m.opts.Snapshot == nil {
		return fmt.Errorf("no status collector configured")
	}
	// Snapshot, marshal and publish as one ordered unit. Otherwise a slow
	// periodic snapshot could overwrite a newer command-triggered state.
	m.publishMu.Lock()
	defer m.publishMu.Unlock()

	snapshot := m.opts.Snapshot(ctx)
	m.lastAudio = snapshot.Audio
	if m.opts.OnState != nil {
		m.opts.OnState(snapshot)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Publish(publishCtx, m.opts.Config.StateTopic(), payload, m.opts.Config.Reporting.Retain, 1); err != nil {
		return fmt.Errorf("publish state: %w", err)
	}
	m.mu.Lock()
	m.status.LastPublish = time.Now()
	m.status.Published++
	m.status.LastError = ""
	status := m.status
	m.mu.Unlock()
	if m.opts.OnStatus != nil {
		m.opts.OnStatus(status)
	}
	return nil
}

func (m *Manager) publishOffline(client broker.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.publishMu.Lock()
	defer m.publishMu.Unlock()
	_ = client.Publish(ctx, m.opts.Config.AvailabilityTopic(), []byte("offline"), true, 1)
}

func (m *Manager) entities(probe system.Snapshot) []homeassistant.Entity {
	return homeassistant.SystemEntities(m.entityOptions(probe))
}

// entityOptions describes this Mac to discovery: its model and macOS
// version in the device registry, and the entities its hardware has.
func (m *Manager) entityOptions(probe system.Snapshot) homeassistant.Options {
	cfg := m.opts.Config
	model := probe.Device.Model
	if model == "" {
		model = "Mac"
	}
	swVersion := ""
	if v := probe.Device.OSVersion; v != "" && v != "darwin" {
		swVersion = "macOS " + v
	}
	return homeassistant.Options{
		BaseTopic:         cfg.HomeAssistant.BaseTopic,
		StateTopic:        cfg.StateTopic(),
		AvailabilityTopic: cfg.AvailabilityTopic(),
		Device: homeassistant.Device{
			ID:           cfg.NodeID(),
			Name:         cfg.HomeAssistant.DeviceName,
			Manufacturer: "Apple",
			Model:        model,
			ModelID:      probe.Device.ModelID,
			SWVersion:    swVersion,
		},
		HasBattery: probe.Power.BatteryPercent != nil || probe.Power.Charging != nil,
		Disabled:   cfg.HomeAssistant.Disabled(),
		Shortcuts:  m.shortcuts,
		// What the Mac has now, as a camera plugged in after launch.
		HasCamera:     m.lastActivity.HasCamera,
		HasMicrophone: m.lastActivity.HasMicrophone,
	}
}

// mediaEnabled reports whether Now Playing is read and published.
func (m *Manager) mediaEnabled() bool {
	return m.opts.Media != nil && m.opts.Config.HomeAssistant.Publishes(homeassistant.GroupMedia)
}

func (m *Manager) brokerConfig() broker.Config {
	cfg := m.opts.Config
	return broker.Config{
		Host:               cfg.Broker.Host,
		Port:               cfg.Broker.Port,
		TLS:                cfg.Broker.TLS,
		InsecureSkipVerify: cfg.Broker.InsecureSkipVerify,
		Username:           cfg.Broker.Username,
		Password:           cfg.Broker.Password,
		ClientID:           cfg.Broker.ClientID,
		KeepAlive:          60 * time.Second,
		ConnectTimeout:     30 * time.Second,
		WillTopic:          cfg.AvailabilityTopic(),
		WillPayload:        "offline",
		WillRetain:         true,
	}
}

func (m *Manager) setPhase(phase Phase, message string) {
	m.mu.Lock()
	m.status.Phase = phase
	if message != "" {
		m.status.LastError = message
	} else if phase == PhaseOnline {
		m.status.LastError = ""
	}
	status := m.status
	m.mu.Unlock()
	if m.opts.OnStatus != nil {
		m.opts.OnStatus(status)
	}
}

func (m *Manager) setLastError(message string) {
	m.mu.Lock()
	m.status.LastError = message
	status := m.status
	m.mu.Unlock()
	if m.opts.OnStatus != nil {
		m.opts.OnStatus(status)
	}
}

func disconnect(client broker.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return client.Disconnect(ctx)
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextDelay(d time.Duration) time.Duration {
	d *= 2
	if d > time.Minute {
		return time.Minute
	}
	return d
}

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func parseVolume(value string) (int, error) {
	value = strings.TrimSpace(value)
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid volume command %q", value)
	}
	if n < 0 || n > 100 {
		return 0, fmt.Errorf("volume %d is outside 0..100", n)
	}
	return n, nil
}

func parseMute(value string) (bool, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ON", "TRUE", "1":
		return true, nil
	case "OFF", "FALSE", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid mute command %q", value)
	}
}
