package system

import (
	"context"
	"strconv"
	"strings"
)

// parsedPower is the subset of pmset information we collect.
type parsedPower struct {
	source         string
	batteryPercent *float64
	charging       *bool
}

// collectPower reads pmset on macOS. On other platforms it falls back to the
// runtime-provided power state without executing a command.
func (c *Collector) collectPower(ctx context.Context, rt Runtime) PowerInfo {
	info := PowerInfo{
		OnBattery: rt.OnBattery,
		Source:    sourceFromRuntime(rt.OnBattery),
	}
	if c.goos != "darwin" {
		return info
	}

	output, err := runCommand(ctx, c.runner, "/usr/bin/pmset", "-g", "batt")
	if err != nil {
		return info
	}

	parsed := parsePMSet(string(output))
	if parsed.source != "" {
		info.Source = parsed.source
	}
	info.BatteryPercent = parsed.batteryPercent
	info.Charging = parsed.charging
	return info
}

func sourceFromRuntime(onBattery bool) string {
	if onBattery {
		return "Battery"
	}
	return "AC"
}

// parsePMSet parses the human-readable output from "pmset -g batt". It is
// deliberately tolerant: unknown or truncated output yields nil optional
// fields rather than a panic.
func parsePMSet(output string) parsedPower {
	var result parsedPower

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if strings.Contains(lower, "now drawing from") {
			switch {
			case strings.Contains(lower, "battery"):
				result.source = "Battery"
			case strings.Contains(lower, "ac"):
				result.source = "AC"
			}
			continue
		}
		if !strings.Contains(trimmed, "%") || !strings.Contains(lower, "battery") {
			continue
		}

		percent, status, ok := parseBatteryLine(trimmed)
		if !ok {
			continue
		}
		percentCopy := percent
		result.batteryPercent = &percentCopy
		if charging, known := parseChargingStatus(status); known {
			chargingCopy := charging
			result.charging = &chargingCopy
		}
		break
	}

	return result
}

func parseBatteryLine(line string) (float64, string, bool) {
	percentIndex := strings.IndexByte(line, '%')
	if percentIndex < 0 {
		return 0, "", false
	}

	start := percentIndex
	for start > 0 {
		ch := line[start-1]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			start--
			continue
		}
		break
	}
	if start == percentIndex {
		return 0, "", false
	}

	percent, err := strconv.ParseFloat(line[start:percentIndex], 64)
	if err != nil {
		return 0, "", false
	}

	rest := strings.TrimSpace(line[percentIndex+1:])
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ";"))
	status := rest
	if index := strings.IndexByte(status, ';'); index >= 0 {
		status = status[:index]
	}
	return percent, strings.TrimSpace(status), true
}

func parseChargingStatus(status string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "charging", "finishing charge":
		return true, true
	case "discharging", "charged", "ac attached", "not charging":
		return false, true
	default:
		return false, false
	}
}
