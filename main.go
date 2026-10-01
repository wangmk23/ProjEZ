//go:build windows

package main

import (
	"bufio"
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//go:embed assets/logo.ico
var logoICO []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	uxtheme  = syscall.NewLazyDLL("uxtheme.dll")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procSetWindowTextW       = user32.NewProc("SetWindowTextW")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procMessageBoxW          = user32.NewProc("MessageBoxW")
	procSetTimer             = user32.NewProc("SetTimer")
	procKillTimer            = user32.NewProc("KillTimer")
	procInvalidateRect       = user32.NewProc("InvalidateRect")
	procBeginPaint           = user32.NewProc("BeginPaint")
	procEndPaint             = user32.NewProc("EndPaint")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procGetDC                = user32.NewProc("GetDC")
	procReleaseDC            = user32.NewProc("ReleaseDC")
	procSetWindowPos         = user32.NewProc("SetWindowPos")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procRegisterHotKey       = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey     = user32.NewProc("UnregisterHotKey")
	procEnumDisplayMonitors  = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW      = user32.NewProc("GetMonitorInfoW")
	procGetCursorInfo        = user32.NewProc("GetCursorInfo")
	procDrawIconEx           = user32.NewProc("DrawIconEx")
	procSetProcessDPIAware   = user32.NewProc("SetProcessDPIAware")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procLoadIconW            = user32.NewProc("LoadIconW")
	procGetSysColorBrush     = user32.NewProc("GetSysColorBrush")
	procDrawTextW            = user32.NewProc("DrawTextW")
	procFillRect             = user32.NewProc("FillRect")
	procCreateIconFromResEx  = user32.NewProc("CreateIconFromResourceEx")
	procSetLayeredWindowAttr = user32.NewProc("SetLayeredWindowAttributes")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procStretchBlt             = gdi32.NewProc("StretchBlt")
	procSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procRoundRect              = gdi32.NewProc("RoundRect")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procSetBkColor             = gdi32.NewProc("SetBkColor")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procGetStockObject         = gdi32.NewProc("GetStockObject")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procSetWindowTheme        = uxtheme.NewProc("SetWindowTheme")
)

const (
	WS_OVERLAPPED  = 0x00000000
	WS_CAPTION     = 0x00C00000
	WS_SYSMENU     = 0x00080000
	WS_MINIMIZEBOX = 0x00020000
	WS_VISIBLE     = 0x10000000
	WS_CHILD       = 0x40000000
	WS_TABSTOP     = 0x00010000
	WS_BORDER      = 0x00800000
	WS_POPUP       = 0x80000000

	WS_EX_TOPMOST    = 0x00000008
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_NOACTIVATE = 0x08000000
	WS_EX_LAYERED    = 0x00080000

	SW_SHOW           = 5
	SW_SHOWNOACTIVATE = 4

	WM_CREATE          = 0x0001
	WM_DESTROY         = 0x0002
	WM_SIZE            = 0x0005
	WM_CLOSE           = 0x0010
	WM_ERASEBKGND      = 0x0014
	WM_SETFONT         = 0x0030
	WM_COMMAND         = 0x0111
	WM_TIMER           = 0x0113
	WM_PAINT           = 0x000F
	WM_CTLCOLOREDIT    = 0x0133
	WM_CTLCOLORLISTBOX = 0x0134
	WM_LBUTTONUP       = 0x0202
	WM_HOTKEY          = 0x0312

	CB_ADDSTRING    = 0x0143
	CB_GETCURSEL    = 0x0147
	CB_SETCURSEL    = 0x014E
	CB_RESETCONTENT = 0x014B

	CBS_DROPDOWNLIST = 0x0003
	ES_AUTOHSCROLL   = 0x0080

	MB_OK              = 0x00000000
	MB_ICONINFORMATION = 0x00000040
	MB_ICONWARNING     = 0x00000030
	MB_ICONERROR       = 0x00000010

	MOD_ALT      = 0x0001
	MOD_CONTROL  = 0x0002
	MOD_SHIFT    = 0x0004
	MOD_NOREPEAT = 0x4000

	HOTKEY_FREEZE = 1001
	HOTKEY_RESUME = 1002

	MONITORINFOF_PRIMARY = 0x00000001

	SRCCOPY   = 0x00CC0020
	HALFTONE  = 4
	BLACKNESS = 0x00000042

	CURSOR_SHOWING = 0x00000001
	DI_NORMAL      = 0x0003

	IDC_ARROW       = 32512
	IDI_APPLICATION = 32512
	COLOR_WINDOW    = 5

	HWND_TOPMOST   = ^uintptr(0)
	SWP_NOACTIVATE = 0x0010
	SWP_SHOWWINDOW = 0x0040
	SWP_NOZORDER   = 0x0004

	IDC_SOURCE        = 101
	IDC_OUTPUT        = 102
	IDC_FREEZE_HOTKEY = 105
	IDC_RESUME_HOTKEY = 106
	IDC_BLACK_HOTKEY  = 2601
	IDC_STOP_HOTKEY   = 2602

	TIMER_LIVE = 1

	EN_KILLFOCUS  = 0x0200
	CBN_SELCHANGE = 1

	TRANSPARENT = 1
	PS_SOLID    = 0
	NULL_PEN    = 8

	DT_LEFT         = 0x0000
	DT_CENTER       = 0x0001
	DT_RIGHT        = 0x0002
	DT_VCENTER      = 0x0004
	DT_SINGLELINE   = 0x0020
	DT_END_ELLIPSIS = 0x8000
	DT_NOPREFIX     = 0x0800

	LWA_ALPHA = 0x00000002

	DWMWA_USE_IMMERSIVE_DARK_MODE  = 20
	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWA_SYSTEMBACKDROP_TYPE      = 38
	DWMWCP_ROUND                   = 2
	DWMSBT_TRANSIENTWINDOW         = 3
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}
type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}
type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}
type MONITORINFOEX struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
	SzDevice  [32]uint16
}
type CURSORINFO struct {
	CbSize      uint32
	Flags       uint32
	HCursor     uintptr
	PtScreenPos POINT
}
type Monitor struct {
	Name    string
	Handle  uintptr
	Rect    RECT
	Device  string
	Primary bool
}
type Hotkey struct {
	Mods uint32
	VK   uint32
	Text string
}
type Config struct {
	SourceDevice string
	OutputDevice string
	FreezeHotkey string
	ResumeHotkey string
	BlackHotkey  string
	StopHotkey   string
	ShowCursor   bool
}

