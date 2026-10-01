//go:build windows

package main

import (
	"fmt"
	"unsafe"
)

// DEVMODEW's display layout; the printer union occupies the same 16 bytes.
type displayMode struct {
	DeviceName                                                                                     [32]uint16
	SpecVersion, DriverVersion, Size, DriverExtra                                                  uint16
	Fields                                                                                         uint32
	Position                                                                                       POINT
	Orientation, FixedOutput                                                                       uint32
	Color, Duplex, YResolution, TTOption, Collate                                                  int16
	FormName                                                                                       [32]uint16
	LogPixels                                                                                      uint16
	BitsPerPel, Width, Height, Flags, Frequency                                                    uint32
	ICMMethod, ICMIntent, MediaType, DitherType, Reserved1, Reserved2, PanningWidth, PanningHeight uint32
}

type displayRates struct {
	current, maximum int
	mode             displayMode
	known            bool
}

var monitorRates = map[string]displayRates{}
var restoredDevice string
var originalFrequency, appliedFrequency uint32
var activeTargetFPS int

// Isolate the OS mode boundary so fallback/restore behavior can be tested without
// changing the developer's real display settings.
var getOutputRates = queryDisplayRates
var setOutputFrequency = changeFrequency
var getOutputMode = readDisplayMode

const wmRefreshDisplays = 0x8002

var displayRefreshQueued bool

func queueDisplayRefresh() {
	releaseCursorControl()
	if !displayRefreshQueued && hwndMain != 0 {
		ok, _, _ := procPostFrame.Call(hwndMain, wmRefreshDisplays, 0, 0)
		displayRefreshQueued = ok != 0
	}
}

func handleDisplayRefresh() {
	displayRefreshQueued = false
	refreshMonitors()
	refreshConnectedOutput()
}

func refreshConnectedOutput() {
	if !outputRunning {
		return
	}
	si, di := findMonitorIndex(srcMonitor.Device), findMonitorIndex(dstMonitor.Device)
	if si < 0 || di < 0 {
		stopOutput()
		setStatus("显示器连接已变化，投影已停止，请重新选择目标")
		return
	}
	if srcMonitor.Rect != monitors[si].Rect || dstMonitor.Rect != monitors[di].Rect {
		stopOutput()
		setStatus("屏幕尺寸或位置变化，正在停止；请重新开始投影")
		return
	}
	if reason := outputPairError(monitors[si], monitors[di]); reason != "" {
		stopOutput()
		setStatus(reason)
		return
	}
	srcMonitor, dstMonitor = monitors[si], monitors[di]
	updateCursorControl()
	r := dstMonitor.Rect
	procSetWindowPos.Call(hwndOutput, HWND_TOPMOST, uintptr(int64(r.Left)), uintptr(int64(r.Top)), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), SWP_NOACTIVATE)
	// An external Windows mode change takes precedence over our original choice.
	// Follow it without fighting the user's display settings or forcing another switch.
	activeTargetFPS = monitorRates[dstMonitor.Device].current
	if preferences.FPS > 0 && preferences.FPS < activeTargetFPS {
		activeTargetFPS = preferences.FPS
	}
	if !frozen && !blackout {
		startFramePump()
	}
	if gpuOwnsOutput {
		updateGPUSettings()
	}
	procInvalidateRect.Call(hwndOutput, 0, 0)
}

func readDisplayMode(device string, index uint32) (displayMode, bool) {
	dm := displayMode{}
	dm.Size = uint16(unsafe.Sizeof(dm))
	ok, _, _ := user32.NewProc("EnumDisplaySettingsExW").Call(uintptr(unsafe.Pointer(utf16Ptr(device))), uintptr(index), uintptr(unsafe.Pointer(&dm)), 0)
	return dm, ok != 0
}

func maximumModeRate(current displayMode, modes []displayMode) int {
	best := int(current.Frequency)
	if best <= 1 {
		best = 60
	}
	for _, mode := range modes {
		if mode.Width == current.Width && mode.Height == current.Height && mode.BitsPerPel == current.BitsPerPel && mode.Orientation == current.Orientation && mode.Flags&2 == 0 && int(mode.Frequency) > best {
			best = int(mode.Frequency)
		}
	}
	return best
}

