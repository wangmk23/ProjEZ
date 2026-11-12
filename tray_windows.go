//go:build windows && amd64

package main

import (
	"syscall"
	"unsafe"
)

const trayMessage = 0x8040
const trayShow = 0x7100
const trayFreeze = 0x7101
const trayResume = 0x7102
const trayBlack = 0x7103
const trayStop = 0x7104
const trayExit = 0x7105

type trayIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                syscall.GUID
	BalloonIcon         uintptr
}

var trayAdded, controlHidden, pendingTrayRestore bool
var taskbarCreatedMessage, _, _ = user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated"))))

func trayData() trayIconData {
	d := trayIconData{Window: hwndMain, ID: 1, Flags: 7, Callback: trayMessage, Icon: appIcon}
	d.Size = uint32(unsafe.Sizeof(d))
	copy(d.Tip[:], syscall.StringToUTF16("ProjEZ · 双击打开，右键控制投影"))
	return d
}
func ensureTray() bool {
	if trayAdded {
		return true
	}
	if hwndMain == 0 {
		return false
	}
	d := trayData()
	if d.Icon == 0 {
		d.Icon, _, _ = procLoadIconW.Call(0, IDI_APPLICATION)
	}
	ok, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&d)))
	trayAdded = ok != 0
	return trayAdded
}
func removeTray() {
	if trayAdded {
		d := trayData()
		syscall.NewLazyDLL("shell32.dll").NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&d)))
	}
	trayAdded = false
}

var hideControlToTray = func() bool {
	if !ensureTray() {
		setStatus("无法创建托盘图标，窗口保持打开")
		return false
	}
	user32.NewProc("SetFocus").Call(hwndMain)
	controlHidden = true
	procShowWindow.Call(hwndMain, 0)
	pollAudiencePreview()
	return true
}
var exitApplication = func() {
	pendingTrayRestore = false
	if gpuActive != nil {
		closingMain = true
		stopOutput()
		return
	}
	stopOutput()
	procDestroyWindow.Call(hwndMain)
}

func handleMainClose() {
	if preferences.CloseToTray {
		hideControlToTray()
		return
	}
	exitApplication()
}
func showControlWindow() {
	pendingTrayRestore = false
	controlHidden = false
	visible, _, _ := user32.NewProc("IsIconic").Call(hwndMain)
	if visible != 0 {
		procShowWindow.Call(hwndMain, 9)
	} else {
		procShowWindow.Call(hwndMain, 5)
	}
	user32.NewProc("SetForegroundWindow").Call(hwndMain)
	pollAudiencePreview()
}
func requestTrayRestore() {
	// On older systems, never reveal an unexcluded control window over a live mirror.
	if outputRunning && activeSourceWindow.HWND == 0 && !controlCaptureExcluded && confirmedOutputState() != projectionBlack {
		pendingTrayRestore = true
		blackOutput()
		return
	}
	showControlWindow()
}
func finishTrayRestore() {
	if pendingTrayRestore && (!outputRunning || confirmedOutputState() == projectionBlack) {
		showControlWindow()
	}
}
func showTrayMenu() {
	menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user32.NewProc("DestroyMenu").Call(menu)
	for _, entry := range []struct {
		id   uintptr
		text string
	}{{trayShow, "打开 ProjEZ"}, {trayFreeze, "冻结画面"}, {trayResume, "恢复实时"}, {trayBlack, "黑屏"}, {trayStop, "停止投影"}, {trayExit, "退出程序"}} {
		flags := uintptr(0)
		if entry.id >= trayFreeze && entry.id <= trayStop && !outputRunning {
			flags = 1
		}
		user32.NewProc("AppendMenuW").Call(menu, flags, entry.id, uintptr(unsafe.Pointer(utf16Ptr(entry.text))))
	}
	var p POINT
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	user32.NewProc("SetForegroundWindow").Call(hwndMain)
	id, _, _ := user32.NewProc("TrackPopupMenu").Call(menu, 0x0102, uintptr(int64(p.X)), uintptr(int64(p.Y)), 0, hwndMain, 0)
	procPostFrame.Call(hwndMain, 0, 0, 0)
	switch id {
	case trayShow:
		requestTrayRestore()
	case trayFreeze:
		freezeOutput()
	case trayResume:
		resumeOutput()
	case trayBlack:
		blackOutput()
	case trayStop:
		stopOutput()
	case trayExit:
		exitApplication()
	}
}
func trayWindowMessage(msg uint32, wp, lp uintptr) bool {
	if taskbarCreatedMessage != 0 && msg == uint32(taskbarCreatedMessage) {
		trayAdded = false
		if !ensureTray() && controlHidden {
			requestTrayRestore()
		}
		return true
	}
	if msg != trayMessage {
		return false
	}
	switch uint32(lp) {
	case 0x0203:
		requestTrayRestore()
	case 0x0205, 0x007b:
		showTrayMenu()
	}
	return true
}
