package main

import (
	"bytes"
	"context"
	"image/png"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/config"
	"mac2mqtt/internal/homeassistant"
	"mac2mqtt/internal/service"
	"mac2mqtt/internal/system"
	"mac2mqtt/locales"
)

const fallbackVersion = "0.1.0"

type app struct {
	version string

	cfgPath string
	cfg     config.Config
	draft   config.Config
	// Numeric fields edit float64 values while the user types.
	editPort     float64
	editInterval float64

	win    *mygo.Window
	tray   *mygo.Tray
	tab    int
	latest system.Snapshot

	mqttStatus    service.Status
	settingsError string
	configError   string
	quitting      atomic.Bool

	// publishing mirrors the published entity groups, in entityGroups'
	// order, for the switches that turn them on and off.
	publishing []bool
	// skipMedia is true while Now Playing is not published, so that
	// snapshots, on the service's goroutine, do not read it.
	skipMedia atomic.Bool

	openAtLogin        bool
	loginNeedsApproval bool
	loginError         string
	trayPhase          service.Phase

	// appIcons are the icons of the apps that played, as the Now Playing
	// card shows them; a nil entry is one being fetched or missing.
	appIcons map[string]*ui.Bitmap

	uiMu       sync.Mutex
	uiQueue    []func()
	uiRunning  bool
	copiedYAML bool

	// The updater's preferences, as the Settings switches show them.
	autoUpdateChecks    bool
	autoUpdateDownloads bool
	lastUpdateCheck     time.Time

	collector  *system.Collector
	controller *system.Controller

	runtimeOnline    atomic.Bool
	runtimeMetered   atomic.Bool
	runtimeOnBattery atomic.Bool
	runtimeIdleNanos atomic.Int64
	runtimeCancel    context.CancelFunc

	svcMu  sync.Mutex
	svcGen uint64
	svc    *service.Manager
	svcRun *serviceRun
}

type serviceRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func newApp() *app {
	v := mygo.App.Version()
	if v == "" {
		v = fallbackVersion
	}
	return &app{version: v}
}

func (a *app) ready() {
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("Mac2MQTT: user data directory: %v", err)
		userData = "."
	}
	a.cfgPath = filepath.Join(userData, "config.json")

	cfg, exists, loadErr := config.Load(a.cfgPath)
	a.applyConfig(cfg)
	// The app's language follows the system unless the user chose one.
	mygo.App.SetLocale(a.cfg.UI.Language)

	if loadErr != nil {
		a.configError = loadErr.Error()
		a.settingsError = locales.ErrorReadConfig(a.cfgPath, loadErr.Error())
	} else if a.cfg.Broker.Host == "" {
		a.configError = locales.ErrorNotConfigured()
		a.settingsError = locales.ErrorEnterHost()
	}
	a.collector = system.NewCollector(a.version)
	a.controller = system.NewController()

	a.readOpenAtLogin()
	a.readUpdatePrefs()
	updater.OnChange(func() { a.ui(a.readUpdatePrefs) })
	a.createTray()
	a.updateTray()
	a.installMenu()
	a.registerLifecycle()
	mygo.App.OnLocaleChanged(func(string) {
		if a.win != nil {
			a.win.SetTitle(locales.AppName())
		}
		a.installMenu()
		a.updateTray()
		// Home Assistant shows the entities' names in the new language.
		a.restartService()
	})

	// The window opens on the first run, to set up; otherwise the app
	// starts in the menu bar, with no window to keep in memory.
	if exists == false || a.cfg.Broker.Host == "" || loadErr != nil {
		a.tab = tabSettings
		a.showWindow()
	}
	a.captureRuntime()
	a.startRuntimePoller()
	a.refreshSnapshot()
	a.restartService()
}

