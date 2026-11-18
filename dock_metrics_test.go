package main

import (
	"strings"
	"testing"
)

func TestDockMetricsRemainVisible(t *testing.T) {
	oldW, oldH := logicalWidth, logicalHeight
	defer func() { logicalWidth, logicalHeight = oldW, oldH }()
	for _, w := range []int32{640, 800, 939, 940, 999, 1000, 1120} {
		logicalWidth = w
		logicalHeight = 820
		right := w - 28 + contentShift()
		state, status, fps := dockHeaderRects(right)
		if fps.Right-fps.Left < 150 || fps.Right-contentShift() > w-28 || state.Right > fps.Left || status.Right > fps.Left {
			t.Fatalf("width %d hides/overlaps FPS: %v %v %v", w, state, status, fps)
		}
		hint := dockHoverHintRect(right)
		if hint.Left <= state.Left+60 || hint.Right > fps.Left || hint.Right-hint.Left < 120 {
			t.Fatalf("width %d hover hint overlaps left state or FPS: state=%v hint=%v fps=%v", w, state, hint, fps)
		}
	}
}
func TestDockMetricsReflectState(t *testing.T) {
	oldRunning, oldGPU, oldEvent, oldFrozen, oldBlack := outputRunning, gpuOwnsOutput, gpuLastEvent, frozen, blackout
	oldAwait, oldPresented, oldFPS := gdiAwaitLive, gdiPresented, measuredFPS
	defer func() {
		outputRunning, gpuOwnsOutput, gpuLastEvent, frozen, blackout = oldRunning, oldGPU, oldEvent, oldFrozen, oldBlack
		gdiAwaitLive, gdiPresented, measuredFPS = oldAwait, oldPresented, oldFPS
	}()
	outputRunning = true
	gpuOwnsOutput = true
	frozen = false
	blackout = false
	gpuLastEvent = projectionEvent{State: projectionLive, PresentFPS: 59.8}
	if got := dockMetricText(); got != "60 FPS · GPU" {
		t.Fatal(got)
	}
	gpuLastEvent.State = projectionStarting
	if got := dockMetricText(); strings.Contains(got, "60") {
		t.Fatal("stale live rate", got)
	}
	frozen = true
	if got := dockMetricText(); !strings.Contains(got, "冻结") {
		t.Fatal(got)
	}
	frozen = false
	blackout = true
	if got := dockMetricText(); !strings.Contains(got, "黑屏") {
		t.Fatal(got)
	}
	blackout = false
	gpuOwnsOutput = false
	gdiAwaitLive = false
	gdiPresented = true
	measuredFPS = 28.7
	if got := dockMetricText(); got != "29 FPS · GDI" {
		t.Fatal(got)
	}
	outputRunning = false
	if got := dockMetricText(); got != "— FPS" {
		t.Fatal(got)
	}
}
