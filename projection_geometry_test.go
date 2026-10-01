package main

import "testing"

func TestGPUSampling(t *testing.T) {
	for _, c := range []struct {
		sw, sh, dw, dh int32
		smooth, want   bool
	}{
		{2560, 1440, 2560, 1440, true, true}, {1920, 1080, 2560, 1440, false, false},
		{1280, 720, 2560, 1440, false, true}, {2560, 1440, 1920, 1080, false, false},
		{0, 0, 1920, 1080, false, false},
	} {
		if got := usePointSampling(c.sw, c.sh, c.dw, c.dh, c.smooth); got != c.want {
			t.Fatalf("%+v got %v", c, got)
		}
	}
}
