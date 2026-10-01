package main

import (
	"fmt"
	"strings"
	"time"
	"unsafe"
)

const previewTimer = 0x4752

var previewWanted = true
var audienceFrame previewFrame
var audienceError string
var lastPreviewPoll time.Time
var gdiPreviewDC, gdiPreviewBitmap, gdiPreviewOld uintptr
var gdiPreviewBits unsafe.Pointer

func previewTop() int32 {
	if currentPage == 0 {
		return 236
	}
	return 404
}
func previewVisible() bool {
	if !previewWanted || !outputRunning || (currentPage != 0 && currentPage != 1) {
		return false
	}
	if v, _, _ := user32.NewProc("IsIconic").Call(hwndMain); v != 0 {
		return false
	}
	if v, _, _ := user32.NewProc("IsWindowVisible").Call(hwndMain); v == 0 {
		return false
	}
	top := previewTop() - pageScroll
	return top+268 > 58 && top < footerTop()-8
}
func confirmedOutputState() projectionState {
	if gpuOwnsOutput {
		return gpuLastEvent.State
	}
	if blackout {
		if blackAcknowledged {
			return projectionBlack
		}
		return projectionStarting
	}
	if frozen {
		return projectionFrozen
	}
	if gdiAwaitLive || !gdiPresented {
		return projectionStarting
	}
	return projectionLive
}
func pollAudiencePreview() {
	visible := previewVisible()
	if visible && time.Since(lastPreviewPoll) < time.Duration(1000/previewRate(preferences))*time.Millisecond {
		return
	}
	lastPreviewPoll = time.Now()
	if gpuActive != nil {
		s := gpuActive
		s.mu.Lock()
		if visible && !s.previewEnabled {
			s.previewError = ""
		}
		s.previewEnabled = visible
		s.previewFPS = previewRate(preferences)
		f, err := s.preview, s.previewError
		s.preview = nil
		s.mu.Unlock()
		if visible {
			audienceError = err
			if f != nil && f.matches(gpuController.generation, gpuController.sequence, gpuLastEvent.State) {
				audienceFrame = *f
			}
		}
	} else if visible && !gpuOwnsOutput && gdiPresented {
		pixels, err := readGDIPreview()
		if err != nil {
			audienceError = err.Error()
		} else {
			audienceError = ""
			audienceFrame = previewFrame{State: confirmedOutputState(), Pixels: pixels, At: time.Now()}
		}
	}
	if visible {
		top := previewTop() - pageScroll
		if top < 58 {
			top = 58
		}
		bottom := previewTop() + 280 - pageScroll
		if bottom > footerTop()-8 {
			bottom = footerTop() - 8
		}
		if bottom > top {
			invalidateLogical(RECT{244 - contentShift(), top, logicalWidth - 28, bottom})
		}
	}
}
func readGDIPreview() ([]byte, error) {
	if gdiLastBuffer.dc == 0 {
		return nil, fmt.Errorf("等待输出帧")
	}
	if gdiPreviewDC == 0 {
		gdiPreviewDC, _, _ = procCreateCompatibleDC.Call(gdiLastBuffer.dc)
		h := previewHeader()
		gdiPreviewBitmap, _, _ = gdi32.NewProc("CreateDIBSection").Call(gdiPreviewDC, uintptr(unsafe.Pointer(&h)), 0, uintptr(unsafe.Pointer(&gdiPreviewBits)), 0, 0)
		if gdiPreviewDC == 0 || gdiPreviewBitmap == 0 {
			releaseGDIPreview()
			return nil, fmt.Errorf("预览内存不足")
		}
		gdiPreviewOld, _, _ = procSelectObject.Call(gdiPreviewDC, gdiPreviewBitmap)
	}
	procBitBlt.Call(gdiPreviewDC, 0, 0, previewW, previewH, 0, 0, 0, BLACKNESS)
	w, h := fitRect(gdiLastBuffer.w, gdiLastBuffer.h, previewW, previewH)
	procSetStretchBltMode.Call(gdiPreviewDC, HALFTONE)
	gdi32.NewProc("SetBrushOrgEx").Call(gdiPreviewDC, 0, 0, 0)
	ok, _, _ := procStretchBlt.Call(gdiPreviewDC, uintptr((previewW-w)/2), uintptr((previewH-h)/2), uintptr(w), uintptr(h), gdiLastBuffer.dc, 0, 0, uintptr(gdiLastBuffer.w), uintptr(gdiLastBuffer.h), SRCCOPY)
	if ok == 0 {
		return nil, fmt.Errorf("预览缩放失败")
	}
	gdi32.NewProc("GdiFlush").Call()
	return append([]byte(nil), unsafe.Slice((*byte)(gdiPreviewBits), previewW*previewH*4)...), nil
}
func releaseGDIPreview() {
	if gdiPreviewOld != 0 {
		procSelectObject.Call(gdiPreviewDC, gdiPreviewOld)
	}
	if gdiPreviewBitmap != 0 {
		procDeleteObject.Call(gdiPreviewBitmap)
	}
	if gdiPreviewDC != 0 {
		procDeleteDC.Call(gdiPreviewDC)
	}
	gdiPreviewDC, gdiPreviewBitmap, gdiPreviewOld = 0, 0, 0
	gdiPreviewBits = nil
}
func resetAudiencePreview() { audienceFrame = previewFrame{}; audienceError = ""; releaseGDIPreview() }
func previewCurrent() bool {
	if !outputRunning || len(audienceFrame.Pixels) != previewW*previewH*4 {
		return false
	}
	if gpuOwnsOutput {
		return audienceFrame.matches(gpuController.generation, gpuController.sequence, gpuLastEvent.State)
	}
	return audienceFrame.State == confirmedOutputState()
}
func previewRoute() string {
	a, b := selectedDevice(cbSource), selectedDevice(cbOutput)
	if outputRunning {
		a, b = srcMonitor.Device, dstMonitor.Device
	}
	if a == "" || b == "" {
		return "选择我的屏幕 → 观众屏幕"
	}
	return strings.TrimPrefix(a, `\\.\`) + "  →  " + strings.TrimPrefix(b, `\\.\`)
}
func swapScreens() {
	if outputRunning {
		setStatus("请先停止投影再交换屏幕")
		return
	}
	a, b := comboIndex(cbSource), comboIndex(cbOutput)
	if a < 0 || b < 0 || a == b {
		return
	}
	procSendMessageW.Call(cbSource, CB_SETCURSEL, uintptr(b), 0)
	procSendMessageW.Call(cbOutput, CB_SETCURSEL, uintptr(a), 0)
	saveConfig()
	procInvalidateRect.Call(hwndMain, 0, 0)
}
func drawAudiencePreview(dc uintptr, right, top int32) {
	textLine(dc, "投影预览", RECT{264, top, right - 270, top + 28}, fontRow, false)
	mode := "标准预览"
	if preferences.PreviewEconomy {
		mode = "节能预览"
	}
	uiButton(dc, RECT{right - 260, top, right - 140, top + 28}, mode, false, true, func() {
		preferences.PreviewEconomy = !preferences.PreviewEconomy
		lastPreviewPoll = time.Time{}
		saveConfig()
		pollAudiencePreview()
	})
	toggle := "关闭预览"
	if !previewWanted {
		toggle = "开启预览"
	}
	uiButton(dc, RECT{right - 128, top, right - 20, top + 28}, toggle, false, true, func() {
		previewWanted = !previewWanted
		audienceFrame = previewFrame{}
		audienceError = ""
		pollAudiencePreview()
	})
	w := min32(400, right-284)
	h := w * 9 / 16
	r := RECT{264, top + 36, 264 + w, top + 36 + h}
	black, _, _ := procGetStockObject.Call(4)
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), black)
	current := previewCurrent()
	label := "等待输出"
	if !outputRunning {
		label = "尚未开始投影"
	} else if !previewWanted {
		label = "预览已关闭"
	} else if audienceError != "" {
		label = audienceError
	} else if current {
		header := previewHeader()
		gdi32.NewProc("StretchDIBits").Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(w), uintptr(h), 0, 0, previewW, previewH, uintptr(unsafe.Pointer(&audienceFrame.Pixels[0])), uintptr(unsafe.Pointer(&header)), 0, SRCCOPY)
		switch audienceFrame.State {
		case projectionFrozen:
			label = "已冻结"
		case projectionBlack:
			label = "已黑屏"
		default:
			label = "实时预览"
		}
		if time.Since(audienceFrame.At) > time.Second {
			label = "预览可能延迟"
		}
	}
	if current && previewWanted && audienceError == "" {
		badge := RECT{r.Right - 130, r.Top + 8, r.Right - 8, r.Top + 34}
		drawRounded(dc, badge, 5, brushControl, 0)
		textLine(dc, label, badge, fontSmall, false)
	} else {
		drawText(dc, label, r, fontDesc, uiMutedColor(), DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	}
	// Do not label the configured preview limit as measured FPS.
}
func drawHome(dc uintptr, right int32) {
	drawRounded(dc, RECT{244, 158, right, 220}, 8, brushCard, penBorder)
	textLine(dc, fmt.Sprintf("%d 个显示器 · %s", len(monitors), previewRoute()), RECT{264, 166, right - 20, 192}, fontRow, false)
	key := "黑屏快捷键正常"
	if !blackHotkeyOK {
		key = "黑屏快捷键不可用，可使用底栏按钮"
	}
	textLine(dc, key, RECT{264, 191, right - 20, 215}, fontSmall, true)
	drawAudiencePreview(dc, right, 236)
	_, target := mirrorPair(monitors, selectedDevice(cbOutput))
	uiButton(dc, RECT{264, 540, 444, 582}, "复制主屏", true, target >= 0 && !outputRunning && gpuActive == nil, startPrimaryMirror)
	uiButton(dc, RECT{456, 540, 636, 582}, "投影设置", false, true, func() { selectPage(1) })
	textLine(dc, "实时同步主屏；冻结后，本机可继续操作", RECT{264, 590, right - 20, 618}, fontSmall, true)
}
func showUsageGuide() {
	messageBox(hwndMain, "连接投影仪，按 Win+P 选择扩展。\n首页点击“复制主屏”，完整同步主屏画面。\n也可在投影控制中单独选择应用窗口。\n\n冻结：保留当前画面。黑屏：临时隐藏内容。\n恢复：继续投影。停止：关闭输出窗口，露出副屏桌面。\n\n窗口投影时可以操作其他软件；选定窗口最小化或关闭后会黑屏。", "使用说明", MB_ICONINFORMATION)
}
