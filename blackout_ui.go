package main

import (
	"fmt"
	"time"
	"unsafe"
)

const hotkeyBlack = 1003
const dockTimer = 0x4751

var blackout, blackAcknowledged, gdiPresented, gdiAwaitLive bool
var gdiLastBuffer paintBuffer
var gdiSourceValid bool
var capturedGDI uint64
var blackHotkeyOK bool
var sessionStarted time.Time

func registerReleaseCursorHotkey() {
	procUnregisterHotKey.Call(hwndMain, hotkeyReleaseCursor)
	ok, _, _ := procRegisterHotKey.Call(hwndMain, hotkeyReleaseCursor, 0x4003, 0x7a)
	releaseCursorHotkeyOK = ok != 0
}

func blackOutput() {
	if !outputRunning || hwndOutput == 0 {
		return
	}
	if gpuActive != nil && gpuController.desired == projectionStopping {
		return
	}
	blackout, blackAcknowledged = true, false
	gdiAwaitLive = false
	setStatus("正在黑屏…")
	if gpuActive != nil {
		sendGPUCommand(projectionBlack)
		return
	}
	if gpuOwnsOutput {
		setStatus("输出故障，无法确认黑屏")
		return
	}
	stopFramePump()
	frozen = false
	procInvalidateRect.Call(hwndOutput, 0, 0)
	procUpdateWindow.Call(hwndOutput)
}
func dockState() string {
	if !outputRunning {
		return "待机"
	}
	if gpuActive != nil && gpuController.desired == projectionStopping {
		return "正在停止"
	}
	if blackout {
		if blackAcknowledged {
			return "已黑屏"
		}
		return "正在黑屏"
	}
	if frozen {
		return "已冻结"
	}
	if gpuActive != nil && gpuLastEvent.State != projectionLive {
		return "处理中"
	}
	if gdiAwaitLive {
		return "等待画面"
	}
	return "实时投影"
}
func dockMetricText() string {
	if !outputRunning {
		return "— FPS"
	}
	if blackout {
		return "— FPS · 黑屏"
	}
	if frozen {
		return "— FPS · 冻结"
	}
	if confirmedOutputState() != projectionLive {
		return "— FPS · 等待"
	}
	if gpuOwnsOutput {
		return fmt.Sprintf("%.0f FPS · GPU", gpuLastEvent.PresentFPS)
	}
	return fmt.Sprintf("%.0f FPS · GDI", measuredFPS)
}
func dockHeaderRects(right int32) (RECT, RECT, RECT) {
	top := footerTop()
	metric := RECT{right - 172, top + 2, right - 12, top + 26}
	state := RECT{256, top + 2, min32(438, metric.Left-8), top + 26}
	status := RECT{450, top + 2, metric.Left - 8, top + 26}
	return state, status, metric
}
func drawControlDock(dc uintptr, right int32) {
	top := footerTop()
	drawRounded(dc, RECT{244, top, right, top + 72}, 8, brushCard, penBorder)
	label := dockState()
	if outputRunning && logicalWidth >= 940 && !sessionStarted.IsZero() {
		n := int(time.Since(sessionStarted).Seconds())
		label += fmt.Sprintf("  %02d:%02d:%02d", n/3600, n/60%60, n%60)
	}
	stateRect, statusRect, metricRect := dockHeaderRects(right)
	textLine(dc, label, stateRect, fontDesc, false)
	if statusRect.Right-statusRect.Left >= 120 {
		textLine(dc, statusText, statusRect, fontSmall, true)
	}
	drawText(dc, dockMetricText(), metricRect, fontDesc, uiTextColor(), DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)

	stopping := gpuActive != nil && gpuController.desired == projectionStopping
	ready := sourceReady()
	first, run := "开始投影", startOutput
	if outputRunning {
		first, run = "恢复实时", resumeOutput
	}
	labels := []string{first, "冻结", "黑屏", "停止"}
	runs := []func(){run, freezeOutput, blackOutput, stopOutput}
	enabled := []bool{(!outputRunning && ready) || (outputRunning && (frozen || blackAcknowledged)), outputRunning && !frozen && !blackout, outputRunning && !blackAcknowledged, outputRunning}
	w := (right - 268 - 24) / 4
	for i := 0; i < 4; i++ {
		x := int32(256) + int32(i)*(w+8)
		r := RECT{x, top + 29, x + w, top + 63}
		uiButton(dc, r, labels[i], i == 0, enabled[i] && !stopping, runs[i])
		if ptInRect(actionHoverPoint(), r) && (!enabled[i] || i == 3) {
			reason := "当前状态不可用"
			if i == 0 && !outputRunning {
				reason = "请连接副屏，并选择不同的来源和目标"
			}
			if i == 3 {
				reason = "停止会关闭遮罩，目标返回 Windows 桌面"
			}
			// Replace the status line while hovering; disabled actions remain non-clickable.
			hintRect := RECT{256, top + 2, metricRect.Left - 8, top + 27}
			procFillRect.Call(dc, uintptr(unsafe.Pointer(&hintRect)), brushCard)
			textLine(dc, reason, hintRect, fontSmall, true)
		}
	}
}
