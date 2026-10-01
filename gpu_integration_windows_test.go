package main

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestGPUWindowLifecycle(t *testing.T)         { testGPUWindowLifecycle(t, false) }
func TestGPUSelectedWindowLifecycle(t *testing.T) { testGPUWindowLifecycle(t, true) }
func testGPUWindowLifecycle(t *testing.T, windowSource bool) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, err := enumerateGPUOutputs()
	if err != nil || len(outs) == 0 {
		t.Fatal(err)
	}
	instance, _, _ := procGetModuleHandleW.Call(0)
	callback := syscall.NewCallback(func(h uintptr, m uint32, w, l uintptr) uintptr {
		if m == WM_ERASEBKGND {
			return 1
		}
		r, _, _ := procDefWindowProcW.Call(h, uintptr(m), w, l)
		return r
	})
	name := utf16Ptr("PFHardwareLifecycleTest")
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), LpfnWndProc: callback, HInstance: instance, LpszClassName: name}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := procCreateWindowExW.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, uintptr(unsafe.Pointer(name)), 0, WS_POPUP, 20, 20, 320, 180, 0, 0, instance, 0)
	if hwnd == 0 {
		t.Fatal("window")
	}
	defer procDestroyWindow.Call(hwnd)
	procShowWindow.Call(hwnd, SW_SHOWNOACTIVATE)
	procSetWindowPos.Call(hwnd, HWND_TOPMOST, 20, 20, 320, 180, SWP_NOACTIVATE|SWP_SHOWWINDOW)
	cfg := projectionConfig{Generation: 42, HWND: hwnd, Source: Monitor{Device: outs[0].Device, Rect: outs[0].Rect}, Target: Monitor{Device: outs[0].Device, Rect: RECT{20, 20, 340, 200}}, TargetFPS: 60, Cursor: true, StartBlack: true}
	if windowSource {
		source, _, _ := procCreateWindowExW.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, uintptr(unsafe.Pointer(utf16Ptr("STATIC"))), 0, WS_POPUP|4, 400, 20, 240, 160, 0, 0, instance, 0)
		if source == 0 {
			t.Fatal("source window")
		}
		defer procDestroyWindow.Call(source)
		procShowWindow.Call(source, SW_SHOWNOACTIVATE)
		procUpdateWindow.Call(source)
		cfg.SourceWindow = source
		cfg.SourcePID = windowPID(source)
	}
	s := startGPUSession(cfg)
	peek := user32.NewProc("PeekMessageW")
	pump := func() {
		var m MSG
		for {
			ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
			if ok == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}
	defer func() {
		s.request(projectionCommand{Generation: 42, Sequence: 99, State: projectionStopping})
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			pump()
			select {
			case <-s.done:
				return
			default:
			}
			time.Sleep(time.Millisecond)
		}
		t.Error("GPU cleanup timeout")
	}()
	wait := func(state projectionState, sequence uint64) projectionEvent {
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			pump()
			if e, ok := s.drain(); ok {
				t.Logf("observed state=%d seq=%d reason=%s", e.State, e.Sequence, e.Reason)
				if e.State == projectionFailed {
					t.Fatalf("GPU failed: %s", e.Reason)
				}
				if e.State == state && e.Sequence >= sequence {
					return e
				}
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("timeout waiting %v", state)
		return projectionEvent{}
	}
	first := wait(projectionBlack, 0)
	if first.Captured != 0 || first.Presented != 0 {
		t.Fatal("protected start captured desktop before explicit resume")
	}
	s.request(projectionCommand{Generation: 42, Sequence: 0, State: projectionLive, Config: cfg})
	wait(projectionLive, 0)
	// Allow at least one full presentation before freeze.
	until := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(until) {
		pump()
		time.Sleep(time.Millisecond)
	}
	s.request(projectionCommand{Generation: 42, Sequence: 1, State: projectionFreezing, Config: cfg})
	e := wait(projectionFrozen, 1)
	t.Logf("freeze acknowledged; captured=%d presented=%d", e.Captured, e.Presented)
	if e.Presented == 0 {
		t.Fatal("nothing presented")
	}
	s.request(projectionCommand{Generation: 42, Sequence: 2, State: projectionLive, Config: cfg})
	wait(projectionLive, 2)
	s.request(projectionCommand{Generation: 42, Sequence: 3, State: projectionBlack, Config: cfg})
	wait(projectionBlack, 3)
	s.request(projectionCommand{Generation: 42, Sequence: 4, State: projectionFreezing, Config: cfg})
	wait(projectionBlack, 4)
	s.request(projectionCommand{Generation: 42, Sequence: 5, State: projectionLive, Config: cfg})
	wait(projectionLive, 5)
	s.request(projectionCommand{Generation: 42, Sequence: 6, State: projectionStopping})
	wait(projectionStopped, 6)
}
