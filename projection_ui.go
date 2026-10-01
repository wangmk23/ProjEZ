package main

import "fmt"

var gpuActive *gpuSession
var gpuController projectionController
var gpuOwnsOutput bool
var gpuLastEvent projectionEvent
var closingMain bool
var gpuFallbackReason string

func currentGPUConfig() projectionConfig {
	fps := activeTargetFPS
	if source := monitorRates[srcMonitor.Device]; source.known && source.current > 0 && (fps <= 0 || source.current < fps) {
		fps = source.current
	}
	return projectionConfig{Generation: gpuController.generation, HWND: hwndOutput, Source: srcMonitor, Target: dstMonitor, TargetFPS: fps, Fill: preferences.Fill, Smooth: preferences.Smooth, Cursor: showCursor, StartBlack: blackout, SourceWindow: activeSourceWindow.HWND, SourcePID: activeSourceWindow.PID}
}
func beginProjectionBackend() {
	gpuFallbackReason = ""
	if preferences.Compatibility && activeSourceWindow.HWND == 0 {
		beginGDIProjection("手动兼容模式")
		return
	}
	gpuController.begin()
	gpuOwnsOutput = true
	gpuLastEvent = projectionEvent{State: projectionStarting, Backend: "DirectX GPU"}
	gpuActive = startGPUSession(currentGPUConfig())
	procSetTimer.Call(hwndMain, gpuPollTimer, 100, 0)
	setStatus("正在初始化 GPU 投影…")
}
func beginGDIProjection(reason string) {
	if activeSourceWindow.HWND != 0 {
		blackout = true
	}
	resetAudiencePreview()
	gpuOwnsOutput = false
	gpuFallbackReason = reason
	if blackout {
		blackOutput()
		return
	}
	frozen = false
	gdiAwaitLive = true
	captureFrame()
	startFramePump()
	procInvalidateRect.Call(hwndOutput, 0, 0)
	setStatus("GDI 兼容模式：" + reason)
}
func sendGPUCommand(state projectionState) {
	if gpuActive == nil {
		return
	}
	cmd := gpuController.request(state)
	cmd.Config = currentGPUConfig()
	gpuActive.request(cmd)
}
func updateGPUSettings() {
	if gpuActive == nil {
		return
	}
	state := gpuController.desired
	if state == projectionStarting || state == projectionRecovering {
		return
	}
	sendGPUCommand(state)
}
func pollGPUEvents() {
	if gpuActive == nil {
		return
	}
	e, ok := gpuActive.drain()
	if !ok {
		return
	}
	terminal := e.State == projectionStopped || e.State == projectionFailed
	if e.Generation != gpuController.generation {
		return
	}
	if !terminal && !gpuController.accept(e) {
		return
	}
	gpuLastEvent = e
	if terminal {
		select {
		case <-gpuActive.done:
		default:
			return
		}
		stopping := gpuController.desired == projectionStopping || closingMain
		gpuActive = nil
		procKillTimer.Call(hwndMain, gpuPollTimer)
		if activeSourceWindow.HWND != 0 && !stopping {
			blackout = true
			gpuOwnsOutput = false
			beginGDIProjection("窗口投影已暂停：" + e.Reason)
			setStatus("窗口已暂停：" + e.Reason + "；请停止后重新开始")
			return
		}
		if blackout && !stopping {
			gpuOwnsOutput = false
			beginGDIProjection("GPU故障，保持黑屏：" + e.Reason)
			return
		}
		if e.Fallback && !stopping {
			beginGDIProjection(e.Reason)
			return
		}
		if e.State == projectionFailed && !stopping {
			frozen = true
			setStatus("投影故障（未恢复实时）：" + e.Reason)
			return
		}
		gpuOwnsOutput = false
		stopOutput()
		if closingMain && hwndMain != 0 {
			procDestroyWindow.Call(hwndMain)
		}
		return
	}
	switch e.State {
	case projectionLive:
		blackout, blackAcknowledged = false, false
		frozen = false
		setStatus("GPU 实时投影")
	case projectionBlack:
		blackout, blackAcknowledged = true, true
		frozen = false
		setStatus("已黑屏：恢复需手动确认")
	case projectionFrozen:
		frozen = true
		setStatus("已冻结：本机可继续操作")
	case projectionRecovering:
		setStatus("显示设备变化，正在恢复…")
	}
	if e.Reason != "" {
		setStatus(e.Reason)
	}
}
func projectionBackendSummary() string {
	if gpuOwnsOutput {
		if gpuLastEvent.State == projectionFailed {
			return "GPU 已停止更新 · 请停止输出后重新开始"
		}
		return fmt.Sprintf("DirectX GPU · 捕获 %.0f / 提交 %.0f FPS", gpuLastEvent.CaptureFPS, gpuLastEvent.PresentFPS)
	}
	if outputRunning {
		return fmt.Sprintf("GDI 兼容 · 绘制 %.0f FPS", measuredFPS)
	}
	return "自动 GPU 优先 · 不支持时使用兼容模式"
}

func projectionScaleSummary() string {
	if preferences.WindowMode || (outputRunning && activeSourceWindow.HWND != 0) {
		return "窗口投影：画面尺寸随应用窗口变化"
	}
	a, b := comboIndex(cbSource), comboIndex(cbOutput)
	var source, target Monitor
	if outputRunning {
		source, target = srcMonitor, dstMonitor
	} else {
		if a < 0 || b < 0 || a >= len(monitors) || b >= len(monitors) {
			return "选择两块屏幕后显示缩放尺寸"
		}
		source, target = monitors[a], monitors[b]
	}
	sw, sh := source.Rect.Right-source.Rect.Left, source.Rect.Bottom-source.Rect.Top
	tw, th := target.Rect.Right-target.Rect.Left, target.Rect.Bottom-target.Rect.Top
	dw, dh := outputSize(sw, sh, tw, th, preferences.Fill)
	mode := "缩放"
	if sw == dw && sh == dh {
		mode = "原尺寸"
	} else if dw < sw || dh < sh {
		mode = "缩小"
	}
	return fmt.Sprintf("%d×%d → %d×%d · %s", sw, sh, dw, dh, mode)
}
