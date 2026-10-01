package main

import (
	"testing"
	"time"
)

func TestPreviewRejectsStaleAndUnconfirmedOutput(t *testing.T) {
	f := previewFrame{Generation: 2, Sequence: 4, State: projectionLive, Pixels: make([]byte, previewW*previewH*4), At: time.Now()}
	if !f.matches(2, 4, projectionLive) {
		t.Fatal("current frame rejected")
	}
	if f.matches(1, 4, projectionLive) || f.matches(2, 5, projectionLive) || f.matches(2, 4, projectionBlack) {
		t.Fatal("unsafe preview accepted")
	}
	f.Pixels = nil
	if f.matches(2, 4, projectionLive) {
		t.Fatal("empty image accepted")
	}
}