var (
	hInstance  uintptr
	hwndMain   uintptr
	hwndOutput uintptr
	appIcon    uintptr

	cbSource   uintptr
	cbOutput   uintptr
	editFreeze uintptr
	editResume uintptr
	editBlack  uintptr
	editStop   uintptr

	monitors       []Monitor
	outputRunning  bool
	frozen         bool
	showCursor     = true
	frameBitmap    uintptr
	frameDC        uintptr
	frameW, frameH int32
	srcMonitor     Monitor
	dstMonitor     Monitor

	currentFreeze Hotkey
	currentResume Hotkey

	statusText = "尚未开始投影"
	statusKind = 0 // 0 idle, 1 live, 2 frozen, 3 warning

	brushBG      uintptr
	brushSidebar uintptr
	brushCard    uintptr
	brushControl uintptr
	brushActive  uintptr
	brushAccent  uintptr
	brushGreen   uintptr
	brushMuted   uintptr
	penBorder    uintptr
	penAccent    uintptr

	fontBrand    uintptr
	fontTitle    uintptr
	fontSubtitle uintptr
	fontRow      uintptr
	fontDesc     uintptr
	fontNav      uintptr
	fontButton   uintptr
	fontSmall    uintptr
	fontIcon     uintptr

	rStart     RECT
	rFreeze    RECT
	rResume    RECT
	rToggle    RECT
	rRefresh   RECT
	rStop      RECT
	brushWhite uintptr
)

func main() {
	// Win32 windows and their message queues belong to the creating OS thread.
	// Keep creation, dispatch, and cleanup on that thread even across Go GC/syscalls.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if p := user32.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		if ok, _, _ := p.Call(^uintptr(3)); ok == 0 {
			procSetProcessDPIAware.Call()
		}
	} else {
		procSetProcessDPIAware.Call()
	}
	hInstance, _, _ = procGetModuleHandleW.Call(0)
	appIcon = loadEmbeddedIcon(64)
	initUIResources()
	registerClasses()

	hwndMain = createMainWindow()
	if hwndMain == 0 {
		messageBox(0, "程序窗口创建失败。", "ProjEZ", MB_ICONERROR)
		return
	}
	preserveTaskbarControls(hwndMain)
	applyWindowEffects(hwndMain)
	refreshMonitors()
	loadConfigIntoUI()
	applyHotkeys(false)

	procShowWindow.Call(hwndMain, SW_SHOW)
	procUpdateWindow.Call(hwndMain)

	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	cleanup()
}

func initUIResources() {
	brushBG = makeBrush(17, 20, 26)
	brushSidebar = makeBrush(12, 15, 20)
	brushCard = makeBrush(27, 32, 40)
	brushControl = makeBrush(34, 40, 50)
	brushActive = makeBrush(29, 49, 78)
	brushAccent = makeBrush(45, 128, 237)
	brushGreen = makeBrush(55, 210, 122)
	brushMuted = makeBrush(42, 49, 59)
	brushWhite = makeBrush(242, 246, 252)
	penBorder = makePen(43, 51, 63)
	penAccent = makePen(55, 145, 255)

	fontBrand = createFont(15, 600, "Segoe UI")
	fontTitle = createFont(22, 600, "Microsoft YaHei UI")
	fontSubtitle = createFont(14, 400, "Microsoft YaHei UI")
	fontRow = createFont(14, 400, "Microsoft YaHei UI")
	fontDesc = createFont(13, 400, "Microsoft YaHei UI")
	fontNav = createFont(14, 400, "Microsoft YaHei UI")
	fontButton = createFont(14, 400, "Microsoft YaHei UI")
	fontSmall = createFont(12, 400, "Microsoft YaHei UI")
	fontIcon = createFont(16, 400, "Segoe UI")
	fontNavIcon = createFont(16, 400, "Segoe MDL2 Assets")
	applyPalette()
}

func registerClasses() {
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	icon := appIcon
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, IDI_APPLICATION)
	}

	mainClass := utf16Ptr("ProjectorFreezerMain")
	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(mainWndProc),
		HInstance:     hInstance,
		HIcon:         icon,
		HCursor:       cursor,
		HbrBackground: brushBG,
		LpszClassName: mainClass,
		HIconSm:       icon,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	outClass := utf16Ptr("ProjectorFreezerOutput")
	blackBrush, _, _ := procCreateSolidBrush.Call(0)
	wc2 := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(outputWndProc),
		HInstance:     hInstance,
		HIcon:         icon,
		HCursor:       cursor,
		HbrBackground: blackBrush,
		LpszClassName: outClass,
		HIconSm:       icon,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc2)))
}

func createMainWindow() uintptr {
	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX | 0x00010000 | 0x00040000 | 0x02000000)
	if p := user32.NewProc("GetDpiForSystem"); p.Find() == nil {
		if d, _, _ := p.Call(); d > 0 {
			windowDPI = int32(d)
		}
	}
	w, h := px(1120), px(820)
	x, y := px(100), px(60)
	if mi, ok := windowMonitor(0); ok {
		w = min32(w, mi.RcWork.Right-mi.RcWork.Left-px(24))
		h = min32(h, mi.RcWork.Bottom-mi.RcWork.Top-px(24))
		x = mi.RcWork.Left + (mi.RcWork.Right-mi.RcWork.Left-w)/2
		y = mi.RcWork.Top + (mi.RcWork.Bottom-mi.RcWork.Top-h)/2
	}
	hwnd, _, _ := procCreateWindowExW.Call(
		WS_EX_LAYERED,
		uintptr(unsafe.Pointer(utf16Ptr("ProjectorFreezerMain"))),
		uintptr(unsafe.Pointer(utf16Ptr("ProjEZ"))),
		style,
		uintptr(int64(x)), uintptr(int64(y)), uintptr(w), uintptr(h),
		0, 0, hInstance, 0,
	)
	return hwnd
}

