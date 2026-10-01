package main

import (
	"fmt"
	"strings"
	"syscall"
)

var recordingEdit uintptr
var recordingOriginal string
var hotkeyEditProcs = map[uintptr]uintptr{}
var hotkeyEditCallback = syscall.NewCallback(hotkeyEditProc)
var recordingKeyDown = func(vk uint32) bool {
	state, _, _ := user32.NewProc("GetKeyState").Call(uintptr(vk))
	return state&0x8000 != 0
}

func recordedHotkey(vk uint32, ctrl, alt, shift bool) string {
	key := ""
	switch {
	case vk >= 0x70 && vk <= 0x87:
		key = fmt.Sprintf("F%d", vk-0x70+1)
	case vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9':
		key = string(rune(vk))
	default:
		key = map[uint32]string{0x2d: "Insert", 0x24: "Home", 0x23: "End", 0x21: "PageUp", 0x22: "PageDown", 0x13: "Pause"}[vk]
	}
	if key == "" {
		return ""
	}
	parts := []string{}
	if ctrl {
		parts = append(parts, "Ctrl")
	}
	if alt {
		parts = append(parts, "Alt")
	}
	if shift {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, key), "+")
}
func styleHotkeyRecorder(hwnd uintptr) {
	old, _, _ := user32.NewProc("SetWindowLongPtrW").Call(hwnd, ^uintptr(3), hotkeyEditCallback)
	if old != 0 {
		hotkeyEditProcs[hwnd] = old
		procSendMessageW.Call(hwnd, 0x00cf, 1, 0)
	} // EM_SETREADONLY
}
func hotkeyEditProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	call := func() uintptr {
		r, _, _ := user32.NewProc("CallWindowProcW").Call(hotkeyEditProcs[hwnd], hwnd, uintptr(msg), wp, lp)
		return r
	}
	switch msg {
	case 7: // WM_SETFOCUS: suspend our global actions while recording.
		recordingEdit = hwnd
		recordingOriginal = getText(hwnd)
		for _, id := range actionHotkeyIDs {
			procUnregisterHotKey.Call(hwndMain, uintptr(id))
		}
		procUnregisterHotKey.Call(hwndMain, hotkeyReleaseCursor)
		r := call()
		procSendMessageW.Call(hwnd, 0x00b1, 0, ^uintptr(0))
		procInvalidateRect.Call(hwndMain, 0, 0)
		return r
	case 8:
		recordingEdit = 0
		r := call()
		procInvalidateRect.Call(hwndMain, 0, 0)
		return r
	case 0x0100, 0x0104: // WM_KEYDOWN / WM_SYSKEYDOWN
		if wp == 0x1b {
			setText(hwnd, recordingOriginal)
			user32.NewProc("SetFocus").Call(hwndMain)
			return 0
		}
		if wp == 0x09 {
			return call()
		}
		if recordingKeyDown(0x5b) || recordingKeyDown(0x5c) {
			return 0
		}
		text := recordedHotkey(uint32(wp), recordingKeyDown(0x11), recordingKeyDown(0x12) || msg == 0x0104, recordingKeyDown(0x10))
		if text != "" {
			setText(hwnd, text)
			procSendMessageW.Call(hwnd, 0x00b1, 0, ^uintptr(0))
			procInvalidateRect.Call(hwndMain, 0, 0)
		}
		return 0
	case 0x0102, 0x0106, 0x0101, 0x0105:
		return 0 // Suppress generated characters and Alt menus.
	case 0x0082:
		r := call()
		delete(hotkeyEditProcs, hwnd)
		return r
	}
	return call()
}
