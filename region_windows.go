package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var selectedRegion, activeRegion RECT // Coordinates relative to the selected source monitor.
var regionMonitor Monitor
var regionPicker uintptr
var pickerScreen Monitor
var pickerStart, pickerEnd POINT
var pickerDragging bool
var pickerWindow sourceWindow
var pickerWindows []sourceWindow

func regionFor(m Monitor) RECT {
	if regionMonitor.Device != m.Device || regionMonitor.Rect != m.Rect {
		return RECT{}
	}
	return selectedRegion
}
func regionRect(a, b POINT, width, height int32) RECT {
	return RECT{max(0, min(a.X, b.X)), max(0, min(a.Y, b.Y)), min(width, max(a.X, b.X)), min(height, max(a.Y, b.Y))}
}
func absDelta(a, b int32) int32 { return max(a-b, b-a) }
func validRegion(r RECT) bool   { return r.Right-r.Left >= 16 && r.Bottom-r.Top >= 16 }
func rotatedRegion(r RECT, w, h uint32, rotation uint32) RECT {
	switch rotation {
	case 2:
		return RECT{r.Top, int32(h) - r.Right, r.Bottom, int32(h) - r.Left}
	case 3:
		return RECT{int32(w) - r.Right, int32(h) - r.Bottom, int32(w) - r.Left, int32(h) - r.Top}
	case 4:
		return RECT{int32(w) - r.Bottom, r.Left, int32(w) - r.Top, r.Right}
	default:
		return r
	}
}
func pickProjectionSource() {
	if outputRunning || regionPicker != 0 {
		return
	}
	i := comboIndex(cbSource)
	if i < 0 || i >= len(monitors) {
		setStatus("请先选择来源屏幕")
		return
	}
	pickerScreen = monitors[i]
	refreshSourceWindows()
	pickerWindows = append(pickerWindows[:0], sourceWindows...)
	pickerStart, pickerEnd, pickerWindow = POINT{}, POINT{}, sourceWindow{}
	pickerDragging = false
	name := utf16Ptr("ProjEZRegionPicker")
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), LpfnWndProc: regionPickerCallback, HInstance: hInstance, LpszClassName: name}
	wc.HCursor, _, _ = procLoadCursorW.Call(0, 32515)
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	r := pickerScreen.Rect
	regionPicker, _, _ = procCreateWindowExW.Call(WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_LAYERED, uintptr(unsafe.Pointer(name)), 0, WS_POPUP, uintptr(int64(r.Left)), uintptr(int64(r.Top)), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0, 0, hInstance, 0)
	if regionPicker == 0 {
		setStatus("无法打开选区")
		return
	}
	// A translucent selection surface leaves the actual live desktop visible.
	user32.NewProc("SetLayeredWindowAttributes").Call(regionPicker, 0, 105, 2)
	procShowWindow.Call(hwndMain, 0)
	procShowWindow.Call(regionPicker, SW_SHOW)
	user32.NewProc("SetForegroundWindow").Call(regionPicker)
	user32.NewProc("SetFocus").Call(regionPicker)
}

var regionPickerCallback = syscall.NewCallback(regionPickerProc)

