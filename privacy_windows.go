package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const privacyTimer = 0x4753
const wmPrivacyForeground = 0x8006

var privacy privacyGuard
var privacyHook uintptr
var privacyDeadline time.Time
var privacyTitle string
var privacyNote = "辅助保护已关闭"
var privacyCallback = syscall.NewCallback(func(hook, event, hwnd, obj, child, thread, ms uintptr) uintptr {
	// OUTOFCONTEXT delivery uses the installing UI thread. Defer UI/output work.
	if hwndMain != 0 {
		procPostFrame.Call(hwndMain, wmPrivacyForeground, hwnd, 0)
	}
	return 0
})

func windowPID(hwnd uintptr) uint32 {
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}
func foregroundWindow() uintptr { h, _, _ := user32.NewProc("GetForegroundWindow").Call(); return h }
func privacyWindowInfo(hwnd uintptr) (uint32, uintptr, bool) {
	pid := windowPID(hwnd)
	owner, _, _ := user32.NewProc("GetAncestor").Call(hwnd, 3) // GA_ROOTOWNER
	valid, _, _ := user32.NewProc("IsWindow").Call(privacy.target)
	return pid, owner, valid != 0 && windowPID(privacy.target) == privacy.pid
}
func privacyResumeAllowed() bool {
	if !privacy.enabled || activeSourceWindow.HWND != 0 {
		return true
	}
	hwnd := foregroundWindow()
	pid, owner, valid := privacyWindowInfo(hwnd)
	if !privacyDeadline.IsZero() || !privacy.resume(hwnd, pid, owner, valid) {
		setStatus("保护中：切回已选演示窗口，再按恢复快捷键")
		return false
	}
	privacyNote = "正在监测演示窗口；切走将请求黑屏"
	return true
}
func checkPrivacyForeground(hwnd uintptr) {
	if !privacy.enabled || !outputRunning || activeSourceWindow.HWND != 0 {
		return
	}
	pid, owner, valid := privacyWindowInfo(hwnd)
	if privacy.observe(hwnd, pid, owner, valid) {
		blackOutput()
		privacyNote = "已请求保护黑屏；回到演示窗口后按恢复快捷键"
		procInvalidateRect.Call(hwndMain, 0, 0)
	}
}
func setPrivacyEnabled(on bool) {
	if !on {
		privacy.enabled = false
		privacyDeadline = time.Time{}
		privacyNote = "辅助保护已关闭；已有黑屏不会自动恢复"
		if privacyHook != 0 {
			user32.NewProc("UnhookWinEvent").Call(privacyHook)
			privacyHook = 0
		}
		procKillTimer.Call(hwndMain, privacyTimer)
		return
	}
	if privacyHook == 0 {
		privacyHook, _, _ = user32.NewProc("SetWinEventHook").Call(3, 3, 0, privacyCallback, 0, 0, 0)
	}
	if privacyHook == 0 {
		privacyNote = "无法启用前台监测，辅助保护未开启"
		setStatus(privacyNote)
		return
	}
	if timer, _, _ := procSetTimer.Call(hwndMain, privacyTimer, 100, 0); timer == 0 {
		user32.NewProc("UnhookWinEvent").Call(privacyHook)
		privacyHook = 0
		privacyNote = "无法创建监测计时器，辅助保护未开启"
		return
	}
	privacy = privacyGuard{enabled: true, latched: true}
	privacyTitle = ""
	privacyNote = "请点击选择演示窗口；开启后从黑屏开始"
	if outputRunning {
		blackOutput()
	}
}
func choosePrivacyWindow() {
	if !privacy.enabled {
		setPrivacyEnabled(true)
	}
	if !privacy.enabled {
		return
	}
	privacy.latched = true
	privacy.target = 0
	privacy.pid = 0
	privacyTitle = ""
	privacyDeadline = time.Now().Add(5 * time.Second)
	privacyNote = "5秒内切到演示窗口；选择后仍保持黑屏"
	if outputRunning {
		blackOutput()
	}
}
func pollPrivacy() {
	if !privacy.enabled {
		return
	}
	if !privacyDeadline.IsZero() {
		left := time.Until(privacyDeadline)
		if left > 0 {
			note := fmt.Sprintf("%d秒内切到演示窗口；选择后不会自动开播", int(left.Seconds())+1)
			if note != privacyNote {
				privacyNote = note
				procInvalidateRect.Call(hwndMain, 0, 0)
			}
			return
		}
		privacyDeadline = time.Time{}
		h := foregroundWindow()
		pid := windowPID(h)
		ownPID, _, _ := kernel32.NewProc("GetCurrentProcessId").Call()
		shell, _, _ := user32.NewProc("GetShellWindow").Call()
		var class [128]uint16
		user32.NewProc("GetClassNameW").Call(h, uintptr(unsafe.Pointer(&class[0])), 128)
		name := syscall.UTF16ToString(class[:])
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(h)
		if h == 0 || pid == 0 || pid == uint32(ownPID) || h == shell || visible == 0 || name == "Progman" || name == "WorkerW" || name == "Shell_TrayWnd" || name == "Shell_SecondaryTrayWnd" {
			privacyNote = "未选到演示窗口，请重新选择（不支持桌面或本程序）"
		} else {
			var title [256]uint16
			user32.NewProc("GetWindowTextW").Call(h, uintptr(unsafe.Pointer(&title[0])), 256)
			privacy.target, privacy.pid = h, pid
			privacyTitle = syscall.UTF16ToString(title[:])
			privacyNote = "窗口已选定；开始投影后，切回该窗口按恢复快捷键"
		}
		procInvalidateRect.Call(hwndMain, 0, 0)
	}
	checkPrivacyForeground(foregroundWindow())
}
func drawPrivacyControls(dc uintptr, right int32) {
	drawRounded(dc, RECT{244, 554, right, 788}, 8, brushCard, penBorder)
	textLine(dc, "切离演示窗口时黑屏（辅助保护）", RECT{264, 564, right - 88, 594}, fontRow, false)
	uiToggle(dc, RECT{right - 76, 566, right - 28, 592}, privacy.enabled, func() { setPrivacyEnabled(!privacy.enabled) })
	uiButton(dc, RECT{264, 604, 466, 642}, "选择演示窗口（5秒）", false, true, choosePrivacyWindow)
	textLine(dc, privacyNote, RECT{264, 650, right - 20, 678}, fontSmall, true)
	title := privacyTitle
	if title == "" {
		title = "尚未选定窗口；每次启动需要重新启用"
	}
	textLine(dc, title, RECT{264, 680, right - 20, 706}, fontDesc, false)
	textLine(dc, "返回窗口不自动恢复；请使用快捷键页的恢复组合键。", RECT{264, 713, right - 20, 739}, fontSmall, true)
	textLine(dc, "可能有过渡帧；不抢焦点的通知不会触发。敏感操作前请先黑屏。", RECT{264, 745, right - 20, 775}, fontSmall, true)
}
