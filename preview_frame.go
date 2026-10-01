package main

import "time"

const previewW, previewH = 640, 360

type previewFrame struct {
	Generation, Sequence uint64
	State                projectionState
	Pixels               []byte
	At                   time.Time
}

func (f previewFrame) matches(g, s uint64, state projectionState) bool {
	return f.Generation == g && f.Sequence >= s && f.State == state && len(f.Pixels) == previewW*previewH*4
}

type bitmapHeader struct {
	Size                   uint32
	Width, Height          int32
	Planes, Bits           uint16
	Compression, ImageSize uint32
	X, Y                   int32
	Used, Important        uint32
}

func previewHeader() bitmapHeader {
	return bitmapHeader{Size: 40, Width: previewW, Height: -previewH, Planes: 1, Bits: 32}
}

func previewRate(p Preferences) int {
	if p.PreviewEconomy {
		return 5
	}
	return 30
}
