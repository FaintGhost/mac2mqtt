package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/homeassistant"
	"mac2mqtt/internal/service"
	"mac2mqtt/internal/system"
	"mac2mqtt/locales"
)

func (a *app) overviewPage(c *ui.Context, t *ui.Theme, p palette) {
	configured := strings.TrimSpace(a.cfg.Broker.Host) != ""
	var actions func()
	if configured {
		actions = func() {
			online := a.mqttStatus.Phase == service.PhaseOnline && a.cfg.Reporting.Enabled
			glassButton(c, iconRefresh, locales.Reconnect(), false).OnClick(a.restartService)
			glassButton(c, iconSend, locales.PublishNow(), true).Disabled(!online).OnClick(a.publishNow)
		}
	}
	a.page(c, t, p, locales.PageOverviewTitle(), 0, actions, func() {
		if !configured {
			a.onboarding(c, t, p)
			return
		}
		a.statusCard(c, t, p)
		a.nowPlayingCard(c, t, p)
		a.stateTiles(c, t, p)
	})
}

func (a *app) onboarding(c *ui.Context, t *ui.Theme, p palette) {
	group(c, p).Padding(28, 24).Gap(16).AlignItems(ui.Center).Children(func() {
		if appIcon != nil {
			ui.Image(c, appIcon).Size(72, 72)
		}
		ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.OnboardingTitle()).FontSize(20).FontWeight(700).TextAlign(ui.Center).Wrap()
			ui.Text(c, locales.OnboardingBody()).FontSize(13).TextColor(t.TextMuted).TextAlign(ui.Center).Wrap().MaxWidth(420)
		})
		ui.Row(c).Gap(10).Children(func() {
			ui.PrimaryButton(c, locales.OpenMQTTSettings()).OnClick(func() { a.tab = tabSettings })
			ui.Button(c, locales.MqttIntegrationDocs()).OnClick(func() {
				mygo.Shell.OpenExternal("https://www.home-assistant.io/integrations/mqtt/")
			})
		})
	})
	section(c, t, p, locales.HowItWorks(), "", func() {
		step := func(n int, title, body string) {
			ui.Row(c).FillWidth().Padding(10, 14).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(24, 24).Radius(12).Background(t.Accent.Alpha(0.14)).Center().Children(func() {
					ui.Text(c, fmt.Sprint(n)).FontSize(12).FontWeight(700).TextColor(t.Accent)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, title).FontSize(13)
					ui.Text(c, body).FontSize(11).TextColor(t.TextMuted).Wrap()
				})
			})
		}
		step(1, locales.StepBrokerTitle(), locales.StepBrokerBody())
		step(2, locales.StepDiscoveryTitle(), locales.StepDiscoveryBody())
		step(3, locales.StepSaveTitle(), locales.StepSaveBody())
	})
}

func (a *app) statusCard(c *ui.Context, t *ui.Theme, p palette) {
	st := a.status()
	group(c, p).Padding(16).Gap(14).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			badge(c, st.glyph, st.color, 40, true)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, st.headline).FontSize(15).FontWeight(650).Wrap()
				ui.Text(c, st.detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
			if a.cfg.Reporting.Enabled == false {
				ui.Button(c, locales.OpenSettings()).OnClick(func() { a.tab = tabSettings })
			}
		})
		if a.cfg.Reporting.Enabled {
			last := locales.Never()
			if !a.mqttStatus.LastPublish.IsZero() {
				last = a.mqttStatus.LastPublish.Local().Format("15:04:05")
			}
			ui.Row(c).Gap(1).Dividers(1, p.separator).Children(func() {
				statusFact(c, t, locales.LastReportStatLabel(), last)
				statusFact(c, t, locales.ReportsStatLabel(), fmt.Sprint(a.mqttStatus.Published))
				statusFact(c, t, locales.IntervalLabel(), locales.EverySeconds(a.cfg.Reporting.IntervalSeconds))
				statusFact(c, t, locales.EntitiesStatLabel(), fmt.Sprint(len(a.publishedEntities())))
			})
		}
	})
}

