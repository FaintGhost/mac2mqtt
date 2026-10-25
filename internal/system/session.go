package system

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// readScreenLocked asks the window server, through the I/O Registry's
// console users, whether the session of the user uid is locked. Lock and
// unlock notifications only report changes; this reads the state at launch
// and whenever one was missed.
func readScreenLocked(ctx context.Context, runner commandRunner, uid int) (bool, error) {
	output, err := runCommand(ctx, runner, "/usr/sbin/ioreg", "-n", "Root", "-d1", "-k", "IOConsoleUsers")
	if err != nil {
		return false, fmt.Errorf("system: read console users: %w", err)
	}
	return parseScreenLocked(string(output), uid)
}

// parseScreenLocked reads ioreg's IOConsoleUsers: one dictionary per
// logged-in user. CGSSessionScreenIsLocked appears in a session's while its
// screen is locked; a session off the console, after switching users, is
// locked too.
func parseScreenLocked(output string, uid int) (bool, error) {
	_, users, ok := strings.Cut(output, `"IOConsoleUsers" = (`)
	if !ok {
		return false, errors.New("system: no console users")
	}
	users, _, _ = strings.Cut(users, "})")
	want := `"kCGSSessionUserIDKey"=` + strconv.Itoa(uid)
	for _, user := range strings.Split(users, "},{") {
		fields := strings.Split(strings.Trim(user, "{}"), ",")
		mine := false
		for _, field := range fields {
			if field == want {
				mine = true
			}
		}
		if !mine {
			continue
		}
		for _, field := range fields {
			if field == `"CGSSessionScreenIsLocked"=Yes` || field == `"kCGSSessionOnConsoleKey"=No` {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("system: no console session for uid %d", uid)
}
