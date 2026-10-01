package main

import "testing"

func TestControlScreenBoundaries(t *testing.T) {
	a := Monitor{Device: "a", Primary: true, Rect: RECT{0, 0, 2560, 1440}}
	b := Monitor{Device: "b", Rect: RECT{2560, 0, 5120, 1440}}
	if outputPairError(a, b) != "" {
		t.Fatal("valid extended screens rejected")
	}
	if outputPairError(b, a) == "" || outputPairError(a, a) == "" {
		t.Fatal("primary screen could be covered")
	}
	b.Rect = a.Rect
	if outputPairError(a, b) == "" {
		t.Fatal("duplicate desktop accepted")
	}
}
func TestCursorBoundaryReleasedWithoutOverwritingAnotherApp(t *testing.T) {
	oldSet, oldGet := setCursorBoundary, getCursorBoundary
	oldPrefs, oldSource, oldRunning := preferences, srcMonitor, outputRunning
	defer func() {
		setCursorBoundary, getCursorBoundary = oldSet, oldGet
		preferences, srcMonitor, outputRunning = oldPrefs, oldSource, oldRunning
		cursorControlled = false
	}()
	full := virtualDesktopRect()
	current := full
	released := false
	getCursorBoundary = func() (RECT, bool) { return current, true }
	setCursorBoundary = func(r *RECT) bool {
		if r == nil {
			current = full
			released = true
		} else {
			current = *r
		}
		return true
	}
	srcMonitor.Rect = RECT{-1920, 0, 0, 1080}
	preferences.FreeCursor = false
	outputRunning = true
	updateCursorControl()
	if !cursorControlled || current != srcMonitor.Rect {
		t.Fatal("control monitor not confined")
	}
	releaseCursorControl()
	if !released || current != full {
		t.Fatal("stop did not release")
	}
	updateCursorControl()
	current = full // A focus change or another app released the shared clip.
	maintainCursorControl()
	if current != srcMonitor.Rect {
		t.Fatal("released clip was not restored")
	}
	preferences.FreeCursor = true
	releaseCursorControl()
	maintainCursorControl()
	if current != full {
		t.Fatal("explicit mouse release was undone")
	}
	preferences.FreeCursor = false
	updateCursorControl()
	other := RECT{2, 2, 20, 20}
	current = other
	maintainCursorControl()
	if current != other {
		t.Fatal("watchdog replaced another app clip")
	}
	releaseCursorControl()
	if current != other {
		t.Fatal("overwrote another program's restriction")
	}
}

func TestDisplayRefreshRestoresCursorBoundary(t *testing.T) {
	oldSet, oldGet := setCursorBoundary, getCursorBoundary
	oldPrefs, oldSource, oldTarget, oldRunning, oldFrozen, oldMonitors := preferences, srcMonitor, dstMonitor, outputRunning, frozen, monitors
	defer func() {
		setCursorBoundary, getCursorBoundary = oldSet, oldGet
		preferences, srcMonitor, dstMonitor, outputRunning, frozen, monitors = oldPrefs, oldSource, oldTarget, oldRunning, oldFrozen, oldMonitors
		cursorControlled = false
	}()
	srcMonitor = Monitor{Device: "source", Primary: true, Rect: RECT{0, 0, 2560, 1600}}
	dstMonitor = Monitor{Device: "target", Rect: RECT{2560, 0, 5120, 1440}}
	monitors = []Monitor{srcMonitor, dstMonitor}
	full := virtualDesktopRect()
	current := full
	getCursorBoundary = func() (RECT, bool) { return current, true }
	setCursorBoundary = func(r *RECT) bool {
		if r == nil {
			current = full
		} else {
			current = *r
		}
		return true
	}
	outputRunning = true
	frozen = true
	preferences.FreeCursor = false
	updateCursorControl()
	queueDisplayRefresh()
	if current != full {
		t.Fatal("display change must temporarily release old bounds")
	}
	refreshConnectedOutput()
	if current != srcMonitor.Rect {
		t.Fatalf("display refresh lost cursor constraint: %v", current)
	}
}