func statusFact(c *ui.Context, t *ui.Theme, label, value string) {
	ui.Column(c).Grow(1).Basis(0).Gap(2).Padding(0, 12).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, value).FontSize(14).FontWeight(600).FontFeatures("tnum").SingleLine()
	})
}

// publishedEntities lists the entities discovery publishes with the
// current settings, on this Mac.
func (a *app) publishedEntities() []homeassistant.Entity {
	return homeassistant.SystemEntities(homeassistant.Options{
		BaseTopic:  a.cfg.HomeAssistant.BaseTopic,
		HasBattery: a.hasBattery(),
		Disabled:   a.cfg.HomeAssistant.Disabled(),
	})
}

func (a *app) nowPlayingCard(c *ui.Context, t *ui.Theme, p palette) {
	media := a.latest.Media
	published := a.cfg.HomeAssistant.Publishes(homeassistant.GroupMedia)
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Text(c, locales.NowPlaying()).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Padding(0, 12)
		group(c, p).Padding(14).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				art := ui.Box(c).Size(64, 64).Radius(14).Center()
				if media != nil {
					art.Gradient(ui.Hex("#ff5e7e"), ui.Hex("#ff9f45"), 135)
				} else {
					art.Background(p.fill)
				}
				art.Children(func() {
					glyph := ui.Icon(c, iconMusic).FontSize(26)
					if media != nil {
						glyph.TextColor(ui.RGB(255, 255, 255))
					} else {
						glyph.TextColor(t.TextMuted)
					}
				})
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					switch {
					case !published:
						ui.Text(c, locales.MediaNotPublished()).FontSize(14).FontWeight(600)
						ui.Text(c, locales.MediaNotPublishedBody()).FontSize(12).TextColor(t.TextMuted).Wrap()
					case media == nil:
						ui.Text(c, locales.NothingPlaying()).FontSize(14).FontWeight(600)
						ui.Text(c, locales.NothingPlayingBody()).FontSize(12).TextColor(t.TextMuted).Wrap()
					default:
						a.mediaDetails(c, t, p, media)
					}
				})
				if !published {
					ui.Button(c, locales.ChooseEntities()).OnClick(func() { a.tab = tabEntities })
				} else if media != nil {
					a.mediaControls(c, t, p, media.State == "playing")
				}
			})
		})
	})
}

func (a *app) mediaDetails(c *ui.Context, t *ui.Theme, p palette, media *system.MediaInfo) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, media.Title).FontSize(14).FontWeight(650).MaxLines(2).Ellipsis("…").Grow(1).Shrink(1).MinWidth(0)
		label, color := locales.MediaPaused(), colorGray
		if media.State == "playing" {
			label, color = locales.MediaPlaying(), colorGreen
		}
		ui.Text(c, label).FontSize(10).FontWeight(650).TextColor(color).Padding(2, 7).Radius(8).Background(color.Alpha(0.14)).Shrink(0)
	})
	var by []string
	for _, s := range []string{media.Artist, media.Album, media.AppName} {
		if s != "" {
			by = append(by, s)
		}
	}
	if len(by) > 0 {
		ui.Text(c, strings.Join(by, " · ")).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
	}
	if media.DurationSeconds <= 0 {
		return
	}
	position := media.PositionSeconds
	if media.State == "playing" && media.PlaybackRate > 0 && !a.latest.LastSeen.IsZero() {
		// The position was read at the last report: run it on, a second
		// at a time, until the next.
		// No further than a report past it, should reports stop.
		elapsed := min(c.Now().Sub(a.latest.LastSeen).Seconds(), float64(a.cfg.Reporting.IntervalSeconds+5))
		position += max(elapsed, 0) * media.PlaybackRate
		c.After(time.Second)
	}
	position = min(max(position, 0), media.DurationSeconds)
	ui.Column(c).FillWidth().Gap(3).Padding(4, 0, 0).Children(func() {
		ui.Row(c).FillWidth().Height(4).Radius(2).Background(p.fill).Children(func() {
			ui.Box(c).WidthPercent(float32(position / media.DurationSeconds * 100)).FillHeight().Radius(2).Background(t.TextMuted)
		})
		ui.Row(c).Children(func() {
			ui.Text(c, clock(position)).FontSize(10).TextColor(t.TextMuted).FontFeatures("tnum")
			ui.Spacer(c)
			ui.Text(c, "-"+clock(media.DurationSeconds-position)).FontSize(10).TextColor(t.TextMuted).FontFeatures("tnum")
		})
	})
}