func applyWindowEffects(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	dark := int32(1)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	corner := int32(DWMWCP_ROUND)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_WINDOW_CORNER_PREFERENCE, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
	backdrop := int32(2)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_SYSTEMBACKDROP_TYPE, uintptr(unsafe.Pointer(&backdrop)), unsafe.Sizeof(backdrop))
	// A very light transparency keeps the DeskBox-like glass feeling without hurting readability.
	applyOpacity(hwnd)
}

func mainWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if result, handled := chromeMessage(hwnd, msg, wParam, lParam); handled {
		return result
	}
	switch msg {
	case wmPrivacyForeground:
		checkPrivacyForeground(wParam)
		return 0
	case 0x0113:
		if wParam == privacyTimer {
			pollPrivacy()
			return 0
		}
		if wParam == previewTimer {
			maintainCursorControl()
			pollAudiencePreview()
			return 0
		}
		if wParam == dockTimer {
			invalidateLogical(RECT{0, footerTop(), logicalWidth, logicalHeight})
			return 0
		}
		if wParam == gpuPollTimer {
			pollGPUEvents()
			return 0
		}
	case 0x007E: // WM_DISPLAYCHANGE: defer enumeration until mode switching returns.
		queueDisplayRefresh()
		return 0
	case wmRefreshDisplays:
		handleDisplayRefresh()
		return 0
	case 0x002b:
		if drawComboItem(lParam) {
			return 1
		}
	case WM_CREATE:
		procSetTimer.Call(hwnd, previewTimer, 33, 0)
		windowDPI = queryWindowDPI(hwnd)
		buildUI(hwnd)
		return 0
	case WM_SIZE:
		layoutControls(hwnd)
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_COMMAND:
		id := int(wParam & 0xFFFF)
		notify := int((wParam >> 16) & 0xFFFF)
		if (notify == 7 || notify == 8) && lParam != 0 {
			procInvalidateRect.Call(lParam, 0, 0)
		}
		if isHotkeyControl(id) && (notify == 0x0100 || notify == EN_KILLFOCUS) {
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		if handleSettingsCommand(id, notify) {
			return 0
		}
		if isHotkeyControl(id) && notify == EN_KILLFOCUS {
			applyHotkeys(false)
			saveConfig()
			procInvalidateRect.Call(hwnd, 0, 0)
			return 0
		}
		if (id == IDC_SOURCE || id == IDC_OUTPUT || id == idSourceWindow) && notify == CBN_SELCHANGE {
			saveConfig()
			procInvalidateRect.Call(hwnd, 0, 0)
			return 0
		}
		return 0
	case 0x0201:
		p := logicalPoint(lParam)
		for i := 1; i <= 3; i++ {
			if ptInRect(p, chromeButtonRect(logicalWidth, i)) {
				chromePressed = i
				user32.NewProc("SetCapture").Call(hwnd)
				return 0
			}
		}
	case WM_LBUTTONUP:
		p := logicalPoint(lParam)
		if chromePressed > 0 {
			pressed := chromePressed
			chromePressed = 0
			user32.NewProc("ReleaseCapture").Call()
			if ptInRect(p, chromeButtonRect(logicalWidth, pressed)) {
				clickChrome(hwnd, p)
			}
			return 0
		}
		handleClick(p)
		return 0
	case 0x0200: // WM_MOUSEMOVE
		updateHover(logicalPoint(lParam))
		return 0
	case 0x02A3: // WM_MOUSELEAVE
		updateHover(POINT{-1, -1})
		return 0
	case 0x0020:
		if lParam&0xffff == 1 {
			id := uintptr(IDC_ARROW)
			if (lastHoverID > 0 && lastHoverID < 100) || (lastHoverID >= 100 && lastHoverID-100 < len(actions) && actions[lastHoverID-100].enabled) {
				id = 32649
			}
			cursor, _, _ := procLoadCursorW.Call(0, id)
			user32.NewProc("SetCursor").Call(cursor)
			return 1
		}
	case WM_HOTKEY:
		if recordingEdit != 0 {
			return 0
		}
		if int(wParam) == hotkeyReleaseCursor {
			preferences.FreeCursor = true
			releaseCursorControl()
			saveConfig()
			setStatus("鼠标已释放")
			return 0
		}
		if int(wParam) == hotkeyStop {
			stopOutput()
			return 0
		}
		if int(wParam) == hotkeyBlack {
			blackOutput()
			return 0
		}
		if int(wParam) == HOTKEY_FREEZE {
			freezeOutput()
		}
		if int(wParam) == HOTKEY_RESUME {
			resumeOutput()
		}
		return 0
	case WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX, 0x0138:
		hdc := wParam
		procSetTextColor.Call(hdc, uintptr(uiTextColor()))
		procSetBkColor.Call(hdc, uintptr(uiControlColor()))
		return brushControl
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		paintMainWindow(hwnd, hdc)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_CLOSE:
		if gpuActive != nil {
			closingMain = true
			stopOutput()
			return 0
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func outputWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmGPUEvent:
		pollGPUEvents()
		return 0
	case WM_ERASEBKGND:
		// paintOutput presents the complete frame, including any black borders.
		return 1
	case wmFrameReady:
		handleFrameTick(wParam)
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if !gpuOwnsOutput {
			paintOutput(hdc)
		}
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_CLOSE:
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func buildUI(parent uintptr) {
	cbSource = createControl("COMBOBOX", "", WS_CHILD|WS_VISIBLE|WS_TABSTOP|CBS_DROPDOWNLIST|themedComboStyle, 0, 0, 260, 300, parent, IDC_SOURCE)
	cbOutput = createControl("COMBOBOX", "", WS_CHILD|WS_VISIBLE|WS_TABSTOP|CBS_DROPDOWNLIST|themedComboStyle, 0, 0, 260, 300, parent, IDC_OUTPUT)
	editFreeze = createControl("EDIT", "Ctrl+Alt+F8", WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL, 0, 0, 260, 34, parent, IDC_FREEZE_HOTKEY)
	editResume = createControl("EDIT", "Ctrl+Alt+F9", WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL, 0, 0, 260, 34, parent, IDC_RESUME_HOTKEY)

	editBlack = createControl("EDIT", "Ctrl+Alt+F10", WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL, 0, 0, 260, 34, parent, IDC_BLACK_HOTKEY)
	editStop = createControl("EDIT", "Ctrl+Alt+F12", WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL, 0, 0, 260, 34, parent, IDC_STOP_HOTKEY)
	for _, c := range []uintptr{cbSource, cbOutput, editFreeze, editResume, editBlack, editStop} {
		procSendMessageW.Call(c, WM_SETFONT, fontRow, 1)
		procSetWindowTheme.Call(c, uintptr(unsafe.Pointer(utf16Ptr("DarkMode_CFD"))), 0)
	}
	for _, edit := range []uintptr{editFreeze, editResume, editBlack, editStop} {
		styleHotkeyRecorder(edit)
	}
	buildSettingsControls(parent)
	layoutControls(parent)
}

func paintMainWindow(hwnd, hdc uintptr) {
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top
	if w <= 0 || h <= 0 {
		return
	}

	if !mainBuffer.ensure(hdc, w, h) {
		return
	}
	procFillRect.Call(mainBuffer.dc, uintptr(unsafe.Pointer(&rc)), brushBG)
	saved := beginLogicalDC(mainBuffer.dc)
	logicalWidth, logicalHeight = dip(w), dip(h)
	drawMainUI(mainBuffer.dc, RECT{0, 0, logicalWidth, logicalHeight})
	endLogicalDC(mainBuffer.dc, saved)
	procBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mainBuffer.dc, 0, 0, SRCCOPY)
}

func drawActionButton(hdc uintptr, r RECT, icon, label string, primary bool) {
	brush := brushControl
	pen := penBorder
	color := rgb(233, 239, 247)
	if primary {
		brush = brushAccent
		pen = penAccent
		color = rgb(255, 255, 255)
	}
	drawRounded(hdc, r, 10, brush, pen)
	drawText(hdc, icon, RECT{r.Left + 12, r.Top, r.Left + 48, r.Bottom}, fontIcon, color, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)
	drawText(hdc, label, RECT{r.Left + 48, r.Top, r.Right - 10, r.Bottom}, fontButton, color, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)
}

func drawRounded(hdc uintptr, r RECT, radius int32, brush, pen uintptr) {
	if brush == 0 {
		brush = brushCard
	}
	if pen == 0 {
		pen, _, _ = procGetStockObject.Call(NULL_PEN)
	}
	oldB, _, _ := procSelectObject.Call(hdc, brush)
	oldP, _, _ := procSelectObject.Call(hdc, pen)
	procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius*2), uintptr(radius*2))
	procSelectObject.Call(hdc, oldB)
	procSelectObject.Call(hdc, oldP)
}

