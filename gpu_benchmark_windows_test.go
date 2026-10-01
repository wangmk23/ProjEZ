package main

import (
	"os"
	"runtime"
	"sort"
	"testing"
	"time"
	"unsafe"
)

func TestGPUResourceCycles(t *testing.T) {
	if os.Getenv("PF_GPU_STRESS") != "1" {
		t.Skip("PF_GPU_STRESS=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	handles := func() uint32 {
		var n uint32
		kernel32.NewProc("GetProcessHandleCount").Call(^uintptr(0), uintptr(unsafe.Pointer(&n)))
		return n
	}
	var before uint32
	for i := 0; i < 110; i++ {
		d, e := openDuplication(outs[0].Device)
		if e != nil {
			t.Fatalf("cycle %d: %v", i, e)
		}
		cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 4, 4}}, Target: Monitor{Rect: RECT{0, 0, 4, 4}}}
		r, e := newGPURenderer(0, d, cfg)
		if e != nil {
			d.close()
			t.Fatal(e)
		}
		e = r.uploadDesktop(make([]byte, 64), 4, 4)
		if e == nil {
			_, e = r.present()
		}
		if e == nil {
			e = r.freeze()
		}
		r.close()
		d.close()
		if i == 9 {
			runtime.GC()
			before = handles()
			t.Logf("after driver/Go warmup: %d handles", before)
		}
		if i >= 9 && (i+1)%20 == 10 {
			t.Logf("cycle %d after warmup: %d handles", i-9, handles())
		}
		if e != nil {
			t.Fatalf("cycle %d: %v", i, e)
		}
	}
	runtime.GC()
	after := handles()
	t.Logf("100 device/capture/render/freeze/close cycles; process handles %d -> %d", before, after)
	if after > before+20 {
		t.Fatal("persistent native handle growth")
	}
}
func TestGPUOffscreenSustained(t *testing.T) {
	if os.Getenv("PF_GPU_BENCH") != "1" {
		t.Skip("PF_GPU_BENCH=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outs, e := enumerateGPUOutputs()
	if e != nil || len(outs) == 0 {
		t.Fatal(e)
	}
	d, e := openDuplication(outs[0].Device)
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 2560, 1440}}, Target: Monitor{Rect: RECT{0, 0, 2560, 1440}}}
	r, e := newGPURenderer(0, d, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	tex, e := makeTexture(d.device, 2560, 1440, 87, 0x20)
	if e != nil {
		t.Fatal(e)
	}
	defer releaseCOM(&tex)
	view, e := makeView(d.device, tex, 9)
	if e != nil {
		t.Fatal(e)
	}
	defer releaseCOM(&view)
	start := time.Now()
	samples := []float64{}
	var count int
	for time.Since(start) < 60*time.Second {
		tick := time.Now()
		color := [4]float32{float32(count%200) / 200, 0.3, 0.6, 1}
		comCall(d.context, 50, uintptr(unsafe.Pointer(view)), uintptr(unsafe.Pointer(&color)))
		if e = r.copyFrame(desktopFrame{Texture: tex, Width: 2560, Height: 1440}); e != nil {
			t.Fatal(e)
		}
		if _, e = r.present(); e != nil {
			t.Fatal(e)
		}
		// Synchronize the test only every 200 frames, boundedly, using a readback checkpoint.
		// Real projection never does this; timing below is CPU submission, not monitor FPS.
		if count%200 == 0 {
			if _, e = r.readTexture(r.last, 2560, 1440); e != nil {
				t.Fatal(e)
			}
		}
		samples = append(samples, float64(time.Since(tick).Microseconds())/1000)
		count++
		time.Sleep(frameDelay(200, time.Since(tick)))
	}
	elapsed := time.Since(start).Seconds()
	sort.Float64s(samples)
	t.Logf("synthetic GPU offscreen 2560x1440 %.1fs: %d cycles %.1f/s CPU submission p50 %.3fms p95 %.3fms; NOT desktop capture or physical-output FPS", elapsed, count, float64(count)/elapsed, samples[len(samples)/2], samples[len(samples)*95/100])
}

func TestGPUPreviewOverhead(t *testing.T) {
	if os.Getenv("PF_PREVIEW_BENCH") != "1" {
		t.Skip("PF_PREVIEW_BENCH=1")
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
	cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 2560, 1440}}, Target: Monitor{Rect: RECT{0, 0, 2560, 1440}}}
	r, e := newGPURenderer(0, d, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	tex, e := makeTexture(d.device, 2560, 1440, 87, 0x20)
	if e != nil {
		t.Fatal(e)
	}
	defer releaseCOM(&tex)
	view, e := makeView(d.device, tex, 9)
	if e != nil {
		t.Fatal(e)
	}
	defer releaseCOM(&view)
	for _, enabled := range []bool{false, true, false, true} {
		start := time.Now()
		samples := []float64{}
		var count int
		var previews int
		for time.Since(start) < 3*time.Second {
			tick := time.Now()
			color := [4]float32{float32(count%200) / 200, 0.3, 0.6, 1}
			comCall(d.context, 50, uintptr(unsafe.Pointer(view)), uintptr(unsafe.Pointer(&color)))
			if e = r.copyFrame(desktopFrame{Texture: tex, Width: 2560, Height: 1440}); e != nil {
				t.Fatal(e)
			}
			if _, e = r.present(); e != nil {
				t.Fatal(e)
			}
			if enabled {
				if f, err := r.thumbnail(1, 1, projectionLive, 30); err != nil {
					t.Fatal(err)
				} else if f != nil {
					previews++
				}
			}
			// Synchronize the test only every 200 frames, boundedly, using a readback checkpoint.
			// Real projection never does this; timing below is CPU submission, not monitor FPS.
			if count%200 == 0 {
				if _, e = r.readTexture(r.last, 2560, 1440); e != nil {
					t.Fatal(e)
				}
			}
			samples = append(samples, float64(time.Since(tick).Microseconds())/1000)
			count++
			time.Sleep(frameDelay(200, time.Since(tick)))
		}
		elapsed := time.Since(start).Seconds()
		t.Logf("thumbnail delivered %.1f FPS", float64(previews)/elapsed)
		if enabled && float64(previews)/elapsed < 15 {
			t.Fatal("preview still too slow in synthetic test")
		}
		sort.Float64s(samples)
		t.Logf("preview=%v synthetic GPU offscreen 2560x1440 %.1fs: %d cycles %.1f/s CPU submission p50 %.3fms p95 %.3fms; NOT desktop capture or physical-output FPS", enabled, elapsed, count, float64(count)/elapsed, samples[len(samples)/2], samples[len(samples)*95/100])
	}
}
