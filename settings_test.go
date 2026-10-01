//go:build windows

package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestPreferencePersistence(t *testing.T) {
	want := Preferences{FPS: 30, Fill: true, Transparent: true, Accent: 2, Smooth: true, WindowMode: true, LightTheme: true, FreeCursor: true}
	if got := parsePreferences(strings.Join(preferenceLines(want), "\r\n")); got != want {
		t.Fatalf("saved settings changed on reload: got %+v want %+v", got, want)
	}
	for _, input := range []string{"", "FPS=0\nAccent=99", "FPS=-2\nFill=invalid\nTransparent=no", "FPS=1000\nAccent=-1"} {
		if got := parsePreferences(input); got != (Preferences{FPS: 0}) {
			t.Fatalf("invalid/legacy config %q did not use safe defaults: %+v", input, got)
		}
	}
}

func TestAutomaticRefreshPreferences(t *testing.T) {
	if got := parsePreferences("FPS=0"); got.FPS != 0 {
		t.Fatalf("automatic refresh was replaced by a fixed cap: %d", got.FPS)
	}
	if got := parsePreferences("RefreshPolicy=2\nFPS=60"); got.FPS != 60 {
		t.Fatalf("60 FPS preference was not preserved: %d", got.FPS)
	}
}

func TestUpgradeUsesAutomaticRefresh(t *testing.T) {
	if got := parsePreferences("FPS=30\nAccent=2\nFill=true"); got.FPS != 0 || got.Accent != 2 || !got.Fill {
		t.Fatalf("upgrade must remove the old 30 FPS limit while retaining other choices: %+v", got)
	}
}

func TestSearchFindsSettings(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"帧率", []int{3}}, {"透明", []int{4}}, {"配置", []int{5}},
		{"F8", []int{2}}, {"  快捷键  ", []int{2}},
		{"鼠标 fps", []int{3}}, {"不存在的功能", nil}, {"", nil},
	} {
		if got := searchPages(tc.query); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("search %q: got %v want %v", tc.query, got, tc.want)
		}
	}
}

func TestProjectionScaling(t *testing.T) {
	for _, tc := range []struct {
		sw, sh, dw, dh int32
		fill           bool
		w, h           int32
	}{
		{1920, 1080, 1024, 768, false, 1024, 576},
		{1920, 1080, 1024, 768, true, 1024, 768},
		{1080, 1920, 1920, 1080, false, 607, 1080},
		{0, 1080, 1024, 768, true, 0, 0},
	} {
		w, h := outputSize(tc.sw, tc.sh, tc.dw, tc.dh, tc.fill)
		if w != tc.w || h != tc.h {
			t.Errorf("scaling %+v returned %dx%d", tc, w, h)
		}
	}
}