func drawText(hdc uintptr, text string, r RECT, font uintptr, color uint32, flags uintptr) {
	oldF, _, _ := procSelectObject.Call(hdc, font)
	procSetTextColor.Call(hdc, uintptr(color))
	t := utf16Ptr(text)
	rc := r
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(t)), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), flags)
	procSelectObject.Call(hdc, oldF)
}

func createControl(class, text string, style uintptr, x, y, w, h int, parent uintptr, id int) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	return hwnd
}

func refreshMonitors() {
	oldSrc := selectedDevice(cbSource)
	oldDst := selectedDevice(cbOutput)
	monitors = nil
	procEnumDisplayMonitors.Call(0, 0, syscall.NewCallback(enumMonitorProc), 0)
	refreshDisplayRates()
	names := connectedMonitorNames()
	for i := range monitors {
		monitors[i].Name = names[monitors[i].Device]
	}

	if cbSource == 0 || cbOutput == 0 {
		return
	}
	procSendMessageW.Call(cbSource, CB_RESETCONTENT, 0, 0)
	procSendMessageW.Call(cbOutput, CB_RESETCONTENT, 0, 0)
	for _, m := range monitors {
		label := monitorChoiceLabel(m)
		p := utf16Ptr(label)
		procSendMessageW.Call(cbSource, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(p)))
		procSendMessageW.Call(cbOutput, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(p)))
	}

	src := findMonitorIndex(oldSrc)
	if src < 0 {
		src = primaryIndex()
	}
	if src < 0 && len(monitors) > 0 {
		src = 0
	}
	dst := findMonitorIndex(oldDst)
	if dst < 0 {
		dst = nonPrimaryIndex()
	}
	if dst < 0 && len(monitors) > 1 {
		if src == 0 {
			dst = 1
		} else {
			dst = 0
		}
	}
	if dst < 0 && len(monitors) > 0 {
		dst = 0
	}

	if src >= 0 {
		procSendMessageW.Call(cbSource, CB_SETCURSEL, uintptr(src), 0)
	}
	if dst >= 0 {
		procSendMessageW.Call(cbOutput, CB_SETCURSEL, uintptr(dst), 0)
	}

	if len(monitors) < 2 {
		setStatus("只检测到 1 个显示器，请连接投影并设置为“扩展”")
	} else if !outputRunning {
		setStatus("显示器已就绪，可开始投影")
	}
	procInvalidateRect.Call(hwndMain, 0, 0)
}

