package main

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestGDI2KOutputIdentity(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const w, h = 2560, 1440
	makeDIB := func() (uintptr, []byte, func()) {
		dc, _, _ := procCreateCompatibleDC.Call(0)
		header := struct {
			Size          uint32
			Width, Height int32
			Planes, Bits  uint16
			Rest          [6]uint32
		}{Size: 40, Width: w, Height: -h, Planes: 1, Bits: 32}
		var bits unsafe.Pointer
		bmp, _, _ := gdi32.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
		if bmp == 0 {
			t.Fatal("DIB allocation")
		}
		old, _, _ := procSelectObject.Call(dc, bmp)
		return dc, unsafe.Slice((*byte)(bits), w*h*4), func() { procSelectObject.Call(dc, old); procDeleteObject.Call(bmp); procDeleteDC.Call(dc) }
	}
	source, pixels, freeSource := makeDIB()
	defer freeSource()
	target, got, freeTarget := makeDIB()
	defer freeTarget()
	for i := 0; i < len(pixels); i += 4 {
		pixels[i] = byte(i / 4)
		pixels[i+1] = byte(i / (4 * w))
		pixels[i+2] = byte((i / 4) % 2 * 255)
		pixels[i+3] = 255
	}
	hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr("STATIC"))), 0, WS_POPUP, 0, 0, w, h, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal("output window")
	}
	defer procDestroyWindow.Call(hwnd)
	oldPrefs := preferences
	defer func() {
		preferences = oldPrefs
		hwndOutput = 0
		frameDC = 0
		frameW = 0
		frameH = 0
		outputRunning = false
		gdiSourceValid = false
		gdiPresented = false
		outputBuffer.release()
		gdiLastBuffer.release()
	}()
	hwndOutput = hwnd
	frameDC = source
	frameW = w
	frameH = h
	outputRunning = true
	gdiSourceValid = true
	blackout = false
	frozen = false
	for _, smooth := range []bool{false, true} {
		preferences.Smooth = smooth
		preferences.Fill = false
		paintOutput(target)
		gdi32.NewProc("GdiFlush").Call()
		for i := 0; i < len(got); i++ {
			if i%4 != 3 && got[i] != pixels[i] {
				t.Fatalf("GDI pixel mismatch at %d smooth=%t", i, smooth)
			}
		}
	}
}
