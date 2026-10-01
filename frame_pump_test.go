//go:build windows

package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestFramePumpCoalescesBusyFramesAndStops(t *testing.T) {
	posted := make(chan uintptr, 10)
	p := newFramePump(120, 42, func(g uintptr) bool { posted <- g; return true })
	select {
	case g := <-posted:
		if g != 42 {
			t.Fatal("wrong generation")
		}
	case <-time.After(time.Second):
		t.Fatal("frame not scheduled")
	}
	select {
	case <-posted:
		t.Fatal("queued a duplicate while UI was busy")
	case <-time.After(50 * time.Millisecond):
	}
	p.pending.Store(false)
	select {
	case <-posted:
	case <-time.After(time.Second):
		t.Fatal("did not resume after UI caught up")
	}
	close(p.stop)
	select {
	case <-p.finished:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
}

func TestFramePumpRetriesFailedPosts(t *testing.T) {
	var calls atomic.Int32
	ok := make(chan struct{}, 1)
	p := newFramePump(60, 7, func(uintptr) bool {
		if calls.Add(1) == 1 {
			return false
		}
		ok <- struct{}{}
		return true
	})
	defer close(p.stop)
	select {
	case <-ok:
	case <-time.After(time.Second):
		t.Fatal("failed PostMessage permanently stopped frames")
	}
}

func TestOldFrameNotificationCannotReleaseNewPendingFrame(t *testing.T) {
	p := &framePump{generation: 9}
	p.pending.Store(true)
	livePump = p
	defer func() { livePump = nil }()
	handleFrameTick(8)
	if !p.pending.Load() {
		t.Fatal("stale frame changed the new scheduler")
	}
}