func enumMonitorProc(hMonitor, hdcMonitor, lprcMonitor, dwData uintptr) uintptr {
	var mi MONITORINFOEX
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	r, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if r != 0 {
		dev := syscall.UTF16ToString(mi.SzDevice[:])
		monitors = append(monitors, Monitor{Handle: hMonitor, Rect: mi.RcMonitor, Device: dev, Primary: (mi.DwFlags & MONITORINFOF_PRIMARY) != 0})
	}
	return 1
}

func startOutput() {
	if gpuActive != nil {
		setStatus("请先停止当前投影，等待资源释放")
		return
	}
	refreshMonitors()
	if len(monitors) < 2 {
		messageBox(hwndMain, "当前只检测到一个显示器。\n\n请先连接投影仪，并按 Win+P 选择“扩展”。", "无法开始", MB_ICONWARNING)
		return
	}
	si := comboIndex(cbSource)
	if preferences.WindowMode {
		si = primaryIndex()
	}
	di := comboIndex(cbOutput)
	if si < 0 || di < 0 || si >= len(monitors) || di >= len(monitors) {
		messageBox(hwndMain, "请选择镜像来源和投影屏幕。", "提示", MB_ICONINFORMATION)
		return
	}
	if reason := outputPairError(monitors[si], monitors[di]); reason != "" {
		messageBox(hwndMain, reason, "请选择观众副屏", MB_ICONWARNING)
		return
	}
	selectedWindow := sourceWindow{}
	if preferences.WindowMode {
		selectedWindow = chosenSourceWindow()
		if selectedWindow.HWND == 0 || windowPID(selectedWindow.HWND) != selectedWindow.PID {
			messageBox(hwndMain, "请先选择要投影的应用窗口。", "选择窗口", MB_ICONINFORMATION)
			return
		}
	}
	if si == di {
		messageBox(hwndMain, "镜像来源和投影屏幕不能是同一个显示器。", "设置错误", MB_ICONWARNING)
		return
	}

	applyHotkeys(false)
	stopOutput()
	if restoredDevice != "" {
		setStatus("原刷新率未能恢复，请先在 Windows 显示设置中检查")
		return
	}
	activeSourceWindow = selectedWindow
	srcMonitor = monitors[si]
	dstMonitor = monitors[di]
	keepControlWindowOnSource()
	rateNote := configureOutputRate()
	w := dstMonitor.Rect.Right - dstMonitor.Rect.Left
	h := dstMonitor.Rect.Bottom - dstMonitor.Rect.Top

	hwndOutput, _, _ = procCreateWindowExW.Call(
		WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
		uintptr(unsafe.Pointer(utf16Ptr("ProjectorFreezerOutput"))),
		uintptr(unsafe.Pointer(utf16Ptr("Projector Output"))),
		WS_POPUP,
		uintptr(int64(dstMonitor.Rect.Left)), uintptr(int64(dstMonitor.Rect.Top)), uintptr(w), uintptr(h),
		0, 0, hInstance, 0,
	)
	if hwndOutput == 0 {
		restoreRefreshRate()
		messageBox(hwndMain, "无法创建投影输出窗口。", "启动失败", MB_ICONERROR)
		return
	}
	if !preserveTaskbarControls(hwndOutput) {
		stopOutput()
		setStatus("无法保护主屏任务栏，投影未启动")
		return
	}
	procSetWindowPos.Call(hwndOutput, HWND_TOPMOST, uintptr(int64(dstMonitor.Rect.Left)), uintptr(int64(dstMonitor.Rect.Top)), uintptr(w), uintptr(h), SWP_NOACTIVATE|SWP_SHOWWINDOW)
	procShowWindow.Call(hwndOutput, SW_SHOWNOACTIVATE)
	resetAudiencePreview()
	outputRunning = true
	blackout, blackAcknowledged, gdiPresented = privacy.enabled && activeSourceWindow.HWND == 0, false, false
	if privacy.enabled {
		privacy.latched = true
	}
	sessionStarted = time.Now()
	procSetTimer.Call(hwndMain, dockTimer, 1000, 0)
	frozen = false
	beginProjectionBackend()
	updateCursorControl()
	layoutControls(hwndMain)
	if rateNote != "" {
		setStatus(rateNote)
	} else {
		if !gpuOwnsOutput {
			setStatus("GDI 兼容模式：" + gpuFallbackReason)
		}
	}
	saveConfig()
}

func freezeOutput() {
	if blackout {
		return
	}
	if gpuActive != nil {
		sendGPUCommand(projectionFreezing)
		setStatus("正在冻结…")
		return
	}
	if gpuOwnsOutput {
		return
	}
	if !outputRunning || hwndOutput == 0 {
		setStatus("尚未开始投影，请先点击“开始投影”")
		return
	}
	if !gdiPresented {
		blackOutput()
		return
	}
	frozen = true
	procInvalidateRect.Call(hwndOutput, 0, 0)
	stopFramePump()
	setStatus("已冻结：本机可继续操作，投影保持不变")
}

func resumeOutput() {
	if activeSourceWindow.HWND != 0 && gpuActive == nil {
		setStatus("窗口投影已暂停，请停止后重新开始")
		return
	}
	if !privacyResumeAllowed() {
		return
	}
	if gpuActive != nil {
		sendGPUCommand(projectionLive)
		setStatus("正在恢复实时…")
		return
	}
	if gpuOwnsOutput {
		setStatus("请停止输出后重新开始")
		return
	}
	if !outputRunning || hwndOutput == 0 {
		setStatus("尚未开始投影，请先点击“开始投影”")
		return
	}
	before := capturedGDI
	captureFrame()
	if capturedGDI == before {
		setStatus("未捕获到新画面，保持遮挡")
		return
	}
	blackout, blackAcknowledged = false, false
	frozen = false
	gdiAwaitLive = true
	startFramePump()
	procInvalidateRect.Call(hwndOutput, 0, 0)
	setStatus("正在恢复实时…")
	procUpdateWindow.Call(hwndOutput)
}

