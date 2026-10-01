package main

import (
	"bytes"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestGPUSizeABI(t *testing.T) {
	if unsafe.Sizeof(swapDesc1{}) != 48 || unsafe.Sizeof(samplerDesc{}) != 52 || unsafe.Sizeof(gpuConstants{}) != 48 {
		t.Fatal("incorrect D3D ABI")
	}
}

func TestGPUCursorPixels(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	d, e := openRenderTestDevice(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 2, 2}}, Target: Monitor{Rect: RECT{0, 0, 2, 2}}, Cursor: true}
	r, e := newGPURenderer(0, d, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	background := bytes.Repeat([]byte{0x56, 0x34, 0x12, 255}, 4)
	if e = r.uploadDesktop(background, 2, 2); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		name      string
		shape     pointerShapeInfo
		raw, want []byte
	}{
		{"mono invert", pointerShapeInfo{Type: 1, Width: 1, Height: 2, Pitch: 1}, []byte{128, 128}, []byte{0xa9, 0xcb, 0xed, 255}},
		{"color", pointerShapeInfo{Type: 2, Width: 1, Height: 1, Pitch: 4}, []byte{7, 8, 9, 255}, []byte{7, 8, 9, 255}},
		{"masked xor", pointerShapeInfo{Type: 4, Width: 1, Height: 1, Pitch: 4}, []byte{255, 0, 255, 255}, []byte{0xa9, 0x34, 0xed, 255}},
	} {
		d.shape = c.shape
		d.pointer = c.raw
		f := desktopFrame{Info: dupFrameInfo{MouseTime: 1, ShapeSize: uint32(len(c.raw)), Visible: 1}}
		if e = r.updatePointer(d, f); e != nil {
			t.Fatal(e)
		}
		if e = r.render(); e != nil {
			t.Fatal(e)
		}
		got, e := r.readTexture(r.composite, 2, 2)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(got[:4], c.want) || !bytes.Equal(got[4:], background[4:]) {
			t.Fatalf("%s got %x", c.name, got)
		}
	}
}

func TestGPURotationPixels(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	d, e := openRenderTestDevice(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	for _, c := range []struct {
		rotation uint32
		want     []byte
	}{{1, []byte{1, 2, 3, 4}}, {2, []byte{3, 1, 4, 2}}, {3, []byte{4, 3, 2, 1}}, {4, []byte{2, 4, 1, 3}}} {
		cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 2, 2}}, Target: Monitor{Rect: RECT{0, 0, 2, 2}}}
		r, e := newGPURenderer(0, d, cfg)
		if e != nil {
			t.Fatal(e)
		}
		d.output.Rotation = c.rotation
		if e = r.uploadDesktop([]byte{1, 0, 0, 255, 2, 0, 0, 255, 3, 0, 0, 255, 4, 0, 0, 255}, 2, 2); e != nil {
			t.Fatal(e)
		}
		if e = r.render(); e != nil {
			t.Fatal(e)
		}
		got, e := r.readTexture(r.composite, 2, 2)
		if e != nil {
			t.Fatal(e)
		}
		r.close()
		for i, want := range c.want {
			if got[i*4] != want {
				t.Fatalf("rotation %d pixel %d got %d want %d", c.rotation, i, got[i*4], want)
			}
		}
	}
}
func TestGPUOffscreenPixels(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatalf("outputs %v", e)
	}
	d, e := openRenderTestDevice(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 4, 4}}, Target: Monitor{Rect: RECT{0, 0, 4, 4}}}
	r, e := newGPURenderer(0, d, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	pixels := make([]byte, 64)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i] = byte(i)
		pixels[i+1] = 100
		pixels[i+2] = 200
		pixels[i+3] = 255
	}
	if e = r.uploadDesktop(pixels, 4, 4); e != nil {
		t.Fatal(e)
	}
	if e = r.render(); e != nil {
		t.Fatal(e)
	}
	got, e := r.readTexture(r.composite, 4, 4)
	if e != nil {
		t.Fatal(e)
	}
	for i := range pixels {
		if got[i] != pixels[i] {
			t.Fatalf("pixel[%d] %d != %d", i, got[i], pixels[i])
		}
	}
}

