package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/homeassistant"
	"mac2mqtt/locales"
)

// entityGroup is how the app shows a group of entities.
type entityGroup struct {
	id    string
	glyph *ui.SVG
	tint  ui.Color
	title func() string
	about func() string
}

var entityGroups = []entityGroup{
	{homeassistant.GroupNetwork, iconNetwork, colorBlue, locales.GroupNetworkTitle, locales.GroupNetworkBody},
	{homeassistant.GroupPower, iconPower, colorGreen, locales.GroupPowerTitle, locales.GroupPowerBody},
	{homeassistant.GroupSession, iconLock, colorPurple, locales.GroupSessionTitle, locales.GroupSessionBody},
	{homeassistant.GroupActivity, iconCamera, colorRed, locales.GroupActivityTitle, locales.GroupActivityBody},
	{homeassistant.GroupAudio, iconVolume, colorPink, locales.GroupAudioTitle, locales.GroupAudioBody},
	{homeassistant.GroupMedia, iconMusic, colorOrange, locales.GroupMediaTitle, locales.GroupMediaBody},
	{homeassistant.GroupNotify, iconBell, colorTeal, locales.GroupNotifyTitle, locales.GroupNotifyBody},
	{homeassistant.GroupShortcuts, iconShortcuts, colorIndigo, locales.GroupShortcutsTitle, locales.GroupShortcutsBody},
	{homeassistant.GroupDiagnostics, iconDiagnostics, colorGray, locales.GroupDiagnosticsTitle, locales.GroupDiagnosticsBody},
}

func (a *app) entitiesPage(c *ui.Context, t *ui.Theme, p palette) {
	a.page(c, t, p, locales.PageEntitiesTitle(), 0, nil, func() {
		a.entitiesSummary(c, t, p)
		section(c, t, p, locales.PublishedToHomeAssistant(), "", func() {
			for i, g := range entityGroups {
				// Without a camera or a microphone there is nothing to choose.
				if g.id == homeassistant.GroupActivity && !a.hasCameraOrMicrophone() {
					continue
				}
				c.Key(g.id)
				ui.Row(c).FillWidth().MinHeight(52).Padding(8, 14).Gap(12).AlignItems(ui.Center).Children(func() {
					badge(c, g.glyph, g.tint, 28, true)
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
						title := ui.Text(c, g.title()).FontSize(13)
						if !a.publishing[i] {
							title.TextColor(t.TextMuted)
						}
						ui.Text(c, g.about()).FontSize(11).TextColor(t.TextMuted).Wrap()
					})
					value := a.groupValue(g.id)
					if !a.publishing[i] {
						value = locales.GroupOff()
					}
					ui.Text(c, value).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MaxWidth(150)
					if ui.Switch(c, &a.publishing[i]).Label(g.title()).Changed() {
						a.setPublishes(g.id, a.publishing[i])
					}
				})
			}
		})
		if a.cfg.HomeAssistant.Publishes(homeassistant.GroupMedia) {
			a.mediaPlayerSection(c, t, p)
		}
		section(c, t, p, locales.HomeAssistantTitle(), locales.RefreshAlwaysPublished(), func() {
			valueRow(c, t, locales.DeviceLabel(), a.cfg.HomeAssistant.DeviceName, false)
			valueRow(c, t, locales.DiscoveryLabel(), a.cfg.HomeAssistant.DiscoveryPrefix, true)
			valueRow(c, t, locales.StateTopicLabel(), a.cfg.StateTopic(), true)
			valueRow(c, t, locales.EntitiesStatLabel(), locales.EntityCount(len(a.publishedEntities())), false)
		})
	})
}

// entitiesSummary heads the page: how many entities Home Assistant has from
// this Mac, and how many groups are on and off. Counts, not a progress:
// nothing is left to finish.
func (a *app) entitiesSummary(c *ui.Context, t *ui.Theme, p palette) {
	on, off := 0, 0
	for i, g := range entityGroups {
		if g.id == homeassistant.GroupActivity && !a.hasCameraOrMicrophone() {
			continue
		}
		if a.publishing[i] {
			on++
		} else {
			off++
		}
	}
	group(c, p).Padding(16).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			badge(c, iconEntities, colorBlue, 40, true)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, locales.EntitiesSummary(len(a.publishedEntities()))).FontSize(15).FontWeight(650).Wrap()
				ui.Text(c, locales.EntitiesSummaryBody()).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
			count := func(label string) {
				ui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(t.TextMuted).Padding(3, 10).Radius(11).Background(p.fill).SingleLine().Shrink(0)
			}
			count(locales.GroupsOn(on))
			if off > 0 {
				count(locales.GroupsOff(off))
			}
		})
	})
}

// mediaPlayerSection explains the two ways to a media_player entity in
// Home Assistant, whose MQTT integration has none of its own.
func (a *app) mediaPlayerSection(c *ui.Context, t *ui.Theme, p palette) {
	section(c, t, p, locales.MediaPlayerTitle(), locales.MediaPlayerFooter(), func() {
		row(c, t, locales.HacsPlayerTitle(), locales.HacsPlayerBody(), func() {
			ui.Button(c, locales.OpenIntegration()).OnClick(func() {
				mygo.Shell.OpenExternal("https://github.com/bkbilly/mqtt_media_player")
			})
		})
		row(c, t, locales.UniversalPlayerTitle(), locales.UniversalPlayerBody(), func() {
			label := locales.CopyYAML()
			if a.copiedYAML {
				label = locales.Copied()
			}
			ui.Button(c, label).OnClick(func() {
				mygo.Clipboard.WriteText(homeassistant.UniversalMediaPlayerYAML(homeassistant.Options{
					BaseTopic: a.cfg.HomeAssistant.BaseTopic,
					Device:    homeassistant.Device{ID: a.cfg.NodeID(), Name: a.cfg.HomeAssistant.DeviceName},
				}))
				a.copiedYAML = true
			})
		})
	})
}
