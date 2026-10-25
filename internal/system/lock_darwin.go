//go:build darwin

package system

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
)

var (
	lockOnce sync.Once
	lockFunc func()
)

// lockScreenNow locks the screen at once, as Control Center's Lock Screen
// does, through the login framework's SACLockScreenImmediate.
func lockScreenNow() error {
	lockOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/PrivateFrameworks/login.framework/Versions/Current/login", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			purego.RegisterLibFunc(&lockFunc, lib, "SACLockScreenImmediate")
		}
	})
	if lockFunc == nil {
		return errors.New("locking the screen is unavailable")
	}
	lockFunc()
	return nil
}
