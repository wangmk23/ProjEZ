package main

import (
	"os"
	"runtime"
	"testing"
	"unsafe"
)

func TestDXGIABI(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want uintptr
	}{
		{"output", unsafe.Sizeof(dxgiOutputDesc{}), 96},
		{"frame", unsafe.Sizeof(dupFrameInfo{}), 48},
		{"pointer", unsafe.Sizeof(pointerShapeInfo{}), 24},
		{"texture", unsafe.Sizeof(textureDesc{}), 44},
		{"adapter", unsafe.Sizeof(adapterDesc{}), 304},
	} {
		if c.got != c.want {
			t.Fatalf("%s %d != %d", c.name, c.got, c.want)
		}
	}
}
func TestOutputSelectionDoesNotUseFirst(t *testing.T) {
	out, err := selectGPUOutput([]gpuOutput{{Device: "A", AdapterLUID: 1}, {Device: "B", AdapterLUID: 2}}, "B")
	if err != nil || out.AdapterLUID != 2 {
		t.Fatalf("%+v %v", out, err)
	}
	if _, err = selectGPUOutput(nil, "absent"); err == nil {
		t.Fatal("missing output")
	}
}
func TestDXGICaptureHardware(t *testing.T) {
	if os.Getenv("PF_GPU_TEST") != "1" {
		t.Skip("PF_GPU_TEST=1 required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	outputs, err := enumerateGPUOutputs()
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) == 0 {
		t.Fatal("no outputs")
	}
	d, err := openDuplication(outputs[0].Device)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	for i := 0; i < 10; i++ {
		f, ok, err := d.acquire(100)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Logf("capture %s %dx%d", outputs[0].Device, f.Width, f.Height)
			if err = d.releaseFrame(); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("no initial frame")
}
