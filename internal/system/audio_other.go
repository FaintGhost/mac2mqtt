//go:build !darwin

package system

func readNativeAudio() (*AudioState, error) { return nil, ErrUnsupported }
func setNativeVolume(int) error             { return ErrUnsupported }
func setNativeMuted(bool) error             { return ErrUnsupported }

func readNativeScreenLocked() (bool, error) { return false, ErrUnsupported }

func readOutputs() (string, []string) { return "", nil }
func setOutput(string) error          { return ErrUnsupported }
