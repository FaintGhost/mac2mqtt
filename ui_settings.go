package main

import (
	"reflect"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/config"
	"mac2mqtt/locales"
)

const fieldWidth = 240

func (a *app) settingsPage(c *ui.Context, t *ui.Theme, p palette) {
	dirty := a.settingsDirty()
	var bottom float32
	if dirty || a.settingsError != "" {
		bottom = 64
	}
	a.page(c, t, p, locales.PageSettingsTitle(), bottom, nil, func() {
		section(c, t, p, locales.GeneralTitle(), a.loginError, func() {
			row(c, t, locales.LanguageLabel(), "", func() { a.languageSelect(c, t, p) })
			row(c, t, locales.OpenAtLoginLabel(), locales.OpenAtLoginHelp(), func() {
				if ui.Switch(c, &a.openAtLogin).Label(locales.OpenAtLoginLabel()).Changed() {
					a.setOpenAtLogin(a.openAtLogin)
				}
			})
			if a.loginNeedsApproval {
				row(c, t, locales.LoginItemsApproval(), "", func() {
					ui.Button(c, locales.OpenLoginItems()).OnClick(openLoginItems)
				})
			}
		})
		section(c, t, p, locales.MqttConnectionTitle(), locales.MqttConnectionDescription(), func() {
			row(c, t, locales.ReportingLabel(), locales.ReportingHelp(), func() {
				ui.Switch(c, &a.draft.Reporting.Enabled).Label(locales.ReportingLabel())
			})
			row(c, t, locales.HostLabel(), "", func() {
				ui.TextInput(c, &a.draft.Broker.Host).Placeholder("homeassistant.local").Width(fieldWidth)
			})
			row(c, t, locales.PortLabel(), "", func() {
				ui.NumberInput(c, &a.editPort, 1, 65535, 1).Width(fieldWidth)
			})
			row(c, t, locales.TlsLabel(), locales.TlsHelp(), func() {
				if ui.Switch(c, &a.draft.Broker.TLS).Label(locales.TlsLabel()).Changed() {
					if a.draft.Broker.TLS && a.editPort == 1883 {
						a.editPort = 8883
					} else if !a.draft.Broker.TLS && a.editPort == 8883 {
						a.editPort = 1883
					}
				}
			})
			row(c, t, locales.UsernameLabel(), "", func() {
				ui.TextInput(c, &a.draft.Broker.Username).Placeholder(locales.OptionalPlaceholder()).Width(fieldWidth)
			})
			row(c, t, locales.PasswordLabel(), "", func() {
				ui.TextInput(c, &a.draft.Broker.Password).Password().Placeholder(locales.OptionalPlaceholder()).Width(fieldWidth)
			})
		})
		section(c, t, p, locales.HomeAssistantSettingsTitle(), locales.HomeAssistantSettingsDescription(), func() {
			row(c, t, locales.DiscoveryLabel(), "", func() {
				ui.TextInput(c, &a.draft.HomeAssistant.DiscoveryPrefix).Placeholder("homeassistant").Width(fieldWidth)
			})
			row(c, t, locales.BaseTopicLabel(), "", func() {
				ui.TextInput(c, &a.draft.HomeAssistant.BaseTopic).Placeholder("mac2mqtt/my-mac").Width(fieldWidth)
			})
			row(c, t, locales.DeviceLabel(), "", func() {
				ui.TextInput(c, &a.draft.HomeAssistant.DeviceName).Width(fieldWidth)
			})
			row(c, t, locales.IntervalLabel(), locales.IntervalHelp(), func() {
				ui.NumberInput(c, &a.editInterval, 5, 3600, 5).Width(fieldWidth)
			})
		})
		section(c, t, p, locales.Advanced(), locales.IdentityRetentionDescription(), func() {
			row(c, t, locales.ClientIDLabel(), locales.ClientIDHelp(), func() {
				ui.TextInput(c, &a.draft.Broker.ClientID).Width(fieldWidth)
			})
			row(c, t, locales.RetainStateLabel(), locales.RetainStateHelp(), func() {
				ui.Switch(c, &a.draft.Reporting.Retain).Label(locales.RetainStateLabel())
			})
			if a.draft.Broker.TLS {
				row(c, t, locales.SkipVerifyLabel(), locales.SkipVerifyHelp(), func() {
					ui.Switch(c, &a.draft.Broker.InsecureSkipVerify).Label(locales.SkipVerifyLabel())
				})
			}
			ui.Row(c).FillWidth().Padding(10, 14).Gap(8).Children(func() {
				ui.Button(c, locales.UseHostnameDefaults()).OnClick(func() {
					name := config.Default().HomeAssistant.DeviceName
					a.draft.HomeAssistant.DeviceName = name
					a.draft.Broker.ClientID = "mac2mqtt-" + config.SanitizeID(name)
					a.draft.HomeAssistant.BaseTopic = "mac2mqtt/" + config.SanitizeID(name)
				})
				ui.Spacer(c)
				ui.Button(c, locales.OpenConfigFolder()).OnClick(func() { mygo.Shell.ShowItemInFolder(a.cfgPath) })
			})
		})
	})
	if bottom > 0 {
		a.saveBar(c, t, dirty)
	}
}

