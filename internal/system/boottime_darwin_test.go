//go:build darwin

package system

import (
	"testing"
	"time"
)

func TestReadBootTime(t *testing.T) {
	boot, err := readBootTime()
	if err != nil {
		t.Fatal(err)
	}
	// Before now, and after macOS 13 came out.
	if !boot.Before(time.Now()) || boot.Year() < 2022 {
		t.Fatalf("boot time = %v", boot)
	}
}
