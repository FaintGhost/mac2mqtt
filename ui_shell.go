package main

import (
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"mac2mqtt/internal/service"
	"mac2mqtt/locales"
)

// The pages of the window, in the sidebar's order.
const (
	tabOverview = iota
	tabEntities
	tabSettings
	tabAbout
)

const (
	sidebarWidth = 212
	sidebarInset = 8
	headerHeight = 52
)

// palette holds the window's colors: a canvas, and grouped content on it,
// as System Settings draws them.
type palette struct {
	canvas    ui.Color // the window
	group     ui.Color // inset groups of rows, tiles and cards
	separator ui.Color // between the rows of a group
	fill      ui.Color // tracks, hovered and selected rows
	selection ui.Color // the chosen page in the sidebar
	wash      ui.Color // the brand color the sidebar's glass shows
	brand     ui.Color
}

func paletteOf(t *ui.Theme) palette {
	brand := ui.Hex("#0e7c7b")
	if t.Dark {
		return palette{
			canvas:    ui.Hex("#1c1c1e"),
			group:     ui.Hex("#2a2a2d"),
			separator: ui.RGBA(255, 255, 255, 0.07),
			fill:      ui.RGBA(255, 255, 255, 0.09),
			selection: ui.RGBA(255, 255, 255, 0.12),
			wash:      brand.Alpha(0.30),
			brand:     ui.Hex("#2dd4bf"),
		}
	}
	return palette{
		canvas:    ui.Hex("#f2f2f5"),
		group:     ui.RGB(255, 255, 255),
		separator: ui.RGBA(0, 0, 0, 0.07),
		fill:      ui.RGBA(0, 0, 0, 0.05),
		selection: ui.RGBA(0, 0, 0, 0.07),
		wash:      brand.Alpha(0.16),
		brand:     brand,
	}
}

// The colors of macOS's system green, orange, red and gray.
var (
	colorGreen  = ui.Hex("#34c759")
	colorOrange = ui.Hex("#ff9f0a")
	colorRed    = ui.Hex("#ff453a")
	colorGray   = ui.Hex("#8e8e93")
	colorBlue   = ui.Hex("#0a84ff")
	colorPurple = ui.Hex("#bf5af2")
	colorPink   = ui.Hex("#ff375f")
	colorTeal   = ui.Hex("#30b0c7")
)

// view draws the window: pages on a canvas, beside a sidebar of glass
// floating over it, as macOS 26 draws a window's sidebar.
func (a *app) view(c *ui.Context) {
	t := c.Theme()
	p := paletteOf(t)
	c.Root().Background(p.canvas)
	ui.Box(c).Fill().Children(func() {
		// What the glass shows through: the brand's color, fading out
		// before the pages.
		ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).Width(sidebarWidth + 24).PassThrough().
			LinearGradient(ui.LinearGradient{From: p.wash, To: p.wash.Alpha(0), Angle: 90, Start: 0.5, End: 1})
		ui.Box(c).Absolute().Left(sidebarWidth + sidebarInset).Top(0).Right(0).Bottom(0).Children(func() {
			switch a.tab {
			case tabEntities:
				a.entitiesPage(c, t, p)
			case tabSettings:
				a.settingsPage(c, t, p)
			case tabAbout:
				a.aboutPage(c, t, p)
			default:
				a.overviewPage(c, t, p)
			}
		})
		a.sidebar(c, t, p)
	})
}

func (a *app) sidebar(c *ui.Context, t *ui.Theme, p palette) {
	bar := c.TitleBar()
	ui.Column(c).Absolute().Left(sidebarInset).Top(sidebarInset).Bottom(sidebarInset).Width(sidebarWidth-sidebarInset).
		Radius(18).Material(glass.Glass{}).Padding(max(bar.Height, 20)+6, 10, 12).Gap(2).Children(func() {
		// Under the traffic lights the sidebar drags the window.
		ui.Box(c).Absolute().Left(0).Top(0).Right(0).Height(max(bar.Height, 20)).DragWindow()
		ui.Row(c).Padding(4, 8, 14).Gap(9).AlignItems(ui.Center).Children(func() {
			if appIcon != nil {
				ui.Image(c, appIcon).Size(30, 30)
			}
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, locales.AppName()).FontSize(13).FontWeight(650)
				ui.Text(c, locales.AppTagline()).FontSize(11).TextColor(t.TextMuted)
			})
		})
		a.navItem(c, t, p, iconOverview, locales.NavOverview(), tabOverview)
		a.navItem(c, t, p, iconEntities, locales.NavEntities(), tabEntities)
		a.navItem(c, t, p, iconSettings, locales.NavSettings(), tabSettings)
		a.navItem(c, t, p, iconAbout, locales.NavAbout(), tabAbout)
		ui.Spacer(c)
		a.sidebarStatus(c, t, p)
	})
}

