package main

import (
	"syscall"
	"unsafe"
)

const themedComboStyle = 0x0010 | 0x0200 // CBS_OWNERDRAWFIXED | CBS_HASSTRINGS

type comboInfo struct {
	Size              uint32
	Item, Button      RECT
	State             uint32
	Combo, Edit, List uintptr
}

var dropdownOwners = map[uintptr]uintptr{}
var dropdownProcs = map[uintptr]uintptr{}
var dropdownSizes = map[uintptr][3]int32{}
var dropdownCallback = syscall.NewCallback(dropdownProc)

func dropdownBounds(anchor, bounds RECT, height, dpi int32) RECT {
	pad := scaleAt(8, dpi)
	bounds.Left += pad
	bounds.Right -= pad
	bounds.Top += scaleAt(50, dpi)
	bounds.Bottom -= pad
	w := anchor.Right - anchor.Left
	if w > bounds.Right-bounds.Left {
		w = bounds.Right - bounds.Left
	}
	x := anchor.Left
	if x+w > bounds.Right {
		x = bounds.Right - w
	}
	if x < bounds.Left {
		x = bounds.Left
	}
	gap := scaleAt(4, dpi)
	below, above := bounds.Bottom-anchor.Bottom-gap, anchor.Top-bounds.Top-gap
	y := anchor.Bottom + gap
	if below < height && above > below {
		if height > above {
			height = above
		}
		y = anchor.Top - gap - height
	} else if height > below {
		height = below
	}
	if height < 1 {
		height = 1
	}
	if y < bounds.Top {
		y = bounds.Top
	}
	return RECT{x, y, x + w, y + height}
}

func styleDropdown(combo uintptr) {
	info := comboInfo{Size: uint32(unsafe.Sizeof(comboInfo{}))}
	if ok, _, _ := procGetComboBoxInfo.Call(combo, uintptr(unsafe.Pointer(&info))); ok == 0 || info.List == 0 {
		return
	}
	if _, ok := dropdownProcs[info.List]; ok {
		return
	}
	old, _, _ := procSetWindowLongPtrW.Call(info.List, ^uintptr(3), dropdownCallback)
	if old != 0 {
		dropdownProcs[info.List] = old
		dropdownOwners[info.List] = combo
	}
}

func dropdownProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	old := dropdownProcs[hwnd]
	r, _, _ := procCallWindowProcW.Call(old, hwnd, uintptr(msg), wp, lp)
	if msg == 0x0046 && lp != 0 { // WM_WINDOWPOSCHANGING: constrain the real popup, not just its requested width.
		p := nativeMessagePointer[struct {
			Hwnd, After uintptr
			X, Y, W, H  int32
			Flags       uint32
		}](lp)
		if p.Flags&3 != 3 && p.W > 0 && p.H > 0 {
			var anchor, bounds RECT
			procGetWindowRect.Call(dropdownOwners[hwnd], uintptr(unsafe.Pointer(&anchor)))
			procGetClientRect.Call(hwndMain, uintptr(unsafe.Pointer(&bounds)))
			procMapWindowPoints.Call(hwndMain, 0, uintptr(unsafe.Pointer(&bounds)), 2)
			b := dropdownBounds(anchor, bounds, p.H, windowDPI)
			p.X, p.Y, p.W, p.H = b.Left, b.Top, b.Right-b.Left, b.Bottom-b.Top
			p.Flags &^= 3
		}
	}
	if msg == 0x0047 { // Region changes also send WINDOWPOSCHANGED; cache before applying.
		var rect RECT
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		size := [3]int32{rect.Right - rect.Left, rect.Bottom - rect.Top, windowDPI}
		if size[0] > 0 && size[1] > 0 && dropdownSizes[hwnd] != size {
			dropdownSizes[hwnd] = size
			region, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(size[0]+1), uintptr(size[1]+1), uintptr(px(12)), uintptr(px(12)))
			if region != 0 {
				if ok, _, _ := procSetWindowRgn.Call(hwnd, region, 1); ok == 0 {
					procDeleteObject.Call(region)
				}
			}
		}
	}
	if msg == 0x0085 || msg == 0x0317 { // Non-client border, including window-print captures.
		dc := wp
		if msg == 0x0085 {
			dc, _, _ = procGetWindowDC.Call(hwnd)
		}
		if dc != 0 {
			var rect RECT
			procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
			rect = RECT{0, 0, rect.Right - rect.Left, rect.Bottom - rect.Top}
			hollow, _, _ := procGetStockObject.Call(5) // NULL_BRUSH
			drawRounded(dc, rect, px(6), hollow, penBorder)
			if msg == 0x0085 {
				procReleaseDC.Call(hwnd, dc)
			}
		}
	}
	if msg == 0x0082 {
		delete(dropdownSizes, hwnd)
		delete(dropdownProcs, hwnd)
		delete(dropdownOwners, hwnd)
	}
	return r
}

