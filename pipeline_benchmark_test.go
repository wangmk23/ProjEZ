//go:build windows

package main

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestCaptureSurfaceTiming(t *testing.T) {
	if os.Getenv("PF_BENCH") != "1" {
		t.Skip("opt-in desktop benchmark")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	for _, dib := range []bool{false, true} {
		dc, _, _ := procCreateCompatibleDC.Call(screen)
		var bmp uintptr
		if dib {
			info := struct {
				Size                        uint32
				Width, Height               int32
				Planes, BitCount            uint16
				Compression, SizeImage      uint32
				XPels, YPels                int32
				ColorsUsed, ColorsImportant uint32
			}{Size: 40, Width: 2560, Height: -1440, Planes: 1, BitCount: 32}
			var bits uintptr
			bmp, _, _ = gdi32.NewProc("CreateDIBSection").Call(screen, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
		} else {
			bmp, _, _ = procCreateCompatibleBitmap.Call(screen, 2560, 1440)
		}
		if bmp == 0 {
			t.Fatal("capture surface allocation")
		}
		old, _, _ := procSelectObject.Call(dc, bmp)
		var capture, flush time.Duration
		for i := 0; i < 60; i++ {
			start := time.Now()
			procBitBlt.Call(dc, 0, 0, 2560, 1440, screen, 0, 0, SRCCOPY)
			capture += time.Since(start)
			start = time.Now()
			gdi32.NewProc("GdiFlush").Call()
			flush += time.Since(start)
		}
		t.Logf("DIB=%t persistent source DC: copy %.2f ms, flush %.2f ms", dib, float64(capture.Microseconds())/60000, float64(flush.Microseconds())/60000)
		procSelectObject.Call(dc, old)
		procDeleteObject.Call(bmp)
		procDeleteDC.Call(dc)
	}
}

// Opt-in diagnostic: captures the local desktop into memory; no images are saved.
func TestPipelineTiming(t *testing.T) {
	if os.Getenv("PF_BENCH") != "1" {
		t.Skip("set PF_BENCH=1 for local desktop timing")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := procGetModuleHandleW.Call(0)
	name := utf16Ptr("ProjectorFreezerTiming")
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), HInstance: instance, LpszClassName: name, LpfnWndProc: syscall.NewCallback(outputWndProc)}
	if ok, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); ok == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), instance)
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	flush := gdi32.NewProc("GdiFlush")
	for _, tc := range []struct {
		name           string
		sw, sh, dw, dh int32
	}{{"native", 2560, 1440, 2560, 1440}, {"downscale", 2560, 1440, 1920, 1080}, {"upscale", 1920, 1080, 2560, 1440}} {
		t.Run(tc.name, func(t *testing.T) {
			hwndOutput, _, _ = procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(name)), 0, WS_POPUP, 0, 0, uintptr(tc.dw), uintptr(tc.dh), 0, 0, instance, 0)
			if hwndOutput == 0 {
				t.Fatal("create hidden timing window")
			}
			defer stopOutput()
			outputRunning = true
			srcMonitor = Monitor{Rect: RECT{0, 0, tc.sw, tc.sh}}
			defer func() { srcMonitor = Monitor{} }()
			target := paintBuffer{}
			if !target.ensure(screen, tc.dw, tc.dh) {
				t.Fatal("timing target allocation")
			}
			defer target.release()
			captureFrame()
			paintOutput(target.dc)
			flush.Call()
			var capture, paint time.Duration
			for i := 0; i < 60; i++ {
				start := time.Now()
				captureFrame()
				flush.Call()
				capture += time.Since(start)
				start = time.Now()
				paintOutput(target.dc)
				flush.Call()
				paint += time.Since(start)
			}
			t.Logf("%dx%d -> %dx%d: capture %.2f ms, compose+copy %.2f ms, theoretical ceiling %.1f FPS", tc.sw, tc.sh, tc.dw, tc.dh, float64(capture.Microseconds())/60000, float64(paint.Microseconds())/60000, 60/(capture+paint).Seconds())
		})
	}
}
