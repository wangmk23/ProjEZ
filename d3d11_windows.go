//go:build windows && amd64

package main

import (
	_ "embed"
	"fmt"
	"runtime"
	"unsafe"
)

//go:embed assets/projection_vs.cso
var vertexShader []byte

//go:embed assets/projection_ps.cso
var pixelShader []byte

type swapDesc1 struct{ Width, Height, Format, Stereo, SampleCount, SampleQuality, Usage, Count, Scaling, Effect, Alpha, Flags uint32 }
type samplerDesc struct {
	Filter, U, V, W        uint32
	Bias                   float32
	Anisotropy, Comparison uint32
	Border                 [4]float32
	Min, Max               float32
}
type bufferDesc struct{ Size, Usage, Bind, CPU, Misc, Stride uint32 }
type gpuConstants struct{ Source, Cursor, Options [4]float32 }
type viewport struct{ X, Y, W, H, Min, Max float32 }
type mappedTexture struct {
	Data                 unsafe.Pointer
	RowPitch, DepthPitch uint32
}
type gpuRenderer struct {
	preview                                                                                *gpuPreview
	capture                                                                                *desktopDuplication
	cfg                                                                                    projectionConfig
	swap, back, composite, compositeView, last, desktop, desktopView, pointer, pointerView comPtr
	vs, ps, constants, point, linear                                                       comPtr
	width, height, srcW, srcH                                                              uint32
	data                                                                                   gpuConstants
	valid, lastValid                                                                       bool
	snapshot                                                                               []byte
	snapshotW, snapshotH                                                                   uint32
}

