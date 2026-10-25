// Package service owns the MQTT connection, Home Assistant discovery and the
// periodic publishing of Mac2MQTT's state document.
package service

import (
	"context"
	"encoding/json"
	"fmt"
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
}

// Manager runs one MQTT lifecycle at a time. Run blocks until ctx is canceled
// or the manager gives up after an unrecoverable local error.
type Manager struct {
	opts Options

	mu       sync.Mutex
	status   Status
	publish  chan struct{}
	commands chan commandRequest

	publishMu sync.Mutex

	lastMedia *system.MediaInfo
	mediaSet  bool
	// lastAudio is the audio state of the last state published, which the
	// media player's volume and mute topics repeat.
	lastAudio *system.AudioState

	lastActivity system.Activity
	shortcuts    []string
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
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = 2 * time.Second
	}
	m := &Manager{
		opts:     opts,
		publish:  make(chan struct{}, 1),
		commands: make(chan commandRequest, 32),
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
		if err := client.Connect(ctx); err != nil {
			_ = disconnect(client)
			if ctx.Err() != nil {
				m.setPhase(PhaseIdle, "")
				return
			}
			m.setPhase(PhaseError, friendlyError(err))
			if sleep(ctx, delay) == false {
				m.setPhase(PhaseIdle, "")
				return
			}
			delay = nextDelay(delay)
			continue
		}
		if err := m.afterConnect(ctx, client); err != nil {
			m.setPhase(PhaseError, friendlyError(err))
			m.publishOffline(client)
			_ = disconnect(client)
			if sleep(ctx, delay) == false {
				m.setPhase(PhaseIdle, "")
				return
			}
			delay = nextDelay(delay)
			continue
		}
		m.setPhase(PhaseOnline, "")
		sessionStart := time.Now()
		reconnect := m.runConnected(ctx, client)
		m.publishOffline(client)
		_ = disconnect(client)
		if reconnect == false {
			m.setPhase(PhaseIdle, "")
			return
		}
		if ctx.Err() != nil {
			m.setPhase(PhaseIdle, "")
			return
		}
		if time.Since(sessionStart) >= time.Minute {
			delay = m.opts.RetryDelay
		}
		if sleep(ctx, delay) == false {
			m.setPhase(PhaseIdle, "")
			return
		}
		delay = nextDelay(delay)
	}
}

// runConnected returns false when Run should stop.
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
	for {
		select {
		case <-ctx.Done():
			return false
		case <-client.Lost():
			m.setPhase(PhaseConnecting, "connection lost; reconnecting")
			return true
		case <-ticker.C:
			if err := m.publishState(ctx, client); err != nil {
				m.setPhase(PhaseError, friendlyError(err))
				return true
			}
			if m.mediaEnabled() {
				media := m.opts.Media(ctx)
				m.lastMedia, m.mediaSet = media, true
				if err := m.publishMedia(ctx, client, media); err != nil {
					m.setPhase(PhaseError, friendlyError(err))
					return true
				}
			}
		case <-m.publish:
			if err := m.publishState(ctx, client); err != nil {
				m.setPhase(PhaseError, friendlyError(err))
				return true
			}
		case <-activityC:
			// A camera or microphone in use, or the display waking, is
			// published at once, for automations to follow.
			activity := m.opts.Activity()
			if activity == m.lastActivity {
				continue
			}
			m.lastActivity = activity
			if err := m.publishState(ctx, client); err != nil {
				m.setPhase(PhaseError, friendlyError(err))
				return true
			}
		case <-mediaC:
			media := m.opts.Media(ctx)
			if m.mediaSet && sameMedia(m.lastMedia, media) {
				continue
			}
			m.lastMedia, m.mediaSet = media, true
			if err := m.publishMedia(ctx, client, media); err != nil {
				m.setPhase(PhaseError, friendlyError(err))
				return true
			}
			if err := m.publishState(ctx, client); err != nil {
				m.setPhase(PhaseError, friendlyError(err))
				return true
			}
		}
	}
}