func (a *app) navItem(c *ui.Context, t *ui.Theme, p palette, glyph *ui.SVG, label string, tab int) {
	selected := a.tab == tab
	c.Key(tab)
	item := ui.ButtonBase(c).FillWidth().Padding(6, 8).Radius(9).Gap(9).AlignItems(ui.Center).Justify(ui.Start).
		Label(label).TextColor(t.Text)
	if selected {
		item.Background(p.selection)
	} else if item.Hovered() {
		item.Background(p.fill)
	}
	item.Children(func() {
		mark := ui.Icon(c, glyph).FontSize(15)
		if selected {
			mark.TextColor(t.Accent)
		} else {
			mark.TextColor(t.TextMuted)
		}
		text := ui.Text(c, label).FontSize(13)
		if selected {
			text.FontWeight(600)
		}
	})
	if item.Clicked() {
		a.tab = tab
	}
}

func (a *app) sidebarStatus(c *ui.Context, t *ui.Theme, p palette) {
	st := a.status()
	ui.Column(c).FillWidth().Padding(10, 10).Gap(3).Radius(12).Background(p.fill).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(st.color)
			ui.Text(c, st.label).FontSize(12).FontWeight(600)
		})
		detail := locales.NoBrokerConfigured()
		if strings.TrimSpace(a.cfg.Broker.Host) != "" {
			detail = a.cfg.BrokerAddress()
		}
		ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
	})
}

// page lays out a page: content scrolling under a header that floats over
// it, with the page's title and actions on glass.
func (a *app) page(c *ui.Context, t *ui.Theme, p palette, title string, bottomRoom float32, actions func(), content func()) {
	top := float32(headerHeight + 6) // where content starts at rest
	ui.Scroll(c).Fill().Padding(top, 24, 24+bottomRoom, 16).Gap(20).ScrollbarInsets(headerHeight, 2, bottomRoom, 0).
		Children(content)
	// The canvas covers what scrolls under the title: solid behind it, then
	// fading out down to where content starts at rest, so that at rest it
	// covers nothing. Not a blur, whose spread would end sharply at the
	// content's edge beside the sidebar: the canvas is the color of the gap
	// there, so it ends unseen.
	const solid = 38
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(top).PassThrough().
		LinearGradient(ui.LinearGradient{From: p.canvas, To: p.canvas.Alpha(0), Angle: 180, Start: solid / top, End: 1})
	ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(headerHeight).Padding(0, 20, 0, 16).Gap(8).AlignItems(ui.Center).
		DragWindow().Children(func() {
		ui.Text(c, title).FontSize(20).FontWeight(700).Grow(1).SingleLine()
		if actions != nil {
			actions()
		}
	})
}

// glassButton is a capsule of glass holding an icon and a label, the
// buttons of a macOS 26 toolbar; prominent tints it with the accent.
func glassButton(c *ui.Context, glyph *ui.SVG, label string, prominent bool) ui.Element {
	t := c.Theme()
	g := glass.Glass{Interactive: true}
	text := t.Text
	if prominent {
		g.Tint = t.Accent
		text = t.AccentText
	}
	c.Key(label)
	b := ui.ButtonBase(c).Padding(6, 12).Radius(16).Gap(6).AlignItems(ui.Center).Label(label).
		Material(g).TextColor(text)
	b.Children(func() {
		if glyph != nil {
			ui.Icon(c, glyph).FontSize(13)
		}
		ui.Text(c, label).FontSize(12).FontWeight(600).SingleLine()
	})
	return b
}