func stopOutput() {
	releaseCursorControl()
	if gpuActive != nil {
		sendGPUCommand(projectionStopping)
		setStatus("正在停止 GPU 输出…")
		return
	}
	gpuOwnsOutput = false
	stopFramePump()
	if hwndOutput != 0 {
		procKillTimer.Call(hwndOutput, TIMER_LIVE)
		procDestroyWindow.Call(hwndOutput)
		hwndOutput = 0
	}
	resetAudiencePreview()
	outputRunning = false
	activeSourceWindow = sourceWindow{}
	layoutControls(hwndMain)
	blackout, blackAcknowledged, gdiPresented, gdiAwaitLive = false, false, false, false
	procKillTimer.Call(hwndMain, dockTimer)
	sessionStarted = time.Time{}
	frozen = false
	releaseFrame()
	outputBuffer.release()
	gdiLastBuffer.release()
	gdiSourceValid = false
	restored := restoreRefreshRate()
	if hwndMain != 0 {
		if restored {
			setStatus("投影输出已关闭")
		} else {
			setStatus("投影已停止，但原刷新率未能恢复，请检查 Windows 显示设置")
		}
	}
}

func captureFrame() {
	gdiSourceValid = false
	if !outputRunning || hwndOutput == 0 {
		return
	}
	w := srcMonitor.Rect.Right - srcMonitor.Rect.Left
	h := srcMonitor.Rect.Bottom - srcMonitor.Rect.Top
	if w <= 0 || h <= 0 {
		return
	}

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return
	}
	defer procReleaseDC.Call(0, screenDC)

	if frameDC == 0 || frameW != w || frameH != h {
		releaseFrame()
		frameDC, _, _ = procCreateCompatibleDC.Call(screenDC)
		frameBitmap, _, _ = procCreateCompatibleBitmap.Call(screenDC, uintptr(w), uintptr(h))
		if frameDC == 0 || frameBitmap == 0 {
			releaseFrame()
			return
		}
		procSelectObject.Call(frameDC, frameBitmap)
		frameW, frameH = w, h
	}

	copied, _, _ := procBitBlt.Call(frameDC, 0, 0, uintptr(w), uintptr(h), screenDC,
		uintptr(int64(srcMonitor.Rect.Left)), uintptr(int64(srcMonitor.Rect.Top)), SRCCOPY)
	if copied == 0 {
		return
	}
	capturedGDI++
	gdiSourceValid = true

	if showCursor {
		var ci CURSORINFO
		ci.CbSize = uint32(unsafe.Sizeof(ci))
		ok, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci)))
		if ok != 0 && (ci.Flags&CURSOR_SHOWING) != 0 &&
			ci.PtScreenPos.X >= srcMonitor.Rect.Left && ci.PtScreenPos.X < srcMonitor.Rect.Right &&
			ci.PtScreenPos.Y >= srcMonitor.Rect.Top && ci.PtScreenPos.Y < srcMonitor.Rect.Bottom {
			x := ci.PtScreenPos.X - srcMonitor.Rect.Left
			y := ci.PtScreenPos.Y - srcMonitor.Rect.Top
			procDrawIconEx.Call(frameDC, uintptr(x), uintptr(y), ci.HCursor, 0, 0, 0, 0, DI_NORMAL)
		}
	}
}

func paintOutput(hdc uintptr) {
	if hdc == 0 {
		return
	}
	var rc RECT
	procGetClientRect.Call(hwndOutput, uintptr(unsafe.Pointer(&rc)))
	cw := rc.Right - rc.Left
	ch := rc.Bottom - rc.Top
	if (frozen || !gdiSourceValid) && !blackout && gdiPresented && gdiLastBuffer.dc != 0 {
		procBitBlt.Call(hdc, 0, 0, uintptr(cw), uintptr(ch), gdiLastBuffer.dc, 0, 0, SRCCOPY)
		return
	}
	if !outputBuffer.ensure(hdc, cw, ch) {
		return
	}
	// Compose off-screen. Never expose the clear/scale steps on the visible DC.
	if ok, _, _ := procBitBlt.Call(outputBuffer.dc, 0, 0, uintptr(cw), uintptr(ch), 0, 0, 0, BLACKNESS); ok == 0 {
		return
	}
	if !blackout && gdiSourceValid && frameDC != 0 && frameW > 0 && frameH > 0 {
		dw, dh := outputSize(frameW, frameH, cw, ch, preferences.Fill)
		dx := (cw - dw) / 2
		dy := (ch - dh) / 2
		var ok uintptr
		if dw == frameW && dh == frameH {
			// Preserve source pixels exactly instead of resampling equal-size images.
			ok, _, _ = procBitBlt.Call(outputBuffer.dc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh), frameDC, 0, 0, SRCCOPY)
		} else {
			mode := uintptr(HALFTONE)
			if !preferences.Smooth && dw >= frameW && dh >= frameH {
				mode = 3
			} // COLORONCOLOR: crisp enlargement
			procSetStretchBltMode.Call(outputBuffer.dc, mode)
			gdi32.NewProc("SetBrushOrgEx").Call(outputBuffer.dc, 0, 0, 0)
			ok, _, _ = procStretchBlt.Call(outputBuffer.dc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh), frameDC, 0, 0, uintptr(frameW), uintptr(frameH), SRCCOPY)
		}
		if ok == 0 {
			return
		} // Keep the displayed frame on composition failure.
	}
	if ok, _, _ := procBitBlt.Call(hdc, 0, 0, uintptr(cw), uintptr(ch), outputBuffer.dc, 0, 0, SRCCOPY); ok != 0 {
		outputBuffer, gdiLastBuffer = gdiLastBuffer, outputBuffer
		gdiPresented = true
		if blackout && !blackAcknowledged {
			blackAcknowledged = true
			setStatus("已黑屏：恢复需手动确认")
		}
		if !blackout && gdiSourceValid && gdiAwaitLive {
			gdiAwaitLive = false
			setStatus("GDI 实时投影")
		}
		recordPresentedFrame()
	}
}