func pickerHover(p POINT) (sourceWindow, RECT) {
	screen := pickerScreen.Rect
	x, y := p.X+screen.Left, p.Y+screen.Top
	for _, w := range pickerWindows {
		if windowPID(w.HWND) != w.PID {
			continue
		}
		var r RECT
		if ok, _, _ := user32.NewProc("GetWindowRect").Call(w.HWND, uintptr(unsafe.Pointer(&r))); ok == 0 {
			continue
		}
		if iconic, _, _ := user32.NewProc("IsIconic").Call(w.HWND); iconic != 0 {
			continue
		}
		if x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom {
			return w, regionRect(POINT{r.Left - screen.Left, r.Top - screen.Top}, POINT{r.Right - screen.Left, r.Bottom - screen.Top}, screen.Right-screen.Left, screen.Bottom-screen.Top)
		}
	}
	return sourceWindow{}, RECT{}
}
func pickerCommit(h uintptr, r RECT, window sourceWindow) {
	if window.HWND != 0 {
		refreshSourceWindows()
		found := false
		for i, w := range sourceWindows {
			if w.HWND == window.HWND && w.PID == window.PID {
				preferences.WindowMode = true
				selectedRegion = RECT{}
				procSendMessageW.Call(sourceWindowCombo, CB_SETCURSEL, uintptr(i+1), 0)
				found = true
				break
			}
		}
		if !found {
			setStatus("窗口已关闭，请重新选择")
		}
	} else {
		preferences.WindowMode = false
		selectedRegion = r
		regionMonitor = pickerScreen
	}
	procDestroyWindow.Call(h)
	saveConfig()
	layoutControls(hwndMain)
	procInvalidateRect.Call(hwndMain, 0, 0)
}
func regionPickerProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	p := POINT{int32(int16(lp)), int32(int16(lp >> 16))}
	screen := pickerScreen.Rect
	width, height := screen.Right-screen.Left, screen.Bottom-screen.Top
	switch msg {
	case WM_ERASEBKGND:
		return 1
	case 0x0200:
		pickerEnd = p
		procInvalidateRect.Call(h, 0, 0)
		return 0
	case 0x0201:
		pickerStart, pickerEnd = p, p
		pickerDragging = true
		pickerWindow, _ = pickerHover(p)
		user32.NewProc("SetCapture").Call(h)
		return 0
	case WM_LBUTTONUP:
		if !pickerDragging {
			return 0
		}
		pickerDragging = false
		user32.NewProc("ReleaseCapture").Call()
		r := regionRect(pickerStart, p, width, height)
		if validRegion(r) {
			pickerCommit(h, r, sourceWindow{})
		} else if pickerWindow.HWND != 0 && absDelta(p.X, pickerStart.X) <= 4 && absDelta(p.Y, pickerStart.Y) <= 4 {
			pickerCommit(h, RECT{}, pickerWindow)
		}
		return 0
	case 0x0100:
		switch wp {
		case 0x1b:
			procDestroyWindow.Call(h)
		case 0x0d:
			pickerCommit(h, RECT{}, sourceWindow{})
		}
		return 0
	case 0x0205, WM_CLOSE:
		procDestroyWindow.Call(h)
		return 0
	case WM_DESTROY:
		regionPicker = 0
		pickerDragging = false
		procShowWindow.Call(hwndMain, SW_SHOW)
		user32.NewProc("SetForegroundWindow").Call(hwndMain)
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		dc, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		defer procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		black, _, _ := procGetStockObject.Call(4)
		rc := RECT{0, 0, width, height}
		procFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), black)
		_, r := pickerHover(pickerEnd)
		if pickerDragging {
			r = regionRect(pickerStart, pickerEnd, width, height)
		}
		if validRegion(r) {
			user32.NewProc("FrameRect").Call(dc, uintptr(unsafe.Pointer(&r)), brushAccent)
			inner := RECT{r.Left + 1, r.Top + 1, r.Right - 1, r.Bottom - 1}
			user32.NewProc("FrameRect").Call(dc, uintptr(unsafe.Pointer(&inner)), brushAccent)
			drawText(dc, fmt.Sprintf("%d × %d", r.Right-r.Left, r.Bottom-r.Top), RECT{r.Left + 8, max(54, r.Top+6), r.Right, r.Top + 80}, controlFont, 0xffffff, DT_SINGLELINE)
		}
		procSetBkMode.Call(dc, TRANSPARENT)
		drawText(dc, "拖动框选区域   点击选择窗口   Enter 选择全屏   Esc 取消", RECT{0, 8, width, 55}, controlFont, 0xffffff, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), wp, lp)
	return r
}
