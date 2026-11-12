package main

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestProjectionAndControlTaskbarPolicies(t *testing.T) {
	if shell, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(utf16Ptr("Shell_TrayWnd"))), 0); shell == 0 {
		t.Skip("interactive Explorer desktop required for native Shell integration")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := procGetModuleHandleW.Call(0)
	create := func() uintptr {
		h, _, _ := procCreateWindowExW.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, uintptr(unsafe.Pointer(utf16Ptr("STATIC"))), 0, WS_POPUP, 0, 0, 160, 90, 0, 0, instance, 0)
		if h == 0 {
			t.Fatal("create hidden test window")
		}
		return h
	}
	control, output := create(), create()
	defer procDestroyWindow.Call(control)
	defer procDestroyWindow.Call(output)
	prop := uintptr(unsafe.Pointer(utf16Ptr("NonRudeHWND")))
	getProp := user32.NewProc("GetPropW")
	if !preserveTaskbarControls(control) || !preserveTaskbarControls(output) {
		t.Fatal("set baseline protection")
	}
	oldDWM := setProjectionDWMAttribute
	excluded := false
	setProjectionDWMAttribute = func(h uintptr, attribute uint32, value int32) error {
		if h != output {
			t.Fatal("modified unrelated window Peek policy")
		}
		err := oldDWM(h, attribute, value)
		if attribute == 12 && value == 1 && err == nil {
			excluded = true
		}
		return err
	}
	defer func() { setProjectionDWMAttribute = oldDWM }()
	// An output window must not inherit the control window's opt-out from fullscreen.
	if err := configureProjectionTaskbar(output); err != nil {
		t.Fatal(err)
	}
	defer markProjectionFullscreen(output, false)
	if !excluded {
		t.Fatal("projection did not opt out of taskbar Peek transparency")
	}

	if v, _, _ := getProp.Call(output, prop); v != 0 {
		t.Fatal("projection still forces native taskbar above captured taskbar")
	}
	if v, _, _ := getProp.Call(control, prop); v != 1 {
		t.Fatal("control taskbar protection changed")
	}
	if err := markProjectionFullscreen(output, false); err != nil {
		t.Fatal("fullscreen cleanup", err)
	}
	if err := configureProjectionTaskbar(0); err == nil {
		t.Fatal("invalid window accepted")
	}
}