func (a *app) applyConfig(cfg config.Config) {
	cfg.Normalize()
	a.cfg = cfg
	a.draft = cfg
	a.editPort = float64(cfg.Broker.Port)
	a.editInterval = float64(cfg.Reporting.IntervalSeconds)
	a.publishing = make([]bool, len(entityGroups))
	for i, g := range entityGroups {
		a.publishing[i] = cfg.HomeAssistant.Publishes(g.id)
	}
	a.skipMedia.Store(!cfg.HomeAssistant.Publishes(homeassistant.GroupMedia))
}

// saveConfig saves the settings in effect, for those that apply at once.
func (a *app) saveConfig() {
	if a.cfgPath == "" {
		return
	}
	if err := a.cfg.Save(a.cfgPath); err != nil {
		a.settingsError = err.Error()
	}
}

// setPublishes publishes an entity group or stops publishing it, at once:
// the service reconnects, publishing the discovery of what is on and
// clearing that of what is off.
func (a *app) setPublishes(group string, on bool) {
	a.cfg.HomeAssistant.SetPublishes(group, on)
	a.draft.HomeAssistant.DisabledGroups = a.cfg.HomeAssistant.DisabledGroups
	a.skipMedia.Store(!a.cfg.HomeAssistant.Publishes(homeassistant.GroupMedia))
	if a.cfg.Validate() == nil {
		a.saveConfig()
	}
	a.refreshSnapshot()
	a.restartService()
}

// setOpenAtLogin makes the app a login item, or stops it being one. On
// macOS 13 and later the user may have to allow it in System Settings,
// which MyGo reports as an error naming Login Items.
func (a *app) setOpenAtLogin(open bool) {
	err := mygo.App.SetOpenAtLogin(open)
	a.loginError = ""
	a.loginNeedsApproval = open && err != nil && strings.Contains(err.Error(), "Login Items")
	if err != nil && !a.loginNeedsApproval {
		a.loginError = locales.ErrorOpenAtLogin(err.Error())
	}
	a.openAtLogin = mygo.App.OpenAtLogin() || a.loginNeedsApproval
}

// readOpenAtLogin reads whether the app opens at login, as the user may
// allow or remove it in System Settings while the app runs.
func (a *app) readOpenAtLogin() {
	if mygo.App.OpenAtLogin() {
		a.openAtLogin, a.loginNeedsApproval = true, false
	} else if !a.loginNeedsApproval {
		a.openAtLogin = false
	}
}

func (a *app) createWindow() {
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:         locales.AppName(),
		Width:         820,
		Height:        660,
		MinWidth:      720,
		MinHeight:     520,
		StateKey:      "main-v3",
		TitleBarStyle: mygo.TitleBarHidden,
		// Inside the sidebar's glass, clear of its rounded corner, as
		// macOS 26 places them in a window with a floating sidebar.
		TrafficLightPosition: &mygo.Point{X: sidebarInset + 12, Y: sidebarInset + 12},
		DisableMinimize:      false,
		DisableFullScreen:    true,
		Content:              ui.View(a.view),
	})
	// Closing the window lets it go, with what it holds on the GPU, which
	// is most of the app's memory: opening it again makes a new one.
	win := a.win
	win.OnClosed(func() {
		if a.win == win {
			a.win = nil
		}
	})
}

// ui runs fn on the main thread, where the app's state lives, then redraws
// the window if one is open. It returns at once, and runs fns in the order
// they came: the MQTT service reports its status through it, and must not
// wait for the main thread, which may be waiting for it to stop.
func (a *app) ui(fn func()) {
	a.uiMu.Lock()
	a.uiQueue = append(a.uiQueue, fn)
	start := !a.uiRunning
	a.uiRunning = true
	a.uiMu.Unlock()
	if start {
		go a.runUI()
	}
}

