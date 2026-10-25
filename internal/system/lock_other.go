//go:build !darwin

package system

func lockScreenNow() error { return ErrUnsupported }
