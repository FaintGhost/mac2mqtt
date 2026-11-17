//go:build darwin

package system

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unicode/utf16"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	cgPreflightPostEventAccess      func() bool
	cgRequestPostEventAccess        func() bool
	cgEventSourceCreate             func(state int32) uintptr
	cgEventCreateKeyboardEvent      func(source uintptr, key uint16, down bool) uintptr
	cgEventKeyboardSetUnicodeString func(event uintptr, length uint64, s *uint16)
	cgEventSetFlags                 func(event uintptr, flags uint64)
	cgEventPost                     func(tap uint32, event uintptr)
	keyEventSource                  uintptr

	axIsProcessTrusted                                func() bool
	axUIElementCreateApplication                      func(pid int32) uintptr
	axUIElementCopyAttributeValue                     func(element, attribute uintptr, value *uintptr) int32
	axUIElementSetAttributeValue                      func(element, attribute, value uintptr) int32
	axUIElementSetMessagingTimeout                    func(element uintptr, seconds float32) int32
	axUIElementGetTypeID                              func() uint64
	cfGetTypeID                                       func(value uintptr) uint64
	cfEqual                                           func(a, b uintptr) bool
	cfStringCreateWithCharacters                      func(allocator uintptr, chars *uint16, length int64) uintptr
	cfStringGetLength                                 func(uintptr) int64
	cfStringGetTypeID                                 func() uint64
	cfAttributedStringGetTypeID                       func() uint64
	cfAttributedStringGetString                       func(uintptr) uintptr
	cfComposedCharacterRange                          uintptr
	axFocusedElement, axRole, axSubrole, axValue      uintptr
	axTextField, axSecureTextField, loginWindowBundle uintptr
)

