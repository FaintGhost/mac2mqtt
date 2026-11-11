//go:build !darwin

package system

type nativeTyper struct{}

func (nativeTyper) Allowed() bool               { return false }
func (nativeTyper) Type(rune)                   {}
func (nativeTyper) Return()                     {}
func (nativeTyper) State() (screenState, error) { return screenState{}, ErrUnsupported }

func requestKeyAccess() bool { return false }
