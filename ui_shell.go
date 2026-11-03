package main

import (
	"net"
	"strings"

	"github.com/egoist/mygo"
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
	// The sidebar nests in the window's corners on macOS 26 (16): its
	// radius is theirs less the space between. Nothing inside it sits in
	// its corners, so its items keep room around them, and their own
	// radius, with no corner to match.
	windowRadius   = 16
	sidebarInset   = 6
	sidebarRadius  = windowRadius - sidebarInset // 10
	sidebarPadding = 10
	itemRadius     = 8
	headerHeight   = 52
)

// palette holds the window's colors: a canvas, and grouped content on it,
// as System Settings draws them.
type palette struct {
	canvas     ui.Color // the window
	group      ui.Color // inset groups of rows, tiles and cards
	separator  ui.Color // between the rows of a group
	fill       ui.Color // tracks, hovered and selected rows
	selection  ui.Color // the chosen page in the sidebar
	wash       ui.Color // what the sidebar's glass shows: a cool tint, near the canvas
	brand      ui.Color
	ring       ui.Color // the edge of a group, a hairline
	positive   ui.Color // text and dots saying all is well, as "Live"
	positiveBg ui.Color
}

func paletteOf(t *ui.Theme) palette {
	brand := ui.Hex("#0e7c7b")
	if t.Dark {
		return palette{
			canvas:     ui.Hex("#1c1c1e"),
			group:      ui.Hex("#2a2a2d"),
			separator:  ui.RGBA(255, 255, 255, 0.07),
			fill:       ui.RGBA(255, 255, 255, 0.09),
			selection:  ui.RGBA(255, 255, 255, 0.12),
			wash:       ui.Hex("#5b6b85").Alpha(0.20),
			brand:      ui.Hex("#2dd4bf"),
			ring:       ui.RGBA(255, 255, 255, 0.06),
			positive:   ui.Hex("#89d298"),
			positiveBg: ui.Hex("#1f3a25"),
		}
	}
	return palette{
		canvas:     ui.Hex("#f2f2f5"),
		group:      ui.RGB(255, 255, 255),
		separator:  ui.RGBA(0, 0, 0, 0.07),
		fill:       ui.RGBA(0, 0, 0, 0.05),
		selection:  ui.RGBA(0, 0, 0, 0.07),
		wash:       ui.Hex("#8aa4c4").Alpha(0.18),
		brand:      brand,
		ring:       ui.RGBA(0, 0, 0, 0.05),
		positive:   ui.Hex("#317a45"),
		positiveBg: ui.Hex("#e3f6e6"),
	}
}

// The tints of badges, dots and states: macOS's system hues at one bright
// lightness and one vividness (OKLCH 0.72, 0.15), so that none shouts over
// the others.
var (
	colorGreen  = ui.Hex("#5fbc76")
	colorOrange = ui.Hex("#e08f40")
	colorRed    = ui.Hex("#f47c70")
	colorGray   = ui.Hex("#a0a5ae")
	colorBlue   = ui.Hex("#52a9fe")
	colorPurple = ui.Hex("#bb8aef")
	colorPink   = ui.Hex("#f07998")
	colorTeal   = ui.Hex("#1db8ce")
	colorIndigo = ui.Hex("#9499fd")
)

