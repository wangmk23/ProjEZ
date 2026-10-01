package main

import "testing"

func TestCursorDecode(t *testing.T) {
	p, w, h, e := decodePointer(pointerShapeInfo{Type: 1, Width: 2, Height: 2, Pitch: 1}, []byte{0x80, 0x40})
	if e != nil || w != 2 || h != 1 || p[0] != 255 || p[1] != 0 || p[4] != 0 || p[5] != 255 {
		t.Fatalf("%v %v %v %v", p, w, h, e)
	}
	p, _, _, e = decodePointer(pointerShapeInfo{Type: 2, Width: 1, Height: 1, Pitch: 4}, []byte{1, 2, 3, 255})
	if e != nil || p[0] != 3 || p[2] != 1 {
		t.Fatal(p, e)
	}
	if _, _, _, e = decodePointer(pointerShapeInfo{Type: 1, Width: 64, Height: 4, Pitch: 1}, []byte{0}); e == nil {
		t.Fatal("invalid buffer")
	}
}
