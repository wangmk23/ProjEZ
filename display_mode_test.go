//go:build windows

package main

import (
	"testing"
	"unsafe"
)

func TestAutomaticRateAndRestoration(t *testing.T) {
	oldGet, oldSet, oldRead := getOutputRates, setOutputFrequency, getOutputMode
	oldPrefs, oldDst, oldRates := preferences, dstMonitor, monitorRates
	defer func() {
		getOutputRates, setOutputFrequency, getOutputMode = oldGet, oldSet, oldRead
		preferences, dstMonitor, monitorRates = oldPrefs, oldDst, oldRates
		restoredDevice = ""
	}()
	dstMonitor.Device = "test-projector"
	rates := displayRates{current: 60, maximum: 120, known: true}
	failChange := false
	getOutputRates = func(string) displayRates { return rates }
	getOutputMode = func(string, uint32) (displayMode, bool) { return displayMode{Frequency: uint32(rates.current)}, true }
	setOutputFrequency = func(_ string, hz uint32) bool {
		if failChange {
			return false
		}
		rates.current = int(hz)
		return true
	}
	preferences.FPS = 0
	if note := configureOutputRate(); note != "" || activeTargetFPS != 120 || rates.current != 120 {
		t.Fatalf("auto highest failed: %s target=%d actual=%d", note, activeTargetFPS, rates.current)
	}
	configureOutputRate() // Reconfiguration must not forget the initial 60 Hz.
	if !restoreRefreshRate() || rates.current != 60 {
		t.Fatal("stop did not restore initial rate")
	}
	failChange = true
	if note := configureOutputRate(); note == "" || activeTargetFPS != 60 {
		t.Fatal("failed mode switch must report fallback and target actual 60 Hz")
	}
	failChange = false
	configureOutputRate()
	failChange = true
	if restoreRefreshRate() || restoredDevice == "" {
		t.Fatal("failed restore lost the information needed to retry")
	}
	failChange = false
	if !restoreRefreshRate() || rates.current != 60 {
		t.Fatal("restore retry failed")
	}
	configureOutputRate()
	rates.current = 90 // User independently changed Windows settings.
	observeExternalRate(dstMonitor.Device, rates)
	rates.current = 120 // User subsequently selects the same rate the app had used.
	if !restoreRefreshRate() || rates.current != 120 {
		t.Fatal("restore overwrote an observed external display change")
	}
	rates.current = 90
	if !restoreRefreshRate() || rates.current != 90 {
		t.Fatal("restore overwrote an external display change")
	}
	preferences.FPS = -1
	configureOutputRate()
	if activeTargetFPS != 90 || rates.current != 90 {
		t.Fatal("follow-current mode changed the screen")
	}
	preferences.FPS = 30
	configureOutputRate()
	if activeTargetFPS != 30 || rates.current != 90 {
		t.Fatal("manual FPS cap changed screen refresh rate")
	}
}

func TestHighestRatePreservesResolutionAndProgressiveScan(t *testing.T) {
	current := displayMode{Width: 1920, Height: 1080, BitsPerPel: 32, Frequency: 60}
	modes := []displayMode{
		{Width: 1920, Height: 1080, BitsPerPel: 32, Frequency: 120},
		{Width: 1280, Height: 720, BitsPerPel: 32, Frequency: 240},
		{Width: 1920, Height: 1080, BitsPerPel: 32, Frequency: 144, Flags: 2},
		{Width: 1920, Height: 1080, BitsPerPel: 16, Frequency: 240},
		{Width: 1920, Height: 1080, BitsPerPel: 32, Frequency: 240, Orientation: 1},
	}
	if got := maximumModeRate(current, modes); got != 120 {
		t.Fatalf("got %d Hz, want 120 without losing resolution/color/progressive scan", got)
	}
	if got := maximumModeRate(displayMode{}, nil); got != 60 {
		t.Fatalf("unknown display fallback: %d", got)
	}
}

func TestReadCurrentDisplayMode(t *testing.T) {
	if unsafe.Sizeof(displayMode{}) != 220 {
		t.Fatal("DEVMODEW ABI size mismatch")
	}
	dm, ok := readDisplayMode(`\\.\DISPLAY1`, ^uint32(0))
	if !ok {
		t.Skip("DISPLAY1 not attached")
	}
	if dm.Width == 0 || dm.Height == 0 || dm.BitsPerPel != 32 {
		t.Fatalf("invalid display data: %+v", dm)
	}
	r := queryDisplayRates(`\\.\DISPLAY1`)
	if r.maximum < r.current {
		t.Fatalf("max=%d below current=%d", r.maximum, r.current)
	}
	t.Logf("read-only display probe: %dx%d, current %d Hz, maximum %d Hz", dm.Width, dm.Height, r.current, r.maximum)
}