func (a *app) runUI() {
	for {
		a.uiMu.Lock()
		if len(a.uiQueue) == 0 {
			a.uiRunning = false
			a.uiMu.Unlock()
			return
		}
		fns := a.uiQueue
		a.uiQueue = nil
		a.uiMu.Unlock()
		mygo.RunOnMain(func() {
			for _, fn := range fns {
				fn()
			}
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}
}

func (a *app) createTray() {
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		ID:             "mac2mqtt",
		Icon:           trayIcon(false),
		IconIsTemplate: true,
		ToolTip:        locales.AppName(),
	})
	if err != nil {
		log.Printf("Mac2MQTT: tray: %v", err)
		return
	}
	// Either click opens the menu, as a menu bar extra's does; the menu is
	// built as it opens, so it shows the current state.
	popUp := func() {
		a.tray.SetMenu(a.trayMenu())
		defer a.tray.SetMenu(nil)
		a.tray.PopUpMenu()
	}
	a.tray.OnClick(popUp)
	a.tray.OnRightClick(popUp)
}

func (a *app) trayMenu() *mygo.Menu {
	st := a.status()
	configured := strings.TrimSpace(a.cfg.Broker.Host) != ""
	// The status and what plays read as items, not grayed out, and open
	// the window, where they show in full.
	open := func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }
	status := "● " + st.label
	if configured {
		status += " · " + a.cfg.BrokerAddress()
	}
	items := []*mygo.MenuItem{{Label: status, Click: open}}
	if m := a.latest.Media; m != nil {
		label := "♪ " + m.Title
		if m.Artist != "" {
			label += " — " + m.Artist
		}
		items = append(items, &mygo.MenuItem{Label: truncate(label, 48), Click: open})
	}
	pause := locales.MenuPauseReporting()
	if !a.cfg.Reporting.Enabled {
		pause = locales.MenuResumeReporting()
	}
	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{Label: locales.MenuOpenApp(), Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		&mygo.MenuItem{Label: locales.MenuPublishNow(), Disabled: st.phase != service.PhaseOnline, Click: func(*mygo.MenuItem, *mygo.Window) { a.publishNow() }},
		&mygo.MenuItem{Label: locales.MenuReconnect(), Disabled: !configured || !a.cfg.Reporting.Enabled, Click: func(*mygo.MenuItem, *mygo.Window) { a.restartService() }},
		&mygo.MenuItem{Label: pause, Disabled: !configured, Click: func(*mygo.MenuItem, *mygo.Window) { a.setReporting(!a.cfg.Reporting.Enabled) }},
		mygo.Separator(),
		&mygo.MenuItem{Label: locales.OpenAtLoginLabel(), Type: mygo.MenuItemCheckbox, Checked: a.openAtLogin, Click: func(item *mygo.MenuItem, _ *mygo.Window) {
			a.setOpenAtLogin(item.Checked)
			if a.loginNeedsApproval {
				openLoginItems()
			}
		}},
		&mygo.MenuItem{Label: locales.MenuSettings(), Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
		updater.MenuItem(),
		mygo.Separator(),
		&mygo.MenuItem{Role: mygo.RoleQuit, Label: locales.QuitApp()},
	)
	return mygo.NewMenu(items)
}

// setReporting pauses or resumes reporting from the tray, saving it.
func (a *app) setReporting(on bool) {
	a.cfg.Reporting.Enabled = on
	a.draft.Reporting.Enabled = on
	if a.cfg.Validate() != nil {
		a.cfg.Reporting.Enabled = !on
		a.draft.Reporting.Enabled = !on
		return
	}
	a.saveConfig()
	a.restartService()
}

