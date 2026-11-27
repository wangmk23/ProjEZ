package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestRegionCropRotation(t *testing.T) {
	r := RECT{16, 20, 48, 60}
	cases := []struct {
		rotation, w, h uint32
		want           RECT
	}{
		{1, 100, 80, r}, {2, 80, 100, RECT{20, 52, 60, 84}}, {3, 100, 80, RECT{52, 20, 84, 60}}, {4, 80, 100, RECT{20, 16, 60, 48}},
	}
	for _, c := range cases {
		if got := rotatedRegion(r, c.w, c.h, c.rotation); got != c.want {
			t.Fatalf("rotation %d: %v", c.rotation, got)
		}
	}
	if got := regionRect(POINT{200, 50}, POINT{-5, 20}, 100, 80); got != (RECT{0, 20, 100, 50}) {
		t.Fatal(got)
	}
}
func TestSceneTransparentImageBehindText(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	path := filepath.Join(t.TempDir(), "overlay.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, img)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	picture := defaultLayer()
	picture.File = path
	picture.X, picture.Y, picture.W, picture.H = 0, 0, 100, 100
	text := defaultLayer()
	text.Text = "ProjEZ"
	text.X, text.Y, text.W, text.H = 0, 5, 100, 50
	canvas, err := rasterScene([]sceneLayer{picture, text}, 320, 320)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canvas.Pixels[:4], []byte{0, 0, 128, 128}) {
		t.Fatalf("PNG alpha lost: %v", canvas.Pixels[:4])
	}
	white := false
	for i := 0; i < len(canvas.Pixels); i += 4 {
		if canvas.Pixels[i] > 100 && canvas.Pixels[i+1] > 100 {
			white = true
			break
		}
	}
	if !white {
		t.Fatal("text not composited above image")
	}
}
func TestSceneGPUComposesCroppedPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var device, ctx comPtr
	hr, _, _ := syscall.NewLazyDLL("d3d11.dll").NewProc("D3D11CreateDevice").Call(0, 5, 0, 0x20, 0, 0, 7, uintptr(unsafe.Pointer(&device)), 0, uintptr(unsafe.Pointer(&ctx)))
	if err := hrError("D3D11 WARP", hr); err != nil {
		t.Fatal(err)
	}
	d := &desktopDuplication{device: device, context: ctx, output: gpuOutput{Rotation: 1}}
	defer d.close()
	source, err := makeTexture(device, 48, 32, 87, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCOM(&source)
	original := bytes.Repeat([]byte{255, 0, 0, 255}, 48*32)
	for y := 0; y < 32; y++ {
		for x := 16; x < 32; x++ {
			i := (y*48 + x) * 4
			copy(original[i:i+4], []byte{0, 100, 0, 255})
		}
	}
	comCall(ctx, 48, uintptr(unsafe.Pointer(source)), 0, 0, uintptr(unsafe.Pointer(&original[0])), 48*4, 0)
	runtime.KeepAlive(original)
	overlay := &sceneRaster{1, 1, []byte{0, 0, 128, 128}}
	config := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 48, 32}}, Target: Monitor{Rect: RECT{0, 0, 16, 32}}, Region: RECT{16, 0, 32, 32}, Overlay: overlay}
	r, err := newGPURenderer(0, d, config)
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	if err = r.copyFrame(desktopFrame{Texture: source, Width: 48, Height: 32}); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.present(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	got, err := r.readTexture(r.last, 16, 32)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(got); i += 4 {
		if !bytes.Equal(got[i:i+4], []byte{0, 50, 128, 255}) {
			t.Fatal("crop or overlay pixel", got[i:i+4])
		}
	}
	if ok, err := r.presentBlack(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	got, err = r.readTexture(r.last, 16, 32)
	if err != nil || !bytes.Equal(got[:4], []byte{0, 0, 0, 255}) {
		t.Fatal("black retained overlay", err)
	}
}

func TestFrozenClockUpdatesOnlyItsOverlay(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var device, ctx comPtr
	hr, _, _ := syscall.NewLazyDLL("d3d11.dll").NewProc("D3D11CreateDevice").Call(0, 5, 0, 0x20, 0, 0, 7, uintptr(unsafe.Pointer(&device)), 0, uintptr(unsafe.Pointer(&ctx)))
	if err := hrError("D3D11 WARP", hr); err != nil {
		t.Fatal(err)
	}
	d := &desktopDuplication{device: device, context: ctx, output: gpuOutput{Rotation: 1}}
	defer d.close()
	const w, h = 320, 180
	l := defaultLayer()
	l.Clock = true
	l.Font = "Consolas"
	l.Size = 96
	l.X, l.Y, l.W, l.H = 5, 5, 75, 28
	clock := newOutputClock(l, w, h)
	r, err := newGPURenderer(0, d, projectionConfig{Source: Monitor{Rect: RECT{0, 0, w, h}}, Target: Monitor{Rect: RECT{0, 0, w, h}}, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	original := bytes.Repeat([]byte{80, 50, 20, 255}, w*h)
	if err := r.uploadDesktop(original, w, h); err != nil {
		t.Fatal(err)
	}
	if shown, err := r.present(); !shown || err != nil {
		t.Fatal(shown, err)
	}
	if err := r.freeze(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(r.snapshot, original) {
		t.Fatal("clock leaked into frozen base")
	}
	// Simulate another source frame arriving while frozen. Only the stored base may be shown.
	if err := r.uploadDesktop(bytes.Repeat([]byte{31, 61, 91, 255}, w*h), w, h); err != nil {
		t.Fatal(err)
	}
	read := func(at time.Time) []byte {
		t.Helper()
		if shown, err := r.renderFrozenClock(at); !shown || err != nil {
			t.Fatal(shown, err)
		}
		got, err := r.readTexture(r.last, w, h)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got[len(got)-4:], original[len(original)-4:]) {
			t.Fatal("frozen base was replaced")
		}
		return got
	}
	now := time.Date(2026, 11, 27, 12, 34, 56, 0, time.Local)
	first, second := read(now), read(now.Add(time.Second))
	if bytes.Equal(first, second) || bytes.Equal(second, original) {
		t.Fatal("frozen clock did not change")
	}
	if shown, err := r.presentBlack(); !shown || err != nil {
		t.Fatal(shown, err)
	}
	black, err := r.readTexture(r.last, w, h)
	if err != nil || !bytes.Equal(black, bytes.Repeat([]byte{0, 0, 0, 255}, w*h)) {
		t.Fatal("clock still visible on black output", err)
	}
}
func TestClockGDIFrozenBase(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldClock := activeSceneClock
	defer func() { releaseGDIClock(); activeSceneClock = oldClock }()
	l := defaultLayer()
	l.Clock = true
	l.Font = "Consolas"
	l.Size = 96
	l.X, l.Y, l.W, l.H = 0, 0, 75, 25
	activeSceneClock = newOutputClock(l, 320, 180)
	dc, bmp, old, pixels, err := createSceneDIB(320, 180)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSceneDIB(dc, bmp, old)
	initial := bytes.Repeat([]byte{10, 20, 30, 255}, 320*180)
	copy(pixels, initial)
	at := time.Date(2026, 11, 27, 8, 0, 0, 0, time.Local)
	if err := blendGDIClock(dc, at); err != nil {
		t.Fatal(err)
	}
	gdi32.NewProc("GdiFlush").Call()
	first := append([]byte(nil), pixels...)
	copy(pixels, initial)
	if err := blendGDIClock(dc, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	gdi32.NewProc("GdiFlush").Call()
	if bytes.Equal(first, pixels) || bytes.Equal(pixels, initial) {
		t.Fatal("GDI clock did not change")
	}
	if !bytes.Equal(pixels[len(pixels)-4:], initial[len(initial)-4:]) {
		t.Fatal("unrelated pixels changed")
	}
}
