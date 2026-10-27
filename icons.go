package main

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon.png
var appIconPNG []byte

// appIcon is the app's icon, made once: MyGo keeps a bitmap on the GPU.
var appIcon, _ = ui.DecodeBitmap(appIconPNG)

// icon makes a stroked 24×24 icon, drawn in the text color, from Lucide's
// shapes (ISC license).
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// filledIcon makes a filled 24×24 icon, drawn in the text color.
func filledIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconOverview = icon(`<rect x="3" y="3" width="7" height="9" rx="1.5"/><rect x="14" y="3" width="7" height="5" rx="1.5"/><rect x="14" y="12" width="7" height="9" rx="1.5"/><rect x="3" y="16" width="7" height="5" rx="1.5"/>`)
	iconEntities = icon(`<path d="M13 5h8M13 12h8M13 19h8"/><path d="m3 5 2 2 4-4M3 12l2 2 4-4M3 19l2 2 4-4"/>`)
	iconSettings = icon(`<path d="M20 7h-9M14 17H5"/><circle cx="17" cy="17" r="3"/><circle cx="7" cy="7" r="3"/>`)
	iconAbout    = icon(`<circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/>`)

	iconNetwork     = icon(`<path d="M12 20h.01M2 8.82a15 15 0 0 1 20 0M5 12.86a10 10 0 0 1 14 0M8.5 16.43a5 5 0 0 1 7 0"/>`)
	iconPower       = icon(`<path d="M12 22v-5M9 8V2M15 8V2"/><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8z"/>`)
	iconBattery     = icon(`<rect x="2" y="7" width="16" height="10" rx="2"/><path d="M22 11v2M6 11v2"/>`)
	iconLock        = icon(`<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>`)
	iconUnlock      = icon(`<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/>`)
	iconVolume      = icon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="M16 9a5 5 0 0 1 0 6M19.4 18.4a9 9 0 0 0 0-12.8"/>`)
	iconMuted       = icon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="m22 9-6 6M16 9l6 6"/>`)
	iconMusic       = icon(`<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`)
	iconCamera      = icon(`<path d="m16 13 5.2 3.5a.5.5 0 0 0 .8-.4V7.9a.5.5 0 0 0-.8-.4L16 11"/><rect x="2" y="6" width="14" height="12" rx="2"/>`)
	iconMic         = icon(`<rect x="9" y="2" width="6" height="12" rx="3"/><path d="M19 10v1a7 7 0 0 1-14 0v-1M12 18v4"/>`)
	iconDisplay     = icon(`<rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/>`)
	iconBell        = icon(`<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>`)
	iconShortcuts   = icon(`<rect x="3" y="3" width="8" height="8" rx="2.5"/><rect x="13" y="13" width="8" height="8" rx="2.5"/><path d="M13 7h4a2 2 0 0 1 2 2v2M11 17H7a2 2 0 0 1-2-2v-2"/>`)
	iconDiagnostics = icon(`<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>`)

	iconSend    = icon(`<path d="m22 2-7 20-4-9-9-4z"/><path d="M22 2 11 13"/>`)
	iconRefresh = icon(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`)
	iconCheck   = icon(`<path d="M20 6 9 17l-5-5"/>`)
	iconAlert   = icon(`<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4M12 17h.01"/>`)
	iconPause   = icon(`<rect x="14" y="4" width="4" height="16" rx="1"/><rect x="6" y="4" width="4" height="16" rx="1"/>`)
	iconPlay    = icon(`<path d="M7 4.5v15a1 1 0 0 0 1.5.86l11.5-7.5a1 1 0 0 0 0-1.72L8.5 3.64A1 1 0 0 0 7 4.5z"/>`)

	// The transport controls are filled, as Control Center draws them.
	iconPlayFilled    = filledIcon(`<path d="M7 4.5v15a1 1 0 0 0 1.5.86l11.5-7.5a1 1 0 0 0 0-1.72L8.5 3.64A1 1 0 0 0 7 4.5z"/>`)
	iconPauseFilled   = filledIcon(`<rect x="14" y="4" width="4" height="16" rx="1"/><rect x="6" y="4" width="4" height="16" rx="1"/>`)
	iconBackFilled    = filledIcon(`<path d="M19 20 9 12l10-8z"/><rect x="4" y="5" width="2.5" height="14" rx="1"/>`)
	iconForwardFilled = filledIcon(`<path d="m5 4 10 8-10 8z"/><rect x="17.5" y="5" width="2.5" height="14" rx="1"/>`)
	iconBroker        = icon(`<path d="M4.9 19.1C1 15.2 1 8.8 4.9 4.9M7.8 16.2c-2.3-2.3-2.3-6.1 0-8.5"/><circle cx="12" cy="12" r="2"/><path d="M16.2 7.8c2.3 2.3 2.3 6.1 0 8.5M19.1 4.9C23 8.8 23 15.1 19.1 19"/>`)
	iconHome          = icon(`<path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8"/><path d="M3 10a2 2 0 0 1 .7-1.5l7-6a2 2 0 0 1 2.6 0l7 6A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	iconLink          = icon(`<path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`)
	iconChevron       = icon(`<path d="m7 15 5 5 5-5M7 9l5-5 5 5"/>`)
)
