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
