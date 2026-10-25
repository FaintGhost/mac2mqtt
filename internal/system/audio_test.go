package system

import (
	"context"
	"errors"
	"testing"
)

func TestParseAudioSettings(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		want      AudioState
		wantError bool
	}{
		{
			name:   "unmuted",
			output: "output volume:50, input volume:75, alert volume:100, output muted:false\n",
			want:   AudioState{Volume: 50, Muted: false},
		},
		{
			name:   "muted with whitespace and case",
			output: "OUTPUT VOLUME: 17, input volume: 75, OUTPUT MUTED: TRUE\n",
			want:   AudioState{Volume: 17, Muted: true},
		},
		{
			name:      "malformed",
			output:    "not apple script output",
			wantError: true,
		},
		{
			name:      "missing mute value",
			output:    "output volume:30, input volume:75",
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseAudioSettings(test.output)
			if test.wantError {
				if err == nil {
					t.Fatal("parseAudioSettings() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAudioSettings() error = %v", err)
			}
			requireEqual(t, "audio state", got, test.want)
		})
	}
}

func TestReadAudioPropagatesCommandError(t *testing.T) {
	sentinel := errors.New("osascript failed")
	runner := &fakeRunner{runFn: func(context.Context, string, ...string) ([]byte, error) {
		return nil, sentinel
	}}

	state, err := readAudio(context.Background(), runner)
	if !errors.Is(err, sentinel) {
		t.Fatalf("readAudio() error = %v, want wrapped sentinel", err)
	}
	if state == nil {
		return
	}
	t.Fatal("readAudio() returned a state with an error")
}
