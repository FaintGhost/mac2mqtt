//go:build !darwin

package system

type nativeTyper struct{}

func (nativeTyper) Allowed() bool                         { return false }
func (nativeTyper) Type(rune) error                       { return ErrUnsupported }
func (nativeTyper) Key(uint16, uint64) error              { return ErrUnsupported }
func (nativeTyper) State() (screenState, error)           { return screenState{}, ErrUnsupported }
func (nativeTyper) PasswordField() (passwordField, error) { return nil, ErrUnsupported }

func requestKeyAccess() bool { return false }
