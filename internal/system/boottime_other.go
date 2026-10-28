//go:build !darwin

package system

import "time"

func readBootTime() (time.Time, error) { return time.Time{}, ErrUnsupported }
