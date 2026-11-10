package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The app's motion: few things move, quickly, and only to show where
// something went or that it changed. While the desktop asks for less
// motion, ui shows them in place and only fades.
var (
	// easeOut settles fast and long, for what the user asked for.
	easeOut = ui.CubicBezier(0.23, 1, 0.32, 1)

	// navSelection slides the sidebar's selection from the item chosen
	// before to the one chosen now.
	navSelection = ui.ElementTransition{
		Match:     "nav-selection",
		Animation: &ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond}},
	}

	// pageEnter brings a page's content in as its tab is chosen: rising a
	// little as it fades in. The page before goes at once, so that two
	// pages of glass never show together.
	pageEnter = ui.ElementTransition{
		// Its Enter alone: what moves it otherwise, as the window opening
		// and laying it out, shows at once.
		Colors:   true,
		Duration: 220 * time.Millisecond,
		Ease:     easeOut,
		Enter:    &ui.Motion{Y: 8},
	}

	// appear brings in and out what comes and goes in place, as the header's
	// buttons and the Live mark.
	appear = ui.ElementTransition{
		Duration:     180 * time.Millisecond,
		Ease:         easeOut,
		ExitDuration: 120 * time.Millisecond,
		Enter:        &ui.Motion{Scale: 0.96},
		Exit:         &ui.Motion{Scale: 0.96},
	}

	// rowCollapse opens and closes a row that comes and goes in a list; the
	// rows after it follow its height.
	rowCollapse = ui.ElementTransition{
		Duration: 240 * time.Millisecond,
		Ease:     easeOut,
		Enter:    &ui.Motion{Collapse: true},
		Exit:     &ui.Motion{Collapse: true},
	}

	// colorChange changes a color, as a badge's from awake to asleep.
	colorChange = ui.ElementTransition{Colors: true, Duration: 240 * time.Millisecond}

	// crossfade changes a text or an icon in place; rolling changes the
	// digits of a number that changed, those that stay staying.
	crossfade = ui.ContentTransition{Animation: &ui.Animation{Duration: 160 * time.Millisecond, Ease: easeOut}}
	rolling   = ui.ContentTransition{Numeric: true, Animation: &ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}}
)