// Isolate synthetic events from physical Command, Shift and Caps Lock state.
var loadKeys = sync.OnceValue(func() bool {
	if !loadSession() {
		return false
	}
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	purego.RegisterLibFunc(&cgPreflightPostEventAccess, cg, "CGPreflightPostEventAccess")
	purego.RegisterLibFunc(&cgRequestPostEventAccess, cg, "CGRequestPostEventAccess")
	purego.RegisterLibFunc(&cgEventSourceCreate, cg, "CGEventSourceCreate")
	purego.RegisterLibFunc(&cgEventCreateKeyboardEvent, cg, "CGEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&cgEventKeyboardSetUnicodeString, cg, "CGEventKeyboardSetUnicodeString")
	purego.RegisterLibFunc(&cgEventSetFlags, cg, "CGEventSetFlags")
	purego.RegisterLibFunc(&cgEventPost, cg, "CGEventPost")
	keyEventSource = cgEventSourceCreate(-1) // kCGEventSourceStatePrivate; kept for the process.
	return keyEventSource != 0
})

var loadLoginAX = sync.OnceValue(func() bool {
	if !loadSession() {
		return false
	}
	ax, err := purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return false
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	purego.RegisterLibFunc(&axIsProcessTrusted, ax, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&axUIElementCreateApplication, ax, "AXUIElementCreateApplication")
	purego.RegisterLibFunc(&axUIElementCopyAttributeValue, ax, "AXUIElementCopyAttributeValue")
	purego.RegisterLibFunc(&axUIElementSetAttributeValue, ax, "AXUIElementSetAttributeValue")
	purego.RegisterLibFunc(&axUIElementSetMessagingTimeout, ax, "AXUIElementSetMessagingTimeout")
	purego.RegisterLibFunc(&axUIElementGetTypeID, ax, "AXUIElementGetTypeID")
	purego.RegisterLibFunc(&cfGetTypeID, cf, "CFGetTypeID")
	purego.RegisterLibFunc(&cfEqual, cf, "CFEqual")
	purego.RegisterLibFunc(&cfStringCreateWithCharacters, cf, "CFStringCreateWithCharacters")
	purego.RegisterLibFunc(&cfStringGetLength, cf, "CFStringGetLength")
	purego.RegisterLibFunc(&cfStringGetTypeID, cf, "CFStringGetTypeID")
	purego.RegisterLibFunc(&cfAttributedStringGetTypeID, cf, "CFAttributedStringGetTypeID")
	purego.RegisterLibFunc(&cfAttributedStringGetString, cf, "CFAttributedStringGetString")
	cfComposedCharacterRange, err = purego.Dlsym(cf, "CFStringGetRangeOfComposedCharactersAtIndex")
	if err != nil {
		return false
	}
	// Constants live for the process, independently of any autorelease pool.
	axFocusedElement = cfStringCreateWithCString(0, "AXFocusedUIElement", cfStringEncodingUTF8)
	axRole = cfStringCreateWithCString(0, "AXRole", cfStringEncodingUTF8)
	axSubrole = cfStringCreateWithCString(0, "AXSubrole", cfStringEncodingUTF8)
	axValue = cfStringCreateWithCString(0, "AXValue", cfStringEncodingUTF8)
	axTextField = cfStringCreateWithCString(0, "AXTextField", cfStringEncodingUTF8)
	axSecureTextField = cfStringCreateWithCString(0, "AXSecureTextField", cfStringEncodingUTF8)
	loginWindowBundle = cfStringCreateWithCString(0, "com.apple.loginwindow", cfStringEncodingUTF8)
	return axFocusedElement != 0 && axRole != 0 && axSubrole != 0 && axValue != 0 &&
		axTextField != 0 && axSecureTextField != 0 && loginWindowBundle != 0
})

type nativeTyper struct{}

func (nativeTyper) Allowed() bool {
	return loadKeys() && cgPreflightPostEventAccess() && loadLoginAX() && axIsProcessTrusted()
}

func requestKeyAccess() bool { return loadKeys() && cgRequestPostEventAccess() }

func (nativeTyper) Type(r rune) error {
	s := utf16.Encode([]rune{r})
	defer clear(s)
	return postKeyPair(0, 0, s)
}

func (nativeTyper) Key(code uint16, flags uint64) error {
	return postKeyPair(code, flags, nil)
}

func postKeyPair(code uint16, flags uint64, text []uint16) error {
	if !loadKeys() {
		return ErrNoKeyAccess
	}
	down := cgEventCreateKeyboardEvent(keyEventSource, code, true)
	if down == 0 {
		return errors.New("system: could not create key-down event")
	}
	defer cfRelease(down)
	up := cgEventCreateKeyboardEvent(keyEventSource, code, false)
	if up == 0 {
		return errors.New("system: could not create key-up event")
	}
	defer cfRelease(up)
	cgEventSetFlags(down, flags)
	cgEventSetFlags(up, 0)
	if len(text) != 0 {
		cgEventKeyboardSetUnicodeString(down, uint64(len(text)), &text[0])
		cgEventKeyboardSetUnicodeString(up, uint64(len(text)), &text[0])
	}
	// kCGHIDEventTap; both events exist before either is posted.
	cgEventPost(0, down)
	cgEventPost(0, up)
	return nil
}

func (nativeTyper) PasswordField() (passwordField, error) {
	if !loadLoginAX() || !axIsProcessTrusted() {
		return nil, ErrNoKeyAccess
	}
	// NSRunningApplication's results are autoreleased on this thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	apps := objc.ID(objc.GetClass("NSRunningApplication")).Send(objc.RegisterName("runningApplicationsWithBundleIdentifier:"), loginWindowBundle)
	count := apps.Send(objc.RegisterName("count"))
	for i := uintptr(0); i < uintptr(count); i++ {
		app := apps.Send(objc.RegisterName("objectAtIndex:"), i)
		pid := int32(app.Send(objc.RegisterName("processIdentifier")))
		if pid <= 0 {
			continue
		}
		application := axUIElementCreateApplication(pid)
		if application == 0 {
			continue
		}
		// A hung loginwindow must not hold the MQTT command worker forever.
		if axUIElementSetMessagingTimeout(application, 0.2) != 0 {
			cfRelease(application)
			continue
		}
		field, err := copyFocusedPasswordField(application)
		if err == nil {
			return &nativePasswordField{application: application, element: field}, nil
		}
		cfRelease(application)
		if errors.Is(err, ErrNoKeyAccess) {
			return nil, err
		}
	}
	return nil, ErrNoPasswordField
}

type nativePasswordField struct {
	application uintptr
	element     uintptr
}

func copyFocusedPasswordField(application uintptr) (uintptr, error) {
	var field uintptr
	code := axUIElementCopyAttributeValue(application, axFocusedElement, &field)
	if code != 0 || field == 0 {
		if field != 0 {
			cfRelease(field)
		}
		if code == -25211 { // kAXErrorAPIDisabled
			return 0, ErrNoKeyAccess
		}
		return 0, ErrNoPasswordField
	}
	if cfGetTypeID(field) != axUIElementGetTypeID() ||
		axUIElementSetMessagingTimeout(field, 0.2) != 0 ||
		!axAttributeEquals(field, axRole, axTextField) ||
		!axAttributeEquals(field, axSubrole, axSecureTextField) {
		cfRelease(field)
		return 0, ErrNoPasswordField
	}
	return field, nil
}

func axAttributeEquals(element, attribute, expected uintptr) bool {
	var value uintptr
	code := axUIElementCopyAttributeValue(element, attribute, &value)
	if value == 0 {
		return false
	}
	defer cfRelease(value)
	return code == 0 && cfEqual(value, expected)
}

func (f *nativePasswordField) Focused() (bool, error) {
	field, err := copyFocusedPasswordField(f.application)
	if err != nil {
		return false, err
	}
	defer cfRelease(field)
	return cfEqual(field, f.element), nil
}

func makePasswordValue(password string) uintptr {
	chars := utf16.Encode([]rune(password))
	defer clear(chars)
	var ptr *uint16
	if len(chars) != 0 {
		ptr = &chars[0]
	}
	return cfStringCreateWithCharacters(0, ptr, int64(len(chars)))
}

func (f *nativePasswordField) SetValue(password string) error {
	value := makePasswordValue(password)
	if value == 0 {
		return errors.New("system: could not create password field value")
	}
	defer cfRelease(value)
	code := axUIElementSetAttributeValue(f.element, axValue, value)
	if code == -25211 {
		return ErrNoKeyAccess
	}
	if code != 0 {
		return fmt.Errorf("system: cannot set password field (AX error %d)", code)
	}
	return nil
}

// ValueHasLength checks only the number of composed characters, as secure
// fields may expose bullets instead of the password. No field contents are
// copied to Go or logged. loginwindow can return a string or attributed string.
func (f *nativePasswordField) ValueHasLength(password string) (bool, error) {
	var value uintptr
	code := axUIElementCopyAttributeValue(f.element, axValue, &value)
	if value != 0 {
		defer cfRelease(value)
	}
	if code == -25211 {
		return false, ErrNoKeyAccess
	}
	if code == -25212 || code == -25204 || value == 0 { // no value yet, or temporarily unresponsive
		return false, nil
	}
	if code != 0 {
		return false, fmt.Errorf("system: cannot confirm password input (AX error %d)", code)
	}
	text := value
	if cfGetTypeID(value) == cfAttributedStringGetTypeID() {
		text = cfAttributedStringGetString(value) // borrowed from value
	}
	if text == 0 || cfGetTypeID(text) != cfStringGetTypeID() {
		return false, ErrPasswordInput
	}
	expected := makePasswordValue(password)
	if expected == 0 {
		return false, ErrPasswordInput
	}
	defer cfRelease(expected)
	actualCount, expectedCount := composedCharacterCount(text), composedCharacterCount(expected)
	return actualCount >= 0 && expectedCount >= 0 && actualCount == expectedCount, nil
}

func composedCharacterCount(text uintptr) int {
	length := uintptr(cfStringGetLength(text))
	count := 0
	for offset := uintptr(0); offset < length; count++ {
		// CFRange is two integer words on both supported macOS architectures.
		start, size, _ := purego.SyscallN(cfComposedCharacterRange, text, offset)
		if size == 0 || start > offset || size > length-start || start+size <= offset {
			return -1
		}
		offset = start + size
	}
	return count
}

func (f *nativePasswordField) Close() {
	cfRelease(f.element)
	cfRelease(f.application)
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
		Locked:    v != 0 && cfBooleanGetValue(v),
		OnConsole: c != 0 && cfBooleanGetValue(c),
	}, nil
}