// view draws the window: pages on a canvas, beside a sidebar of glass
// floating over it, as macOS 26 draws a window's sidebar.
func (a *app) view(c *ui.Context) {
	t := c.Theme()
	p := paletteOf(t)
	c.Root().Background(p.canvas)
	window := ui.Box(c).Fill()
	// The window's size as the last frame laid it out, for the pages' edge
	// to draw the backdrop again.
	a.windowSize = window.Bounds()
	window.Children(func() {
		backdrop(c, t, p)
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
		Radius(sidebarRadius).Material(glass.Glass{}).Padding(sidebarTop(c), sidebarPadding, sidebarPadding).Gap(2).Children(func() {
		// Under the traffic lights the sidebar drags the window.
		ui.Box(c).Absolute().Left(0).Top(0).Right(0).Height(max(bar.Height, 20)).DragWindow()
		ui.Row(c).Padding(0, 8, 14).Gap(9).AlignItems(ui.Center).Children(func() {
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
	item := ui.ButtonBase(c).FillWidth().Padding(6, 8).Radius(itemRadius).Gap(9).AlignItems(ui.Center).Justify(ui.Start).
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
	// Text on the glass, under a hairline, not a card: a card in the
	// sidebar's bottom corners would need their radius less the padding.
	ui.Row(c).FillWidth().Padding(0, 8, 8).Children(func() {
		ui.Box(c).Grow(1).Height(1).Background(p.separator)
	})
	ui.Column(c).FillWidth().Padding(2, 8, 4).Gap(3).Children(func() {
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

// sidebarTop is where the sidebar's content starts, just under the
// traffic lights, so that the app's name sits close to them; brandCenter is the middle of the app's icon there, which the
// pages' titles line up with.
func sidebarTop(c *ui.Context) float32 { return max(c.TitleBar().Height, 20) - 6 }

func brandCenter(c *ui.Context) float32 { return sidebarInset + sidebarTop(c) + 15 }

// page lays out a page: content scrolling under a header that floats over
// it, with the page's title and actions on glass.
func (a *app) page(c *ui.Context, t *ui.Theme, p palette, title string, bottomRoom float32, actions func(), content func()) {
	// The header sits level with the sidebar's name, its title centered on
	// the app's icon there, not up among the traffic lights.
	headerTop := brandCenter(c) - headerHeight/2
	// Content scrolls below the header, not under it, and fades out at the
	// scroll's top edge: nothing covers the canvas's light behind the
	// header, so it shows it as the rest of the window does.
	const fade = 18
	scrollTop := headerTop + headerHeight - fade + 6
	// A scroll of each page's own: one page's place is not another's.
	c.Key(a.tab)
	ui.Scroll(c).Absolute().Top(scrollTop).Left(0).Right(0).Bottom(0).
		Padding(fade, 24, 24+bottomRoom, 16).Gap(20).ScrollbarInsets(fade, 2, bottomRoom, 0).
		Children(content)
	a.scrollEdge(c, t, p, scrollTop, fade)
	// Above the header, the window drags as the title bar does.
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(headerTop).DragWindow()
	ui.Row(c).Absolute().Top(headerTop).Left(0).Right(0).Height(headerHeight).Padding(0, 20, 0, 16).Gap(8).AlignItems(ui.Center).
		DragWindow().Children(func() {
		ui.Text(c, title).FontSize(20).FontWeight(700).Grow(1).SingleLine()
		if actions != nil {
			actions()
		}
	})
}

// scrollEdge fades what scrolls out at the top edge of a page's scroll, from
// gone to whole over fade DIPs, where content at rest does not reach. ui has
// no mask to fade content by, so it lays the backdrop over it again, a line
// at a time, each less opaque than the last: the backdrop's light included,
// it meets the window's unseen.
func (a *app) scrollEdge(c *ui.Context, t *ui.Theme, p palette, top float32, fade int) {
	size := a.windowSize
	if size.W == 0 || size.H == 0 {
		return // the first frame, before the window's size is known
	}
	left := float32(sidebarWidth + sidebarInset)
	for i := range fade {
		y := top + float32(i)
		ui.Box(c).Absolute().Top(y).Left(0).Right(0).Height(1).Clip().PassThrough().
			Opacity(1 - (float32(i)+0.5)/float32(fade)).Children(func() {
			ui.Box(c).Absolute().Left(-left).Top(-y).Width(size.W).Height(size.H).PassThrough().Children(func() {
				backdrop(c, t, p)
			})
		})
	}
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

// sectionTitle names a section over its group, perhaps after a small
// icon in the section's tint.
func sectionTitle(c *ui.Context, t *ui.Theme, glyph *ui.SVG, tint ui.Color, title string) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 12).Children(func() {
		if glyph != nil {
			ui.Icon(c, glyph).FontSize(13).TextColor(tint)
		}
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
	})
}

// backdrop draws what is behind everything, filling its parent as large as
// the window: the canvas, the tint the sidebar's glass shows, and the light
// the pages' glass bends.
func backdrop(c *ui.Context, t *ui.Theme, p palette) {
	ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).PassThrough().Background(p.canvas)
	ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).Width(sidebarWidth + 24).PassThrough().
		LinearGradient(ui.LinearGradient{From: p.wash, To: p.wash.Alpha(0), Angle: 90, Start: 0.5, End: 1})
	ambient(c, t)
}

// ambient lays soft light of the badges' colors on the canvas behind the
// pages, for their glass to bend: without color under it, glass on a flat
// canvas shows nothing of itself.
func ambient(c *ui.Context, t *ui.Theme) {
	strength := float32(0.20)
	if t.Dark {
		strength = 0.26
	}
	glow := func(color ui.Color, x, y float32, radius float32) {
		ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).PassThrough().RadialGradient(ui.RadialGradient{
			Stops:  []ui.GradientStop{{Color: color.Alpha(strength), At: 0}, {Color: color.Alpha(0), At: 1}},
			Center: ui.Point{X: x, Y: y}, Radius: radius, Oklab: true,
		})
	}
	glow(colorBlue, 0.85, 0.18, 360)
	glow(colorTeal, 0.12, 0.42, 320)
	glow(colorPink, 0.65, 0.78, 340)
}

// glassCard is a card of glass over the canvas's light, for what heads a
// page: what is happening now.
func glassCard(c *ui.Context) ui.Element {
	return ui.Column(c).FillWidth().Radius(16).Material(glass.Glass{Clarity: 0.3})
}

// section is a titled group of rows, as System Settings lists settings.
func section(c *ui.Context, t *ui.Theme, p palette, title, footer string, rows func()) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		if title != "" {
			sectionTitle(c, t, nil, ui.Transparent, title)
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
	return ui.Column(c).Radius(14).Background(p.group).Border(1, p.ring).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04))
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

// badge is an icon on a flat rounded square, as System Settings marks its
// rows; solid fills it with the tint and draws the icon white.
func badge(c *ui.Context, glyph *ui.SVG, tint ui.Color, size float32, solid bool) {
	b := ui.Box(c).Size(size, size).Radius(size * 0.26).Center()
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

// pill is a short word on a rounded tint, as "Live" or "Playing".
func pill(c *ui.Context, label string, text, background ui.Color, dot bool) {
	ui.Row(c).Gap(5).Padding(2, 8).Radius(9).AlignItems(ui.Center).Background(background).Shrink(0).Children(func() {
		if dot {
			ui.Box(c).Size(6, 6).Radius(3).Background(text)
		}
		ui.Text(c, label).FontSize(10.5).FontWeight(600).TextColor(text).SingleLine()
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
	// action, when set, is what the status card offers to do about it.
	actionLabel string
	action      func()
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
		if localNetworkBlocked(a.mqttStatus.LastError, a.cfg.Broker.Host) {
			return statusInfo{phase: phase, color: colorOrange, glyph: iconAlert, label: locales.StatusConnectionError(),
				headline: locales.LocalNetworkBlocked(), detail: locales.LocalNetworkBlockedBody(),
				actionLabel: locales.OpenLocalNetworkSettings(), action: openLocalNetworkSettings}
		}
		return statusInfo{phase: phase, color: colorRed, glyph: iconAlert, label: locales.StatusConnectionError(),
			headline: locales.CannotReachBroker(), detail: displayOr(a.mqttStatus.LastError, locales.BrokerErrorFallback())}
	default:
		return statusInfo{phase: service.PhaseIdle, color: colorGray, glyph: iconBroker, label: locales.StatusPaused(),
			headline: locales.WaitingToConnect(), detail: locales.WaitingDetail()}
	}
}

// localNetworkBlocked reports whether an error connecting to a broker on
// the local network is macOS's Local Network privacy refusing it: the
// system then answers that there is no route to the host. It asks once,
// and an app whose signature changed, as an update signed ad hoc, is
// refused until allowed again.
func localNetworkBlocked(err, host string) bool {
	if !strings.Contains(err, "no route to host") {
		return false
	}
	host = strings.TrimSpace(host)
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	return strings.HasSuffix(host, ".local") || !strings.Contains(host, ".")
}

func openLocalNetworkSettings() {
	mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_LocalNetwork")
}

func displayOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
