package main

import (
	"strings"
	"testing"
)

func TestActionHotkeyValidation(t *testing.T) {
	defaults := []string{"Ctrl+Alt+F8", "Ctrl+Alt+F9", "Ctrl+Alt+F10", "Ctrl+Alt+F12"}
	keys, err := validateActionHotkeys(defaults)
	if err != nil || len(keys) != 4 {
		t.Fatal(err)
	}
	for _, pair := range [][2]int{{0, 2}, {1, 3}, {2, 3}} {
		values := append([]string{}, defaults...)
		values[pair[1]] = values[pair[0]]
		if _, err := validateActionHotkeys(values); err == nil {
			t.Fatal("duplicate accepted", pair)
		}
	}
	values := append([]string{}, defaults...)
	values[3] = "Ctrl+Alt+F11"
	if _, err := validateActionHotkeys(values); err == nil {
		t.Fatal("reserved release shortcut accepted")
	}
	values[3] = "badkey"
	if _, err := validateActionHotkeys(values); err == nil || !strings.Contains(err.Error(), "停止") {
		t.Fatal("missing action-specific invalid key message", err)
	}
}
func TestBlackAndStopHotkeyDispatch(t *testing.T) {
	oldRunning, oldOutput, oldGPU, oldController, oldBlack := outputRunning, hwndOutput, gpuActive, gpuController, blackout
	oldStatus := statusText
	defer func() {
		outputRunning = oldRunning
		hwndOutput = oldOutput
		gpuActive = oldGPU
		gpuController = oldController
		blackout = oldBlack
		statusText = oldStatus
	}()
	outputRunning = true
	hwndOutput = 1
	gpuController = projectionController{}
	id := gpuController.begin()
	gpuActive = newGPUMailbox(projectionConfig{Generation: id})
	mainWndProc(0, WM_HOTKEY, hotkeyBlack, 0)
	if !blackout || gpuController.desired != projectionBlack {
		t.Fatal("black hotkey did not hide projection")
	}
	mainWndProc(0, WM_HOTKEY, hotkeyStop, 0)
	if gpuController.desired != projectionStopping {
		t.Fatal("stop hotkey did not stop projection")
	}
}

func TestHotkeyConflictDoesNotDisableOtherActions(t *testing.T) {
	old := registerActionHotkey
	defer func() { registerActionHotkey = old }()
	called := map[int]bool{}
	registerActionHotkey = func(id int, key Hotkey) bool { called[id] = true; return id != HOTKEY_FREEZE }
	keys, err := validateActionHotkeys([]string{"Ctrl+Alt+F8", "Ctrl+Alt+F9", "Ctrl+Alt+F10", "Ctrl+Alt+F12"})
	if err != nil {
		t.Fatal(err)
	}
	got := tryActionRegistrations(keys)
	if got != [4]bool{false, true, true, true} || len(called) != 4 {
		t.Fatal("registration failure disabled other actions", got)
	}
}