// mediaControls are the previous, play or pause, and next buttons, as
// Control Center shows them.
func (a *app) mediaControls(c *ui.Context, t *ui.Theme, p palette, playing bool) {
	button := func(glyph *ui.SVG, label string, main bool, command system.MediaCommand) {
		c.Key(label)
		size := float32(30)
		if main {
			size = 38
		}
		b := ui.ButtonBase(c).Size(size, size).Radius(size / 2).Center().Label(label).Tooltip(label)
		switch {
		case main:
			b.Background(t.Accent).TextColor(t.AccentText)
		case b.Hovered():
			b.Background(p.fill).TextColor(t.Text)
		default:
			b.TextColor(t.Text)
		}
		b.Children(func() { ui.Icon(c, glyph).FontSize(size * 0.42) })
		b.OnClick(func() { a.mediaControl(command) })
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		button(iconBackFilled, locales.PreviousTrack(), false, system.MediaPrevious)
		if playing {
			button(iconPauseFilled, locales.PauseAction(), true, system.MediaPause)
		} else {
			button(iconPlayFilled, locales.PlayAction(), true, system.MediaPlay)
		}
		button(iconForwardFilled, locales.NextTrack(), false, system.MediaNext)
	})
}

// clock formats seconds as m:ss, or h:mm:ss from an hour.
func clock(seconds float64) string {
	s := int(seconds)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (a *app) stateTiles(c *ui.Context, t *ui.Theme, p palette) {
	s := a.latest
	collecting := s.LastSeen.IsZero()
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Text(c, locales.ThisMac()).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Padding(0, 12)
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			ui.Row(c).Gap(12).Children(func() {
				net := a.groupValue(homeassistant.GroupNetwork)
				detail := locales.InternetRouteAvailable()
				tint := colorBlue
				switch {
				case collecting:
					detail = locales.WaitingFirstReport()
				case !s.Network.Online:
					detail, tint = locales.NoInternetRoute(), colorRed
				case s.Network.Metered:
					detail = locales.MeteredConnection()
				}
				tile(c, t, p, iconNetwork, tint, locales.MetricNetwork(), net, detail)

				glyph, tint := iconPower, colorGreen
				detail = locales.PowerSource()
				if s.Power.BatteryPercent != nil {
					glyph, detail = iconBattery, localizedPowerSource(s.Power.Source)
				}
				tile(c, t, p, glyph, tint, locales.MetricPower(), a.groupValue(homeassistant.GroupPower), detail)
			})
			ui.Row(c).Gap(12).Children(func() {
				glyph, tint, detail := iconVolume, colorPink, locales.Unmuted()
				if s.Audio == nil {
					detail = locales.NoOutputDeviceReadable()
				} else if s.Audio.Muted {
					glyph, detail = iconMuted, locales.Muted()
				}
				value := locales.AudioUnavailable()
				if s.Audio != nil {
					value = fmt.Sprintf("%d%%", s.Audio.Volume)
				}
				tile(c, t, p, glyph, tint, locales.MetricVolume(), value, detail)

				glyph, tint = iconUnlock, colorPurple
				value, detail = locales.Unlocked(), localizedIdle(s.Session.IdleSeconds)
				switch {
				case collecting:
					value, detail = locales.Collecting(), locales.WaitingSessionState()
				case s.Session.Locked:
					glyph, value = iconLock, locales.Locked()
				}
				tile(c, t, p, glyph, tint, locales.MetricScreen(), value, detail)
			})
			ui.Row(c).Gap(12).Children(func() {
				glyph, value := iconCamera, a.groupValue(homeassistant.GroupActivity)
				if s.Activity.MicrophoneInUse && !s.Activity.CameraInUse {
					glyph = iconMic
				}
				tint := colorGray
				if s.Activity.CameraInUse || s.Activity.MicrophoneInUse {
					tint = colorRed
				}
				tile(c, t, p, glyph, tint, locales.MetricActivity(), value, locales.ActivityDetail())

				value, tint = locales.DisplayAwake(), colorTeal
				if !s.Display.Awake && !collecting {
					value, tint = locales.DisplayAsleep(), colorGray
				}
				if collecting {
					value = locales.Collecting()
				}
				tile(c, t, p, iconDisplay, tint, locales.MetricDisplay(), value, locales.DisplayDetail())
			})
		})
	})
}

