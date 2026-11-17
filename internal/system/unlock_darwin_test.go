//go:build darwin

package system

import (
	"errors"
	"os"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/ebitengine/purego"
)

// These tests construct real Core Graphics events but intercept posting:
// no characters or shortcuts reach the desktop.
func TestNativeUnlockKeyEvents(t *testing.T) {
	if !loadKeys() {
		t.Fatal("could not load Core Graphics event APIs")
	}
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(cg)
	var getFlags func(uintptr) uint64
	var getText func(uintptr, uint64, *uint64, *uint16)
	var getType func(uintptr) uint32
	purego.RegisterLibFunc(&getFlags, cg, "CGEventGetFlags")
	purego.RegisterLibFunc(&getText, cg, "CGEventKeyboardGetUnicodeString")
	purego.RegisterLibFunc(&getType, cg, "CGEventGetType")
	oldPost := cgEventPost
	t.Cleanup(func() { cgEventPost = oldPost })
	var flags []uint64
	var texts [][]uint16
	var types []uint32
	cgEventPost = func(tap uint32, event uintptr) {
		if tap != 0 {
			t.Errorf("event tap = %d, want HID", tap)
		}
		flags = append(flags, getFlags(event))
		types = append(types, getType(event))
		var buf [8]uint16
		var length uint64
		getText(event, uint64(len(buf)), &length, &buf[0])
		texts = append(texts, slices.Clone(buf[:length]))
	}
	for _, r := range []rune{'é', '密', '🔐'} {
		flags, texts, types = nil, nil, nil
		if err := (nativeTyper{}).Type(r); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(types, []uint32{10, 11}) || !slices.Equal(flags, []uint64{0, 0}) {
			t.Fatalf("types = %v, flags = %v", types, flags)
		}
		want := utf16.Encode([]rune{r})
		for _, text := range texts {
			if !slices.Equal(text, want) {
				t.Fatalf("Unicode payload = %v, want %v", text, want)
			}
		}
	}
	flags = nil
	if err := (nativeTyper{}).Key(keyA, flagCommand); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(flags, []uint64{flagCommand, 0}) {
		t.Fatalf("select-all modifier not released: %v", flags)
	}
}

func TestNativeUnlockDoesNotPostHalfAKeyPair(t *testing.T) {
	if !loadKeys() {
		t.Fatal("could not load Core Graphics event APIs")
	}
	oldCreate, oldPost := cgEventCreateKeyboardEvent, cgEventPost
	t.Cleanup(func() { cgEventCreateKeyboardEvent, cgEventPost = oldCreate, oldPost })
	cgEventCreateKeyboardEvent = func(source uintptr, key uint16, down bool) uintptr {
		if !down {
			return 0
		}
		return oldCreate(source, key, down)
	}
	posts := 0
	cgEventPost = func(uint32, uintptr) { posts++ }
	if err := (nativeTyper{}).Key(keyReturn, 0); err == nil || posts != 0 {
		t.Fatalf("err = %v, posted %d events despite missing key-up", err, posts)
	}
}

func TestNativePasswordCharacterCount(t *testing.T) {
	if !loadLoginAX() {
		t.Fatal("could not load password field APIs")
	}
	for _, tc := range []struct {
		value string
		count int
	}{{"", 0}, {"ascii", 5}, {"e\u0301", 1}, {"密码🔐", 3}, {"👩‍💻", 1}, {"a\x00b", 3}} {
		value := makePasswordValue(tc.value)
		if value == 0 {
			t.Fatal("could not create CFString")
		}
		count := composedCharacterCount(value)
		cfRelease(value)
		if count != tc.count {
			t.Errorf("composed character count = %d, want %d", count, tc.count)
		}
	}
}

// Opt-in, read-only integration check. Never locks the screen or reads secrets.
func TestNativeUnlockDesktopProbe(t *testing.T) {
	if os.Getenv("MAC2MQTT_TEST_NATIVE_UNLOCK") != "1" {
		t.Skip("set MAC2MQTT_TEST_NATIVE_UNLOCK=1 in an unlocked desktop session")
	}
	native := nativeTyper{}
	if !native.Allowed() {
		t.Fatal(ErrNoKeyAccess)
	}
	state, err := native.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Locked || !state.OnConsole {
		t.Fatal("this read-only probe requires an unlocked console session")
	}
	field, err := native.PasswordField()
	if field != nil {
		field.Close()
		t.Fatal("found a lock-screen password field on an unlocked desktop")
	}
	if !errors.Is(err, ErrNoPasswordField) {
		t.Fatalf("field probe = %v, want ErrNoPasswordField", err)
	}
}
