package main

import (
	"fmt"
	"time"
	"unsafe"
)

type gpuPreview struct {
	renderer *gpuRenderer
	staging  comPtr
	pending  bool
	frame    previewFrame
	next     time.Time
	interval time.Duration
}

func (p *gpuPreview) close() {
	if p.renderer != nil {
		p.renderer.close()
	}
	releaseCOM(&p.staging)
}
func (r *gpuRenderer) thumbnail(g, s uint64, state projectionState, fps int) (*previewFrame, error) {
	if !r.lastValid {
		return nil, nil
	}
	if r.preview == nil {
		// The submitted texture is already rotated/composited, so render it with identity rotation.
		capture := *r.capture
		capture.output.Rotation = 1
		cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, int32(r.width), int32(r.height)}}, Target: Monitor{Rect: RECT{0, 0, previewW, previewH}}, Smooth: true}
		thumb, err := newGPURenderer(0, &capture, cfg)
		if err != nil {
			return nil, err
		}
		p := &gpuPreview{renderer: thumb}
		thumb.desktopView, err = makeView(r.capture.device, r.last, 7)
		if err != nil {
			p.close()
			return nil, err
		}
		thumb.valid = true
		desc := textureDesc{Width: previewW, Height: previewH, Mips: 1, ArraySize: 1, Format: 87, SampleCount: 1, Usage: 3, CPUAccess: 0x20000}
		err = hrError("preview staging", comCall(r.capture.device, 5, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&p.staging))))
		if err != nil {
			p.close()
			return nil, err
		}
		r.preview = p
	}
	p := r.preview
	ctx := r.capture.context
	if fps != 5 {
		fps = 30
	}
	interval := time.Second / time.Duration(fps)
	if p.interval != interval {
		p.interval = interval
		p.next = time.Time{}
	}
	var ready *previewFrame
	if p.pending {
		var mapped mappedTexture
		hr := comCall(ctx, 14, uintptr(unsafe.Pointer(p.staging)), 0, 1, 0x100000, uintptr(unsafe.Pointer(&mapped))) // DO_NOT_WAIT
		if uint32(hr) == 0x887a000a {
			if time.Since(p.frame.At) > time.Second {
				return nil, fmt.Errorf("预览读回超时")
			}
			return nil, nil
		}
		if err := hrError("preview map", hr); err != nil {
			return nil, err
		}
		unmap := func() { comCall(ctx, 15, uintptr(unsafe.Pointer(p.staging)), 0) }
		p.pending = false
		if mapped.Data == nil || mapped.RowPitch < previewW*4 {
			unmap()
			return nil, fmt.Errorf("预览像素无效")
		}
		if p.frame.Generation != g || p.frame.Sequence != s || p.frame.State != state {
			p.next = time.Time{}
			unmap()
			return nil, nil
		}
		f := p.frame
		f.Pixels = make([]byte, previewW*previewH*4)
		for y := 0; y < previewH; y++ {
			copy(f.Pixels[y*previewW*4:], unsafe.Slice((*byte)(unsafe.Add(mapped.Data, uintptr(y)*uintptr(mapped.RowPitch))), previewW*4))
		}
		unmap()
		ready = &f
	}
	if time.Now().Before(p.next) {
		return ready, nil
	}
	if err := p.renderer.render(); err != nil {
		return nil, err
	}
	comCall(ctx, 47, uintptr(unsafe.Pointer(p.staging)), uintptr(unsafe.Pointer(p.renderer.composite)))
	comCall(ctx, 111) // Submit the small copy without waiting on the CPU.
	p.frame = previewFrame{Generation: g, Sequence: s, State: state, At: time.Now()}
	p.pending = true
	p.next = time.Now().Add(p.interval)
	return ready, nil
}
