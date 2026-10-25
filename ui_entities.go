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
	{homeassistant.GroupShortcuts, iconShortcuts, colorPurple, locales.GroupShortcutsTitle, locales.GroupShortcutsBody},
	{homeassistant.GroupDiagnostics, iconDiagnostics, colorGray, locales.GroupDiagnosticsTitle, locales.GroupDiagnosticsBody},
}

func (a *app) entitiesPage(c *ui.Context, t *ui.Theme, p palette) {
	a.page(c, t, p, locales.PageEntitiesTitle(), 0, nil, func() {
		section(c, t, p, locales.PublishedToHomeAssistant(), locales.EntitiesFooter(), func() {
			for i, g := range entityGroups {
				c.Key(g.id)
				ui.Row(c).FillWidth().MinHeight(52).Padding(8, 14).Gap(12).AlignItems(ui.Center).Children(func() {
					badge(c, g.glyph, g.tint, 28, true)
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, g.title()).FontSize(13)
						ui.Text(c, g.about()).FontSize(11).TextColor(t.TextMuted).Wrap()
					})
					ui.Text(c, a.groupValue(g.id)).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MaxWidth(150)
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
