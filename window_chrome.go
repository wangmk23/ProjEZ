//go:build windows

package main

import (
	"unsafe"
)

const titleHeight int32 = 50

var windowDPI int32 = 96
var logicalWidth int32 = 1120
var logicalHeight int32 = 820
var pageScroll int32
var paintingPage bool
var controlFont uintptr
var controlFontDPI int32
var chromeHover int
var chromePressed int
var mainBuffer paintBuffer
var lastHoverID int
var lastHoverRect RECT
var searchFocused bool

// Win32 owns these message structures for the duration of the synchronous callback.
// Reinterpret LPARAM's pointer-sized storage; never use this for a Go object address.
func nativeMessagePointer[T any](value uintptr) *T { return *(**T)(unsafe.Pointer(&value)) }

func scaleAt(v, dpi int32) int32 {
	if dpi <= 0 {
		dpi = 96
	}
	return (v*dpi + 48) / 96
}
func px(v int32) int32 { return scaleAt(v, windowDPI) }
func dip(v int32) int32 {
	if windowDPI <= 0 {
		return v
	}
	return v * 96 / windowDPI
}
func contentShift() int32 {
	if logicalWidth < 940 {
		return 156
	}
	return 0
}
func footerTop() int32 { return logicalHeight - 84 }
func maxPageScroll() int32 {
	bottom := int32(562)
	if currentPage == 5 {
		bottom = 806
	}
	if currentPage == 0 {
		bottom = 654
	}
	if currentPage == 1 {
		bottom = 704
	}
	n := bottom - (footerTop() - 8)
	if n < 0 {
		return 0
	}
	return n
}
func searchBounds(w int32) RECT {
	left := int32(244)
	if w < 940 {
		left = 180
	}
	width := int32(450)
	if max := w - 154 - left; max < width {
		width = max
	}
	if width < 100 {
		width = 100
	}
	return RECT{left, 8, left + width, 42}
}
func chromeButtonRect(w int32, button int) RECT {
	return RECT{w - int32(4-button)*46, 0, w - int32(3-button)*46, titleHeight}
}
func chromeHit(p POINT, w, h, dpi int32, maximized bool) uintptr {
	edge := scaleAt(6, dpi)
	if !maximized {
		l, r, t, b := p.X < edge, p.X >= w-edge, p.Y < edge, p.Y >= h-edge
		switch {
		case t && l:
			return 13
		case t && r:
			return 14
		case b && l:
			return 16
		case b && r:
			return 17
		case l:
			return 10
		case r:
			return 11
		case t:
			return 12
		case b:
			return 15
		}
	}
	q := POINT{p.X * 96 / dpi, p.Y * 96 / dpi}
	lw := w * 96 / dpi
	if q.Y < titleHeight {
		if ptInRect(q, chromeButtonRect(lw, 2)) {
			return 9
		}
		if q.X < 154 || ptInRect(q, searchBounds(lw)) || q.X >= lw-138 {
			return 1
		}
		return 2
	}
	return 1
}
func maximizeBounds(m, w RECT) (POINT, POINT) {
	return POINT{w.Left - m.Left, w.Top - m.Top}, POINT{w.Right - w.Left, w.Bottom - w.Top}
}
func windowMonitor(hwnd uintptr) (MONITORINFOEX, bool) {
	h, _, _ := user32.NewProc("MonitorFromWindow").Call(hwnd, 2)
	mi := MONITORINFOEX{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ok, _, _ := procGetMonitorInfoW.Call(h, uintptr(unsafe.Pointer(&mi)))
	return mi, ok != 0
}
func queryWindowDPI(hwnd uintptr) int32 {
	p := user32.NewProc("GetDpiForWindow")
	if p.Find() == nil {
		d, _, _ := p.Call(hwnd)
		if d > 0 {
			return int32(d)
		}
	}
	return 96
}
func isMaximized(hwnd uintptr) bool { v, _, _ := user32.NewProc("IsZoomed").Call(hwnd); return v != 0 }
func logicalPoint(lp uintptr) POINT {
	return POINT{dip(int32(int16(lp & 0xffff))), dip(int32(int16(lp >> 16)))}
}
func screenPointToClient(hwnd uintptr, lp uintptr) POINT {
	p := POINT{int32(int16(lp & 0xffff)), int32(int16(lp >> 16))}
	user32.NewProc("ScreenToClient").Call(hwnd, uintptr(unsafe.Pointer(&p)))
	return p
}
func invalidateLogical(r RECT) {
	if hwndMain == 0 {
		return
	}
	r = RECT{px(r.Left), px(r.Top), px(r.Right), px(r.Bottom)}
	procInvalidateRect.Call(hwndMain, uintptr(unsafe.Pointer(&r)), 0)
}
func refreshChromeHover(p POINT) {
	next := 0
	for i := 1; i <= 3; i++ {
		if ptInRect(p, chromeButtonRect(logicalWidth, i)) {
			next = i
		}
	}
	if next != chromeHover {
		old := chromeHover
		chromeHover = next
		if old > 0 {
			invalidateLogical(chromeButtonRect(logicalWidth, old))
		}
		if next > 0 {
			invalidateLogical(chromeButtonRect(logicalWidth, next))
		}
	}
}
func clickChrome(hwnd uintptr, p POINT) bool {
	for i := 1; i <= 3; i++ {
		if ptInRect(p, chromeButtonRect(logicalWidth, i)) {
			chromePressed = 0
			user32.NewProc("ReleaseCapture").Call()
			command := uintptr(0xf020)
			if i == 2 {
				command = 0xf030
				if isMaximized(hwnd) {
					command = 0xf120
				}
			}
			if i == 3 {
				command = 0xf060
			}
			procPostFrame.Call(hwnd, 0x0112, command, 0)
			return true
		}
	}
	return false
}
func chromeMessage(hwnd uintptr, msg uint32, wp, lp uintptr) (uintptr, bool) {
	switch msg {
	case 0x0083: // WM_NCCALCSIZE: full client surface, work-area inset only when maximized
		if wp != 0 && lp != 0 && isMaximized(hwnd) {
			if mi, ok := windowMonitor(hwnd); ok {
				r := nativeMessagePointer[RECT](lp)
				*r = mi.RcWork
			}
		}
		return 0, true
	case 0x0084:
		var r RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		return chromeHit(screenPointToClient(hwnd, lp), r.Right, r.Bottom, queryWindowDPI(hwnd), isMaximized(hwnd)), true
	case 0x0024: // WM_GETMINMAXINFO
		if lp == 0 {
			return 0, false
		}
		info := nativeMessagePointer[struct{ Reserved, MaxSize, MaxPosition, MinTrack, MaxTrack POINT }](lp)
		dpi := queryWindowDPI(hwnd)
		info.MinTrack = POINT{scaleAt(640, dpi), scaleAt(480, dpi)}
		if mi, ok := windowMonitor(hwnd); ok {
			info.MaxPosition, info.MaxSize = maximizeBounds(mi.RcMonitor, mi.RcWork)
			info.MaxTrack = info.MaxSize
			if info.MinTrack.X > info.MaxSize.X {
				info.MinTrack.X = info.MaxSize.X
			}
			if info.MinTrack.Y > info.MaxSize.Y {
				info.MinTrack.Y = info.MaxSize.Y
			}
		}
		return 0, true
	case 0x02e0: // WM_DPICHANGED
		windowDPI = int32(wp & 0xffff)
		if windowDPI == 0 {
			windowDPI = 96
		}
		mainBuffer.release()
		if lp != 0 {
			r := *nativeMessagePointer[RECT](lp)
			procSetWindowPos.Call(hwnd, 0, uintptr(int64(r.Left)), uintptr(int64(r.Top)), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), SWP_NOZORDER|SWP_NOACTIVATE)
		}
		layoutControls(hwnd)
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0, true
	case 0x00a0: // WM_NCMOUSEMOVE, HTMAXBUTTON keeps Windows 11 snap-layout eligibility
		p := screenPointToClient(hwnd, lp)
		refreshChromeHover(POINT{dip(p.X), dip(p.Y)})
		tme := struct {
			Size, Flags uint32
			Hwnd        uintptr
			Time        uint32
		}{Flags: 0x12, Hwnd: hwnd}
		tme.Size = uint32(unsafe.Sizeof(tme))
		user32.NewProc("TrackMouseEvent").Call(uintptr(unsafe.Pointer(&tme)))
	case 0x02a2:
		refreshChromeHover(POINT{-1, -1})
	case 0x00a1:
		if wp == 9 {
			chromePressed = 2
			user32.NewProc("SetCapture").Call(hwnd)
			return 0, true
		}
	case 0x00a2: // alternate NC up delivery
		if wp == 9 {
			p := screenPointToClient(hwnd, lp)
			clickChrome(hwnd, POINT{dip(p.X), dip(p.Y)})
			return 0, true
		}
	case 0x0215:
		chromePressed = 0
	case 0x020a: // body scroll, no display re-enumeration during wheel/resize
		if maxPageScroll() > 0 {
			p := screenPointToClient(hwnd, lp)
			if dip(p.X) >= 220-contentShift() && dip(p.Y) >= titleHeight {
				pageScroll -= int32(int16(wp>>16)) * 48 / 120
				if pageScroll < 0 {
					pageScroll = 0
				}
				if pageScroll > maxPageScroll() {
					pageScroll = maxPageScroll()
				}
				layoutControls(hwnd)
				procInvalidateRect.Call(hwnd, 0, 0)
				return 0, true
			}
		}
	case 0x0318:
		if wp != 0 {
			paintMainWindow(hwnd, wp)
		}
		return 0, true
	}
	return 0, false
}
func drawChrome(hdc uintptr, w int32) {
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&RECT{0, 0, w, titleHeight})), brushSidebar)
	if appIcon != 0 {
		procDrawIconEx.Call(hdc, 20, 15, appIcon, 20, 20, 0, 0, DI_NORMAL)
	}
	textLine(hdc, "ProjEZ", RECT{49, 0, 154, titleHeight}, fontButton, false)
	search := searchBounds(w)
	pen := uintptr(0)
	if searchFocused {
		pen = penAccent
	}
	drawRounded(hdc, search, 11, brushControl, pen)
	for i := 1; i <= 3; i++ {
		r := chromeButtonRect(w, i)
		if chromeHover == i {
			b := brushControl
			if i == 3 {
				b = makeBrush(196, 43, 28)
				defer procDeleteObject.Call(b)
			}
			procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), b)
		}
		symbol := "\uE921"
		if i == 2 {
			symbol = "\uE922"
			if isMaximized(hwndMain) {
				symbol = "\uE923"
			}
		}
		if i == 3 {
			symbol = "\uE8BB"
		}
		drawText(hdc, symbol, r, fontNavIcon, uiTextColor(), DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	}
}
func beginLogicalDC(dc uintptr) int {
	saved, _, _ := gdi32.NewProc("SaveDC").Call(dc)
	gdi32.NewProc("SetMapMode").Call(dc, 8)
	gdi32.NewProc("SetWindowExtEx").Call(dc, 96, 96, 0)
	gdi32.NewProc("SetViewportExtEx").Call(dc, uintptr(windowDPI), uintptr(windowDPI), 0)
	return int(saved)
}
func endLogicalDC(dc uintptr, saved int) { gdi32.NewProc("RestoreDC").Call(dc, uintptr(saved)) }
func beginBody(dc uintptr, scroll bool) int {
	saved, _, _ := gdi32.NewProc("SaveDC").Call(dc)
	shift := contentShift()
	top, bottom := int32(58), footerTop()-8
	if !scroll {
		top = footerTop()
		bottom = logicalHeight
	}
	gdi32.NewProc("IntersectClipRect").Call(dc, uintptr(220-shift), uintptr(top), uintptr(logicalWidth), uintptr(bottom))
	y := int32(0)
	if scroll {
		y = pageScroll
	}
	gdi32.NewProc("SetWindowOrgEx").Call(dc, uintptr(shift), uintptr(y), 0)
	paintingPage = scroll
	return int(saved)
}
func addAction(r RECT, run func(), enabled bool) {
	r.Left -= contentShift()
	r.Right -= contentShift()
	if paintingPage {
		r.Top -= pageScroll
		r.Bottom -= pageScroll
		if r.Top < 58 {
			r.Top = 58
		}
		if r.Bottom > footerTop()-8 {
			r.Bottom = footerTop() - 8
		}
	}
	if r.Bottom > r.Top {
		actions = append(actions, uiAction{r, run, enabled})
	}
}
func actionHoverPoint() POINT {
	p := hoverPoint
	p.X += contentShift()
	if paintingPage {
		p.Y += pageScroll
	}
	return p
}
func hoverRegion(p POINT) (int, RECT) {
	for i := range pageNames {
		r := navRect(i)
		if ptInRect(p, r) {
			return i + 1, r
		}
	}
	for i, a := range actions {
		if ptInRect(p, a.rect) {
			return i + 100, a.rect
		}
	}
	return 0, RECT{}
}