func makeTexture(device comPtr, w, h, format, bind uint32) (comPtr, error) {
	desc := textureDesc{Width: w, Height: h, Mips: 1, ArraySize: 1, Format: format, SampleCount: 1, Bind: bind}
	var tex comPtr
	hr := comCall(device, 5, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&tex)))
	return tex, hrError("CreateTexture2D", hr)
}
func makeView(device, texture comPtr, slot int) (comPtr, error) {
	var view comPtr
	hr := comCall(device, slot, uintptr(unsafe.Pointer(texture)), 0, uintptr(unsafe.Pointer(&view)))
	return view, hrError("CreateView", hr)
}
func newGPURenderer(hwnd uintptr, capture *desktopDuplication, cfg projectionConfig) (r *gpuRenderer, err error) {
	r = &gpuRenderer{capture: capture, cfg: cfg}
	defer func() {
		if err != nil {
			r.close()
		}
	}()
	r.width = uint32(cfg.Target.Rect.Right - cfg.Target.Rect.Left)
	r.height = uint32(cfg.Target.Rect.Bottom - cfg.Target.Rect.Top)
	if r.width == 0 || r.height == 0 || r.width > 16384 || r.height > 16384 {
		return r, fmt.Errorf("无效输出尺寸")
	}
	device := capture.device
	for _, s := range []struct {
		slot  int
		bytes []byte
		dst   *comPtr
	}{{12, vertexShader, &r.vs}, {15, pixelShader, &r.ps}} {
		if err = hrError("CreateShader", comCall(device, s.slot, uintptr(unsafe.Pointer(&s.bytes[0])), uintptr(len(s.bytes)), 0, uintptr(unsafe.Pointer(s.dst)))); err != nil {
			return
		}
		runtime.KeepAlive(s.bytes)
	}
	cb := bufferDesc{Size: 48, Bind: 4}
	if err = hrError("CreateBuffer", comCall(device, 3, uintptr(unsafe.Pointer(&cb)), 0, uintptr(unsafe.Pointer(&r.constants)))); err != nil {
		return
	}
	for _, s := range []struct {
		filter uint32
		dst    *comPtr
	}{{0, &r.point}, {0x15, &r.linear}} {
		desc := samplerDesc{Filter: s.filter, U: 3, V: 3, W: 3, Comparison: 1, Max: 3.402823466e38}
		if err = hrError("CreateSampler", comCall(device, 23, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(s.dst)))); err != nil {
			return
		}
	}
	if hwnd != 0 {
		factory, e := queryCOM(capture.factory, &iidFactory2)
		if e != nil {
			return r, e
		}
		defer releaseCOM(&factory)
		desc := swapDesc1{Width: r.width, Height: r.height, Format: 87, SampleCount: 1, Usage: 0x20, Count: 2, Effect: 3}
		if err = hrError("CreateSwapChainForHwnd", comCall(factory, 15, uintptr(unsafe.Pointer(device)), hwnd, uintptr(unsafe.Pointer(&desc)), 0, 0, uintptr(unsafe.Pointer(&r.swap)))); err != nil {
			return
		}
		comCall(factory, 8, hwnd, 3) // no DXGI Alt+Enter/window ownership
		if err = hrError("GetBuffer", comCall(r.swap, 9, 0, uintptr(unsafe.Pointer(&iidTexture2D)), uintptr(unsafe.Pointer(&r.back)))); err != nil {
			return
		}
	}
	r.composite, err = makeTexture(device, r.width, r.height, 87, 0x20)
	if err != nil {
		return
	}
	r.compositeView, err = makeView(device, r.composite, 9)
	if err != nil {
		return
	}
	r.last, err = makeTexture(device, r.width, r.height, 87, 8)
	if err != nil {
		return
	}
	r.pointer, err = makeTexture(device, 1, 1, 28, 8)
	if err != nil {
		return
	}
	r.pointerView, err = makeView(device, r.pointer, 7)
	return
}
func (r *gpuRenderer) ensureDesktop(w, h uint32) error {
	if r.desktop != nil && w == r.srcW && h == r.srcH {
		return nil
	}
	comCall(r.capture.context, 8, 0, 0, 0)
	releaseCOM(&r.desktopView)
	releaseCOM(&r.desktop)
	var err error
	r.desktop, err = makeTexture(r.capture.device, w, h, 87, 8)
	if err != nil {
		return err
	}
	r.desktopView, err = makeView(r.capture.device, r.desktop, 7)
	if err != nil {
		return err
	}
	r.srcW, r.srcH = w, h
	return nil
}
func (r *gpuRenderer) copyFrame(f desktopFrame) error {
	if err := r.ensureDesktop(f.Width, f.Height); err != nil {
		return err
	}
	comCall(r.capture.context, 47, uintptr(unsafe.Pointer(r.desktop)), uintptr(unsafe.Pointer(f.Texture)))
	r.valid = true
	return nil
}
func (r *gpuRenderer) uploadDesktop(pixels []byte, w, h uint32) error {
	if uint64(len(pixels)) != uint64(w)*uint64(h)*4 {
		return fmt.Errorf("invalid pixel size")
	}
	if err := r.ensureDesktop(w, h); err != nil {
		return err
	}
	comCall(r.capture.context, 48, uintptr(unsafe.Pointer(r.desktop)), 0, 0, uintptr(unsafe.Pointer(&pixels[0])), uintptr(w*4), 0)
	runtime.KeepAlive(pixels)
	r.valid = true
	return nil
}
func (r *gpuRenderer) render() error {
	if !r.valid {
		return fmt.Errorf("尚无有效画面")
	}
	ctx := r.capture.context
	var clear = [4]float32{0, 0, 0, 1}
	comCall(ctx, 50, uintptr(unsafe.Pointer(r.compositeView)), uintptr(unsafe.Pointer(&clear)))
	comCall(ctx, 33, 1, uintptr(unsafe.Pointer(&r.compositeView)), 0)
	sw := r.cfg.Source.Rect.Right - r.cfg.Source.Rect.Left
	sh := r.cfg.Source.Rect.Bottom - r.cfg.Source.Rect.Top
	dw, dh := outputSize(sw, sh, int32(r.width), int32(r.height), r.cfg.Fill)
	vp := viewport{X: float32((int32(r.width) - dw) / 2), Y: float32((int32(r.height) - dh) / 2), W: float32(dw), H: float32(dh), Max: 1}
	comCall(ctx, 44, 1, uintptr(unsafe.Pointer(&vp)))
	r.data.Source[0] = float32(sw)
	r.data.Source[1] = float32(sh)
	r.data.Source[2] = float32(r.capture.output.Rotation)
	r.data.Options[2] = 0
	r.data.Options[3] = 0
	if dw < sw || dh < sh {
		r.data.Options[3] = 1
	}
	if r.cfg.Smooth && r.data.Options[3] == 0 && !usePointSampling(sw, sh, dw, dh, true) {
		r.data.Options[2] = 0.2
	}
	comCall(ctx, 48, uintptr(unsafe.Pointer(r.constants)), 0, 0, uintptr(unsafe.Pointer(&r.data)), 0, 0)
	comCall(ctx, 16, 0, 1, uintptr(unsafe.Pointer(&r.constants)))
	views := [2]comPtr{r.desktopView, r.pointerView}
	comCall(ctx, 8, 0, 2, uintptr(unsafe.Pointer(&views[0])))
	sampler := r.linear
	if usePointSampling(sw, sh, dw, dh, r.cfg.Smooth) {
		sampler = r.point
	}
	comCall(ctx, 10, 0, 1, uintptr(unsafe.Pointer(&sampler)))
	comCall(ctx, 17, 0)
	comCall(ctx, 24, 4)
	comCall(ctx, 11, uintptr(unsafe.Pointer(r.vs)), 0, 0)
	comCall(ctx, 9, uintptr(unsafe.Pointer(r.ps)), 0, 0)
	comCall(ctx, 13, 3, 0)
	var nulls [2]comPtr
	comCall(ctx, 8, 0, 2, uintptr(unsafe.Pointer(&nulls[0])))
	comCall(ctx, 33, 0, 0, 0)
	return hrError("GPU device", comCall(r.capture.device, 39))
}
func (r *gpuRenderer) present() (bool, error) {
	if err := r.render(); err != nil {
		return false, err
	}
	if r.swap != nil {
		comCall(r.capture.context, 47, uintptr(unsafe.Pointer(r.back)), uintptr(unsafe.Pointer(r.composite)))
		// Nonblocking presentation: no driver queue accumulation; commands remain serviceable.
		hr := comCall(r.swap, 8, 1, 8)
		if uint32(hr) == 0x887a000a || uint32(hr) == 0x087a0001 {
			return false, nil
		}
		if err := hrError("Present", hr); err != nil {
			return false, err
		}
	}
	comCall(r.capture.context, 47, uintptr(unsafe.Pointer(r.last)), uintptr(unsafe.Pointer(r.composite)))
	r.lastValid = true
	return true, nil
}
func (r *gpuRenderer) readTexture(tex comPtr, w, h uint32) ([]byte, error) {
	desc := textureDesc{Width: w, Height: h, Mips: 1, ArraySize: 1, Format: 87, SampleCount: 1, Usage: 3, CPUAccess: 0x20000}
	var staging comPtr
	if err := hrError("Create staging", comCall(r.capture.device, 5, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&staging)))); err != nil {
		return nil, err
	}
	defer releaseCOM(&staging)
	ctx := r.capture.context
	comCall(ctx, 47, uintptr(unsafe.Pointer(staging)), uintptr(unsafe.Pointer(tex)))
	var mapped mappedTexture
	if err := hrError("Map", comCall(ctx, 14, uintptr(unsafe.Pointer(staging)), 0, 1, 0, uintptr(unsafe.Pointer(&mapped)))); err != nil {
		return nil, err
	}
	defer comCall(ctx, 15, uintptr(unsafe.Pointer(staging)), 0)
	if mapped.Data == nil || mapped.RowPitch < w*4 {
		return nil, fmt.Errorf("invalid mapped texture")
	}
	pixels := make([]byte, int(w)*int(h)*4)
	for y := uint32(0); y < h; y++ {
		row := unsafe.Slice((*byte)(unsafe.Add(mapped.Data, uintptr(y)*uintptr(mapped.RowPitch))), int(w)*4)
		copy(pixels[int(y)*int(w)*4:], row)
	}
	return pixels, nil
}
func (r *gpuRenderer) freeze() error {
	if !r.lastValid {
		return fmt.Errorf("尚无可冻结画面")
	}
	p, err := r.readTexture(r.last, r.width, r.height)
	if err != nil {
		return err
	}
	r.snapshot = p
	r.snapshotW = r.width
	r.snapshotH = r.height
	return nil
}
func (r *gpuRenderer) presentFrozen() (bool, error) {
	if !r.lastValid {
		return false, fmt.Errorf("冻结画面不可用")
	}
	if r.swap == nil {
		return true, nil
	}
	comCall(r.capture.context, 47, uintptr(unsafe.Pointer(r.back)), uintptr(unsafe.Pointer(r.last)))
	hr := comCall(r.swap, 8, 1, 8)
	if uint32(hr) == 0x887a000a || uint32(hr) == 0x087a0001 {
		return false, nil
	}
	return hr == 0, hrError("Present frozen", hr)
}
func (r *gpuRenderer) restoreSnapshot(p []byte, w, h uint32) error {
	if r.width != w || r.height != h || uint64(len(p)) != uint64(w)*uint64(h)*4 {
		return fmt.Errorf("显示尺寸改变，停止输出以保护冻结内容")
	}
	comCall(r.capture.context, 48, uintptr(unsafe.Pointer(r.last)), 0, 0, uintptr(unsafe.Pointer(&p[0])), uintptr(w*4), 0)
	runtime.KeepAlive(p)
	r.lastValid = true
	r.snapshot = p
	r.snapshotW = w
	r.snapshotH = h
	return nil
}
func (r *gpuRenderer) close() {
	if r.preview != nil {
		r.preview.close()
		r.preview = nil
	}
	if r.capture != nil && r.capture.context != nil {
		comCall(r.capture.context, 110)
	}
	for _, p := range []*comPtr{&r.pointerView, &r.pointer, &r.desktopView, &r.desktop, &r.compositeView, &r.composite, &r.last, &r.back, &r.constants, &r.point, &r.linear, &r.vs, &r.ps, &r.swap} {
		releaseCOM(p)
	}
}
