package main

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestWindowCaptureIsolatesSelectedWindow(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := procGetModuleHandleW.Call(0)
	brush := makeBrush(0, 255, 0)
	defer procDeleteObject.Call(brush)
	cb := syscall.NewCallback(func(h uintptr, m uint32, w, l uintptr) uintptr {
		if m == WM_PAINT {
			var ps PAINTSTRUCT
			dc, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			var r RECT
			procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
			procFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), brush)
			procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			return 0
		}
		v, _, _ := procDefWindowProcW.Call(h, uintptr(m), w, l)
		return v
	})
	name := utf16Ptr("PFSelectedWindowPixels")
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), LpfnWndProc: cb, HInstance: instance, LpszClassName: name}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	h, _, _ := procCreateWindowExW.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, uintptr(unsafe.Pointer(name)), 0, WS_POPUP, 40, 40, 240, 160, 0, 0, instance, 0)
	if h == 0 {
		t.Fatal("window")
	}
	defer procDestroyWindow.Call(h)
	if !preserveTaskbarControls(h) {
		t.Fatal("taskbar protection property failed")
	}
	if v, _, _ := user32.NewProc("GetPropW").Call(h, uintptr(unsafe.Pointer(utf16Ptr("NonRudeHWND")))); v != 1 {
		t.Fatal("taskbar protection missing")
	}
	procShowWindow.Call(h, SW_SHOWNOACTIVATE)
	procUpdateWindow.Call(h)
	// Opaque unrelated window fully covers the selected one on the actual desktop.
	cover, _, _ := procCreateWindowExW.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, uintptr(unsafe.Pointer(utf16Ptr("STATIC"))), 0, WS_POPUP|4, 40, 40, 400, 300, 0, 0, instance, 0)
	if cover == 0 {
		t.Fatal("cover")
	}
	defer procDestroyWindow.Call(cover)
	procSetWindowPos.Call(cover, HWND_TOPMOST, 40, 40, 400, 300, SWP_SHOWWINDOW|SWP_NOACTIVATE)
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	d, e := openWindowCapture(h, windowPID(h), outs[0].Device, false)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	r, e := newGPURenderer(0, d, projectionConfig{Source: Monitor{Rect: RECT{0, 0, 240, 160}}, Target: Monitor{Rect: RECT{0, 0, 240, 160}}})
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	check := func(width, height uint32) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			var msg MSG
			for {
				ok, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
				if ok == 0 {
					break
				}
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			}
			f, ok, e := d.acquire(0)
			if e != nil {
				t.Fatal(e)
			}
			if ok {
				t.Logf("frame %dx%d wanted %dx%d", f.Width, f.Height, width, height)
				if f.Width == width && f.Height == height {
					pixels, e := r.readTexture(f.Texture, f.Width, f.Height)
					d.releaseFrame()
					if e != nil {
						t.Fatal(e)
					}
					i := int((f.Height/2*f.Width + f.Width/2) * 4)
					if pixels[i] > 5 || pixels[i+1] < 250 || pixels[i+2] > 5 {
						t.Fatalf("occluder leaked into window: %v", pixels[i:i+4])
					}
					return
				}
				d.releaseFrame()
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("selected window frame timeout; pool=%+v", d.window.size)
	}
	check(240, 160)
	procSetWindowPos.Call(h, 0, 40, 40, 300, 180, SWP_NOZORDER|SWP_NOACTIVATE)
	procInvalidateRect.Call(h, 0, 0)
	procUpdateWindow.Call(h)
	check(300, 180)
	procShowWindow.Call(h, 6)
	if _, _, e := d.acquire(0); e == nil {
		t.Fatal("minimized source did not fail closed")
	}
	procDestroyWindow.Call(h)
	if _, _, e := d.acquire(0); e == nil {
		t.Fatal("closed source did not fail closed")
	}
}
