package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestZZScrolled(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	a := connectedTestApp()
	tt := ui.NewTester(a.view, 820, 660)
	tt.SetScale(2)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 88})
	tt.Frame()
	tt.Scroll(500, 300, 0, 110)
	tt.Frame()
	f, _ := os.Create(os.Getenv("OUT") + "/scrolled.png")
	png.Encode(f, tt.Image())
	f.Close()
}