// Check small text-like contrast edges, while preventing halos and 1:1 changes.
func TestGPUScaledDetailContrast(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	d, e := openRenderTestDevice(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	pixels := []byte{0, 0, 0, 255, 80, 80, 80, 255, 160, 160, 160, 255, 240, 240, 240, 255}
	for _, w := range []int32{4, 8} {
		cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 4, 1}}, Target: Monitor{Rect: RECT{0, 0, w, 1}}, Smooth: true, Fill: true}
		r, e := newGPURenderer(0, d, cfg)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.uploadDesktop(pixels, 4, 1); e != nil {
			t.Fatal(e)
		}
		if e = r.render(); e != nil {
			t.Fatal(e)
		}
		got, e := r.readTexture(r.composite, uint32(w), 1)
		r.close()
		if e != nil {
			t.Fatal(e)
		}
		if w == 4 && !bytes.Equal(got, pixels) {
			t.Fatalf("1:1 altered: %v", got)
		}
		if w == 8 {
			if got[4] >= 20 || got[24] <= 220 {
				t.Fatalf("scaled edge still softened: %v", got)
			}
			for i := 0; i < 8; i++ {
				if got[4*i] > 240 || (i > 0 && got[4*i] < got[4*(i-1)]) {
					t.Fatalf("edge halo: %v", got)
				}
			}
		}
	}
}

// Renderer tests upload their own pixels. Do not acquire the real desktop just
// to create a D3D device: DXGI capture is covered by separate integration tests.
func openRenderTestDevice(device string) (*desktopDuplication, error) {
	d := &desktopDuplication{}
	err := walkOutputs(func(a, o comPtr, info gpuOutput) error {
		if info.Device != device {
			return nil
		}
		d.output = info
		comCall(a, 1)
		d.adapter = a
		var e error
		d.device, d.context, e = createD3DDevice(a)
		return e
	})
	if err != nil {
		d.close()
		return nil, err
	}
	return d, nil
}

// A full-resolution high-frequency pattern catches accidental preview-sized
// intermediates and fractional offsets that small solid-color tests miss.
func TestGPU2KPixelIdentity(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal("no output", e)
	}
	d, e := openRenderTestDevice(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	const w, h = 2560, 1440
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			pixels[i] = byte(x)
			pixels[i+1] = byte(y)
			pixels[i+2] = byte((x + y) % 2 * 255)
			pixels[i+3] = 255
		}
	}
	for _, smooth := range []bool{false, true} {
		cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, w, h}}, Target: Monitor{Rect: RECT{0, 0, w, h}}, Smooth: smooth}
		r, e := newGPURenderer(0, d, cfg)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.uploadDesktop(pixels, w, h); e != nil {
			r.close()
			t.Fatal(e)
		}
		if e = r.render(); e != nil {
			r.close()
			t.Fatal(e)
		}
		got, e := r.readTexture(r.composite, w, h)
		r.close()
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(got, pixels) {
			t.Fatalf("2K pixel identity failed, smooth=%t", smooth)
		}
	}
}

func TestGPUDownscaleThinLines(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	d, e := openRenderTestDevice(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	// One bright one-pixel stroke in every three source pixels must not vanish.
	for _, size := range []struct{ sw, sh, dw, dh int32 }{{6, 3, 2, 1}, {3, 6, 1, 2}} {
		cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, size.sw, size.sh}}, Target: Monitor{Rect: RECT{0, 0, size.dw, size.dh}}, Smooth: true, Fill: true}
		r, e := newGPURenderer(0, d, cfg)
		if e != nil {
			t.Fatal(e)
		}
		pixels := make([]byte, int(size.sw*size.sh)*4)
		for y := int32(0); y < size.sh; y++ {
			for x := int32(0); x < size.sw; x++ {
				i := (y*size.sw + x) * 4
				v := byte(0)
				if (size.sw == 6 && x%3 == 0) || (size.sh == 6 && y%3 == 0) {
					v = 255
				}
				pixels[i] = v
				pixels[i+1] = v
				pixels[i+2] = v
				pixels[i+3] = 255
			}
		}
		if e = r.uploadDesktop(pixels, uint32(size.sw), uint32(size.sh)); e != nil {
			t.Fatal(e)
		}
		if e = r.render(); e != nil {
			t.Fatal(e)
		}
		got, e := r.readTexture(r.composite, uint32(size.dw), uint32(size.dh))
		r.close()
		if e != nil {
			t.Fatal(e)
		}
		for i := 0; i < len(got); i += 4 {
			if got[i] < 84 || got[i] > 86 {
				t.Fatalf("thin stroke lost or biased: %v", got)
			}
		}
	}
}

func TestGPUDownscaleTiming(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	d, e := openRenderTestDevice(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 2560, 1440}}, Target: Monitor{Rect: RECT{0, 0, 1920, 1080}}}
	r, e := newGPURenderer(0, d, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	if e = r.uploadDesktop(make([]byte, 2560*1440*4), 2560, 1440); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	for i := 0; i < 30; i++ {
		if e = r.render(); e != nil {
			t.Fatal(e)
		}
		if _, e = r.readTexture(r.composite, 1920, 1080); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("synthetic 1440p -> 1080p render plus synchronized CPU readback: %.2f ms/frame; not capture or projector FPS", float64(time.Since(start).Microseconds())/30000)
}
