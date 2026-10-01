package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type sourceWindow struct {
	HWND  uintptr
	PID   uint32
	Title string
}

var sourceWindows []sourceWindow
var activeSourceWindow sourceWindow
var sourceWindowCombo uintptr

const idSourceWindow = 260

func chosenSourceWindow() sourceWindow {
	i := comboIndex(sourceWindowCombo) - 1
	if i < 0 || i >= len(sourceWindows) {
		return sourceWindow{}
	}
	return sourceWindows[i]
}
func refreshSourceWindows() {
	if outputRunning {
		return
	}
	old := chosenSourceWindow()
	sourceWindows = nil
	own, _, _ := kernel32.NewProc("GetCurrentProcessId").Call()
	callback := syscall.NewCallback(func(h, l uintptr) uintptr {
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(h)
		if visible == 0 || windowPID(h) == uint32(own) {
			return 1
		}
		var title [256]uint16
		user32.NewProc("GetWindowTextW").Call(h, uintptr(unsafe.Pointer(&title[0])), 256)
		name := syscall.UTF16ToString(title[:])
		if name == "" {
			return 1
		}
		style, _, _ := user32.NewProc("GetWindowLongW").Call(h, ^uintptr(19)) // GWL_EXSTYLE=-20
		if style&WS_EX_TOOLWINDOW != 0 {
			return 1
		}
		var cloaked uint32
		procDwmGetWindowAttribute := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
		procDwmGetWindowAttribute.Call(h, 14, uintptr(unsafe.Pointer(&cloaked)), 4)
		if cloaked != 0 {
			return 1
		}
		sourceWindows = append(sourceWindows, sourceWindow{h, windowPID(h), name})
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	procSendMessageW.Call(sourceWindowCombo, CB_RESETCONTENT, 0, 0)
	procSendMessageW.Call(sourceWindowCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr("选择要投影的应用窗口"))))
	selection := 0
	for i, w := range sourceWindows {
		procSendMessageW.Call(sourceWindowCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(w.Title))))
		if w.HWND == old.HWND && w.PID == old.PID {
			selection = i + 1
		}
	}
	procSendMessageW.Call(sourceWindowCombo, CB_SETCURSEL, uintptr(selection), 0)
}
func outputPairError(source, target Monitor) string {
	if source.Device == target.Device {
		return "主控屏与观众屏不能相同"
	}
	if target.Primary {
		return "请将观众屏设为副屏，主屏保留 Windows 控制"
	}
	if source.Rect.Left < target.Rect.Right && source.Rect.Right > target.Rect.Left && source.Rect.Top < target.Rect.Bottom && source.Rect.Bottom > target.Rect.Top {
		return "屏幕区域重叠，请使用扩展模式"
	}
	return ""
}
func sourceReady() bool {
	a, b := comboIndex(cbSource), comboIndex(cbOutput)
	if preferences.WindowMode {
		a = primaryIndex()
	}
	if a < 0 || b < 0 || a >= len(monitors) || b >= len(monitors) {
		return false
	}
	if outputPairError(monitors[a], monitors[b]) != "" {
		return false
	}
	return !preferences.WindowMode || chosenSourceWindow().HWND != 0
}
func drawSourceModes(dc uintptr) {
	uiButton(dc, RECT{264, 116, 408, 150}, "屏幕复制", !preferences.WindowMode, !outputRunning, func() { preferences.WindowMode = false; saveConfig(); layoutControls(hwndMain) })
	uiButton(dc, RECT{420, 116, 564, 150}, "应用窗口", preferences.WindowMode, !outputRunning, func() { preferences.WindowMode = true; refreshSourceWindows(); saveConfig(); layoutControls(hwndMain) })
}
func sourceRouteLabel() string {
	if activeSourceWindow.HWND != 0 && outputRunning {
		return fmt.Sprintf("%s → %s", activeSourceWindow.Title, dstMonitor.Device)
	}
	if preferences.WindowMode {
		return "应用窗口 → 观众屏幕"
	}
	return previewRoute()
}

// Opt out of Shell fullscreen heuristics without changing taskbar settings.
// https://learn.microsoft.com/windows/win32/api/shobjidl_core/nf-shobjidl_core-itaskbarlist2-markfullscreenwindow
func preserveTaskbarControls(hwnd uintptr) bool {
	ok, _, _ := user32.NewProc("SetPropW").Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr("NonRudeHWND"))), 1)
	return ok != 0
}

func keepControlWindowOnSource() {
	if hwndMain == 0 {
		return
	}
	var bounds RECT
	user32.NewProc("GetWindowRect").Call(hwndMain, uintptr(unsafe.Pointer(&bounds)))
	r := srcMonitor.Rect
	if bounds.Left >= r.Left && bounds.Top >= r.Top && bounds.Right <= r.Right && bounds.Bottom <= r.Bottom {
		return
	}
	info := MONITORINFOEX{CbSize: uint32(unsafe.Sizeof(MONITORINFOEX{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(srcMonitor.Handle, uintptr(unsafe.Pointer(&info))); ok != 0 {
		r = info.RcWork
	}
	width, height := bounds.Right-bounds.Left, bounds.Bottom-bounds.Top
	if width > r.Right-r.Left {
		width = r.Right - r.Left
	}
	if height > r.Bottom-r.Top {
		height = r.Bottom - r.Top
	}
	x, y := r.Left+(r.Right-r.Left-width)/2, r.Top+(r.Bottom-r.Top-height)/2
	procSetWindowPos.Call(hwndMain, 0, uintptr(int64(x)), uintptr(int64(y)), uintptr(width), uintptr(height), SWP_NOZORDER|SWP_NOACTIVATE)
}
