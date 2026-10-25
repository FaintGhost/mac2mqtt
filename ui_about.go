package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/locales"
)

func (a *app) aboutPage(c *ui.Context, t *ui.Theme, p palette) {
	a.page(c, t, p, locales.PageAboutTitle(), 0, nil, func() {
		ui.Column(c).FillWidth().Padding(8, 0, 4).Gap(8).AlignItems(ui.Center).Children(func() {
			if appIcon != nil {
				ui.Image(c, appIcon).Size(88, 88)
			}
			ui.Text(c, locales.AppName()).FontSize(22).FontWeight(700)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.VersionLabel(a.version)).FontSize(12).TextColor(t.TextMuted)
				ui.ButtonBase(c).Label(locales.CheckForUpdates()).TextColor(t.Accent).Children(func() {
					ui.Text(c, locales.CheckForUpdates()).FontSize(12)
				}).OnClick(updater.CheckForUpdates)
			})
			ui.Text(c, locales.AboutBody()).FontSize(13).TextColor(t.TextMuted).TextAlign(ui.Center).Wrap().MaxWidth(420)
			ui.Row(c).Gap(8).Padding(6, 0, 0).Children(func() {
				glassButton(c, iconHome, locales.HomeAssistantMQTT(), false).OnClick(func() {
					mygo.Shell.OpenExternal("https://www.home-assistant.io/integrations/mqtt/")
				})
				glassButton(c, iconLink, locales.MyGoFramework(), false).OnClick(func() {
					mygo.Shell.OpenExternal("https://github.com/egoist/mygo")
				})
			})
		})
		base := a.cfg.HomeAssistant.BaseTopic
		section(c, t, p, locales.Topics(), locales.TopicsDescription(), func() {
			valueRow(c, t, locales.TopicStateLabel(), a.cfg.StateTopic(), true)
			valueRow(c, t, locales.TopicAvailabilityLabel(), a.cfg.AvailabilityTopic(), true)
			valueRow(c, t, locales.TopicMediaLabel(), base+"/media/state · /media/attributes", true)
			valueRow(c, t, locales.TopicVolumeLabel(), base+"/volume/set", true)
			valueRow(c, t, locales.TopicMuteLabel(), base+"/mute/set", true)
			valueRow(c, t, locales.TopicRefreshLabel(), base+"/refresh", true)
		})
		section(c, t, p, "", locales.QuitBody(), func() {
			row(c, t, locales.QuitApp(), "", func() {
				ui.Button(c, locales.Quit()).OnClick(mygo.App.Quit)
			})
		})
	})
}
