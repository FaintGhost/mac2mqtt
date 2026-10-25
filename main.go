// Mac2MQTT is a macOS menu-bar app that reports this Mac's status to Home
// Assistant over MQTT, with MQTT discovery and native volume/mute controls.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater/native"

	"mac2mqtt/locales"
)

func main() {
	a := newApp()
	// Updates from the GitHub releases, in the update window that Mac
	// users know from Sparkle, drawn in native UI.
	mygo.Use(native.Plugin)
	mygo.App.SetName(locales.AppName())
	mygo.App.SetVersion(a.version)
	// A menu-bar utility: no Dock icon, no app switcher entry.
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting.Store(true) })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.shutdown() })
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
