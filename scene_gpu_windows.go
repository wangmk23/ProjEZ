package main

import (
	"runtime"
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
	hr := comCall(device, 15, uintptr(unsafe.Pointer(&overlayShader[0])), uintptr(len(overlayShader)), 0, uintptr(unsafe.Pointer(&r.overlayPS)))
	if err = hrError("Create overlay shader", hr); err != nil {
		return err
	}
	desc := overlayBlendDesc{}
	// ONE + INV_SRC_ALPHA blends premultiplied pixels.
	desc.Targets[0] = overlayBlendTarget{Enable: 1, Source: 2, Destination: 6, Operation: 1, SourceAlpha: 2, DestinationAlpha: 6, OperationAlpha: 1, WriteMask: 15}
	return hrError("Create overlay blend state", comCall(device, 20, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&r.overlayBlend))))
}
func (r *gpuRenderer) drawGPUOverlay() {
	if r.overlayView == nil {
		return
	}
	ctx := r.capture.context
	vp := viewport{W: float32(r.width), H: float32(r.height), Max: 1}
	comCall(ctx, 44, 1, uintptr(unsafe.Pointer(&vp)))
	comCall(ctx, 35, uintptr(unsafe.Pointer(r.overlayBlend)), 0, 0xffffffff)
	comCall(ctx, 8, 0, 1, uintptr(unsafe.Pointer(&r.overlayView)))
	comCall(ctx, 10, 0, 1, uintptr(unsafe.Pointer(&r.linear)))
	comCall(ctx, 9, uintptr(unsafe.Pointer(r.overlayPS)), 0, 0)
	comCall(ctx, 13, 3, 0)
	comCall(ctx, 35, 0, 0, 0xffffffff)
}