func queryDisplayRates(device string) displayRates {
	dm, ok := readDisplayMode(device, ^uint32(0))
	if !ok {
		return displayRates{current: 60, maximum: 60}
	}
	var modes []displayMode
	for i := uint32(0); ; i++ {
		mode, ok := readDisplayMode(device, i)
		if !ok {
			break
		}
		modes = append(modes, mode)
	}
	current := int(dm.Frequency)
	if current <= 1 {
		current = 60
	}
	return displayRates{current: current, maximum: maximumModeRate(dm, modes), mode: dm, known: dm.Frequency > 1}
}

func refreshDisplayRates() {
	monitorRates = map[string]displayRates{}
	for _, m := range monitors {
		monitorRates[m.Device] = queryDisplayRates(m.Device)
		observeExternalRate(m.Device, monitorRates[m.Device])
	}
}

func observeExternalRate(device string, rates displayRates) {
	if restoredDevice == device && rates.known && uint32(rates.current) != appliedFrequency {
		// Once the user changes the mode, it is theirs even if they later select
		// the same frequency we previously applied. Do not restore over that choice.
		restoredDevice = ""
	}
}

func selectedDisplayRates() displayRates {
	device := selectedDevice(cbOutput)
	if outputRunning {
		device = dstMonitor.Device
	}
	if rates, ok := monitorRates[device]; ok {
		return rates
	}
	return displayRates{current: 60, maximum: 60}
}

func changeFrequency(device string, frequency uint32) bool {
	dm := displayMode{Size: uint16(unsafe.Sizeof(displayMode{})), Fields: 0x400000, Frequency: frequency}
	name := utf16Ptr(device)
	proc := user32.NewProc("ChangeDisplaySettingsExW")
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dm)), 0, 2, 0) // CDS_TEST
	if int32(result) != 0 {
		return false
	}
	result, _, _ = proc.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dm)), 0, 4, 0) // CDS_FULLSCREEN: temporary, no registry write
	return int32(result) == 0
}

func restoreRefreshRate() bool {
	if restoredDevice == "" {
		return true
	}
	device, original, applied := restoredDevice, originalFrequency, appliedFrequency
	current, ok := getOutputMode(device, ^uint32(0))
	// Preserve an external change made by the user while projection was running.
	if ok && current.Frequency == applied {
		if !setOutputFrequency(device, original) {
			return false
		}
	}
	restoredDevice = ""
	return true
}

func configureOutputRate() string {
	note := ""
	if preferences.FPS != 0 && restoredDevice != "" {
		if !restoreRefreshRate() {
			note = "未能恢复原刷新率，请在 Windows 显示设置中检查。"
		}
	}
	rates := getOutputRates(dstMonitor.Device)
	if preferences.FPS == 0 && rates.known && rates.maximum > rates.current {
		original := uint32(rates.current)
		if setOutputFrequency(dstMonitor.Device, uint32(rates.maximum)) {
			if restoredDevice == "" {
				restoredDevice, originalFrequency = dstMonitor.Device, original
			}
			appliedFrequency = uint32(rates.maximum)
			rates = getOutputRates(dstMonitor.Device)
		} else {
			note = "最高刷新率切换失败，已跟随当前显示设置。"
		}
	}
	monitorRates[dstMonitor.Device] = rates
	activeTargetFPS = rates.current
	if preferences.FPS > 0 && preferences.FPS < activeTargetFPS {
		activeTargetFPS = preferences.FPS
	}
	return note
}

func displayRateSummary() string {
	r := selectedDisplayRates()
	if !r.known {
		return "刷新率未能读取，自动模式暂按 60 FPS 尝试。"
	}
	text := fmt.Sprintf("目标屏幕：当前 %d Hz · 此分辨率最高 %d Hz", r.current, r.maximum)
	if outputRunning && !frozen && !gpuOwnsOutput {
		text += fmt.Sprintf(" · 目标 %d / 实际绘制 %.0f FPS", activeTargetFPS, measuredFPS)
	}
	return text
}