func fitRect(sw, sh, dw, dh int32) (int32, int32) {
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return 0, 0
	}
	if int64(dw)*int64(sh) <= int64(dh)*int64(sw) {
		return dw, int32(int64(dw) * int64(sh) / int64(sw))
	}
	return int32(int64(dh) * int64(sw) / int64(sh)), dh
}

func releaseFrame() {
	if frameDC != 0 {
		procDeleteDC.Call(frameDC)
		frameDC = 0
	}
	if frameBitmap != 0 {
		procDeleteObject.Call(frameBitmap)
		frameBitmap = 0
	}
	frameW, frameH = 0, 0
}

func applyHotkeys(showResult bool) {
	registerReleaseCursorHotkey()
	texts := []string{getText(editFreeze), getText(editResume), getText(editBlack), getText(editStop)}
	keys, err := validateActionHotkeys(texts)
	if err != nil {
		setStatus(err.Error())
		if showResult {
			messageBox(hwndMain, err.Error(), "快捷键设置", MB_ICONWARNING)
		}
		return
	}
	failures := []string{}
	for _, id := range actionHotkeyIDs {
		procUnregisterHotKey.Call(hwndMain, uintptr(id))
	}
	results := tryActionRegistrations(keys)
	for i := range actionHotkeyIDs {
		key := keys[i]
		ok := results[i]
		actionHotkeyOK[i] = ok
		appliedActionKeys[i] = key
		if !ok {
			failures = append(failures, actionHotkeyNames[i]+"：注册失败或被占用")
		}
	}
	blackHotkeyOK = actionHotkeyOK[2]
	currentFreeze, currentResume = keys[0], keys[1]
	if len(failures) > 0 {
		setStatus(strings.Join(failures, "；"))
	} else {
		setStatus("四项快捷键已启用")
	}
	if showResult {
		messageBox(hwndMain, statusText, "快捷键设置", MB_ICONINFORMATION)
	}
	procInvalidateRect.Call(hwndMain, 0, 0)
}

func parseHotkey(s string) (Hotkey, error) {
	original := strings.TrimSpace(s)
	if original == "" {
		return Hotkey{}, fmt.Errorf("empty")
	}
	parts := strings.Split(strings.ReplaceAll(original, " ", ""), "+")
	var mods uint32 = MOD_NOREPEAT
	var vk uint32
	keyFound := false
	for _, p := range parts {
		t := strings.ToUpper(strings.TrimSpace(p))
		switch t {
		case "CTRL", "CONTROL":
			mods |= MOD_CONTROL
		case "ALT":
			mods |= MOD_ALT
		case "SHIFT":
			mods |= MOD_SHIFT
		default:
			if keyFound {
				return Hotkey{}, fmt.Errorf("multiple keys")
			}
			k, ok := keyToVK(t)
			if !ok {
				return Hotkey{}, fmt.Errorf("bad key")
			}
			vk = k
			keyFound = true
		}
	}
	if !keyFound {
		return Hotkey{}, fmt.Errorf("no key")
	}
	return Hotkey{Mods: mods, VK: vk, Text: original}, nil
}

func keyToVK(t string) (uint32, bool) {
	if strings.HasPrefix(t, "F") {
		n, err := strconv.Atoi(strings.TrimPrefix(t, "F"))
		if err == nil && n >= 1 && n <= 24 {
			return uint32(0x70 + n - 1), true
		}
	}
	switch t {
	case "INSERT", "INS":
		return 0x2D, true
	case "HOME":
		return 0x24, true
	case "END":
		return 0x23, true
	case "PAGEUP", "PGUP":
		return 0x21, true
	case "PAGEDOWN", "PGDN":
		return 0x22, true
	case "PAUSE":
		return 0x13, true
	}
	if len(t) == 1 {
		c := t[0]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return uint32(c), true
		}
	}
	return 0, false
}

func cleanup() {
	releaseCursorControl()
	procUnregisterHotKey.Call(hwndMain, hotkeyReleaseCursor)
	setPrivacyEnabled(false)
	resetAudiencePreview()
	procKillTimer.Call(hwndMain, previewTimer)
	procUnregisterHotKey.Call(hwndMain, hotkeyBlack)
	procUnregisterHotKey.Call(hwndMain, hotkeyStop)
	if gdipToken != 0 {
		gdip.NewProc("GdiplusShutdown").Call(gdipToken)
		gdipToken = 0
	}
	mainBuffer.release()
	if controlFont != 0 {
		procDeleteObject.Call(controlFont)
		controlFont = 0
		controlFontDPI = 0
	}
	procUnregisterHotKey.Call(hwndMain, HOTKEY_FREEZE)
	procUnregisterHotKey.Call(hwndMain, HOTKEY_RESUME)
	stopOutput()
	for _, obj := range []uintptr{brushBG, brushSidebar, brushCard, brushControl, brushActive, brushAccent, brushGreen, brushMuted, brushWhite, penBorder, penAccent, fontBrand, fontTitle, fontSubtitle, fontRow, fontDesc, fontNav, fontButton, fontSmall, fontIcon, fontNavIcon} {
		if obj != 0 {
			procDeleteObject.Call(obj)
		}
	}
}

func configPath() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		appdata = "."
	}
	dir := filepath.Join(appdata, "ProjectorFreezer")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "config.ini")
}

