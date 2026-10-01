//go:build windows

package main

// Reused between frames to avoid allocating a full-screen bitmap on every tick.
type paintBuffer struct {
	dc, bitmap, oldBitmap uintptr
	w, h                  int32
}

var outputBuffer paintBuffer

func (b *paintBuffer) ensure(reference uintptr, w, h int32) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if b.dc != 0 && b.w == w && b.h == h {
		return true
	}
	next := paintBuffer{w: w, h: h}
	next.dc, _, _ = procCreateCompatibleDC.Call(reference)
	if next.dc == 0 {
		return false
	}
	next.bitmap, _, _ = procCreateCompatibleBitmap.Call(reference, uintptr(w), uintptr(h))
	if next.bitmap == 0 {
		next.release()
		return false
	}
	next.oldBitmap, _, _ = procSelectObject.Call(next.dc, next.bitmap)
	if next.oldBitmap == 0 || next.oldBitmap == ^uintptr(0) {
		next.oldBitmap = 0
		next.release()
		return false
	}
	b.release()
	*b = next
	return true
}

func (b *paintBuffer) release() {
	if b.dc != 0 && b.oldBitmap != 0 {
		procSelectObject.Call(b.dc, b.oldBitmap)
	}
	if b.bitmap != 0 {
		procDeleteObject.Call(b.bitmap)
	}
	if b.dc != 0 {
		procDeleteDC.Call(b.dc)
	}
	*b = paintBuffer{}
}
