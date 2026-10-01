//go:build windows

package main

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

func TestOutputPaintDoesNotExposeBlankFrames(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := procGetModuleHandleW.Call(0)
	name := utf16Ptr("ProjectorFreezerPaintTest")
	black, _, _ := procGetStockObject.Call(4) // BLACK_BRUSH
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), HInstance: instance, LpszClassName: name, LpfnWndProc: syscall.NewCallback(outputWndProc), HbrBackground: black}
	if ok, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); ok == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), instance)
	hwndOutput, _, _ = procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(name)), 0, WS_POPUP, 0, 0, 320, 240, 0, 0, instance, 0)
	if hwndOutput == 0 {
		t.Fatal("create output test window")
	}
	defer stopOutput()
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	target, _, _ := procCreateCompatibleDC.Call(screen)
	bmp, _, _ := procCreateCompatibleBitmap.Call(screen, 320, 240)
	old, _, _ := procSelectObject.Call(target, bmp)
	defer func() { procSelectObject.Call(target, old); procDeleteObject.Call(bmp); procDeleteDC.Call(target) }()
	magenta := uintptr(rgb(255, 0, 255))
	brush := makeBrush(255, 0, 255)
	defer procDeleteObject.Call(brush)
	fill := func() { r := RECT{0, 0, 320, 240}; procFillRect.Call(target, uintptr(unsafe.Pointer(&r)), brush) }
	pixel := func(x, y uintptr) uintptr { p, _, _ := gdi32.NewProc("GetPixel").Call(target, x, y); return p }
	fill()
	procSendMessageW.Call(hwndOutput, WM_ERASEBKGND, target, 0)
	if pixel(160, 120) != magenta {
		t.Error("background erase exposed a black frame before painting")
	}
	fill()
	gdiSourceValid = true
	frameDC, frameW, frameH = 0xDEAD, 160, 90 // Failed source rendering must retain the displayed frame.
	paintOutput(target)
	frameDC = 0
	if pixel(160, 120) != magenta {
		t.Error("failed rendering replaced the previous frame with black")
	}
	frameDC, _, _ = procCreateCompatibleDC.Call(screen)
	frameBitmap, _, _ = procCreateCompatibleBitmap.Call(screen, 160, 90)
	procSelectObject.Call(frameDC, frameBitmap)
	green := makeBrush(0, 255, 0)
	defer procDeleteObject.Call(green)
	r := RECT{0, 0, 160, 90}
	procFillRect.Call(frameDC, uintptr(unsafe.Pointer(&r)), green)
	preferences.Fill = false
	defer func() { preferences = Preferences{FPS: 15} }()
	paintOutput(target)
	if pixel(160, 120) != uintptr(rgb(0, 255, 0)) || pixel(160, 5) != 0 {
		t.Error("aspect-fit frame or letterbox pixels are wrong")
	}
	preferences.Fill = true
	paintOutput(target)
	if pixel(160, 5) != uintptr(rgb(0, 255, 0)) {
		t.Error("fill mode did not replace letterbox bars")
	}
	procSetWindowPos.Call(hwndOutput, 0, 0, 0, 160, 120, SWP_NOZORDER|SWP_NOACTIVATE)
	preferences.Fill = false
	paintOutput(target)
	if pixel(80, 5) != 0 || pixel(80, 60) != uintptr(rgb(0, 255, 0)) {
		t.Error("resized output did not recompose the image and borders")
	}

	// Freeze must reuse submitted output, not an unpresented source capture.
	frozen = true
	procFillRect.Call(frameDC, uintptr(unsafe.Pointer(&r)), brush)
	paintOutput(target)
	if pixel(80, 60) != uintptr(rgb(0, 255, 0)) {
		t.Fatal("freeze leaked unpresented capture")
	}
	defer releaseGDIPreview()
	thumb, err := readGDIPreview()
	center := (90*previewW + 160) * 4
	if err != nil || len(thumb) == 0 || thumb[center] != 0 || thumb[center+1] != 255 || thumb[center+2] != 0 {
		t.Fatal("GDI preview leaked unsubmitted capture", err)
	}
	blackout = true
	// Simulate terminal GPU failure while black is requested: fallback cannot start capture.
	id := gpuController.begin()
	gpuController.request(projectionBlack)
	gpuActive = newGPUMailbox(projectionConfig{Generation: id})
	gpuOwnsOutput = true
	close(gpuActive.done)
	gpuActive.publish(projectionEvent{Generation: id, State: projectionFailed, Fallback: true, Reason: "injected"})
	beforeCapture := capturedGDI
	pollGPUEvents()
	if gpuOwnsOutput || gpuActive != nil || livePump != nil || capturedGDI != beforeCapture || !blackout {
		t.Fatal("black failure resumed capture")
	}

	paintOutput(target)
	if pixel(80, 60) != 0 || pixel(80, 5) != 0 {
		t.Fatal("GDI blackout failed")
	}
	thumb, err = readGDIPreview()
	if err != nil || len(thumb) == 0 || thumb[center] != 0 || thumb[center+1] != 0 || thumb[center+2] != 0 {
		t.Fatal("GDI preview not black", err)
	}
	// Failed window capture must never fall back to desktop capture on resume.
	activeSourceWindow = sourceWindow{HWND: hwndOutput, PID: windowPID(hwndOutput), Title: "test"}
	id = gpuController.begin()
	gpuActive = newGPUMailbox(projectionConfig{Generation: id})
	gpuOwnsOutput = true
	close(gpuActive.done)
	gpuActive.publish(projectionEvent{Generation: id, State: projectionFailed, Reason: "window closed"})
	beforeCapture = capturedGDI
	pollGPUEvents()
	resumeOutput()
	if gpuActive != nil || gpuOwnsOutput || !blackout || capturedGDI != beforeCapture {
		t.Fatal("window failure exposed desktop or retained dead worker")
	}
	activeSourceWindow = sourceWindow{}
	blackout, blackAcknowledged, frozen = false, false, false
	procFillRect.Call(frameDC, uintptr(unsafe.Pointer(&r)), green)
	// Equal-size output must preserve individual source pixels exactly.
	procSetWindowPos.Call(hwndOutput, 0, 0, 0, 160, 90, SWP_NOZORDER|SWP_NOACTIVATE)
	colors := []uint32{rgb(1, 2, 3), rgb(255, 255, 255), rgb(0, 0, 0), rgb(200, 40, 120)}
	for i, c := range colors {
		gdi32.NewProc("SetPixel").Call(frameDC, uintptr(20+i), 20, uintptr(c))
	}
	paintOutput(target)
	for i, c := range colors {
		if pixel(uintptr(20+i), 20) != uintptr(c) {
			t.Error("native output blurred or changed source pixels")
		}
	}
	// Crisp enlargement keeps a source pixel a solid block rather than blending it.
	preferences.Smooth = false
	procSetWindowPos.Call(hwndOutput, 0, 0, 0, 320, 180, SWP_NOZORDER|SWP_NOACTIVATE)
	paintOutput(target)
	for i, c := range colors {
		for dx := 0; dx < 2; dx++ {
			if pixel(uintptr(40+2*i+dx), 40) != uintptr(c) {
				t.Error("text mode softened enlarged source pixels")
			}
		}
	}
	dc := outputBuffer.dc
	stopOutput()
	gdi32.NewProc("GdiFlush").Call()
	if bitmap, _, _ := gdi32.NewProc("GetCurrentObject").Call(dc, 7); bitmap != 0 {
		t.Error("stopping output left the presentation DC usable")
	}
}
