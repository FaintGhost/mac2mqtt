package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
)

func TestSinceText(t *testing.T) {
	defer i18n.SetLocaleFunc(func() string { return "en" })
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		locale string
		ago    time.Duration
		want   string
	}{
		{"en", 20 * time.Second, "now"},
		{"en", 5 * time.Minute, "5 minutes ago"},
		{"en", 3*time.Hour + 40*time.Minute, "3 hours ago"},
		{"en", 30 * time.Hour, "yesterday"},
		{"en", 50 * time.Hour, "2 days ago"},
		{"zh-Hans", 5 * time.Minute, "5分钟前"},
		{"zh-Hans", 30 * time.Hour, "昨天"},
	} {
		i18n.SetLocaleFunc(func() string { return tc.locale })
		if got := sinceText(now.Add(-tc.ago), now); got != tc.want {
			t.Errorf("%s, %v ago = %q, want %q", tc.locale, tc.ago, got, tc.want)
		}
	}
}
