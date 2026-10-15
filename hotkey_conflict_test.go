package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestHotkeyConflictMessages(t *testing.T) {
	texts := []string{"Ctrl+Alt+F8", "Ctrl+Alt+F9", "Ctrl+Alt+F10", "Ctrl+Alt+F12"}
	oldErrors := actionHotkeyErrors
	defer func() { actionHotkeyErrors = oldErrors }()
	actionHotkeyErrors[0] = 1409
	got := actionHotkeyRegistrationMessage(0, texts[0])
	if !strings.Contains(got, "Ctrl+Alt+F8") || !strings.Contains(got, "占用") || !strings.Contains(got, "冻结") {
		t.Fatal("missing action/key/conflict diagnosis", got)
	}
	actionHotkeyErrors[0] = 5
	got = actionHotkeyRegistrationMessage(0, texts[0])
	if strings.Contains(got, "占用") || !strings.Contains(got, "5") {
		t.Fatal("generic failure incorrectly attributed to another app", got)
	}
	texts[3] = texts[0]
	for _, i := range []int{0, 3} {
		if got := actionHotkeyInputError(i, texts); !strings.Contains(got, "重复") {
			t.Fatal("missing duplicate on both rows", i, got)
		}
	}
	texts[3] = "Ctrl+Alt+F11"
	if got := actionHotkeyInputError(3, texts); !strings.Contains(got, "释放鼠标") {
		t.Fatal(got)
	}
}

func TestConflictingHotkeysKeepSavedConfiguration(t *testing.T) {
	oldOK, oldKeys, oldSaved, oldRecording := actionHotkeyOK, appliedActionKeys, savedActionHotkeyTexts, recordingEdit
	defer func() {
		actionHotkeyOK, appliedActionKeys, savedActionHotkeyTexts, recordingEdit = oldOK, oldKeys, oldSaved, oldRecording
	}()
	savedActionHotkeyTexts = [4]string{"Ctrl+Alt+F8", "Ctrl+Alt+F9", "Ctrl+Alt+F10", "Ctrl+Alt+F12"}
	texts := []string{"Ctrl+Shift+F8", "Ctrl+Alt+F9", "Ctrl+Alt+F10", "Ctrl+Alt+F12"}
	keys, _ := validateActionHotkeys(texts)
	copy(appliedActionKeys[:], keys)
	actionHotkeyOK = [4]bool{false, true, true, true}
	recordingEdit = 0
	if got := hotkeyTextsForSave(texts); got != savedActionHotkeyTexts {
		t.Fatal("conflicting shortcut persisted", got)
	}
	actionHotkeyOK[0] = true
	if got := hotkeyTextsForSave(texts); got[0] != texts[0] {
		t.Fatal("valid registration not persisted", got)
	}
	texts[3] = texts[0]
	if got := hotkeyTextsForSave(texts); got != savedActionHotkeyTexts {
		t.Fatal("duplicate shortcut persisted", got)
	}
}

func TestNativeHotkeyConflictDiagnosis(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldWindow, oldErrors := hwndMain, actionHotkeyErrors
	defer func() { hwndMain, actionHotkeyErrors = oldWindow, oldErrors }()
	hwndMain = 0
	const ownerID = 0x6f10
	key := Hotkey{Mods: MOD_CONTROL | MOD_ALT | MOD_SHIFT | MOD_NOREPEAT, VK: 0x87, Text: "Ctrl+Alt+Shift+F24"}
	ok, _, _ := procRegisterHotKey.Call(0, ownerID, uintptr(key.Mods), uintptr(key.VK))
	if ok == 0 {
		t.Skip("test shortcut already occupied")
	}
	defer procUnregisterHotKey.Call(0, ownerID)
	defer procUnregisterHotKey.Call(0, HOTKEY_FREEZE)
	if registerActionHotkey(HOTKEY_FREEZE, key) || actionHotkeyErrors[0] != 1409 {
		t.Fatal("native occupied shortcut not diagnosed", actionHotkeyErrors[0])
	}
}
