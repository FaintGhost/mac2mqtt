package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// The app stays open all day: once its motion is done, it draws nothing
// until something changes.
func TestMotionSettles(t *testing.T) {
	defer i18n.SetLocaleFunc(func() string { return "en" })
	i18n.SetLocaleFunc(func() string { return "en" })
	a := siteApp()
	a.tab = tabEntities
	tt := ui.NewTester(a.view, 820, 600)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 88})
	tt.Frame()
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Fatal("frames pending at rest")
	}
	if err := tt.Click("Overview"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.FramePending() {
		t.Error("switching pages does not animate")
	}
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("frames pending once the switch is done")
	}
	if a.tab != tabOverview {
		t.Errorf("tab = %d, want Overview", a.tab)
	}
}
