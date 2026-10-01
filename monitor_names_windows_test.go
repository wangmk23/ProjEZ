package main

import (
	"strings"
	"testing"
	"unsafe"
)

func TestDisplayNameNativeLayout(t *testing.T) {
	if unsafe.Sizeof(displayPath{}) != 72 || unsafe.Sizeof(displaySourceName{}) != 84 || unsafe.Sizeof(displayTargetName{}) != 420 {
		t.Fatal("DisplayConfig ABI mismatch")
	}
	if unsafe.Offsetof(displayPath{}.TargetAdapter) != 20 || unsafe.Offsetof(displayTargetName{}.Name) != 36 {
		t.Fatal("DisplayConfig offsets mismatch")
	}
}
func TestMonitorChoiceLabel(t *testing.T) {
	m := Monitor{Device: `\\.\DISPLAY2`, Name: "DELL U2723QE", Rect: RECT{-2560, 0, 0, 1440}}
	for _, part := range []string{"DELL U2723QE", "DISPLAY2", "2560 × 1440", "副屏"} {
		if !strings.Contains(monitorChoiceLabel(m), part) {
			t.Fatal(monitorChoiceLabel(m))
		}
	}
	m.Name = ""
	m.Primary = true
	if got := monitorChoiceLabel(m); got != "DISPLAY2 · 2560 × 1440 · 主屏" {
		t.Fatal(got)
	}
}
func TestReadConnectedMonitorNames(t *testing.T) {
	for device, name := range connectedMonitorNames() {
		if device == "" || name == "" {
			t.Fatal("invalid mapping")
		}
		t.Logf("%s: %s", device, name)
	}
}
