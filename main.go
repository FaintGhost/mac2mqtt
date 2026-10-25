// Mac2MQTT is a macOS menu-bar app that reports this Mac's status to Home
// Assistant over MQTT, with MQTT discovery and native volume/mute controls.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater/native"
)

func main() {
	a := newApp()
	// Updates from the GitHub releases, in the update window that Mac
	// users know from Sparkle, drawn in native UI.
	mygo.Use(native.Plugin)
	// Not translated: the name keys the user data directory, which holds
	// the configuration, and the single instance lock.
	mygo.App.SetName("Mac2MQTT")
	mygo.App.SetVersion(a.version)
	// One instance at a time: two would publish as one device with one
	// client ID, each disconnecting the other at the broker. Opening the
	// app again shows the running one's window instead.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.OnSecondInstance(func([]string, string) { a.ui(a.showWindow) })
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
