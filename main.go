// Mac2MQTT is a macOS menu-bar app that reports this Mac's status to Home
// Assistant over MQTT, with MQTT discovery and native volume/mute controls.
package main

import (
	"log"

	"github.com/egoist/mygo"

	"mac2mqtt/locales"
)

func main() {
	a := newApp()
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
