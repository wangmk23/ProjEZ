package main

import (
	"strings"
	"testing"
	"unsafe"
)

func TestTrayPreferencePersistence(t *testing.T) {
	p := Preferences{CloseToTray: true}
	if got := parsePreferences(strings.Join(preferenceLines(p), "\n")); !got.CloseToTray {
		t.Fatal("tray close choice not persisted")
	}
	if parsePreferences("").CloseToTray {
		t.Fatal("upgrade must preserve previous exit behavior")
	}
}
func TestTrayCloseFailureDoesNotExitOrStopProjection(t *testing.T) {
	oldP, oldHide, oldExit, oldRunning := preferences, hideControlToTray, exitApplication, outputRunning
	defer func() {
		preferences, hideControlToTray, exitApplication, outputRunning = oldP, oldHide, oldExit, oldRunning
	}()
	outputRunning = true
	preferences.CloseToTray = true
	hidden, exited := 0, 0
	hideControlToTray = func() bool { hidden++; return false }
	exitApplication = func() { exited++ }
	handleMainClose()
	if hidden != 1 || exited != 0 || !outputRunning {
		t.Fatal("failed tray icon lost running application")
	}
	preferences.CloseToTray = false
	handleMainClose()
	if exited != 1 {
		t.Fatal("explicit exit preference ignored")
	}
}
func TestTrayIconStructureLayout(t *testing.T) {
	if size := unsafe.Sizeof(trayIconData{}); size != 976 {
		t.Fatal("incorrect x64 NOTIFYICONDATAW size", size)
	}
}

func TestLegacyCaptureExclusionDoesNotUseBlackRectangleFallback(t *testing.T) {
	old := supportsControlExclusion
	defer func() { supportsControlExclusion = old }()
	supportsControlExclusion = func() bool { return false }
	if excludeControlWindow(0) {
		t.Fatal("old Windows claimed capture exclusion")
	}
}

func TestResumeCannotExposeUnexcludedControlWindow(t *testing.T) {
	oldCheck, oldRunning, oldSource, oldGPU, oldController := ensureControlCapturePolicy, outputRunning, activeSourceWindow, gpuActive, gpuController
	defer func() {
		ensureControlCapturePolicy, outputRunning, activeSourceWindow, gpuActive, gpuController = oldCheck, oldRunning, oldSource, oldGPU, oldController
	}()
	checked := false
	ensureControlCapturePolicy = func() bool { checked = true; return false }
	outputRunning = true
	activeSourceWindow = sourceWindow{}
	gpuController = projectionController{}
	id := gpuController.begin()
	gpuController.request(projectionBlack)
	gpuActive = newGPUMailbox(projectionConfig{Generation: id})
	resumeOutput()
	if !checked || gpuController.desired != projectionBlack {
		t.Fatal("resume bypassed control capture exclusion check")
	}
}
