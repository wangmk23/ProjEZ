package main

import "testing"

func TestChromeHitTargetsAtAllDPI(t *testing.T) {
	for _, dpi := range []int32{96, 120, 144, 168, 192} {
		width, height := scaleAt(1120, dpi), scaleAt(820, dpi)
		tests := []struct {
			p   POINT
			hit uintptr
		}{
			{POINT{0, 0}, 13}, {POINT{width - 1, 0}, 14}, {POINT{0, height - 1}, 16}, {POINT{width - 1, height - 1}, 17},
			{POINT{0, height / 2}, 10}, {POINT{width - 1, height / 2}, 11}, {POINT{width / 2, 0}, 12}, {POINT{width / 2, height - 1}, 15},
			{POINT{scaleAt(180, dpi), scaleAt(25, dpi)}, 2},
			{POINT{scaleAt(30, dpi), scaleAt(25, dpi)}, 1},
			{POINT{scaleAt(300, dpi), scaleAt(25, dpi)}, 1},
			{POINT{width - scaleAt(69, dpi), scaleAt(25, dpi)}, 9},
			{POINT{width - scaleAt(23, dpi), scaleAt(25, dpi)}, 1},
		}
		for _, c := range tests {
			if got := chromeHit(c.p, width, height, dpi, false); got != c.hit {
				t.Fatalf("dpi=%d point=%+v got=%d want=%d", dpi, c.p, got, c.hit)
			}
		}
		if got := chromeHit(POINT{0, height / 2}, width, height, dpi, true); got != 1 {
			t.Fatal("maximized window offers resize")
		}
	}
}
func TestWorkAreaMaximizeNegativeMonitor(t *testing.T) {
	m := RECT{-2560, -200, 0, 1240}
	w := RECT{-2560, -160, 0, 1200}
	pos, size := maximizeBounds(m, w)
	if pos != (POINT{0, 40}) || size != (POINT{2560, 1360}) {
		t.Fatal(pos, size)
	}
}
func TestHoverRegionStableWithinButton(t *testing.T) {
	old := actions
	defer func() { actions = old }()
	actions = []uiAction{{rect: RECT{300, 300, 400, 350}, enabled: true}}
	a, ar := hoverRegion(POINT{305, 305})
	b, br := hoverRegion(POINT{306, 306})
	if a == 0 || a != b || ar != br {
		t.Fatal("pixel motion changes hover target")
	}
	if c, _ := hoverRegion(POINT{500, 350}); c == a {
		t.Fatal("leave not detected")
	}
}
