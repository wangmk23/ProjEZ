package main

import (
	"runtime"
	"time"
	"unsafe"
)

type overlayBlendTarget struct {
	Enable, Source, Destination, Operation, SourceAlpha, DestinationAlpha, OperationAlpha uint32
	WriteMask                                                                             uint8
	_                                                                                     [3]byte
}
type overlayBlendDesc struct {
	AlphaToCoverage, Independent uint32
	Targets                      [8]overlayBlendTarget
}

func (r *gpuRenderer) initGPUOverlay(scene *sceneRaster) error {
	var err error
	device, ctx := r.capture.device, r.capture.context
	if scene != nil {
		r.overlayTexture, err = makeTexture(device, uint32(scene.W), uint32(scene.H), 87, 8)
		if err != nil {
			return err
		}
		r.overlayView, err = makeView(device, r.overlayTexture, 7)
		if err != nil {
			return err
		}
		comCall(ctx, 48, uintptr(unsafe.Pointer(r.overlayTexture)), 0, 0, uintptr(unsafe.Pointer(&scene.Pixels[0])), uintptr(scene.W*4), 0)
		runtime.KeepAlive(scene)
	}
	if c := r.cfg.Clock; c != nil {
		r.clockTexture, err = makeTexture(device, uint32(c.width), uint32(c.height), 87, 8)
		if err != nil {
			return err
		}
		r.clockView, err = makeView(device, r.clockTexture, 7)
		if err != nil {
			return err
		}
		w, h := uint32(c.bounds.Right-c.bounds.Left), uint32(c.bounds.Bottom-c.bounds.Top)
		r.clockBase, err = makeTexture(device, w, h, 87, 0)
		if err != nil {
			return err
		}
		r.clockLastBase, err = makeTexture(device, w, h, 87, 0)
		if err != nil {
			return err
		}
		r.clockPixels = make([]byte, int(c.width*c.height*4))
	}
	hr := comCall(device, 15, uintptr(unsafe.Pointer(&overlayShader[0])), uintptr(len(overlayShader)), 0, uintptr(unsafe.Pointer(&r.overlayPS)))
	if err = hrError("Create overlay shader", hr); err != nil {
		return err
	}
	desc := overlayBlendDesc{}
	// ONE + INV_SRC_ALPHA blends premultiplied pixels.
	desc.Targets[0] = overlayBlendTarget{Enable: 1, Source: 2, Destination: 6, Operation: 1, SourceAlpha: 2, DestinationAlpha: 6, OperationAlpha: 1, WriteMask: 15}
	return hrError("Create overlay blend state", comCall(device, 20, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&r.overlayBlend))))
}
func (r *gpuRenderer) drawOverlayTexture(view comPtr, vp viewport) {
	if view == nil {
		return
	}
	ctx := r.capture.context
	comCall(ctx, 44, 1, uintptr(unsafe.Pointer(&vp)))
	comCall(ctx, 35, uintptr(unsafe.Pointer(r.overlayBlend)), 0, 0xffffffff)
	comCall(ctx, 8, 0, 1, uintptr(unsafe.Pointer(&view)))
	comCall(ctx, 10, 0, 1, uintptr(unsafe.Pointer(&r.linear)))
	comCall(ctx, 17, 0)
	comCall(ctx, 24, 4)
	comCall(ctx, 11, uintptr(unsafe.Pointer(r.vs)), 0, 0)
	comCall(ctx, 9, uintptr(unsafe.Pointer(r.overlayPS)), 0, 0)
	comCall(ctx, 13, 3, 0)
	comCall(ctx, 35, 0, 0, 0xffffffff)
}
func (r *gpuRenderer) refreshGPUClock(now time.Time) error {
	c := r.cfg.Clock
	if c == nil || clockText(now) == r.clockLabel {
		return nil
	}
	if err := c.draw(r.clockPixels, now); err != nil {
		return err
	}
	comCall(r.capture.context, 48, uintptr(unsafe.Pointer(r.clockTexture)), 0, 0, uintptr(unsafe.Pointer(&r.clockPixels[0])), uintptr(c.width*4), 0)
	runtime.KeepAlive(r.clockPixels)
	r.clockLabel = clockText(now)
	return nil
}
func (r *gpuRenderer) drawGPUClock() {
	c := r.cfg.Clock
	if c == nil {
		return
	}
	b := c.bounds
	r.drawOverlayTexture(r.clockView, viewport{X: float32(b.Left), Y: float32(b.Top), W: float32(b.Right - b.Left), H: float32(b.Bottom - b.Top), Max: 1})
}

// Only the rectangle beneath the clock needs extra copies during live output.
func (r *gpuRenderer) saveClockPatch(dst, src comPtr) {
	b := r.cfg.Clock.bounds
	box := [6]uint32{uint32(b.Left), uint32(b.Top), 0, uint32(b.Right), uint32(b.Bottom), 1}
	comCall(r.capture.context, 46, uintptr(unsafe.Pointer(dst)), 0, 0, 0, 0, uintptr(unsafe.Pointer(src)), 0, uintptr(unsafe.Pointer(&box)))
}
func (r *gpuRenderer) restoreFrozenBase() {
	ctx := r.capture.context
	comCall(ctx, 47, uintptr(unsafe.Pointer(r.composite)), uintptr(unsafe.Pointer(r.last)))
	b := r.cfg.Clock.bounds
	comCall(ctx, 46, uintptr(unsafe.Pointer(r.composite)), 0, uintptr(b.Left), uintptr(b.Top), 0, uintptr(unsafe.Pointer(r.clockLastBase)), 0, 0)
}
func (r *gpuRenderer) renderFrozenClock(now time.Time) (bool, error) {
	if err := r.refreshGPUClock(now); err != nil {
		return false, err
	}
	ctx := r.capture.context
	r.restoreFrozenBase()
	comCall(ctx, 33, 1, uintptr(unsafe.Pointer(&r.compositeView)), 0)
	r.drawGPUClock()
	var empty comPtr
	comCall(ctx, 8, 0, 1, uintptr(unsafe.Pointer(&empty)))
	comCall(ctx, 33, 0, 0, 0)
	ok, err := r.presentComposite()
	if ok {
		r.presentedClockLabel = r.clockLabel
	}
	return ok, err
}
