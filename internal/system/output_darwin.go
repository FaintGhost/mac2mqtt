//go:build darwin

package system

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	selName   = fourCC("lnam") // kAudioObjectPropertyName
	selHidden = fourCC("hidn") // kAudioDevicePropertyIsHidden

	outputOnce         sync.Once
	cfStringGetCString func(str uintptr, buf *byte, size int, encoding uint32) bool
	cfReleaseOutput    func(ref uintptr)

	errNoOutput = errors.New("system: no such audio output")
)

func loadOutputs() bool {
	loadActivity()
	outputOnce.Do(func() {
		if cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&cfStringGetCString, cf, "CFStringGetCString")
			purego.RegisterLibFunc(&cfReleaseOutput, cf, "CFRelease")
		}
	})
	return audioGet != nil && cfStringGetCString != nil
}

// outputDevice is an audio device sound can play through, as the Sound
// settings' Output list shows it.
type outputDevice struct {
	id   uint32
	name string
}

// outputDevices lists the devices sound can play through, in CoreAudio's
// order, leaving out hidden ones and repeated names.
func outputDevices() []outputDevice {
	if !loadOutputs() {
		return nil
	}
	var out []outputDevice
	seen := map[string]bool{}
	for _, id := range audioDevices() {
		if !isOutputDevice(id) || isHidden(id) {
			continue
		}
		name := deviceName(id)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, outputDevice{id, name})
	}
	return out
}

func isOutputDevice(device uint32) bool {
	addr := propertyAddress{selStreams, scopeOutput, 0}
	var size uint32
	return audioGetSize(device, &addr, 0, nil, &size) == 0 && size > 0
}

func isHidden(device uint32) bool {
	addr := propertyAddress{selHidden, scopeGlobal, 0}
	var hidden uint32
	size := uint32(4)
	return audioGet(device, &addr, 0, nil, &size, unsafe.Pointer(&hidden)) == 0 && hidden != 0
}

func deviceName(device uint32) string {
	addr := propertyAddress{selName, scopeGlobal, 0}
	var ref uintptr
	size := uint32(unsafe.Sizeof(ref))
	if audioGet(device, &addr, 0, nil, &size, unsafe.Pointer(&ref)) != 0 || ref == 0 {
		return ""
	}
	defer cfReleaseOutput(ref)
	var buf [256]byte
	if !cfStringGetCString(ref, &buf[0], len(buf), cfStringEncodingUTF8) {
		return ""
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf[:])
}

// readOutputs reads the devices sound can play through, and the one it
// plays through now.
func readOutputs() (current string, names []string) {
	devices := outputDevices()
	def, _ := defaultOutput()
	for _, d := range devices {
		names = append(names, d.name)
		if d.id == def {
			current = d.name
		}
	}
	return current, names
}

// setOutput makes sound play through the device of that name, as choosing
// it in the Sound settings does.
func setOutput(name string) error {
	if !loadNativeAudio() || !loadOutputs() {
		return errNoOutput
	}
	for _, d := range outputDevices() {
		if d.name != name {
			continue
		}
		addr := propertyAddress{selDefaultOutput, scopeGlobal, 0}
		id := d.id
		if audioSet(systemObject, &addr, 0, nil, 4, unsafe.Pointer(&id)) != 0 {
			return errors.New("system: the audio output could not be changed")
		}
		return nil
	}
	return errNoOutput
}
