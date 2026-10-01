//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Exercise the real entry point in its own process, without touching user settings.
func TestUIHelper(t *testing.T) {
	if os.Getenv("PROJECTOR_FREEZER_UI_TEST") != "1" {
		return
	}
	go func() {
		for {
			runtime.GC()
			time.Sleep(time.Millisecond)
		}
	}()
	main()
	os.Exit(0)
}

func TestWindowRemainsResponsive(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	restoreDPI := testPhysicalCoordinates(t)
	defer restoreDPI()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestUIHelper$")
	if release := os.Getenv("PROJECTOR_FREEZER_TEST_EXE"); release != "" {
		cmd = exec.Command(release)
	}
	configDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDir, "ProjectorFreezer"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "ProjectorFreezer", "config.ini"), []byte("RefreshPolicy=2\nFPS=10\nFill=true\nAccent=2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd.Env = append(os.Environ(), "PROJECTOR_FREEZER_UI_TEST=1", "APPDATA="+configDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		_ = cmd.Process.Kill()
	}()
	enumWindows := user32.NewProc("EnumWindows")
	getPID := user32.NewProc("GetWindowThreadProcessId")
	getClass := user32.NewProc("GetClassNameW")
	sendTimeout := user32.NewProc("SendMessageTimeoutW")
	var window uintptr
	callback := syscall.NewCallback(func(hwnd, param uintptr) uintptr {
		var pid uint32
		getPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == uint32(cmd.Process.Pid) {
			var name [256]uint16
			getClass.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
			if syscall.UTF16ToString(name[:]) == "ProjectorFreezerMain" {
				window = hwnd
				return 0
			}
		}
		return 1
	})
	deadline := time.Now().Add(10 * time.Second)
	for window == 0 && time.Now().Before(deadline) {
		enumWindows.Call(callback, 0)
		time.Sleep(50 * time.Millisecond)
	}
	if window == 0 {
		t.Fatal("main window was not created within 10 seconds")
	}
	// All input coordinates are in DIPs; the app now supports per-monitor DPI.
	dpi := queryWindowDPI(window)
	pack := func(x, y int32) uintptr {
		return uintptr(uint16(scaleAt(x, dpi))) | uintptr(uint16(scaleAt(y, dpi)))<<16
	}
	// Give startup time to complete, then require the owning thread to service messages.
	time.Sleep(500 * time.Millisecond)
	// Clicking the Shortcuts navigation item must actually switch pages.
	var navResult uintptr
	ok, _, navErr := sendTimeout.Call(window, WM_LBUTTONUP, 0, pack(100, 176), 2, 1000, uintptr(unsafe.Pointer(&navResult)))
	if ok == 0 {
		t.Fatalf("navigation message timed out: %v", navErr)
	}
	source, _, _ := user32.NewProc("GetDlgItem").Call(window, IDC_SOURCE)
	if source == 0 {
		t.Fatal("source selector was not created")
	}
	style, _, _ := user32.NewProc("GetWindowLongW").Call(source, ^uintptr(15)) // GWL_STYLE = -16
	if style&WS_VISIBLE != 0 {
		t.Fatal("Shortcuts navigation did not hide projection controls")
	}
	send := func(hwnd uintptr, msg uint32, wp, lp uintptr) {
		t.Helper()
		var result uintptr
		ok, _, err := sendTimeout.Call(hwnd, uintptr(msg), wp, lp, 2, 2000, uintptr(unsafe.Pointer(&result)))
		if ok == 0 {
			t.Fatalf("message %x timed out: %v", msg, err)
		}
	}
	control := func(id int) uintptr {
		h, _, _ := user32.NewProc("GetDlgItem").Call(window, uintptr(id))
		if h == 0 {
			t.Fatalf("missing control %d", id)
		}
		return h
	}
	checkVisible := func(id int, want bool) {
		t.Helper()
		s, _, _ := user32.NewProc("GetWindowLongW").Call(control(id), ^uintptr(15))
		if (s&WS_VISIBLE != 0) != want {
			t.Fatalf("control %d visibility: want %t", id, want)
		}
	}
	for id, want := range map[int]uintptr{idFPS: 2, idScale: 1, idAccent: 2} {
		var got uintptr
		ok, _, _ := sendTimeout.Call(control(id), CB_GETCURSEL, 0, 0, 2, 2000, uintptr(unsafe.Pointer(&got)))
		if ok == 0 || got != want {
			t.Fatalf("saved control %d: got %d want %d", id, got, want)
		}
	}
	for page := 0; page < 7; page++ {
		send(window, WM_LBUTTONUP, 0, pack(100, 84+46*int32(page)))
		checkVisible(IDC_SOURCE, page == 1)
		checkVisible(IDC_FREEZE_HOTKEY, page == 2)
		checkVisible(IDC_BLACK_HOTKEY, page == 2)
		checkVisible(IDC_STOP_HOTKEY, page == 2)
		checkVisible(idScale, page == 3)
		checkVisible(idQuality, page == 3)
		checkVisible(idAccent, page == 4)
	}
	query := syscall.StringToUTF16("帧率")
	send(control(idSearch), 0x000C, 0, uintptr(unsafe.Pointer(&query[0]))) // WM_SETTEXT
	checkVisible(idFPS, true)
	fps := control(idFPS)
	send(fps, CB_SETCURSEL, 4, 0)
	send(window, WM_COMMAND, uintptr(idFPS|(CBN_SELCHANGE<<16)), fps)
	scale := control(idScale)
	send(scale, CB_SETCURSEL, 0, 0)
	send(window, WM_COMMAND, uintptr(idScale|(CBN_SELCHANGE<<16)), scale)
	data, err := os.ReadFile(filepath.Join(configDir, "ProjectorFreezer", "config.ini"))
	if err != nil {
		t.Fatal(err)
	}
	prefs := parsePreferences(string(data))
	if prefs.FPS != 30 || prefs.Fill {
		t.Fatalf("UI settings did not persist: %+v", prefs)
	}
	// Source mode uses a real window selector, not the auxiliary privacy binding.
	send(window, WM_LBUTTONUP, 0, pack(100, 84+46))
	send(window, WM_PAINT, 0, 0)
	send(window, WM_LBUTTONUP, 0, pack(490, 132))
	checkVisible(idSourceWindow, true)
	checkVisible(IDC_SOURCE, false)
	send(window, WM_LBUTTONUP, 0, pack(330, 132))
	checkVisible(idSourceWindow, false)
	checkVisible(IDC_SOURCE, true)
	// Appearance switches must affect the real window and persist to disk.
	send(window, WM_LBUTTONUP, 0, pack(100, 84+46*4))
	send(window, WM_PAINT, 0, 0)
	var client RECT
	procGetClientRect.Call(window, uintptr(unsafe.Pointer(&client)))
	send(window, WM_LBUTTONUP, 0, pack(client.Right*96/dpi-60, 392))
	var key, flags uint32
	var alpha byte
	ok, _, _ = user32.NewProc("GetLayeredWindowAttributes").Call(window, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&alpha)), uintptr(unsafe.Pointer(&flags)))
	if ok == 0 || alpha != 255 {
		t.Fatalf("transparency toggle did not apply: alpha=%d", alpha)
	}
	send(window, WM_LBUTTONUP, 0, pack(client.Right*96/dpi-80, 194))
	accent := control(idAccent)
	send(accent, CB_SETCURSEL, 1, 0)
	send(window, WM_COMMAND, uintptr(idAccent|(CBN_SELCHANGE<<16)), accent)
	data, err = os.ReadFile(filepath.Join(configDir, "ProjectorFreezer", "config.ini"))
	if err != nil {
		t.Fatal(err)
	}
	prefs = parsePreferences(string(data))
	if !prefs.Transparent || prefs.Accent != 1 || !prefs.LightTheme {
		t.Fatalf("appearance settings did not persist: %+v", prefs)
	}
	// A sidebar click must commit the edit and take focus away before hiding it.
	procShowWindow.Call(window, SW_SHOWNOACTIVATE)
	send(window, WM_LBUTTONUP, 0, pack(100, 84+46*2))
	edit := control(IDC_FREEZE_HOTKEY)
	send(edit, 0x0201, 1, uintptr(10|(10<<16))) // WM_LBUTTONDOWN: focus the native edit
	send(edit, WM_LBUTTONUP, 0, uintptr(10|(10<<16)))
	focusedControl := func() uintptr {
		info := struct {
			Size, Flags                                        uint32
			Active, Focus, Capture, MenuOwner, MoveSize, Caret uintptr
			CaretRect                                          RECT
		}{}
		info.Size = uint32(unsafe.Sizeof(info))
		tid, _, _ := getPID.Call(window, 0)
		ok, _, err := user32.NewProc("GetGUIThreadInfo").Call(tid, uintptr(unsafe.Pointer(&info)))
		if ok == 0 {
			t.Fatalf("query focus: %v", err)
		}
		return info.Focus
	}
	if focusedControl() != edit {
		t.Fatal("could not focus hotkey edit for navigation test")
	}
	changedHotkey := syscall.StringToUTF16("Ctrl+Shift+F10")
	send(edit, 0x000C, 0, uintptr(unsafe.Pointer(&changedHotkey[0])))
	send(window, WM_LBUTTONUP, 0, pack(100, 84+46*3))
	if focusedControl() == edit {
		t.Fatal("hidden hotkey edit retained keyboard focus")
	}
	data, err = os.ReadFile(filepath.Join(configDir, "ProjectorFreezer", "config.ini"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"BlackHotkey=Ctrl+Alt+F10", "StopHotkey=Ctrl+Alt+F12"} {
		if !strings.Contains(string(data), line) {
			t.Fatal("new shortcut defaults not persisted", line)
		}
	}
	if !strings.Contains(string(data), "FreezeHotkey=Ctrl+Shift+F10") {
		t.Fatal("navigating away did not save the edited hotkey")
	}
	procShowWindow.Call(window, 0)
	for i := 0; i < 30; i++ {
		var result uintptr
		ok, _, err := sendTimeout.Call(window, 0, 0, 0, 2, 1000, uintptr(unsafe.Pointer(&result)))
		if ok == 0 {
			t.Fatalf("main window stopped responding at probe %d: %v", i, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	user32.NewProc("PostMessageW").Call(window, WM_CLOSE, 0, 0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("window process exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("window did not close within 5 seconds")
	}
}

// Match the application DPI context when sending scaled coordinates. Without
// this, Windows virtualizes calls from the test process and scales them twice.
// The caller must stay on the same OS thread until the returned restore runs.
func testPhysicalCoordinates(t *testing.T) func() {
	t.Helper()
	proc := user32.NewProc("SetThreadDpiAwarenessContext")
	if err := proc.Find(); err != nil {
		t.Fatal(err)
	}
	previous, _, err := proc.Call(^uintptr(3))
	if previous == 0 {
		t.Fatalf("set test DPI awareness: %v", err)
	}
	return func() { proc.Call(previous) }
}
