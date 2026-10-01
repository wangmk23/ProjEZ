package main

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestPrivacyNativeForegroundProtection(t *testing.T) {
	if os.Getenv("PF_PRIVACY_HELPER") != "1" {
		exe, e := os.Executable()
		if e != nil {
			t.Fatal(e)
		}
		cmd := exec.Command(exe, "-test.run=^TestPrivacyNativeForegroundProtection$", "-test.v")
		cmd.Env = append(os.Environ(), "PF_PRIVACY_HELPER=1", "APPDATA="+t.TempDir())
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("privacy helper: %v %s", e, out)
		}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := procGetModuleHandleW.Call(0)
	callback := syscall.NewCallback(func(h uintptr, m uint32, w, l uintptr) uintptr {
		if m == wmPrivacyForeground {
			checkPrivacyForeground(w)
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(h, uintptr(m), w, l)
		return r
	})
	name := utf16Ptr("PFPrivacyNativeTest")
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), LpfnWndProc: callback, HInstance: instance, LpszClassName: name}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	create := func(owner uintptr) uintptr {
		h, _, _ := procCreateWindowExW.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, uintptr(unsafe.Pointer(name)), 0, WS_POPUP, 0, 0, 20, 20, owner, 0, instance, 0)
		if h == 0 {
			t.Fatal("create")
		}
		return h
	}
	hwndMain = create(0)
	defer procDestroyWindow.Call(hwndMain)
	target := create(0)
	defer procDestroyWindow.Call(target)
	modal := create(target)
	defer procDestroyWindow.Call(modal)
	other := create(0)
	defer procDestroyWindow.Call(other)
	setPrivacyEnabled(true)
	defer setPrivacyEnabled(false)
	if !privacy.enabled || privacyHook == 0 {
		t.Fatal("native hook not registered")
	}
	privacy.target, privacy.pid, privacy.latched = target, windowPID(target), false
	pid, owner, valid := privacyWindowInfo(modal)
	if !privacy.allows(modal, pid, owner, valid) {
		t.Fatal("owned native dialog rejected")
	}
	pid, owner, valid = privacyWindowInfo(other)
	if privacy.allows(other, pid, owner, valid) {
		t.Fatal("unrelated same-process window allowed")
	}
	outputRunning = true
	hwndOutput = target
	gpuController.begin()
	gpuActive = newGPUMailbox(currentGPUConfig())
	user32.NewProc("NotifyWinEvent").Call(3, other, 0, 0)
	deadline := time.Now().Add(time.Second)
	for !privacy.latched && time.Now().Before(deadline) {
		var msg MSG
		for {
			ok, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
			if ok == 0 {
				break
			}
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
		time.Sleep(time.Millisecond)
	}
	command, ok := gpuActive.nextCommand()
	if !privacy.latched || !ok || command.State != projectionBlack || !blackout {
		t.Fatal("native foreground event did not request black")
	}
	checkPrivacyForeground(target)
	if !privacy.latched {
		t.Fatal("return auto-resumed")
	}
	setPrivacyEnabled(false)
	if privacyHook != 0 || privacy.enabled || !blackout {
		t.Fatal("disable leaked output or left hook")
	}
}
