package main

import (
	"fmt"
	"time"
)

type outputClock struct {
	layer                          sceneLayer
	bounds                         RECT
	width, height, referenceHeight int32
}

func clockText(now time.Time) string { return now.Format("15:04:05") }
func newOutputClock(l sceneLayer, width, height int32) *outputClock {
	x, y := width*int32(l.X)/100, height*int32(l.Y)/100
	right, bottom := width*int32(l.X+l.W)/100, height*int32(l.Y+l.H)/100
	reference := min(height, 1080)
	return &outputClock{l, RECT{x, y, right, bottom}, max(1, (right-x)*reference/height), max(1, (bottom-y)*reference/height), reference}
}
func (c *outputClock) draw(pixels []byte, now time.Time) error {
	clear(pixels)
	l := c.layer
	l.Text = clockText(now)
	canvas := sceneRaster{c.width, c.height, pixels}
	return paintSceneTextSized(&canvas, l, RECT{0, 0, c.width, c.height}, c.referenceHeight)
}

var activeSceneClock *outputClock
var clockBaseBuffer, clockPendingBuffer paintBuffer
var clockDC, clockBitmap, clockOld uintptr
var clockBits []byte
var gdiClockText string
var gdiClockPresented string

func releaseGDIClock() {
	clockBaseBuffer.release()
	clockPendingBuffer.release()
	releaseSceneDIB(clockDC, clockBitmap, clockOld)
	clockDC, clockBitmap, clockOld, clockBits = 0, 0, 0, nil
	gdiClockText, gdiClockPresented = "", ""
}
func blendGDIClock(dc uintptr, now time.Time) error {
	clock := activeSceneClock
	if clock == nil {
		return nil
	}
	if clockDC == 0 {
		var err error
		clockDC, clockBitmap, clockOld, clockBits, err = createSceneDIB(clock.width, clock.height)
		if err != nil {
			return err
		}
	}
	label := clockText(now)
	if label != gdiClockText {
		if err := clock.draw(clockBits, now); err != nil {
			return err
		}
		gdiClockText = label
	}
	b := clock.bounds
	ok, _, _ := alphaBlend.Call(dc, uintptr(b.Left), uintptr(b.Top), uintptr(b.Right-b.Left), uintptr(b.Bottom-b.Top), clockDC, 0, 0, uintptr(clock.width), uintptr(clock.height), 0x01ff0000)
	if ok == 0 {
		return fmt.Errorf("无法叠加时钟")
	}
	return nil
}
func tickFrozenGDIClock() {
	if !gpuOwnsOutput && outputRunning && frozen && !blackout && activeSceneClock != nil && clockText(time.Now()) != gdiClockPresented {
		procInvalidateRect.Call(hwndOutput, 0, 0)
	}
}

func paintFrozenGDIClock(hdc uintptr, w, h int32) bool {
	b := activeSceneClock.bounds
	if clockBaseBuffer.w != b.Right-b.Left || clockBaseBuffer.h != b.Bottom-b.Top || gdiLastBuffer.w != w || gdiLastBuffer.h != h || !outputBuffer.ensure(hdc, w, h) {
		return false
	}
	ok, _, _ := procBitBlt.Call(outputBuffer.dc, 0, 0, uintptr(w), uintptr(h), gdiLastBuffer.dc, 0, 0, SRCCOPY)
	if ok == 0 {
		return false
	}
	ok, _, _ = procBitBlt.Call(outputBuffer.dc, uintptr(b.Left), uintptr(b.Top), uintptr(b.Right-b.Left), uintptr(b.Bottom-b.Top), clockBaseBuffer.dc, 0, 0, SRCCOPY)
	if ok == 0 || blendGDIClock(outputBuffer.dc, time.Now()) != nil {
		return false
	}
	ok, _, _ = procBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), outputBuffer.dc, 0, 0, SRCCOPY)
	if ok == 0 {
		return false
	}
	gdiClockPresented = gdiClockText
	outputBuffer, gdiLastBuffer = gdiLastBuffer, outputBuffer
	return true
}
