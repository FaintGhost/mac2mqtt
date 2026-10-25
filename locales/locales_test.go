package locales

import (
	"testing"

	"github.com/egoist/mygo/i18n"
)

func TestTranslationCatalog(t *testing.T) {
	if got := In("zh-Hans").NavOverview(); got != "概览" {
		t.Errorf("zh-Hans overview = %q", got)
	}
	if got := In("zh-Hant").NavSettings(); got != "設定" {
		t.Errorf("zh-Hant settings = %q", got)
	}
	if got := In("en").SaveReconnect(); got != "Save & Reconnect" {
		t.Errorf("en save = %q", got)
	}
	if got := In("zh-Hans").VersionLabel("0.1.0"); got != "版本 0.1.0" {
		t.Errorf("zh-Hans version = %q", got)
	}
	if got := In("en").IdleMinutes(3); got != "3m idle" {
		t.Errorf("en idle = %q", got)
	}
}

func TestLanguagesRegistered(t *testing.T) {
	have := make(map[string]bool)
	for _, tag := range i18n.Languages() {
		have[tag] = true
	}
	for _, want := range []string{"en", "zh-Hans", "zh-Hant"} {
		if have[want] == false {
			t.Errorf("catalog language %q is not registered: %v", want, i18n.Languages())
		}
	}
}
