package main

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestControlCaptureExclusionPixels(t *testing.T) {
	if os.Getenv("PF_CAPTURE_EXCLUSION_TEST") != "1" {
		t.Skip("opt-in no-activation desktop pixel test")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	restore := testPhysicalCoordinates(t)
	defer restore()
	outputs, err := enumerateGPUOutputs()
	if err != nil || len(outputs) == 0 {
		t.Fatal("outputs", err)
	}
	o := outputs[0]
	if o.Rotation != 1 {
		t.Skip("requires unrotated test output")
	}
	d, err := openDuplication(o.Device)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	instance, _, _ := procGetModuleHandleW.Call(0)
	create := func(style uintptr) uintptr {
		h, _, _ := procCreateWindowExW.Call(WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE|WS_EX_LAYERED, uintptr(unsafe.Pointer(utf16Ptr("STATIC"))), 0, WS_POPUP|style, uintptr(int64(o.Rect.Left+40)), uintptr(int64(o.Rect.Top+40)), 64, 64, 0, 0, instance, 0)
		if h == 0 {
			t.Fatal("create pixel fixture")
		}
		user32.NewProc("SetLayeredWindowAttributes").Call(h, 0, 255, 2)
		return h
	}
	under, control := create(4), create(6) // SS_BLACKRECT, SS_WHITERECT.
	defer procDestroyWindow.Call(under)
	defer procDestroyWindow.Call(control)
	procShowWindow.Call(under, SW_SHOWNOACTIVATE)
	procUpdateWindow.Call(under)

	sampleGDI := func() uint32 {
		screen, _, _ := procGetDC.Call(0)
		defer procReleaseDC.Call(0, screen)
		dc, _, _ := procCreateCompatibleDC.Call(screen)
		defer procDeleteDC.Call(dc)
		bitmap, _, _ := procCreateCompatibleBitmap.Call(screen, 1, 1)
		defer procDeleteObject.Call(bitmap)
		old, _, _ := procSelectObject.Call(dc, bitmap)
		defer procSelectObject.Call(dc, old)
		procBitBlt.Call(dc, 0, 0, 1, 1, screen, uintptr(int64(o.Rect.Left+56)), uintptr(int64(o.Rect.Top+56)), SRCCOPY)
		v, _, _ := gdi32.NewProc("GetPixel").Call(dc, 0, 0)
		return uint32(v)
	}
	waitColor := func(want uint32) {
		for i := 0; i < 30; i++ {
			syscallDwmFlush()
			if sampleGDI() == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("GDI expected color %x, got %x", want, sampleGDI())
	}
	syscallDwmFlush()
	baseline := sampleGDI()
	procShowWindow.Call(control, SW_SHOWNOACTIVATE)
	procUpdateWindow.Call(control)
	waitColor(0xffffff)
	if !excludeControlWindow(control) {
		t.Fatal("capture exclusion unavailable")
	}
	waitColor(baseline)
	desc := textureDesc{Width: 1, Height: 1, Mips: 1, ArraySize: 1, Format: 87, SampleCount: 1, Usage: 3, CPUAccess: 0x20000}
	var pixel comPtr
	if err := hrError("pixel staging", comCall(d.device, 5, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&pixel)))); err != nil {
		t.Fatal(err)
	}
	defer releaseCOM(&pixel)
	for i := 0; i < 30; i++ {
		f, ok, err := d.acquire(50)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		box := [6]uint32{56, 56, 0, 57, 57, 1}
		comCall(d.context, 46, uintptr(unsafe.Pointer(pixel)), 0, 0, 0, 0, uintptr(unsafe.Pointer(f.Texture)), 0, uintptr(unsafe.Pointer(&box)))
		var mapped mappedTexture
		err = hrError("pixel map", comCall(d.context, 14, uintptr(unsafe.Pointer(pixel)), 0, 1, 0, uintptr(unsafe.Pointer(&mapped))))
		if err != nil {
			d.releaseFrame()
			t.Fatal(err)
		}
		rgb := unsafe.Slice((*byte)(mapped.Data), 4)
		dark := rgb[0] == byte(baseline>>16) && rgb[1] == byte(baseline>>8) && rgb[2] == byte(baseline)
		comCall(d.context, 15, uintptr(unsafe.Pointer(pixel)), 0)
		d.releaseFrame()
		if dark {
			return
		}
	}
	t.Fatal("DXGI still includes control fixture after exclusion")
}
func syscallDwmFlush() { syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmFlush").Call() }