func tile(c *ui.Context, t *ui.Theme, p palette, glyph *ui.SVG, tint ui.Color, label, value, detail string) {
	group(c, p).Grow(1).Basis(0).MinWidth(0).Padding(14).Gap(10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			badge(c, glyph, tint, 26, false)
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, value).FontSize(18).FontWeight(650).SingleLine().Ellipsis("…")
			ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		})
	})
}

// groupValue sums up the current state of an entity group in a few words.
func (a *app) groupValue(group string) string {
	s := a.latest
	if s.LastSeen.IsZero() {
		return locales.Collecting()
	}
	switch group {
	case homeassistant.GroupNetwork:
		if s.Network.Online {
			return locales.NetworkOnline()
		}
		return locales.NetworkOffline()
	case homeassistant.GroupPower:
		if s.Power.BatteryPercent != nil {
			v := fmt.Sprintf("%.0f%%", *s.Power.BatteryPercent)
			if s.Power.Charging != nil && *s.Power.Charging {
				v += " · " + locales.ChargingStatus()
			}
			return v
		}
		return localizedPowerSource(s.Power.Source)
	case homeassistant.GroupSession:
		if s.Session.Locked {
			return locales.Locked()
		}
		return locales.Unlocked()
	case homeassistant.GroupAudio:
		if s.Audio == nil {
			return locales.AudioUnavailable()
		}
		if s.Audio.Muted {
			return locales.Muted()
		}
		return fmt.Sprintf("%d%%", s.Audio.Volume)
	case homeassistant.GroupMedia:
		if s.Media == nil {
			return locales.MediaIdle()
		}
		if s.Media.State == "playing" {
			return locales.MediaPlaying()
		}
		return locales.MediaPaused()
	case homeassistant.GroupActivity:
		switch {
		case s.Activity.CameraInUse && s.Activity.MicrophoneInUse:
			return locales.CameraAndMicInUse()
		case s.Activity.CameraInUse:
			return locales.CameraInUse()
		case s.Activity.MicrophoneInUse:
			return locales.MicInUse()
		}
		return locales.NotInUse()
	case homeassistant.GroupNotify:
		return ""
	case homeassistant.GroupShortcuts:
		return ""
	case homeassistant.GroupDiagnostics:
		return locales.VersionShort(a.version)
	}
	return ""
}

func (a *app) hasBattery() bool {
	return a.latest.Power.BatteryPercent != nil || a.latest.Power.Charging != nil
}

func localizedPowerSource(source string) string {
	switch source {
	case "AC":
		return locales.PowerAC()
	case "Battery":
		return locales.PowerBattery()
	case "":
		return locales.Unknown()
	default:
		return source
	}
}

func localizedIdle(seconds int64) string {
	if seconds < 60 {
		return locales.IdleSeconds(int(seconds))
	}
	if seconds < 3600 {
		return locales.IdleMinutes(int(seconds / 60))
	}
	return locales.IdleHoursMinutes(int(seconds/3600), int(seconds%3600/60))
}