// updateTray shows in the menu bar whether reports reach the broker: the
// icon dims while they do not.
func (a *app) updateTray() {
	if a.tray == nil {
		return
	}
	st := a.status()
	if st.phase != a.trayPhase {
		a.trayPhase = st.phase
		_ = a.tray.SetIcon(trayIcon(st.phase != service.PhaseOnline), true)
	}
	a.tray.SetToolTip(locales.AppName() + " — " + st.label)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (a *app) installMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleEditMenu},
		{Label: locales.AppName(), Submenu: []*mygo.MenuItem{
			{Label: locales.MenuShowWindow(), Accelerator: "CmdOrCtrl+Shift+M", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			{Label: locales.MenuPublishNow(), Accelerator: "CmdOrCtrl+Shift+P", Click: func(*mygo.MenuItem, *mygo.Window) { a.publishNow() }},
			{Label: locales.MenuReconnect(), Click: func(*mygo.MenuItem, *mygo.Window) { a.restartService() }},
			mygo.Separator(),
			{Label: locales.MenuSettings(), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
			updater.MenuItem(),
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}

func (a *app) registerLifecycle() {
	mygo.Power.OnLockScreen(func() {
		a.collector.SetLocked(true)
		a.captureRuntime()
		a.refreshSnapshot()
		a.publishNow()
	})
	mygo.Power.OnUnlockScreen(func() {
		a.collector.SetLocked(false)
		a.captureRuntime()
		a.refreshSnapshot()
		a.publishNow()
	})
	// Waking and a network coming back reconnect at once, rather than
	// after the backoff the dropped connection was waiting out.
	mygo.Power.OnResume(func() {
		a.captureRuntime()
		a.refreshSnapshot()
		a.retryNow()
		a.publishNow()
	})
	mygo.Network.OnChanged(func(mygo.NetworkStatus) {
		a.captureRuntime()
		a.refreshSnapshot()
		a.retryNow()
		a.publishNow()
	})
}

func (a *app) showWindow() {
	if a.win == nil {
		a.createWindow()
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.readOpenAtLogin()
	a.win.Show()
	a.win.Focus()
}

func (a *app) showSettings() {
	a.tab = tabSettings
	a.showWindow()
}

func (a *app) toggleWindow() {
	if a.win == nil {
		return
	}
	if a.win.IsVisible() {
		a.win.Close()
	} else {
		a.showWindow()
	}
}

// readUpdatePrefs reads the updater's preferences for the Settings page.
func (a *app) readUpdatePrefs() {
	a.autoUpdateChecks = updater.AutomaticChecks()
	a.autoUpdateDownloads = updater.AutomaticDownloads()
	a.lastUpdateCheck = updater.LastCheck()
}

// notify shows a notification from Home Assistant.
func notify(_ context.Context, title, message string) error {
	return mygo.NewNotification(mygo.NotificationOptions{
		Title: displayOr(title, "Home Assistant"),
		Body:  message,
	}).Show()
}

// mediaControl sends a command to the Now Playing app from the window,
// then shows and publishes what plays once it has changed.
func (a *app) mediaControl(command system.MediaCommand) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.controller.SendMediaCommand(ctx, command); err != nil {
			log.Printf("Mac2MQTT: %v", err)
			return
		}
		time.Sleep(300 * time.Millisecond)
		a.refreshSnapshot()
		a.publishNow()
	}()
}

func (a *app) retryNow() {
	a.svcMu.Lock()
	svc := a.svc
	a.svcMu.Unlock()
	if svc != nil {
		svc.Retry()
	}
}

func (a *app) publishNow() {
	a.svcMu.Lock()
	svc := a.svc
	a.svcMu.Unlock()
	if svc != nil {
		svc.PublishNow()
	}
}

func (a *app) restartService() {
	a.restartServiceWithCleanup(nil)
}

func (a *app) restartServiceWithCleanup(cleanup []string) {
	cfg := a.cfg
	go a.restartServiceNow(cfg, cleanup)
}

func (a *app) restartServiceNow(cfg config.Config, cleanup []string) {
	a.svcMu.Lock()
	a.svcGen++
	gen := a.svcGen
	old := a.svcRun
	a.svcRun = nil
	a.svc = nil
	a.svcMu.Unlock()

	if old != nil {
		old.cancel()
		<-old.done
	}
	if a.quitting.Load() || cfg.Reporting.Enabled == false || strings.TrimSpace(cfg.Broker.Host) == "" {
		if a.quitting.Load() == false {
			a.setStatus(service.Status{Phase: service.PhaseIdle, Broker: cfg.BrokerAddress()})
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &serviceRun{cancel: cancel, done: make(chan struct{})}
	mgr := service.New(service.Options{
		Config:        cfg,
		Version:       a.version,
		CleanupTopics: cleanup,
		EntityNames:   entityNames(),
		Snapshot:      a.snapshot,
		OnStatus:      a.setStatus,
		OnState: func(s system.Snapshot) {
			a.ui(func() { a.latest = s })
		},
		SetVolume: func(ctx context.Context, volume int) error {
			return a.controller.SetVolume(ctx, volume)
		},
		SetMuted: func(ctx context.Context, muted bool) error {
			return a.controller.SetMuted(ctx, muted)
		},
		Media:         a.collector.Media,
		MediaCommand:  a.controller.SendMediaCommand,
		SeekMedia:     a.controller.SeekMedia,
		Activity:      a.collector.Activity,
		SystemCommand: a.controller.RunSystemCommand,
		Notify:        notify,
		Say:           a.controller.Say,
		Shortcuts: func(ctx context.Context) []string {
			names, err := a.controller.Shortcuts(ctx)
			if err != nil {
				log.Printf("Mac2MQTT: %v", err)
			}
			return names
		},
		RunShortcut: a.controller.RunShortcut,
	})

	a.svcMu.Lock()
	if a.svcGen != gen || a.quitting.Load() {
		a.svcMu.Unlock()
		cancel()
		close(run.done)
		return
	}
	a.svc = mgr
	a.svcRun = run
	a.svcMu.Unlock()

	go func() {
		defer close(run.done)
		mgr.Run(ctx)
	}()
}

func (a *app) captureRuntime() {
	network := mygo.Network.Status()
	a.runtimeOnline.Store(network.Online)
	a.runtimeMetered.Store(network.Metered)
	a.runtimeOnBattery.Store(mygo.Power.IsOnBattery())
	a.runtimeIdleNanos.Store(int64(mygo.Power.IdleTime()))
}

func (a *app) startRuntimePoller() {
	ctx, cancel := context.WithCancel(context.Background())
	a.runtimeCancel = cancel
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.captureRuntime()
			}
		}
	}()
}

func (a *app) snapshot(ctx context.Context) system.Snapshot {
	// Read only cached main-thread values here: the MQTT service may call this
	// while the UI thread is shutting down or waiting for the service.
	return a.collector.Snapshot(ctx, system.Runtime{
		NetworkOnline:  a.runtimeOnline.Load(),
		NetworkMetered: a.runtimeMetered.Load(),
		OnBattery:      a.runtimeOnBattery.Load(),
		Idle:           time.Duration(a.runtimeIdleNanos.Load()),
		SkipMedia:      a.skipMedia.Load(),
	})
}

func (a *app) refreshSnapshot() {
	if a.collector == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		s := a.snapshot(ctx)
		a.ui(func() { a.latest = s })
	}()
}

