//go:build darwin

package system

import (
	"errors"
	"math"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	selDefaultOutput = fourCC("dOut") // kAudioHardwarePropertyDefaultOutputDevice
	selMainVolume    = fourCC("vmvc") // kAudioHardwareServiceDeviceProperty_VirtualMainVolume
	selMute          = fourCC("mute") // kAudioDevicePropertyMute
	scopeOutput      = fourCC("outp")

	audioSet         func(id uint32, addr *propertyAddress, qualifierSize uint32, qualifier unsafe.Pointer, size uint32, data unsafe.Pointer) int32
	audioHas         func(id uint32, addr *propertyAddress) bool
	serviceGetVolume func(id uint32, addr *propertyAddress, qualifierSize uint32, qualifier unsafe.Pointer, size *uint32, data unsafe.Pointer) int32
	serviceSetVolume func(id uint32, addr *propertyAddress, qualifierSize uint32, qualifier unsafe.Pointer, size uint32, data unsafe.Pointer) int32
	errNoNativeAudio = errors.New("system: the output device has no volume control")
)

func loadNativeAudio() bool {
	loadActivity()
	activityAudioOnce.Do(func() {
		if lib, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&audioSet, lib, "AudioObjectSetPropertyData")
			purego.RegisterLibFunc(&audioHas, lib, "AudioObjectHasProperty")
		}
		// The main volume of a device whose channels each have one is
		// AudioToolbox's: CoreAudio has none for it.
		if lib, err := purego.Dlopen("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&serviceGetVolume, lib, "AudioHardwareServiceGetPropertyData")
			purego.RegisterLibFunc(&serviceSetVolume, lib, "AudioHardwareServiceSetPropertyData")
		}
	})
	return audioGet != nil && audioSet != nil && audioHas != nil && serviceGetVolume != nil && serviceSetVolume != nil
}

func defaultOutput() (uint32, bool) {
	addr := propertyAddress{selDefaultOutput, scopeGlobal, 0}
	var id uint32
	size := uint32(4)
	if audioGet(systemObject, &addr, 0, nil, &size, unsafe.Pointer(&id)) != 0 || id == 0 {
		return 0, false
	}
	return id, true
}

// readNativeAudio reads the output volume and mute through CoreAudio, as
// the menu bar's Sound does, without starting a process.
func readNativeAudio() (*AudioState, error) {
	if !loadNativeAudio() {
		return nil, errNoNativeAudio
	}
	device, ok := defaultOutput()
	if !ok {
		return nil, errNoNativeAudio
	}
	volAddr := propertyAddress{selMainVolume, scopeOutput, 0}
	if !audioHas(device, &volAddr) {
		return nil, errNoNativeAudio
	}
	var volume float32
	size := uint32(4)
	if serviceGetVolume(device, &volAddr, 0, nil, &size, unsafe.Pointer(&volume)) != 0 {
		return nil, errNoNativeAudio
	}
	state := &AudioState{Volume: int(math.Round(float64(volume) * 100))}
	muteAddr := propertyAddress{selMute, scopeOutput, 0}
	if audioHas(device, &muteAddr) {
		var muted uint32
		size = 4
		if audioGet(device, &muteAddr, 0, nil, &size, unsafe.Pointer(&muted)) == 0 {
			state.Muted = muted != 0
		}
	}
	return state, nil
}

func setNativeVolume(volume int) error {
	if !loadNativeAudio() {
		return errNoNativeAudio
	}
	device, ok := defaultOutput()
	if !ok {
		return errNoNativeAudio
	}
	addr := propertyAddress{selMainVolume, scopeOutput, 0}
	if !audioHas(device, &addr) {
		return errNoNativeAudio
	}
	v := float32(volume) / 100
	if serviceSetVolume(device, &addr, 0, nil, 4, unsafe.Pointer(&v)) != 0 {
		return errNoNativeAudio
	}
	return nil
}

func setNativeMuted(muted bool) error {
	if !loadNativeAudio() {
		return errNoNativeAudio
	}
	device, ok := defaultOutput()
	if !ok {
		return errNoNativeAudio
	}
	addr := propertyAddress{selMute, scopeOutput, 0}
	if !audioHas(device, &addr) {
		return errNoNativeAudio
	}
	var v uint32
	if muted {
		v = 1
	}
	if audioSet(device, &addr, 0, nil, 4, unsafe.Pointer(&v)) != 0 {
		return errNoNativeAudio
	}
	return nil
}
