//go:build darwin

package system

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
)

var (
	sessionOnce sync.Once

	cgSessionCopyCurrentDictionary func() uintptr
	cfDictionaryGetValue           func(dict, key uintptr) uintptr
	cfStringCreateWithCString      func(alloc uintptr, s string, encoding uint32) uintptr
	cfBooleanGetValue              func(b uintptr) bool
	cfRelease                      func(ref uintptr)

	keyScreenIsLocked, keyOnConsole uintptr
)

const cfStringEncodingUTF8 = 0x08000100

func loadSession() bool {
	sessionOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&cgSessionCopyCurrentDictionary, cg, "CGSessionCopyCurrentDictionary")
		purego.RegisterLibFunc(&cfDictionaryGetValue, cf, "CFDictionaryGetValue")
		purego.RegisterLibFunc(&cfStringCreateWithCString, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfBooleanGetValue, cf, "CFBooleanGetValue")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		// Made once and kept, as constants.
		keyScreenIsLocked = cfStringCreateWithCString(0, "CGSSessionScreenIsLocked", cfStringEncodingUTF8)
		keyOnConsole = cfStringCreateWithCString(0, "kCGSSessionOnConsoleKey", cfStringEncodingUTF8)
	})
	return keyScreenIsLocked != 0 && keyOnConsole != 0
}

// readNativeScreenLocked asks the window server for this session's lock
// state, as ioreg's console users tell it, without starting a process.
func readNativeScreenLocked() (bool, error) {
	if !loadSession() {
		return false, errors.New("system: no session information")
	}
	dict := cgSessionCopyCurrentDictionary()
	if dict == 0 {
		return false, errors.New("system: no window server session")
	}
	defer cfRelease(dict)
	if v := cfDictionaryGetValue(dict, keyScreenIsLocked); v != 0 && cfBooleanGetValue(v) {
		return true, nil
	}
	// A session another user switched away from is locked too.
	if v := cfDictionaryGetValue(dict, keyOnConsole); v != 0 && !cfBooleanGetValue(v) {
		return true, nil
	}
	return false, nil
}
