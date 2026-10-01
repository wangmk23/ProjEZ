//go:build windows

package main

import (
	"sync/atomic"
	"time"
)

const wmFrameReady = 0x8001

var frameGeneration uintptr
var livePump *framePump
var measuredFPS float64
var measureStart time.Time
var measureFrames int
var procPostFrame = user32.NewProc("PostMessageW")

type framePump struct {
	stop       chan struct{}
	finished   chan struct{}
	pending    atomic.Bool
	generation uintptr
}

func newFramePump(fps int, generation uintptr, post func(uintptr) bool) *framePump {
	if fps < 1 {
		fps = 60
	}
	p := &framePump{stop: make(chan struct{}), finished: make(chan struct{}), generation: generation}
	go func() {
		defer close(p.finished)
		ticker := time.NewTicker(time.Second / time.Duration(fps))
		defer ticker.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
				// Coalesce missed frames: never build a queue that blocks user input.
				if p.pending.CompareAndSwap(false, true) && !post(generation) {
					p.pending.Store(false)
				}
			}
		}
	}()
	return p
}

func stopFramePump() {
	if livePump != nil {
		close(livePump.stop)
		livePump = nil
	}
}

func startFramePump() {
	if gpuOwnsOutput {
		updateGPUSettings()
		return
	}
	stopFramePump()
	frameGeneration++
	hwnd := hwndOutput
	livePump = newFramePump(activeTargetFPS, frameGeneration, func(generation uintptr) bool {
		ok, _, _ := procPostFrame.Call(hwnd, wmFrameReady, generation, 0)
		return ok != 0
	})
	measuredFPS = 0
	measureStart = time.Time{}
	measureFrames = 0
}

func handleFrameTick(generation uintptr) {
	if gpuOwnsOutput {
		return
	}
	p := livePump
	if p == nil || p.generation != generation {
		return
	}
	defer p.pending.Store(false)
	if !outputRunning || frozen || blackout {
		return
	}
	captureFrame()
	procInvalidateRect.Call(hwndOutput, 0, 0)
	procUpdateWindow.Call(hwndOutput)
}

func recordPresentedFrame() {
	if !outputRunning || frozen || blackout {
		return
	}
	now := time.Now()
	if measureStart.IsZero() {
		measureStart = now
		measureFrames = 0
		return
	}
	measureFrames++
	if elapsed := now.Sub(measureStart).Seconds(); elapsed >= 1 {
		measuredFPS = float64(measureFrames) / elapsed
		measureStart = now
		measureFrames = 0
		if hwndMain != 0 {
			procInvalidateRect.Call(hwndMain, 0, 0)
		}
	}
}