// section is a titled group of rows, as System Settings lists settings.
func section(c *ui.Context, t *ui.Theme, p palette, title, footer string, rows func()) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Padding(0, 12)
		}
		group(c, p).Gap(1).Dividers(1, p.separator).Children(rows)
		if footer != "" {
			ui.Text(c, footer).FontSize(11).TextColor(t.TextMuted).Wrap().Padding(0, 12)
		}
	})
}

// group is the rounded face that rows and cards sit on, across the page.
func group(c *ui.Context, p palette) ui.Element {
	return card(c, p).FillWidth()
}

// card is the rounded face alone, sized by its parent, as a grid's cell.
func card(c *ui.Context, p palette) ui.Element {
	return ui.Column(c).Radius(12).Background(p.group).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04))
}

// row is a row of a section: a title, perhaps a line under it, and a
// control at its end.
func row(c *ui.Context, t *ui.Theme, title, subtitle string, control func()) {
	ui.Row(c).FillWidth().MinHeight(44).Padding(8, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13)
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted).Wrap()
			}
		})
		if control != nil {
			control()
		}
	})
}

// valueRow is a row showing a value at its end.
func valueRow(c *ui.Context, t *ui.Theme, title, value string, mono bool) {
	row(c, t, title, "", func() {
		v := ui.Text(c, value).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		if mono {
			v.Font("Menlo").FontSize(11)
		}
	})
}

// badge is an icon on a tinted rounded square, as System Settings marks
// its rows; solid fills it with the tint and draws the icon white.
func badge(c *ui.Context, glyph *ui.SVG, tint ui.Color, size float32, solid bool) {
	b := ui.Box(c).Size(size, size).Radius(size * 0.28).Center()
	color := tint
	if solid {
		b.Background(tint)
		color = ui.RGB(255, 255, 255)
	} else {
		b.Background(tint.Alpha(0.15))
	}
	b.Children(func() {
		ui.Icon(c, glyph).FontSize(size * 0.55).TextColor(color)
	})
}

// statusInfo describes the MQTT connection for the views and the tray.
type statusInfo struct {
	phase    service.Phase
	color    ui.Color
	glyph    *ui.SVG
	label    string // short, as in the sidebar
	headline string
	detail   string
}

func (a *app) status() statusInfo {
	phase := a.mqttStatus.Phase
	switch {
	case strings.TrimSpace(a.cfg.Broker.Host) == "":
		return statusInfo{phase: service.PhaseIdle, color: colorGray, glyph: iconBroker, label: locales.StatusNotConfigured(),
			headline: locales.StatusNotConfigured(), detail: locales.NoBrokerConfigured()}
	case a.cfg.Reporting.Enabled == false:
		return statusInfo{phase: service.PhaseIdle, color: colorGray, glyph: iconPause, label: locales.StatusPaused(),
			headline: locales.PausedTitle(), detail: locales.PausedBody()}
	}
	switch phase {
	case service.PhaseOnline:
		if a.mqttStatus.LastError != "" {
			// Connected, but a report did not get through: the next one
			// tries again on the same connection.
			return statusInfo{phase: phase, color: colorOrange, glyph: iconAlert, label: locales.StatusConnected(),
				headline: locales.ReportFailedHeadline(), detail: a.mqttStatus.LastError}
		}
		return statusInfo{phase: phase, color: colorGreen, glyph: iconCheck, label: locales.StatusConnected(),
			headline: locales.ReportingToHomeAssistant(), detail: a.cfg.BrokerAddress()}
	case service.PhaseConnecting:
		return statusInfo{phase: phase, color: colorOrange, glyph: iconBroker, label: locales.StatusConnecting(),
			headline: locales.ConnectingHeadline(), detail: a.cfg.BrokerAddress()}
	case service.PhaseError:
		return statusInfo{phase: phase, color: colorRed, glyph: iconAlert, label: locales.StatusConnectionError(),
			headline: locales.CannotReachBroker(), detail: displayOr(a.mqttStatus.LastError, locales.BrokerErrorFallback())}
	default:
		return statusInfo{phase: service.PhaseIdle, color: colorGray, glyph: iconBroker, label: locales.StatusPaused(),
			headline: locales.WaitingToConnect(), detail: locales.WaitingDetail()}
	}
}

func displayOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
