package main

import (
	"unsafe"
)

const hotkeyReleaseCursor = 1004

var (
	cursorControlled              bool
	cursorControlFailed           bool
	releaseCursorHotkeyOK         bool
	cursorBounds                  RECT
	previousCursorBounds          RECT
	previousCursorFull            bool
	procClipCursor                = user32.NewProc("ClipCursor")
	procGetClipCursor             = user32.NewProc("GetClipCursor")
	procGetSystemMetrics          = user32.NewProc("GetSystemMetrics")
)

var setCursorBoundary = func(r *RECT) bool {
	var p uintptr
	if r != nil {
		p = uintptr(unsafe.Pointer(r))
	}
	ok, _, _ := procClipCursor.Call(p)
	return ok != 0
}
var getCursorBoundary = func() (RECT, bool) {
	var r RECT
	ok, _, _ := procGetClipCursor.Call(uintptr(unsafe.Pointer(&r)))
	return r, ok != 0
}

func virtualDesktopRect() RECT {
	metric := func(i uintptr) int32 { v, _, _ := procGetSystemMetrics.Call(i); return int32(v) }
	x, y := metric(76), metric(77)
	return RECT{x, y, x + metric(78), y + metric(79)}
}
func updateCursorControl() {
	if !outputRunning || preferences.FreeCursor {
		releaseCursorControl()
		return
	}
	r := srcMonitor.Rect
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return
	}
	if !cursorControlled {
		old, ok := getCursorBoundary()
		if !ok {
			return
		}
		previousCursorBounds = old
		previousCursorFull = old == virtualDesktopRect()
	}
	cursorControlFailed = !setCursorBoundary(&r)
	if !cursorControlFailed {
		cursorBounds = r
		cursorControlled = true
	}
}

// Windows or an application can reset the shared clip rectangle on focus changes.
// Recover only an unrestricted desktop, never overwrite another application's clip.
func maintainCursorControl() {
	if !outputRunning || preferences.FreeCursor || displayRefreshQueued {
		return
	}
	current, ok := getCursorBoundary()
	if ok && (current == virtualDesktopRect() || current == cursorBounds) {
		if !cursorControlled || current != srcMonitor.Rect {
			updateCursorControl()
		}
	}
}
func releaseCursorControl() {
	if !cursorControlled {
		return
	}
	current, ok := getCursorBoundary()
	// Do not clear a boundary subsequently installed by another application.
	if ok && current == cursorBounds {
		if previousCursorFull {
			setCursorBoundary(nil)
		} else {
			setCursorBoundary(&previousCursorBounds)
		}
	}
	cursorControlled = false
}
func drawCursorControl(dc uintptr, right, top int32) {
	left := int32(594)
	if right < 840 {
		left = 510
	}
	textLine(dc, "鼠标留在主控屏", RECT{left, top, right - 84, top + 26}, fontDesc, false)
	uiToggle(dc, RECT{right - 76, top, right - 28, top + 26}, !preferences.FreeCursor, func() { preferences.FreeCursor = !preferences.FreeCursor; updateCursorControl(); saveConfig() })
	hint := "Ctrl+Alt+F11 释放"
	if cursorControlFailed {
		hint = "限制未生效，请重新开启"
	}
	if !releaseCursorHotkeyOK {
		hint = "释放快捷键不可用，请关闭上方开关"
	}
	textLine(dc, hint, RECT{left, top + 26, right - 20, top + 48}, fontSmall, true)
}
