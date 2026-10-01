package main

import (
	"bytes"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestGPUCommandStopWins(t *testing.T) {
	s := newGPUMailbox(projectionConfig{Generation: 1})
	if !s.request(projectionCommand{Generation: 1, State: projectionStopping}) {
		t.Fatal("stop")
	}
	if s.request(projectionCommand{Generation: 1, State: projectionLive}) {
		t.Fatal("stop replaced")
	}
	c, ok := s.nextCommand()
	if !ok || c.State != projectionStopping {
		t.Fatal("lost stop")
	}
	if s.request(projectionCommand{Generation: 0, State: projectionLive}) {
		t.Fatal("old generation")
	}
}
func TestGPUFreezeRetainsLastPresented(t *testing.T) {
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
	cfg := projectionConfig{Source: Monitor{Rect: RECT{0, 0, 4, 4}}, Target: Monitor{Rect: RECT{0, 0, 4, 4}}}
	r, e := newGPURenderer(0, d, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	checkPreview := func(seq uint64, state projectionState, color []byte) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			f, err := r.thumbnail(7, seq, state, 30)
			if err != nil {
				t.Fatal(err)
			}
			if f != nil {
				if !f.matches(7, seq, state) {
					t.Fatal("invalid thumbnail metadata")
				}
				center := ((previewH/2)*previewW + previewW/2) * 4
				if !bytes.Equal(f.Pixels[center:center+4], color) {
					t.Fatalf("preview leaked capture: %v", f.Pixels[center:center+4])
				}
				if f.Pixels[0] != 0 || f.Pixels[1] != 0 || f.Pixels[2] != 0 {
					t.Fatal("missing preview letterbox")
				}
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("thumbnail timeout")
	}
	a := bytes.Repeat([]byte{10, 20, 30, 255}, 16)
	b := bytes.Repeat([]byte{99, 88, 77, 255}, 16)
	if e = r.uploadDesktop(a, 4, 4); e != nil {
		t.Fatal(e)
	}
	if _, e = r.present(); e != nil {
		t.Fatal(e)
	}
	if e = r.uploadDesktop(b, 4, 4); e != nil {
		t.Fatal(e)
	} // latest capture not yet presented
	checkPreview(1, projectionLive, a[:4])
	if e = r.freeze(); e != nil {
		t.Fatal(e)
	}
	checkPreview(2, projectionFrozen, a[:4])
	if !bytes.Equal(r.snapshot, a) {
		t.Fatal("froze unpresented frame")
	}
	restored, e := newGPURenderer(0, d, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.close()
	if e = restored.restoreSnapshot(r.snapshot, 4, 4); e != nil {
		t.Fatal(e)
	}
	got, e := restored.readTexture(restored.last, 4, 4)
	if e != nil || !bytes.Equal(got, a) {
		t.Fatal("recovery leaked new content", e)
	}
	if e = restored.restoreSnapshot(r.snapshot, 8, 2); e == nil {
		t.Fatal("unsafe size accepted")
	}
	if ok, err := r.presentBlack(); err != nil || !ok {
		t.Fatal("black present", err)
	}
	checkPreview(3, projectionBlack, []byte{0, 0, 0, 255})
	black, err := r.readTexture(r.last, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(black); i += 4 {
		if black[i] != 0 || black[i+1] != 0 || black[i+2] != 0 || black[i+3] != 255 {
			t.Fatal("blackout contains nonblack pixels", black)
		}
	}
	if err := r.freeze(); err != nil || !bytes.Equal(r.snapshot, black) {
		t.Fatal("freeze after black leaked old content", err)
	}
	if err := r.uploadDesktop(b, 4, 4); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.present(); err != nil || !ok {
		t.Fatal("resume", err)
	}
	checkPreview(4, projectionLive, b[:4])
	resumed, err := r.readTexture(r.last, 4, 4)
	if err != nil || !bytes.Equal(resumed, b) {
		t.Fatal("resume failed", err)
	}

}
