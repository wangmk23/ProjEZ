package main

import "testing"

func TestDropdownContainedAtEveryDPI(t *testing.T) {
	for _, dpi := range []int32{96, 120, 144, 168, 192} {
		for _, y := range []int32{80, 300, 440} {
			bounds := RECT{0, 0, scaleAt(700, dpi), scaleAt(520, dpi)}
			anchor := RECT{scaleAt(302, dpi), scaleAt(y, dpi), scaleAt(652, dpi), scaleAt(y+32, dpi)}
			r := dropdownBounds(anchor, bounds, scaleAt(220, dpi), dpi)
			if r.Left < bounds.Left || r.Right > bounds.Right || r.Top < scaleAt(50, dpi) || r.Bottom > bounds.Bottom || r.Right-r.Left != anchor.Right-anchor.Left || r.Bottom <= r.Top {
				t.Fatalf("dpi %d: %+v", dpi, r)
			}
		}
	}
}
