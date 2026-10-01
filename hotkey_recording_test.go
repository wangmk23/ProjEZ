package main

import "testing"

func TestRecordedHotkey(t *testing.T) {
	for _, c := range []struct {
		key              uint32
		ctrl, alt, shift bool
		want             string
	}{{0x79, true, true, false, "Ctrl+Alt+F10"}, {'K', true, false, true, "Ctrl+Shift+K"}, {0x7b, false, false, false, "F12"}, {0x11, true, false, false, ""}, {0x21, true, false, false, "Ctrl+PageUp"}} {
		got := recordedHotkey(c.key, c.ctrl, c.alt, c.shift)
		if got != c.want {
			t.Fatal(got, c.want)
		}
		if got != "" {
			key, err := parseHotkey(got)
			if err != nil || key.VK != c.key {
				t.Fatal("recorded key not accepted", got, err)
			}
		}
	}
}
func TestRecordingDoesNotTriggerProjectionActions(t *testing.T) {
	old := recordingEdit
	recordingEdit = 1
	defer func() { recordingEdit = old }()
	oldState := gpuController.desired
	mainWndProc(0, WM_HOTKEY, hotkeyStop, 0)
	if gpuController.desired != oldState {
		t.Fatal("recording triggered stop")
	}
}
