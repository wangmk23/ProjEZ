package main

import (
	"fmt"
	"runtime"
	"unsafe"
)

func decodePointer(s pointerShapeInfo, raw []byte) ([]byte, uint32, uint32, error) {
	w, h := s.Width, s.Height
	if s.Type == 1 {
		if h%2 != 0 {
			return nil, 0, 0, fmt.Errorf("invalid monochrome height")
		}
		h /= 2
	}
	if w == 0 || h == 0 || w > 4096 || h > 4096 || s.Pitch == 0 || uint64(s.Pitch)*uint64(s.Height) > uint64(len(raw)) {
		return nil, 0, 0, fmt.Errorf("invalid cursor buffer")
	}
	rowBytes := uint64(w) * 4
	if s.Type == 1 {
		rowBytes = (uint64(w) + 7) / 8
	}
	if uint64(s.Pitch) < rowBytes {
		return nil, 0, 0, fmt.Errorf("invalid cursor pitch")
	}
	if s.Type != 1 && s.Type != 2 && s.Type != 4 {
		return nil, 0, 0, fmt.Errorf("unsupported cursor type %d", s.Type)
	}
	out := make([]byte, int(w)*int(h)*4)
	for y := uint32(0); y < h; y++ {
		for x := uint32(0); x < w; x++ {
			i := (y*w + x) * 4
			if s.Type == 1 {
				mask := byte(0x80 >> (x % 8))
				if raw[y*s.Pitch+x/8]&mask != 0 {
					out[i] = 255
				}
				if raw[(y+h)*s.Pitch+x/8]&mask != 0 {
					out[i+1] = 255
				}
				out[i+3] = 255
			} else {
				j := y*s.Pitch + x*4
				out[i] = raw[j+2]
				out[i+1] = raw[j+1]
				out[i+2] = raw[j]
				out[i+3] = raw[j+3]
			}
		}
	}
	return out, w, h, nil
}
func (r *gpuRenderer) updatePointer(d *desktopDuplication, f desktopFrame) error {
	if f.Info.MouseTime != 0 {
		r.data.Cursor[0] = float32(f.Info.Position.X)
		r.data.Cursor[1] = float32(f.Info.Position.Y)
		r.data.Options[1] = float32(f.Info.Visible)
	}
	if f.Info.ShapeSize != 0 {
		p, w, h, err := decodePointer(d.shape, d.pointer)
		if err != nil {
			return err
		}
		releaseCOM(&r.pointerView)
		releaseCOM(&r.pointer)
		r.pointer, err = makeTexture(d.device, w, h, 28, 8)
		if err != nil {
			return err
		}
		r.pointerView, err = makeView(d.device, r.pointer, 7)
		if err != nil {
			return err
		}
		comCall(d.context, 48, uintptr(unsafe.Pointer(r.pointer)), 0, 0, uintptr(unsafe.Pointer(&p[0])), uintptr(w*4), 0)
		runtime.KeepAlive(p)
		r.data.Source[3] = float32(d.shape.Type)
		r.data.Cursor[2] = float32(w)
		r.data.Cursor[3] = float32(h)
	}
	r.data.Options[0] = 0
	if r.cfg.Cursor {
		r.data.Options[0] = r.data.Options[1]
	}
	return nil
}
