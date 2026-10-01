package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

func saveChromeWindow(hwnd uintptr, path string) error {
	var r RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	screen, _, _ := procGetDC.Call(hwnd)
	defer procReleaseDC.Call(hwnd, screen)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(dc)
	header := struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, ImageSize uint32
		X, Y                   int32
		Used, Important        uint32
	}{Size: 40, Width: r.Right, Height: -r.Bottom, Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bmp, _, _ := gdi32.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		return fmt.Errorf("DIB failed")
	}
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(dc, bmp)
	defer procSelectObject.Call(dc, old)
	procSendMessageW.Call(hwnd, 0x0317, dc, 4|16|8)
	gdi32.NewProc("GdiFlush").Call()
	raw := unsafe.Slice((*byte)(bits), int(r.Right*r.Bottom*4))
	im := image.NewRGBA(image.Rect(0, 0, int(r.Right), int(r.Bottom)))
	for i := 0; i < len(raw); i += 4 {
		im.Pix[i] = raw[i+2]
		im.Pix[i+1] = raw[i+1]
		im.Pix[i+2] = raw[i]
		im.Pix[i+3] = 255
	}
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return png.Encode(f, im)
}

func TestChromeRenderHelper(t *testing.T) {
	if os.Getenv("PROJEZ_CHROME_HELPER") != "1" {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	restoreDPI := testPhysicalCoordinates(t)
	defer restoreDPI()
	hInstance, _, _ = procGetModuleHandleW.Call(0)
	initUIResources()
	appIcon = loadEmbeddedIcon(64)
	registerClasses()
	hwndMain = createMainWindow()
	if hwndMain == 0 {
		t.Fatal("create main")
	}
	defer func() { procDestroyWindow.Call(hwndMain); cleanup() }()
	applyWindowEffects(hwndMain)
	// Native key recording: send keys to the child edits without typing text
	// or sending physical input into the user's desktop.
	currentPage = 2
	layoutControls(hwndMain)
	oldDown := recordingKeyDown
	recordingKeyDown = func(vk uint32) bool { return vk == 0x11 }
	for _, edit := range []uintptr{editFreeze, editResume, editBlack, editStop} {
		before := getText(edit)
		procSendMessageW.Call(edit, 7, 0, 0)
		procSendMessageW.Call(edit, 0x0104, 0x75, 0) // Alt+F6 with Ctrl held.
		if got := getText(edit); got != "Ctrl+Alt+F6" {
			t.Fatal("native key recording failed", got)
		}
		procSendMessageW.Call(edit, 0x0102, 'x', 0)
		if got := getText(edit); got != "Ctrl+Alt+F6" {
			t.Fatal("character appended to recording", got)
		}
		procSendMessageW.Call(edit, 0x0100, 0x1b, 0)
		if got := getText(edit); got != before {
			t.Fatal("escape did not cancel", got, before)
		}
		procSendMessageW.Call(edit, 8, 0, 0)
	}
	recordingKeyDown = oldDown
	currentPage = 0
	layoutControls(hwndMain)
	// True native message/state tests. No mode changes or live projection.
	for i := 0; i < 10; i++ {
		procSendMessageW.Call(hwndMain, 0x0112, 0xf030, 0)
		if !isMaximized(hwndMain) {
			t.Fatal("maximize failed")
		}
		mi, _ := windowMonitor(hwndMain)
		var rect RECT
		procGetClientRect.Call(hwndMain, uintptr(unsafe.Pointer(&rect)))
		if rect.Right > mi.RcWork.Right-mi.RcWork.Left || rect.Bottom > mi.RcWork.Bottom-mi.RcWork.Top {
			t.Fatal("maximized past work area", rect, mi.RcWork)
		}
		procSendMessageW.Call(hwndMain, 0x0112, 0xf120, 0)
		if isMaximized(hwndMain) {
			t.Fatal("restore failed")
		}
	}
	procSendMessageW.Call(hwndMain, 0x0112, 0xf020, 0)
	if v, _, _ := user32.NewProc("IsIconic").Call(hwndMain); v == 0 {
		t.Fatal("minimize failed")
	}
	procSendMessageW.Call(hwndMain, 0x0112, 0xf120, 0)
	procShowWindow.Call(hwndMain, 0)
	loadConfigIntoUI()
	currentPage = 4
	for _, dpi := range []int32{96, 120, 144, 168, 192} {
		windowDPI = dpi
		pageScroll = 0
		procSetWindowPos.Call(hwndMain, 0, 0, 0, uintptr(px(1120)), uintptr(px(820)), SWP_NOZORDER|SWP_NOACTIVATE|2)
		layoutControls(hwndMain)
		// Native windows cannot exceed the runner's work area. Keep the switch
		// visible when high DPI makes the logical viewport shorter than 820.
		pageScroll = max(int32(0), 420-(footerTop()-8))
		layoutControls(hwndMain)
		procShowWindow.Call(hwndMain, 4)
		procUpdateWindow.Call(hwndMain)
		if dir := os.Getenv("PROJEZ_SCREENSHOTS"); dir != "" {
			if err := saveChromeWindow(hwndMain, filepath.Join(dir, fmt.Sprintf("projEZ-%d.png", dpi))); err != nil {
				t.Fatal(err)
			}
		}
		// Verify switch pixels too: GDI+ must not apply the DPI transform twice.
		dc, _, _ := procGetDC.Call(hwndMain)
		paintMainWindow(hwndMain, dc)
		procReleaseDC.Call(hwndMain, dc)
		color, _, _ := gdi32.NewProc("GetPixel").Call(mainBuffer.dc, uintptr(px(logicalWidth-72)), uintptr(px(392-pageScroll)))
		if uint32(color) != rgb(66, 66, 66) {
			t.Fatalf("DPI %d switch missing/misplaced: %x", dpi, color)
		}
		// Child control coordinates use the same scaling as painting and hit testing.
		var edit RECT
		user32.NewProc("GetWindowRect").Call(searchEdit, uintptr(unsafe.Pointer(&edit)))
		user32.NewProc("MapWindowPoints").Call(0, hwndMain, uintptr(unsafe.Pointer(&edit)), 2)
		expected := searchBounds(logicalWidth)
		if edit.Left != px(expected.Left+14) || edit.Top != px(14) {
			t.Fatalf("DPI %d search misplaced %+v", dpi, edit)
		}
	}
	windowDPI = 96
	procSetWindowPos.Call(hwndMain, 0, 0, 0, 1120, 820, SWP_NOZORDER|SWP_NOACTIVATE|2)
	layoutControls(hwndMain)
	procShowWindow.Call(hwndMain, 4)
	dc, _, _ := procGetDC.Call(hwndMain)
	paintMainWindow(hwndMain, dc)
	procReleaseDC.Call(hwndMain, dc)
	pageScroll = 200
	dc, _, _ = procGetDC.Call(hwndMain)
	paintMainWindow(hwndMain, dc)
	procReleaseDC.Call(hwndMain, dc)
	a, _, _ := gdi32.NewProc("GetPixel").Call(mainBuffer.dc, uintptr(px(logicalWidth-72)), uintptr(px(5)))
	b, _, _ := gdi32.NewProc("GetPixel").Call(mainBuffer.dc, uintptr(px(logicalWidth-220)), uintptr(px(5)))
	if a != b {
		t.Fatalf("scrolled switch painted over titlebar: %x vs %x", a, b)
	}
	pageScroll = 0
	dc, _, _ = procGetDC.Call(hwndMain)
	paintMainWindow(hwndMain, dc)
	procReleaseDC.Call(hwndMain, dc)
	updateHover(POINT{logicalWidth - 70, 392})
	if lastHoverID == 0 {
		t.Fatal("toggle hover target missing")
	}
	user32.NewProc("ValidateRect").Call(hwndMain, 0)
	updateHover(POINT{logicalWidth - 69, 393})
	if pending, _, _ := user32.NewProc("GetUpdateRect").Call(hwndMain, 0, 0); pending != 0 {
		t.Fatal("same-button mouse movement still invalidates the window")
	}
	updateHover(POINT{logicalWidth - 320, 550})
	if pending, _, _ := user32.NewProc("GetUpdateRect").Call(hwndMain, 0, 0); pending == 0 {
		t.Fatal("button leave failed to invalidate")
	}
	// Exercise custom mouse routes, including the non-client maximize button.
	pumpCommands := func() {
		var m MSG
		for {
			ok, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&m)), hwndMain, 0x0112, 0x0112, 1)
			if ok == 0 {
				break
			}
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}
	for i := 0; i < 10; i++ {
		r := chromeButtonRect(logicalWidth, 2)
		p := uintptr(uint16(px((r.Left+r.Right)/2))) | uintptr(uint16(px(25)))<<16
		procSendMessageW.Call(hwndMain, 0x00a1, 9, 0)
		procSendMessageW.Call(hwndMain, WM_LBUTTONUP, 0, p)
		pumpCommands()
		if isMaximized(hwndMain) != (i%2 == 0) {
			t.Fatal("custom maximize/restore click failed", i)
		}
	}
	// Narrow/high-DPI viewport: compact sidebar and scrolling keep controls usable.
	procSetWindowPos.Call(hwndMain, 0, 0, 0, 700, 520, SWP_NOZORDER|SWP_NOACTIVATE|2)
	pageScroll = maxPageScroll()
	layoutControls(hwndMain)
	if dir := os.Getenv("PROJEZ_SCREENSHOTS"); dir != "" {
		if e := saveChromeWindow(hwndMain, filepath.Join(dir, "projEZ-compact.png")); e != nil {
			t.Fatal(e)
		}
	}
	// Check every popup in both palettes, including native rounded clipping.
	for _, light := range []bool{false, true} {
		preferences.LightTheme = light
		applyPalette()
		// Open every real dropdown and check its popup stays within the app.
		for _, c := range []struct {
			page int
			hwnd uintptr
		}{{1, sourceWindowCombo}, {1, cbSource}, {1, cbOutput}, {3, scaleCombo}, {3, fpsCombo}, {3, qualityCombo}, {4, accentCombo}} {
			currentPage = c.page
			preferences.WindowMode = c.hwnd == sourceWindowCombo
			pageScroll = 0
			procSetWindowPos.Call(hwndMain, 0, 0, 0, 1120, 820, SWP_NOZORDER|SWP_NOACTIVATE|2)
			layoutControls(hwndMain)
			procSendMessageW.Call(c.hwnd, 0x014f, 1, 0)
			info := comboInfo{Size: uint32(unsafe.Sizeof(comboInfo{}))}
			user32.NewProc("GetComboBoxInfo").Call(c.hwnd, uintptr(unsafe.Pointer(&info)))
			region, _, _ := gdi32.NewProc("CreateRectRgn").Call(0, 0, 0, 0)
			kind, _, _ := user32.NewProc("GetWindowRgn").Call(info.List, region)
			corner, _, _ := gdi32.NewProc("PtInRegion").Call(region, 0, 0)
			procDeleteObject.Call(region)
			if kind != 3 || corner != 0 {
				t.Fatalf("popup has no rounded clipping: kind=%d corner=%d", kind, corner)
			}
			var popup, bounds RECT
			user32.NewProc("GetWindowRect").Call(info.List, uintptr(unsafe.Pointer(&popup)))
			procGetClientRect.Call(hwndMain, uintptr(unsafe.Pointer(&bounds)))
			user32.NewProc("MapWindowPoints").Call(hwndMain, 0, uintptr(unsafe.Pointer(&bounds)), 2)
			if popup.Left < bounds.Left || popup.Right > bounds.Right || popup.Top < bounds.Top || popup.Bottom > bounds.Bottom {
				t.Fatalf("popup escaped: %+v, app %+v", popup, bounds)
			}
			if dir := os.Getenv("PROJEZ_SCREENSHOTS"); dir != "" && c.hwnd == fpsCombo {
				if e := saveChromeWindow(info.List, filepath.Join(dir, fmt.Sprintf("dropdown-fps-light-%t.png", light))); e != nil {
					t.Fatal(e)
				}
			}
			if c.hwnd == fpsCombo {
				procSendMessageW.Call(c.hwnd, 0x0100, 0x23, 0) // VK_END reaches last option.
				last, _, _ := procSendMessageW.Call(c.hwnd, CB_GETCURSEL, 0, 0)
				if last != 5 {
					t.Fatalf("last option not reachable: %d", last)
				}
				procSendMessageW.Call(c.hwnd, 0x0100, 0x24, 0) // VK_HOME
				procSendMessageW.Call(c.hwnd, 0x0100, 0x28, 0) // VK_DOWN
				selected, _, _ := procSendMessageW.Call(c.hwnd, CB_GETCURSEL, 0, 0)
				if selected != 1 {
					t.Fatalf("keyboard dropdown selection failed: %d", selected)
				}
			}
			procSendMessageW.Call(c.hwnd, 0x014f, 0, 0)
		}
	}
	if dir := os.Getenv("PROJEZ_SCREENSHOTS"); dir != "" {
		for _, page := range []int{0, 1, 2, 3, 4, 5, 6} {
			currentPage = page
			pageScroll = 0
			layoutControls(hwndMain)
			name := []string{"home.png", "audience.png", "hotkeys.png", "display.png", "appearance.png", "privacy.png", "about.png"}[page]
			if page == 1 {
				preferences.WindowMode = true
				layoutControls(hwndMain)
				name = "audience.png"
			}
			if page == 5 {
				pageScroll = maxPageScroll()
				layoutControls(hwndMain)
				name = "privacy.png"
			}
			if e := saveChromeWindow(hwndMain, filepath.Join(dir, name)); e != nil {
				t.Fatal(e)
			}
		}

		// Synthetic live statistics: exercise compact and hovered-stop layouts without capturing a desktop.
		currentPage = 0
		pageScroll = 0
		outputRunning = true
		gpuOwnsOutput = true
		gpuLastEvent = projectionEvent{State: projectionLive, PresentFPS: 59.8}
		for _, width := range []int32{1120, 960, 640} {
			windowDPI = 96
			procSetWindowPos.Call(hwndMain, 0, 0, 0, uintptr(width), 820, SWP_NOZORDER|SWP_NOACTIVATE|2)
			layoutControls(hwndMain)
			hoverPoint = POINT{logicalWidth - 85, footerTop() + 45}
			if e := saveChromeWindow(hwndMain, filepath.Join(dir, fmt.Sprintf("fps-synthetic-%d.png", width))); e != nil {
				t.Fatal(e)
			}
		}
		outputRunning = false
		gpuOwnsOutput = false
		gpuLastEvent = projectionEvent{}
		hoverPoint = POINT{-1, -1}
		procSetWindowPos.Call(hwndMain, 0, 0, 0, 1120, 820, SWP_NOZORDER|SWP_NOACTIVATE|2)
		currentPage = 4
		pageScroll = 0
		preferences.LightTheme = true
		applyPalette()
		layoutControls(hwndMain)
		if e := saveChromeWindow(hwndMain, filepath.Join(dir, "appearance-light.png")); e != nil {
			t.Fatal(e)
		}
		preferences.LightTheme = false
		applyPalette()
		currentPage = 2
		pageScroll = 0
		layoutControls(hwndMain)
		if e := saveChromeWindow(hwndMain, filepath.Join(dir, "hotkeys.png")); e != nil {
			t.Fatal(e)
		}
	}
}

func TestChromeNativeLifecycleAndDPI(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, "-test.run=^TestChromeRenderHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "PROJEZ_CHROME_HELPER=1", "APPDATA="+t.TempDir())
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("chrome helper: %v\n%s", e, out)
	}
	t.Log(string(out))
}