func (m *Manager) afterConnect(ctx context.Context, client broker.Client) error {
	setupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
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
	for _, topic := range topics {
		topic := topic
		if err := client.Subscribe(setupCtx, topic, 1, func(msg broker.Message) {
			m.handleCommand(client, msg)
		}); err != nil {
			return fmt.Errorf("subscribe %s: %w", topic, err)
		}
	}
	for _, topic := range m.opts.CleanupTopics {
		if topic == "" {
			continue
		}
		if err := client.Publish(setupCtx, topic, nil, true, 1); err != nil {
			return fmt.Errorf("clear retained topic %s: %w", topic, err)
		}
	}
	// Clear the discovery of every entity not published: groups the user
	// turned off, a battery this Mac lacks, and entities of old versions.
	published := make(map[string]bool, len(entities))
	for _, entity := range entities {
		published[entity.Component+"/"+entity.ObjectID] = true
	}
	for _, entity := range append(homeassistant.AllEntities(m.entityOptions(probe)), homeassistant.LegacyEntities()...) {
		if published[entity.Component+"/"+entity.ObjectID] {
			continue
		}
		topic := entity.DiscoveryTopic(m.opts.Config.HomeAssistant.DiscoveryPrefix, m.opts.Config.NodeID())
		if err := client.Publish(setupCtx, topic, nil, true, 1); err != nil {
			return fmt.Errorf("clear discovery %s: %w", topic, err)
		}
	}
	if m.mediaEnabled() == false {
		stateTopic, attributesTopic := m.mediaTopics()
		cleared := []string{stateTopic, attributesTopic}
		for _, field := range mediaFields {
			cleared = append(cleared, m.opts.Config.HomeAssistant.BaseTopic+"/media/"+field)
		}
		for _, topic := range cleared {
			if err := client.Publish(setupCtx, topic, nil, true, 1); err != nil {
				return fmt.Errorf("clear %s: %w", topic, err)
			}
		}
	}
	if err := client.Publish(setupCtx, m.opts.Config.AvailabilityTopic(), []byte("online"), true, 1); err != nil {
		return fmt.Errorf("publish availability: %w", err)
	}
	for _, entity := range entities {
		payload, err := json.Marshal(entity.Config)
		if err != nil {
			return fmt.Errorf("encode discovery %s: %w", entity.ObjectID, err)
		}
		topic := entity.DiscoveryTopic(m.opts.Config.HomeAssistant.DiscoveryPrefix, m.opts.Config.NodeID())
		if err := client.Publish(setupCtx, topic, payload, true, 1); err != nil {
			return fmt.Errorf("publish discovery %s: %w", topic, err)
		}
	}
	if err := m.publishState(setupCtx, client); err != nil {
		return err
	}
	if m.mediaEnabled() {
		m.lastMedia = probe.Media
		m.mediaSet = true
		return m.publishMedia(setupCtx, client, probe.Media)
	}
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
			if sleep(commandCtx, 300*time.Millisecond) && m.mediaEnabled() {
				media := m.opts.Media(commandCtx)
				m.lastMedia, m.mediaSet = media, true
				if pubErr := m.publishMedia(commandCtx, req.client, media); pubErr != nil {
					m.setLastError(friendlyError(pubErr))
				}
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
	if m.mediaEnabled() && (strings.Contains(req.msg.Topic, "/volume/") || strings.Contains(req.msg.Topic, "/mute/")) {
		if pubErr := m.publishMedia(commandCtx, req.client, m.lastMedia); pubErr != nil {
			m.setLastError(friendlyError(pubErr))
		}
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
	if err := client.Publish(publishCtx, stateTopic, []byte(state), retain, 1); err != nil {
		return fmt.Errorf("publish media state: %w", err)
	}
	if err := client.Publish(publishCtx, attributesTopic, payload, retain, 1); err != nil {
		return fmt.Errorf("publish media attributes: %w", err)
	}
	for _, field := range mediaFields {
		value, ok := fields[field]
		if !ok {
			continue
		}
		if err := client.Publish(publishCtx, base+field, []byte(value), retain, 1); err != nil {
			return fmt.Errorf("publish media %s: %w", field, err)
		}
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
	return a.State == b.State && a.Title == b.Title && a.Artist == b.Artist &&
		a.Album == b.Album && a.AppName == b.AppName && a.DurationSeconds == b.DurationSeconds
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
		KeepAlive:          30 * time.Second,
		ConnectTimeout:     10 * time.Second,
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
