//go:build darwin

package system

import (
	"errors"
	"sync"
	"unicode/utf16"

	"github.com/ebitengine/purego"
)

var (
	keysOnce sync.Once

	cgPreflightPostEventAccess      func() bool
	cgRequestPostEventAccess        func() bool
	cgEventCreateKeyboardEvent      func(source uintptr, key uint16, down bool) uintptr
	cgEventKeyboardSetUnicodeString func(event uintptr, length uint64, s *uint16)
	cgEventPost                     func(tap uint32, event uintptr)
)

const (
	// kCGHIDEventTap posts events as the keyboard would, which the lock
	// screen takes.
	hidEventTap = 0
	keyReturn   = 36
)

func loadKeys() bool {
	keysOnce.Do(func() {
		if !loadSession() {
			return
		}
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&cgPreflightPostEventAccess, cg, "CGPreflightPostEventAccess")
		purego.RegisterLibFunc(&cgRequestPostEventAccess, cg, "CGRequestPostEventAccess")
		purego.RegisterLibFunc(&cgEventCreateKeyboardEvent, cg, "CGEventCreateKeyboardEvent")
		purego.RegisterLibFunc(&cgEventKeyboardSetUnicodeString, cg, "CGEventKeyboardSetUnicodeString")
		purego.RegisterLibFunc(&cgEventPost, cg, "CGEventPost")
	})
	return cgEventPost != nil
}

// nativeTyper posts key events through Core Graphics.
type nativeTyper struct{}

func (nativeTyper) Allowed() bool { return loadKeys() && cgPreflightPostEventAccess() }

func requestKeyAccess() bool { return loadKeys() && cgRequestPostEventAccess() }

// Type types the character itself rather than a key, so that the keyboard
// layout or an input method cannot change what the field gets.
func (nativeTyper) Type(r rune) {
	s := utf16.Encode([]rune{r})
	for _, down := range []bool{true, false} {
		event := cgEventCreateKeyboardEvent(0, 0, down)
		if event == 0 {
			return
		}
		cgEventKeyboardSetUnicodeString(event, uint64(len(s)), &s[0])
		cgEventPost(hidEventTap, event)
		cfRelease(event)
	}
}

func (nativeTyper) Return() {
	for _, down := range []bool{true, false} {
		if event := cgEventCreateKeyboardEvent(0, keyReturn, down); event != 0 {
			cgEventPost(hidEventTap, event)
			cfRelease(event)
		}
	}
}

func (nativeTyper) State() (screenState, error) {
	if !loadSession() {
		return screenState{}, errors.New("system: no session information")
	}
	dict := cgSessionCopyCurrentDictionary()
	if dict == 0 {
		return screenState{}, errors.New("system: no window server session")
	}
	defer cfRelease(dict)
	v := cfDictionaryGetValue(dict, keyScreenIsLocked)
	c := cfDictionaryGetValue(dict, keyOnConsole)
	return screenState{
		Locked: v != 0 && cfBooleanGetValue(v),
		// Missing only from sessions of old systems, all on the console.
		OnConsole: c == 0 || cfBooleanGetValue(c),
	}, nil
}
