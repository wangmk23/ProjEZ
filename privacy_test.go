package main

import (
	"strings"
	"testing"
)

func TestPrivacyRequiresExplicitResume(t *testing.T) {
	p := privacyGuard{enabled: true, target: 10, pid: 20}
	if !p.allows(10, 20, 10, true) || !p.allows(11, 20, 10, true) {
		t.Fatal("selected window and owned modal must be allowed")
	}
	if p.allows(12, 20, 12, true) || p.allows(10, 21, 10, true) || p.allows(10, 20, 10, false) || p.allows(0, 0, 0, true) {
		t.Fatal("unselected window allowed")
	}
	if !p.observe(12, 20, 12, true) || !p.latched {
		t.Fatal("switch did not latch")
	}
	if p.observe(10, 20, 10, true) || !p.latched {
		t.Fatal("return automatically cleared protection")
	}
	if p.resume(12, 20, 12, true) || !p.latched {
		t.Fatal("resume outside selected window")
	}
	if !p.resume(10, 20, 10, true) || p.latched {
		t.Fatal("explicit resume rejected")
	}
	p.enabled = false
	if !p.resume(0, 0, 0, false) {
		t.Fatal("disabled protection blocked normal use")
	}
}
func TestPreviewRefreshPreference(t *testing.T) {
	if previewRate(parsePreferences("")) != 30 {
		t.Fatal("default preview too slow")
	}
	p := parsePreferences("PreviewEconomy=true")
	if previewRate(parsePreferences(strings.Join(preferenceLines(p), "\n"))) != 5 {
		t.Fatal("preference not persisted")
	}
	if previewRate(p) != 5 {
		t.Fatal("economy mode lost")
	}
}
