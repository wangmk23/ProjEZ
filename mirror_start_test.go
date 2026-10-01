package main

import "testing"

func TestMirrorPair(t *testing.T) {
	screens := []Monitor{
		{Device: "audience-a", Rect: RECT{1920, 0, 3840, 1080}},
		{Device: "main", Primary: true, Rect: RECT{0, 0, 1920, 1080}},
		{Device: "audience-b", Rect: RECT{-1920, 0, 0, 1080}},
		{Device: "overlap", Rect: RECT{0, 0, 1920, 1080}},
	}
	for _, tc := range []struct {
		preferred string
		target    int
	}{{"audience-b", 2}, {"main", 0}, {"missing", 0}, {"overlap", 0}} {
		a, b := mirrorPair(screens, tc.preferred)
		if a != 1 || b != tc.target {
			t.Fatalf("%s: got %d -> %d", tc.preferred, a, b)
		}
	}
	if _, b := mirrorPair(screens[1:2], ""); b != -1 {
		t.Fatal("single screen must not project onto itself")
	}
	if _, b := mirrorPair(nil, ""); b != -1 {
		t.Fatal("no screen must not start")
	}
}
