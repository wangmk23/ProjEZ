package main

func usePointSampling(sw, sh, dw, dh int32, smooth bool) bool {
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return false
	}
	if sw == dw && sh == dh {
		return true
	}
	return !smooth && dw >= sw && dh >= sh && dw%sw == 0 && dh%sh == 0
}