func drawComboItem(lp uintptr) bool {
	if lp == 0 {
		return false
	}
	d := nativeMessagePointer[struct {
		Type, ID, Item, Action, State uint32
		Hwnd, DC                      uintptr
		Rect                          RECT
		Data                          uintptr
	}](lp)
	if d.Type != 3 {
		return false
	}
	procFillRect.Call(d.DC, uintptr(unsafe.Pointer(&d.Rect)), brushControl)
	if d.State&1 != 0 {
		selected := d.Rect
		selected.Left += px(4)
		selected.Right -= px(4)
		selected.Top += px(2)
		selected.Bottom -= px(2)
		drawRounded(d.DC, selected, px(4), brushActive, 0)
	}
	if d.Item != ^uint32(0) {
		n, _, _ := procSendMessageW.Call(d.Hwnd, 0x0149, uintptr(d.Item), 0)
		if int32(n) >= 0 && n < 32768 {
			buf := make([]uint16, n+1)
			procSendMessageW.Call(d.Hwnd, 0x0148, uintptr(d.Item), uintptr(unsafe.Pointer(&buf[0])))
			r := d.Rect
			r.Left += px(12)
			r.Right -= px(12)
			procSetBkMode.Call(d.DC, TRANSPARENT)
			drawText(d.DC, syscall.UTF16ToString(buf), r, controlFont, uiTextColor(), DT_VCENTER|DT_SINGLELINE|0x8000)
		}
	}
	return true
}

// GDI+ antialiases the small switch curves at their actual device-pixel size.
var (
	gdip                     = syscall.NewLazyDLL("gdiplus.dll")
	gdipToken                uintptr
	procGdiplusStartup       = gdip.NewProc("GdiplusStartup")
	procGdiplusShutdown      = gdip.NewProc("GdiplusShutdown")
	procGdipCreateFromHDC    = gdip.NewProc("GdipCreateFromHDC")
	procGdipDeleteGraphics   = gdip.NewProc("GdipDeleteGraphics")
	procGdipSetSmoothingMode = gdip.NewProc("GdipSetSmoothingMode")
	procGdipCreateSolidFill  = gdip.NewProc("GdipCreateSolidFill")
	procGdipDeleteBrush      = gdip.NewProc("GdipDeleteBrush")
	procGdipFillEllipseI     = gdip.NewProc("GdipFillEllipseI")
	procGdipFillRectangleI   = gdip.NewProc("GdipFillRectangleI")
)

func smoothPill(dc uintptr, r RECT, brush uintptr) bool {
	if gdipToken == 0 {
		input := struct {
			Version            uint32
			Callback           uintptr
			NoThread, NoCodecs int32
		}{Version: 1}
		if status, _, _ := procGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&input)), 0); status != 0 {
			return false
		}
	}
	var lb struct {
		Style, Color uint32
		Hatch        uintptr
	}
	procGetObjectW.Call(brush, unsafe.Sizeof(lb), uintptr(unsafe.Pointer(&lb)))
	color := uint32(0xff000000) | (lb.Color&255)<<16 | (lb.Color & 0xff00) | (lb.Color >> 16 & 255)
	procLPtoDP.Call(dc, uintptr(unsafe.Pointer(&r)), 2)
	saved, _, _ := procSaveDC.Call(dc)
	defer procRestoreDC.Call(dc, saved)
	procSetMapMode.Call(dc, 1)
	procSetWindowOrgEx.Call(dc, 0, 0, 0)
	procSetViewportOrgEx.Call(dc, 0, 0, 0)
	var graphics, fill uintptr
	if status, _, _ := procGdipCreateFromHDC.Call(dc, uintptr(unsafe.Pointer(&graphics))); status != 0 {
		return false
	}
	defer procGdipDeleteGraphics.Call(graphics)
	procGdipSetSmoothingMode.Call(graphics, 4)
	procGdipCreateSolidFill.Call(uintptr(color), uintptr(unsafe.Pointer(&fill)))
	defer procGdipDeleteBrush.Call(fill)
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if w > h {
		procGdipFillRectangleI.Call(graphics, fill, uintptr(r.Left+h/2), uintptr(r.Top), uintptr(w-h+1), uintptr(h))
	}
	procGdipFillEllipseI.Call(graphics, fill, uintptr(r.Left), uintptr(r.Top), uintptr(h), uintptr(h))
	if w > h {
		procGdipFillEllipseI.Call(graphics, fill, uintptr(r.Right-h), uintptr(r.Top), uintptr(h), uintptr(h))
	}
	return true
}

// Draw a DPI-scaled chevron without depending on a font glyph.
func drawComboArrow(dc, combo uintptr, rect RECT) {
	open, _, _ := procSendMessageW.Call(combo, 0x0157, 0, 0)
	x, y, delta := rect.Right-px(18), rect.Bottom/2, px(3)
	if open != 0 {
		delta = -delta
	}
	pen, _, _ := procCreatePen.Call(0, uintptr(px(1)), uintptr(uiMutedColor()))
	old, _, _ := procSelectObject.Call(dc, pen)
	procMoveToEx.Call(dc, uintptr(x-px(4)), uintptr(y-delta/2), 0)
	procLineTo.Call(dc, uintptr(x), uintptr(y+delta/2))
	procLineTo.Call(dc, uintptr(x+px(4)), uintptr(y-delta/2))
	procSelectObject.Call(dc, old)
	procDeleteObject.Call(pen)
}
