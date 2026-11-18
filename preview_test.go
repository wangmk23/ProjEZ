package main

import (
	"testing"
	"time"
)

func TestPreviewRejectsStaleAndUnconfirmedOutput(t *testing.T) {
	f := previewFrame{Generation: 2, Sequence: 4, State: projectionLive, Pixels: make([]byte, previewW*previewH*4), At: time.Now()}
	if !f.matches(2, 4, projectionLive) {
		t.Fatal("current frame rejected")
	}
	if f.matches(1, 4, projectionLive) || f.matches(2, 5, projectionLive) || f.matches(2, 4, projectionBlack) {
		t.Fatal("unsafe preview accepted")
	}
	f.Pixels = nil
	if f.matches(2, 4, projectionLive) {
		t.Fatal("empty image accepted")
	}
}

func TestPreviewRetainsFrameOnTransientErrorButRejectsOldState(t *testing.T) {
	oldRunning, oldGPU, oldWanted, oldFrame, oldError, oldEvent := outputRunning, gpuOwnsOutput, previewWanted, audienceFrame, audienceError, gpuLastEvent
	oldController := gpuController
	defer func() {
		outputRunning, gpuOwnsOutput, previewWanted, audienceFrame, audienceError, gpuLastEvent = oldRunning, oldGPU, oldWanted, oldFrame, oldError, oldEvent
		gpuController = oldController
	}()
	outputRunning, gpuOwnsOutput, previewWanted = true, true, true
	gpuController.generation, gpuController.sequence = 7, 3
	gpuLastEvent = projectionEvent{State: projectionFrozen}
	audienceFrame = previewFrame{Generation: 7, Sequence: 3, State: projectionFrozen, Pixels: make([]byte, previewW*previewH*4), At: time.Now()}
	audienceError = "预览读回超时"
	if !previewImageAvailable() {
		t.Fatal("transient preview error blanks a valid frozen frame")
	}
	gpuController.sequence = 4
	gpuLastEvent.State = projectionBlack
	if previewImageAvailable() {
		t.Fatal("old frozen frame exposed during black state")
	}
	gpuController.sequence = 3
	gpuLastEvent.State = projectionFrozen
	previewWanted = false
	if previewImageAvailable() {
		t.Fatal("disabled preview exposes cached frame")
	}
}

func TestGDIPreviewBufferReuseAndTimerSleep(t *testing.T) {
	oldRunning, oldGPU, oldPending, oldMain, oldActive := outputRunning, gpuActive, pendingTrayRestore, hwndMain, previewTimerActive
	defer func() {
		outputRunning, gpuActive, pendingTrayRestore, hwndMain, previewTimerActive = oldRunning, oldGPU, oldPending, oldMain, oldActive
		releaseGDIPreview()
		gdiLastBuffer.release()
	}()
	hwndMain = 0
	outputRunning = false
	gpuActive = nil
	pendingTrayRestore = false
	syncPreviewTimer()
	if previewTimerActive {
		t.Fatal("preview timer active without window or session")
	}

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		t.Skip("no screen DC")
	}
	defer procReleaseDC.Call(0, screenDC)
	if !gdiLastBuffer.ensure(screenDC, 320, 180) {
		t.Fatal("failed to allocate gdiLastBuffer")
	}
	b1, err := readGDIPreview()
	if err != nil || len(b1) != previewW*previewH*4 {
		t.Fatalf("first readGDIPreview failed: %v len=%d", err, len(b1))
	}
	b2, err := readGDIPreview()
	if err != nil || len(b2) != previewW*previewH*4 {
		t.Fatalf("second readGDIPreview failed: %v len=%d", err, len(b2))
	}
	b3, err := readGDIPreview()
	if err != nil || &b3[0] != &b1[0] || &b2[0] == &b1[0] {
		t.Fatal("GDI preview double-buffer slices were not reused as expected")
	}
}
