//go:build windows && amd64

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var setProjectionDWMAttribute = func(hwnd uintptr, attribute uint32, value int32) error {
	hr, _, _ := procDwmSetWindowAttribute.Call(hwnd, uintptr(attribute), uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
	return hrError("DwmSetWindowAttribute", hr)
}

// The control window opts out of fullscreen, but the audience window must
// be fullscreen so the target screen's native taskbar does not overlay it.
func configureProjectionTaskbar(hwnd uintptr) error {
	if hwnd == 0 {
		return fmt.Errorf("invalid projection window")
	}
	// Taskbar live previews must not make a frozen/black output transparent.
	// Attribute 12 is DWMWA_EXCLUDED_FROM_PEEK; it affects only this window.
	if err := setProjectionDWMAttribute(hwnd, 12, 1); err != nil {
		return err
	}
	user32.NewProc("RemovePropW").Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr("NonRudeHWND"))))
	return markProjectionFullscreen(hwnd, true)
}

// Use the Shell's per-window API; never hide Shell_TrayWnd, change taskbar
// preferences, move focus, or modify other monitors' taskbar windows.
func markProjectionFullscreen(hwnd uintptr, fullscreen bool) error {
	if valid, _, _ := user32.NewProc("IsWindow").Call(hwnd); valid == 0 {
		return fmt.Errorf("invalid projection window")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := syscall.NewLazyDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) >= 0 {
		defer ole.NewProc("CoUninitialize").Call()
	} else if uint32(hr) != 0x80010106 { // Already initialized in another apartment is usable.
		return hrError("CoInitializeEx", hr)
	}
	clsid := syscall.GUID{Data1: 0x56fdf344, Data2: 0xfd6d, Data3: 0x11d0, Data4: [8]byte{0x95, 0x8a, 0x00, 0x60, 0x97, 0xc9, 0xa0, 0x90}}
	iid := syscall.GUID{Data1: 0x602d4995, Data2: 0xb13a, Data3: 0x429b, Data4: [8]byte{0xa6, 0x6e, 0x19, 0x35, 0xe4, 0x4f, 0x43, 0x17}}
	var taskbar comPtr
	hr, _, _ = ole.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&taskbar)))
	if err := hrError("CoCreateInstance(ITaskbarList2)", hr); err != nil {
		return err
	}
	defer releaseCOM(&taskbar)
	if err := hrError("ITaskbarList.HrInit", comCall(taskbar, 3)); err != nil {
		return err
	}
	flag := uintptr(0)
	if fullscreen {
		flag = 1
	}
	return hrError("ITaskbarList2.MarkFullscreenWindow", comCall(taskbar, 8, hwnd, flag))
}
