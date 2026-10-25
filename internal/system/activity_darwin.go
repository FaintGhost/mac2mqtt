//go:build darwin

package system

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// propertyAddress is CoreAudio's AudioObjectPropertyAddress, and
// CoreMediaIO's CMIOObjectPropertyAddress, which is laid out alike.
type propertyAddress struct {
	selector, scope, element uint32
}

func fourCC(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

var (
	selDevices          = fourCC("dev#") // the hardware's devices
	selRunningSomewhere = fourCC("gone") // a device some process uses
	selStreams          = fourCC("stm#")
	scopeGlobal         = fourCC("glob")
	scopeInput          = fourCC("inpt")
)

const systemObject = 1 // kAudioObjectSystemObject, kCMIOObjectSystemObject

var (
	activityOnce sync.Once

	audioGetSize func(id uint32, addr *propertyAddress, qualifierSize uint32, qualifier unsafe.Pointer, size *uint32) int32
	audioGet     func(id uint32, addr *propertyAddress, qualifierSize uint32, qualifier unsafe.Pointer, size *uint32, data unsafe.Pointer) int32
	cmioGetSize  func(id uint32, addr *propertyAddress, qualifierSize uint32, qualifier unsafe.Pointer, size *uint32) int32
	cmioGet      func(id uint32, addr *propertyAddress, qualifierSize uint32, qualifier unsafe.Pointer, size uint32, used *uint32, data unsafe.Pointer) int32

	mainDisplayID   func() uint32
	displayIsAsleep func(display uint32) bool
)

func loadActivity() {
	activityOnce.Do(func() {
		if lib, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&audioGetSize, lib, "AudioObjectGetPropertyDataSize")
			purego.RegisterLibFunc(&audioGet, lib, "AudioObjectGetPropertyData")
		}
		if lib, err := purego.Dlopen("/System/Library/Frameworks/CoreMediaIO.framework/CoreMediaIO", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&cmioGetSize, lib, "CMIOObjectGetPropertyDataSize")
			purego.RegisterLibFunc(&cmioGet, lib, "CMIOObjectGetPropertyData")
		}
		if lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&mainDisplayID, lib, "CGMainDisplayID")
			purego.RegisterLibFunc(&displayIsAsleep, lib, "CGDisplayIsAsleep")
		}
	})
}

// readActivity reads the activity through CoreAudio, CoreMediaIO and
// CoreGraphics. Asking whether a device is in use needs no permission:
// nothing is recorded.
func readActivity() Activity {
	loadActivity()
	var a Activity
	if audioGet != nil {
		for _, device := range audioDevices() {
			if isInputDevice(device) && audioRunning(device) {
				a.MicrophoneInUse = true
				break
			}
		}
	}
	if cmioGet != nil {
		for _, device := range cameraDevices() {
			if cameraRunning(device) {
				a.CameraInUse = true
				break
			}
		}
	}
	if displayIsAsleep != nil {
		a.DisplayAsleep = displayIsAsleep(mainDisplayID())
	}
	return a
}

func audioDevices() []uint32 {
	addr := propertyAddress{selDevices, scopeGlobal, 0}
	var size uint32
	if audioGetSize(systemObject, &addr, 0, nil, &size) != 0 || size == 0 {
		return nil
	}
	devices := make([]uint32, size/4)
	if audioGet(systemObject, &addr, 0, nil, &size, unsafe.Pointer(&devices[0])) != 0 {
		return nil
	}
	return devices[:size/4]
}

func isInputDevice(device uint32) bool {
	addr := propertyAddress{selStreams, scopeInput, 0}
	var size uint32
	return audioGetSize(device, &addr, 0, nil, &size) == 0 && size > 0
}

func audioRunning(device uint32) bool {
	addr := propertyAddress{selRunningSomewhere, scopeGlobal, 0}
	var running uint32
	size := uint32(4)
	return audioGet(device, &addr, 0, nil, &size, unsafe.Pointer(&running)) == 0 && running != 0
}

func cameraDevices() []uint32 {
	addr := propertyAddress{selDevices, scopeGlobal, 0}
	var size uint32
	if cmioGetSize(systemObject, &addr, 0, nil, &size) != 0 || size == 0 {
		return nil
	}
	devices := make([]uint32, size/4)
	var used uint32
	if cmioGet(systemObject, &addr, 0, nil, size, &used, unsafe.Pointer(&devices[0])) != 0 {
		return nil
	}
	return devices[:used/4]
}

func cameraRunning(device uint32) bool {
	addr := propertyAddress{selRunningSomewhere, scopeGlobal, 0}
	var running, used uint32
	return cmioGet(device, &addr, 0, nil, 4, &used, unsafe.Pointer(&running)) == 0 && running != 0
}
