package main

import "unsafe"

func (r *gpuRenderer) presentBlack() (bool, error) {
	color := [4]float32{0, 0, 0, 1}
	comCall(r.capture.context, 50, uintptr(unsafe.Pointer(r.compositeView)), uintptr(unsafe.Pointer(&color[0])))
	if r.swap != nil {
		comCall(r.capture.context, 47, uintptr(unsafe.Pointer(r.back)), uintptr(unsafe.Pointer(r.composite)))
		hr := comCall(r.swap, 8, 1, 8)
		if uint32(hr) == 0x887a000a || uint32(hr) == 0x087a0001 {
			return false, nil
		}
		if err := hrError("Present black", hr); err != nil {
			return false, err
		}
	}
	comCall(r.capture.context, 47, uintptr(unsafe.Pointer(r.last)), uintptr(unsafe.Pointer(r.composite)))
	r.lastValid = true
	return true, nil
}