// saveBar floats on glass at the bottom while settings have unsaved
// changes, or saving them failed.
func (a *app) saveBar(c *ui.Context, t *ui.Theme, dirty bool) {
	ui.Row(c).Absolute().Left(16).Right(20).Bottom(12).Padding(8, 8, 8, 16).Gap(8).Radius(20).AlignItems(ui.Center).
		Material(glass.Glass{}).Children(func() {
		if a.settingsError != "" {
			ui.Text(c, a.settingsError).FontSize(12).TextColor(t.Danger).Grow(1).Shrink(1).MinWidth(0).Wrap()
		} else {
			ui.Text(c, locales.ChangesInfo()).FontSize(12).TextColor(t.TextMuted).Grow(1).Shrink(1).MinWidth(0).Wrap()
		}
		ui.Button(c, locales.Revert()).Disabled(!dirty).OnClick(a.revertDraft)
		ui.PrimaryButton(c, locales.SaveReconnect()).Disabled(!dirty).OnClick(a.saveSettings)
	})
}

// settingsDirty reports whether the settings differ from what is saved.
func (a *app) settingsDirty() bool {
	d := a.draft
	d.Broker.Port = int(a.editPort)
	d.Reporting.IntervalSeconds = int(a.editInterval)
	return !reflect.DeepEqual(d, a.cfg)
}

func (a *app) languageSelect(c *ui.Context, t *ui.Theme, p palette) {
	selected := &a.draft.UI.Language
	sel := ui.SelectBase[string](c, selected)
	sel.Trigger.Width(fieldWidth).Padding(5, 10).Radius(7).Background(p.fill).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.languageDisplay(*selected)).FontSize(13).Grow(1)
		ui.Icon(c, iconChevron).FontSize(12).TextColor(t.TextMuted)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(10).Background(t.Background).Border(1, t.Border).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.16))
		for _, tag := range []string{"", "en", "zh-Hans", "zh-Hant"} {
			item := sel.Item(tag).Padding(6, 10).Radius(6)
			if item.Highlighted() {
				item.Background(t.Accent).TextColor(t.AccentText)
			}
			item.Children(func() { ui.Text(c, a.languageDisplay(tag)) })
		}
	})
	if sel.Trigger.Changed() {
		a.changeLanguage(*selected)
	}
}

func (a *app) languageDisplay(tag string) string {
	switch tag {
	case "en":
		return locales.LanguageEnglish()
	case "zh-Hans":
		return locales.LanguageSimplifiedChinese()
	case "zh-Hant":
		return locales.LanguageTraditionalChinese()
	default:
		return locales.LanguageFollowSystem()
	}
}

func (a *app) changeLanguage(tag string) {
	a.cfg.UI.Language = tag
	a.draft.UI.Language = tag
	a.saveConfig()
	mygo.App.SetLocale(tag)
}

func openLoginItems() {
	mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.LoginItems-Settings.extension")
}
