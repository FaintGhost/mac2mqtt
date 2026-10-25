package system

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	outputVolumePattern = regexp.MustCompile(`(?i)output\s+volume\s*:\s*(\d+)`)
	outputMutedPattern  = regexp.MustCompile(`(?i)output\s+muted\s*:\s*(true|false)`)
)

// readAudio reads the output volume and mute state through osascript.
func readAudio(ctx context.Context, runner commandRunner) (*AudioState, error) {
	output, err := runCommand(ctx, runner, "/usr/bin/osascript", "-e", "get volume settings")
	if err != nil {
		return nil, fmt.Errorf("system: read audio settings: %w", err)
	}

	state, err := parseAudioSettings(string(output))
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func parseAudioSettings(output string) (AudioState, error) {
	volumeMatch := outputVolumePattern.FindStringSubmatch(output)
	mutedMatch := outputMutedPattern.FindStringSubmatch(output)
	if volumeMatch == nil || mutedMatch == nil {
		return AudioState{}, fmt.Errorf("system: malformed audio settings %q", output)
	}

	volume, err := strconv.Atoi(volumeMatch[1])
	if err != nil {
		return AudioState{}, fmt.Errorf("system: parse audio volume: %w", err)
	}

	return AudioState{
		Volume: volume,
		Muted:  strings.EqualFold(mutedMatch[1], "true"),
	}, nil
}
