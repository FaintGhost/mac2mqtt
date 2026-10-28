//go:build darwin

package system

import (
	"encoding/binary"
	"errors"
	"syscall"
	"time"
)

// readBootTime reads when this Mac started, as kern.boottime holds it: a
// timeval whose first eight bytes are the seconds since the epoch.
func readBootTime() (time.Time, error) {
	value, err := syscall.Sysctl("kern.boottime")
	if err != nil {
		return time.Time{}, err
	}
	if len(value) < 8 {
		return time.Time{}, errors.New("kern.boottime is too short")
	}
	return time.Unix(int64(binary.LittleEndian.Uint64([]byte(value[:8]))), 0), nil
}
