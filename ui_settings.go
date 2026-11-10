package main

import (
	"math"
	"reflect"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/config"
	"mac2mqtt/locales"
)

const fieldWidth = 240

func (a *app) settingsPage(c *ui.Context, t *ui.Theme, p palette) {
	a.editing = false
	a.page(c, t, p, locales.PageSettingsTitle(), 0, nil, func() {
		section(c, t, p, locales.GeneralTitle(), a.loginError, func() {
			row(c, t, locales.LanguageLabel(), "", func() { a.languageSelect(c, t, p) })
			row(c, t, locales.OpenAtLoginLabel(), locales.OpenAtLoginHelp(), func() {
				if ui.Switch(c, &a.openAtLogin).Label(locales.OpenAtLoginLabel()).Changed() {
					a.setOpenAtLogin(a.openAtLogin)
				}
			})
			if a.loginNeedsApproval {
				c.Key("login-approval")
				ui.Column(c).FillWidth().Transition(rowCollapse).Children(func() {
					row(c, t, locales.LoginItemsApproval(), "", func() {
						ui.Button(c, locales.OpenLoginItems()).OnClick(openLoginItems)
					})
				})
			}
		})
		a.updatesSection(c, t, p)
		section(c, t, p, locales.MqttConnectionTitle(), locales.MqttConnectionDescription(), func() {
			row(c, t, locales.ReportingLabel(), locales.ReportingHelp(), func() {
				ui.Switch(c, &a.draft.Reporting.Enabled).Label(locales.ReportingLabel())
			})
			row(c, t, locales.HostLabel(), "", func() {
				a.field(ui.TextInput(c, &a.draft.Broker.Host).Placeholder("homeassistant.local").Width(fieldWidth))
			})
			row(c, t, locales.PortLabel(), "", func() {
				a.field(ui.NumberInput(c, &a.editPort, 1, 65535, 1).Width(fieldWidth))
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
				a.field(ui.TextInput(c, &a.draft.Broker.Username).Placeholder(locales.OptionalPlaceholder()).Width(fieldWidth))
			})
			row(c, t, locales.PasswordLabel(), "", func() {
				a.field(ui.TextInput(c, &a.draft.Broker.Password).Password().Placeholder(locales.OptionalPlaceholder()).Width(fieldWidth))
			})
		})
		section(c, t, p, locales.HomeAssistantSettingsTitle(), locales.HomeAssistantSettingsDescription(), func() {
			row(c, t, locales.DiscoveryLabel(), "", func() {
				a.field(ui.TextInput(c, &a.draft.HomeAssistant.DiscoveryPrefix).Placeholder("homeassistant").Width(fieldWidth))
			})
			row(c, t, locales.BaseTopicLabel(), "", func() {
				a.field(ui.TextInput(c, &a.draft.HomeAssistant.BaseTopic).Placeholder("mac2mqtt/my-mac").Width(fieldWidth))
			})
			row(c, t, locales.DeviceLabel(), "", func() {
				a.field(ui.TextInput(c, &a.draft.HomeAssistant.DeviceName).Width(fieldWidth))
			})
			row(c, t, locales.IntervalLabel(), locales.IntervalHelp(), func() {
				a.field(ui.NumberInput(c, &a.editInterval, 5, 3600, 5).Width(fieldWidth))
			})
		})
		section(c, t, p, locales.Advanced(), locales.IdentityRetentionDescription(), func() {
			row(c, t, locales.ClientIDLabel(), locales.ClientIDHelp(), func() {
				a.field(ui.TextInput(c, &a.draft.Broker.ClientID).Width(fieldWidth))
			})
			row(c, t, locales.RetainStateLabel(), locales.RetainStateHelp(), func() {
				ui.Switch(c, &a.draft.Reporting.Retain).Label(locales.RetainStateLabel())
			})
			if a.draft.Broker.TLS {
				c.Key("skip-verify")
				ui.Column(c).FillWidth().Transition(rowCollapse).Children(func() {
					row(c, t, locales.SkipVerifyLabel(), locales.SkipVerifyHelp(), func() {
						ui.Switch(c, &a.draft.Broker.InsecureSkipVerify).Label(locales.SkipVerifyLabel())
					})
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
		// What saving says: why it could not, or that it needs no button.
		note, color := locales.AutosaveNote(), t.TextMuted
		if a.settingsError != "" {
			note, color = a.settingsError, t.Danger
		}
		ui.Text(c, note).FontSize(11).TextColor(color).Wrap().Padding(0, 12).ContentTransition(crossfade)
	})
	a.autosave()
}

// field notes whether a field of Settings is being edited: its changes save
// when it loses the focus, not at each key.
func (a *app) field(e ui.Element) {
	if e.Focused() || e.FocusWithin() {
		a.editing = true
	}
}

// updatesSection shows the updater's preferences, and checks on demand.
func (a *app) updatesSection(c *ui.Context, t *ui.Theme, p palette) {
	footer := ""
	if !mygo.Updater.Enabled() {
		footer = locales.UpdatesUnavailable()
	}
	section(c, t, p, locales.UpdatesTitle(), footer, func() {
		row(c, t, locales.AutomaticChecksLabel(), locales.AutomaticChecksHelp(), func() {
			if ui.Switch(c, &a.autoUpdateChecks).Label(locales.AutomaticChecksLabel()).Changed() {
				updater.SetAutomaticChecks(a.autoUpdateChecks)
			}
		})
		row(c, t, locales.AutomaticDownloadsLabel(), locales.AutomaticDownloadsHelp(), func() {
			if ui.Switch(c, &a.autoUpdateDownloads).Label(locales.AutomaticDownloadsLabel()).Changed() {
				updater.SetAutomaticDownloads(a.autoUpdateDownloads)
			}
		})
		last := locales.NeverChecked()
		if !a.lastUpdateCheck.IsZero() {
			last = locales.LastChecked(sinceText(a.lastUpdateCheck, time.Now()))
		}
		check := locales.CheckForUpdates()
		if a.availableUpdate != "" {
			last = locales.UpdateAvailable(a.availableUpdate)
			check = locales.MenuUpdateTo(a.availableUpdate)
		}
		row(c, t, locales.VersionLabel(a.version), last, func() {
			if a.availableUpdate != "" {
				ui.PrimaryButton(c, check).OnClick(updater.CheckForUpdates)
			} else {
				ui.Button(c, check).OnClick(updater.CheckForUpdates)
			}
		})
	})
}

// settingsDirty reports whether the settings differ from what is saved.
func (a *app) settingsDirty() bool {
	return !reflect.DeepEqual(a.pendingDraft(), a.cfg)
}

// pendingDraft is the settings as edited, with the numbers' fields.
func (a *app) pendingDraft() config.Config {
	d := a.draft
	d.Broker.Port = int(a.editPort)
	d.Reporting.IntervalSeconds = int(a.editInterval)
	return d
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

// sinceText writes how long ago then was, in the app's language: "now",
// "5 minutes ago", "yesterday", "3 days ago".
func sinceText(then, now time.Time) string {
	d := now.Sub(then)
	value, unit := -d.Minutes(), i18n.RelativeMinute
	switch {
	case d < time.Minute:
		value, unit = 0, i18n.RelativeSecond
	case d >= 24*time.Hour:
		value, unit = -d.Hours()/24, i18n.RelativeDay
	case d >= time.Hour:
		value, unit = -d.Hours(), i18n.RelativeHour
	}
	return i18n.FormatRelativeTime(i18n.Locale(), math.Trunc(value), unit, i18n.RelativeTimeOptions{Auto: true})
}