func loadConfigIntoUI() {
	loadPreferences()
	cfg := Config{FreezeHotkey: "Ctrl+Alt+F8", ResumeHotkey: "Ctrl+Alt+F9", BlackHotkey: "Ctrl+Alt+F10", StopHotkey: "Ctrl+Alt+F12", ShowCursor: true}
	f, err := os.Open(configPath())
	if err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			kv := strings.SplitN(line, "=", 2)
			if len(kv) != 2 {
				continue
			}
			k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			switch strings.ToLower(k) {
			case "sourcedevice":
				cfg.SourceDevice = v
			case "outputdevice":
				cfg.OutputDevice = v
			case "freezehotkey":
				cfg.FreezeHotkey = v
			case "resumehotkey":
				cfg.ResumeHotkey = v
			case "blackhotkey":
				cfg.BlackHotkey = v
			case "stophotkey":
				cfg.StopHotkey = v
			case "showcursor":
				cfg.ShowCursor = strings.EqualFold(v, "true")
			}
		}
	}
	setText(editFreeze, cfg.FreezeHotkey)
	setText(editResume, cfg.ResumeHotkey)
	setText(editBlack, cfg.BlackHotkey)
	setText(editStop, cfg.StopHotkey)
	showCursor = cfg.ShowCursor
	if i := findMonitorIndex(cfg.SourceDevice); i >= 0 {
		procSendMessageW.Call(cbSource, CB_SETCURSEL, uintptr(i), 0)
	}
	if i := findMonitorIndex(cfg.OutputDevice); i >= 0 {
		procSendMessageW.Call(cbOutput, CB_SETCURSEL, uintptr(i), 0)
	}
	procInvalidateRect.Call(hwndMain, 0, 0)
}

func saveConfig() {
	lines := []string{
		"SourceDevice=" + selectedDevice(cbSource),
		"OutputDevice=" + selectedDevice(cbOutput),
		"FreezeHotkey=" + getText(editFreeze),
		"ResumeHotkey=" + getText(editResume),
		"BlackHotkey=" + getText(editBlack),
		"StopHotkey=" + getText(editStop),
		fmt.Sprintf("ShowCursor=%t", showCursor),
	}
	lines = append(lines, preferenceLines(preferences)...)
	if err := os.WriteFile(configPath(), []byte(strings.Join(lines, "\r\n")+"\r\n"), 0644); err != nil {
		setStatus("设置保存失败：" + err.Error())
	}
}

func selectedDevice(cb uintptr) string {
	idx := comboIndex(cb)
	if idx >= 0 && idx < len(monitors) {
		return monitors[idx].Device
	}
	return ""
}

func comboIndex(cb uintptr) int {
	if cb == 0 {
		return -1
	}
	r, _, _ := procSendMessageW.Call(cb, CB_GETCURSEL, 0, 0)
	if int32(r) == -1 {
		return -1
	}
	return int(r)
}

func findMonitorIndex(device string) int {
	if device == "" {
		return -1
	}
	for i, m := range monitors {
		if strings.EqualFold(m.Device, device) {
			return i
		}
	}
	return -1
}
func primaryIndex() int {
	for i, m := range monitors {
		if m.Primary {
			return i
		}
	}
	return -1
}
func nonPrimaryIndex() int {
	for i, m := range monitors {
		if !m.Primary {
			return i
		}
	}
	return -1
}

func setStatus(s string) {
	statusText = s
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "冻结"):
		statusKind = 2
	case strings.Contains(lower, "实时") || strings.Contains(lower, "恢复"):
		statusKind = 1
	case strings.Contains(lower, "错误") || strings.Contains(lower, "失败") || strings.Contains(lower, "只检测") || strings.Contains(lower, "冲突") || strings.Contains(lower, "尚未"):
		statusKind = 3
	default:
		statusKind = 0
	}
	if hwndMain != 0 {
		procInvalidateRect.Call(hwndMain, 0, 0)
	}
}

func monitorRouteText() string {
	if srcMonitor.Device == "" || dstMonitor.Device == "" {
		return statusText
	}
	return fmt.Sprintf("%s  →  %s", srcMonitor.Device, dstMonitor.Device)
}

func setText(hwnd uintptr, s string) {
	p := utf16Ptr(s)
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(p)))
}
func getText(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}
func messageBox(owner uintptr, text, title string, flags uintptr) {
	procMessageBoxW.Call(owner, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(title))), MB_OK|flags)
}

func loadEmbeddedIcon(desired int) uintptr {
	if len(logoICO) < 6 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(logoICO[4:6]))
	bestOffset, bestSize, bestScore := 0, 0, 1<<30
	for i := 0; i < count; i++ {
		p := 6 + i*16
		if p+16 > len(logoICO) {
			break
		}
		w := int(logoICO[p])
		h := int(logoICO[p+1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		sz := int(binary.LittleEndian.Uint32(logoICO[p+8 : p+12]))
		off := int(binary.LittleEndian.Uint32(logoICO[p+12 : p+16]))
		if off < 0 || sz <= 0 || off+sz > len(logoICO) {
			continue
		}
		score := absInt(w-desired) + absInt(h-desired)
		if score < bestScore {
			bestScore, bestOffset, bestSize = score, off, sz
		}
	}
	if bestSize == 0 {
		return 0
	}
	data := logoICO[bestOffset : bestOffset+bestSize]
	r, _, _ := procCreateIconFromResEx.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, 0x00030000, uintptr(desired), uintptr(desired), 0)
	return r
}

func createFont(height, weight int, face string) uintptr {
	f, _, _ := procCreateFontW.Call(
		uintptr(int32(-height)), 0, 0, 0,
		uintptr(weight), 0, 0, 0,
		1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(utf16Ptr(face))),
	)
	return f
}

func makeBrush(r, g, b byte) uintptr {
	br, _, _ := procCreateSolidBrush.Call(uintptr(rgb(r, g, b)))
	return br
}
func makePen(r, g, b byte) uintptr {
	p, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(rgb(r, g, b)))
	return p
}
func rgb(r, g, b byte) uint32   { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }
func utf16Ptr(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func ptInRect(p POINT, r RECT) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}
func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
