//go:build windows && amd64

package main

import (
	"syscall"
	"unsafe"
)

var controlCaptureExcluded, controlCaptureExclusionAvailable bool

var supportsControlExclusion = func() bool {
	version := struct {
		Size, Major, Minor, Build, Platform uint32
		Description                         [128]uint16
	}{}
	version.Size = uint32(unsafe.Sizeof(version))
	hr, _, _ := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&version)))
	return hr == 0 && (version.Major > 10 || version.Major == 10 && version.Build >= 19041)
}

func excludeControlWindow(hwnd uintptr) bool {
	if !supportsControlExclusion() {
		return false
	}
	set := user32.NewProc("SetWindowDisplayAffinity")
	if set.Find() != nil {
		return false
	}
	ok, _, _ := set.Call(hwnd, 0x11)
	if ok == 0 {
		return false
	}
	var affinity uint32
	ok, _, _ = user32.NewProc("GetWindowDisplayAffinity").Call(hwnd, uintptr(unsafe.Pointer(&affinity)))
	return ok != 0 && affinity == 0x11
}
func clearControlCaptureExclusion() {
	if hwndMain != 0 {
		user32.NewProc("SetWindowDisplayAffinity").Call(hwndMain, 0)
	}
	controlCaptureExcluded = false
}
func probeControlCaptureExclusion() {
	controlCaptureExclusionAvailable = excludeControlWindow(hwndMain)
	clearControlCaptureExclusion()
}

var ensureControlCapturePolicy = func() bool {
	controlCaptureExcluded = excludeControlWindow(hwndMain)
	controlCaptureExclusionAvailable = controlCaptureExcluded
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwndMain)
	iconic, _, _ := user32.NewProc("IsIconic").Call(hwndMain)
	if controlCaptureExcluded || visible == 0 || iconic != 0 {
		return true
	}
	choice, _, _ := procMessageBoxW.Call(hwndMain, uintptr(unsafe.Pointer(utf16Ptr("当前系统或捕获环境不支持排除 ProjEZ 窗口。是否先收起到托盘，再开始整屏投影？"))), uintptr(unsafe.Pointer(utf16Ptr("整屏投影"))), 0x124)
	if choice == 6 {
		return hideControlToTray()
	}
	return false
}