func (a *app) setStatus(s service.Status) {
	a.ui(func() {
		a.mqttStatus = s
		a.updateTray()
	})
}

func (a *app) shutdown() {
	a.quitting.Store(true)
	if a.runtimeCancel != nil {
		a.runtimeCancel()
	}
	if a.collector != nil {
		a.collector.Close()
	}
	a.svcMu.Lock()
	run := a.svcRun
	a.svcRun = nil
	a.svc = nil
	a.svcMu.Unlock()
	if run != nil {
		run.cancel()
		select {
		case <-run.done:
		case <-time.After(2 * time.Second):
			log.Printf("Mac2MQTT: MQTT shutdown timed out")
		}
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
}

func (a *app) saveSettings() {
	d := a.draft
	d.Broker.Port = int(a.editPort)
	d.Reporting.IntervalSeconds = int(a.editInterval)
	d.Normalize()
	if err := d.Validate(); err != nil {
		a.settingsError = err.Error()
		return
	}
	if err := d.Save(a.cfgPath); err != nil {
		a.settingsError = err.Error()
		return
	}
	cleanup := cleanupTopics(a.cfg, d)
	a.applyConfig(d)
	a.settingsError = ""
	a.configError = ""
	a.tab = tabOverview
	a.refreshSnapshot()
	a.restartServiceWithCleanup(cleanup)
}

// revertDraft discards unsaved settings.
func (a *app) revertDraft() {
	a.applyConfig(a.cfg)
	a.settingsError = ""
}

// cleanupTopics lists retained topics of the previous configuration that the
// new configuration no longer owns. The MQTT service clears them with empty
// retained publishes after reconnecting.
func cleanupTopics(old, next config.Config) []string {
	if strings.TrimSpace(old.Broker.Host) == "" || strings.TrimSpace(old.HomeAssistant.BaseTopic) == "" {
		return nil
	}
	var topics []string
	discoveryChanged := old.HomeAssistant.DiscoveryPrefix != next.HomeAssistant.DiscoveryPrefix || old.NodeID() != next.NodeID()
	if discoveryChanged {
		entities := homeassistant.SystemEntities(homeassistant.Options{
			BaseTopic:         old.HomeAssistant.BaseTopic,
			StateTopic:        old.StateTopic(),
			AvailabilityTopic: old.AvailabilityTopic(),
			Device: homeassistant.Device{
				ID:   old.NodeID(),
				Name: old.HomeAssistant.DeviceName,
			},
			HasBattery: true,
		})
		entities = append(entities, homeassistant.LegacyEntities()...)
		for _, entity := range entities {
			topics = append(topics, entity.DiscoveryTopic(old.HomeAssistant.DiscoveryPrefix, old.NodeID()))
		}
	}
	if old.HomeAssistant.BaseTopic != next.HomeAssistant.BaseTopic {
		if old.Reporting.Retain {
			base := strings.TrimRight(old.HomeAssistant.BaseTopic, "/")
			topics = append(topics, old.StateTopic(), base+"/media/state", base+"/media/attributes")
			for _, field := range []string{"title", "artist", "album", "duration", "position", "volume", "mute", "mediatype", "next/available", "previous/available"} {
				topics = append(topics, base+"/media/"+field)
			}
		}
		topics = append(topics, old.AvailabilityTopic())
	} else if old.Reporting.Retain && next.Reporting.Retain == false {
		topics = append(topics, old.StateTopic())
	}
	seen := make(map[string]bool, len(topics))
	out := topics[:0]
	for _, topic := range topics {
		if topic != "" && seen[topic] == false {
			seen[topic] = true
			out = append(out, topic)
		}
	}
	return out
}

// trayGlyph is the app icon's house and signal, as a menu bar glyph:
// black, which macOS tints as the menu bar wants it (a template image).
var trayGlyph = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-linejoin="round" stroke-linecap="round">
<path d="M12 3.6 L20.2 10.4 L20.2 20.2 L3.8 20.2 L3.8 10.4 Z" stroke-width="1.9"/>
<path d="M9.3 13.9 A3.8 3.8 0 0 1 14.7 13.9" stroke-width="1.8"/>
<circle cx="12" cy="16.6" r="1.25" fill="#000" stroke="none"/>
</svg>`))

// trayIcon draws the menu bar's glyph, 18 points at 2×, at a third of its
// opacity when dimmed.
func trayIcon(dimmed bool) []byte {
	opacity := float32(1)
	if dimmed {
		opacity = 0.35
	}
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Image(c, trayGlyph).Size(18, 18).Opacity(opacity)
	}, 18, 18, 2)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
