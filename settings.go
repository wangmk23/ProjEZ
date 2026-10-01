//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	idSearch = 201 + iota
	idScale
	idFPS
	idAccent
	idQuality
)

type Preferences struct {
	FPS            int
	Fill           bool
	Transparent    bool
	Accent         int
	Smooth         bool
	Compatibility  bool
	PreviewEconomy bool
	WindowMode     bool
	FreeCursor     bool
	LightTheme     bool
}

var preferences = Preferences{FPS: 0}
var fpsChoices = []int{0, -1, 10, 15, 30, 60}
var currentPage = 0
var searchEdit, scaleCombo, fpsCombo, accentCombo, qualityCombo uintptr
var hoverPoint = POINT{-1, -1}
var loadingSettings bool
var searchMessage string

var pageNames = []string{"首页", "投影控制", "快捷键", "显示设置", "外观", "高级", "关于"}
var pageDescriptions = []string{"", "", "", "", "", "", ""}
var pageKeywords = []string{
	"首页 常规 连接 状态 检测 扩展 帮助 使用 指引",
	"投影 镜像 来源 目标 冻结 恢复 停止 屏幕 显示器",
	"快捷键 热键 按键 冲突 F8 F9",
	"显示 比例 缩放 铺满 帧率 刷新 鼠标 光标 FPS 清晰 平滑 画质 分辨率",
	"外观 主题 强调色 颜色 透明 深色",
	"高级 配置 目录 默认 重置 设置文件 隐私 保护 黑屏 演示窗口",
	"关于 版本 说明 操作 帮助",
}

type uiAction struct {
	rect    RECT
	run     func()
	enabled bool
}

var actions []uiAction

var controlWindowProcs = map[uintptr]uintptr{}
var styledControlCallback = syscall.NewCallback(styledControlProc)

// Retain the native edit/combobox interaction while painting a dark frame and arrow.
var controlBuffers = map[uintptr]*paintBuffer{}

func styledControlProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	old := controlWindowProcs[hwnd]
	call := func(message uint32, w, l uintptr) uintptr {
		r, _, _ := user32.NewProc("CallWindowProcW").Call(old, hwnd, uintptr(message), w, l)
		return r
	}
	if msg == 0x0082 {
		result := call(msg, wp, lp)
		if b := controlBuffers[hwnd]; b != nil {
			b.release()
			delete(controlBuffers, hwnd)
		}
		delete(controlWindowProcs, hwnd)
		return result
	}
	if hwnd == searchEdit {
		if msg == 7 || msg == 8 {
			searchFocused = msg == 7
			invalidateLogical(searchBounds(logicalWidth))
		}
		if msg != WM_PAINT && msg != 0x0318 {
			return call(msg, wp, lp)
		}
		length := call(0x000e, 0, 0)
		if length != 0 || searchFocused {
			return call(msg, wp, lp)
		}
	}
	if msg == WM_ERASEBKGND {
		return 1
	}
	if msg != WM_PAINT && msg != 0x0318 {
		return call(msg, wp, lp)
	}
	var ps PAINTSTRUCT
	hdc := wp
	if msg == WM_PAINT {
		hdc, _, _ = procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	}
	if hdc == 0 {
		return 0
	}
	var r RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	b := controlBuffers[hwnd]
	if b == nil {
		b = &paintBuffer{}
		controlBuffers[hwnd] = b
	}
	if !b.ensure(hdc, r.Right, r.Bottom) {
		return 0
	}
	procFillRect.Call(b.dc, uintptr(unsafe.Pointer(&r)), brushControl)
	if hwnd == searchEdit {
		procSetBkMode.Call(b.dc, TRANSPARENT)
		drawText(b.dc, "搜索设置", r, controlFont, uiMutedColor(), DT_VCENTER|DT_SINGLELINE)
		procBitBlt.Call(hdc, 0, 0, uintptr(r.Right), uintptr(r.Bottom), b.dc, 0, 0, SRCCOPY)
		return 0
	}
	// Paint the closed selector in one pass: no native sunken frame underneath.
	procFillRect.Call(b.dc, uintptr(unsafe.Pointer(&r)), brushCard)
	drawRounded(b.dc, r, px(6), brushControl, 0)
	procSetBkMode.Call(b.dc, TRANSPARENT)
	textRect := r
	textRect.Left += px(12)
	textRect.Right -= px(32)
	var label [1024]uint16
	user32.NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)))
	drawText(b.dc, syscall.UTF16ToString(label[:]), textRect, controlFont, uiTextColor(), DT_VCENTER|DT_SINGLELINE|0x8000)
	drawComboArrow(b.dc, hwnd, r)
	procBitBlt.Call(hdc, 0, 0, uintptr(r.Right), uintptr(r.Bottom), b.dc, 0, 0, SRCCOPY)
	return 0
}
func styleControl(hwnd uintptr) {
	old, _, _ := user32.NewProc("SetWindowLongPtrW").Call(hwnd, ^uintptr(3), styledControlCallback) // GWLP_WNDPROC = -4
	if old != 0 {
		controlWindowProcs[hwnd] = old
	}
}

func searchPages(query string) []int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	var matches []int
	for i, name := range pageNames {
		text := strings.ToLower(name + " " + pageKeywords[i])
		match := true
		for _, word := range strings.Fields(query) {
			if !strings.Contains(text, word) {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, i)
		}
	}
	return matches
}

func parsePreferences(text string) Preferences {
	p := Preferences{FPS: 0}
	currentPolicy := false
	for _, line := range strings.Split(text, "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v := strings.TrimSpace(kv[1])
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "lighttheme":
			p.LightTheme = strings.EqualFold(v, "true")
		case "windowmode":
			p.WindowMode = strings.EqualFold(v, "true")
		case "freecursor":
			p.FreeCursor = strings.EqualFold(v, "true")
		case "previeweconomy":
			p.PreviewEconomy = strings.EqualFold(v, "true")
		case "backend":
			p.Compatibility = strings.EqualFold(v, "gdi")
		case "refreshpolicy":
			currentPolicy = v == "2"
		case "fps":
			n, err := strconv.Atoi(v)
			if err == nil && (n == -1 || n == 0 || n == 10 || n == 15 || n == 30 || n == 60) {
				p.FPS = n
			}
		case "smooth":
			p.Smooth = strings.EqualFold(v, "true")
		case "fill":
			p.Fill = strings.EqualFold(v, "true")
		case "transparent":
			p.Transparent = strings.EqualFold(v, "true")
		case "accent":
			n, _ := strconv.Atoi(v)
			if n >= 0 && n <= 2 {
				p.Accent = n
			}
		}
	}
	// v0.4/v0.4.1 only had fixed 10/15/30 FPS. Adopt automatic refresh once on
	// upgrade, but retain any manual choice saved by this version afterwards.
	if !currentPolicy {
		p.FPS = 0
	}
	return p
}

func preferenceLines(p Preferences) []string {
	backend := "auto"
	if p.Compatibility {
		backend = "gdi"
	}
	return []string{fmt.Sprintf("LightTheme=%t", p.LightTheme), fmt.Sprintf("WindowMode=%t", p.WindowMode), fmt.Sprintf("FreeCursor=%t", p.FreeCursor), fmt.Sprintf("PreviewEconomy=%t", p.PreviewEconomy), "Backend=" + backend, "RefreshPolicy=2", fmt.Sprintf("FPS=%d", p.FPS), fmt.Sprintf("Fill=%t", p.Fill), fmt.Sprintf("Transparent=%t", p.Transparent), fmt.Sprintf("Accent=%d", p.Accent), fmt.Sprintf("Smooth=%t", p.Smooth)}
}

func outputSize(sw, sh, dw, dh int32, fill bool) (int32, int32) {
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return 0, 0
	}
	if fill {
		return dw, dh
	}
	return fitRect(sw, sh, dw, dh)
}

func loadPreferences() {
	data, _ := os.ReadFile(configPath())
	preferences = parsePreferences(string(data))
	syncPreferenceControls()
	applyOpacity(hwndMain)
	applyAccent()
}

func applyOpacity(hwnd uintptr) {
	// Keep text and controls opaque; transparency belongs to the backdrop only.
	procSetLayeredWindowAttr.Call(hwnd, 0, 255, LWA_ALPHA)
	backdrop := int32(1)
	if preferences.Transparent {
		backdrop = 2
	}
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_SYSTEMBACKDROP_TYPE, uintptr(unsafe.Pointer(&backdrop)), unsafe.Sizeof(backdrop))
}
func applyAccent() {
	colors := [][3]byte{{45, 128, 237}, {128, 91, 224}, {13, 148, 136}}
	c := colors[preferences.Accent]
	newBrush := makeBrush(c[0], c[1], c[2])
	newPen := makePen(c[0], c[1], c[2])
	if brushAccent != 0 {
		procDeleteObject.Call(brushAccent)
	}
	if penAccent != 0 {
		procDeleteObject.Call(penAccent)
	}
	brushAccent, penAccent = newBrush, newPen
	applyPalette()
}

func buildSettingsControls(parent uintptr) {
	sourceWindowCombo = createControl("COMBOBOX", "", WS_CHILD|WS_TABSTOP|CBS_DROPDOWNLIST|themedComboStyle, 0, 0, 340, 200, parent, idSourceWindow)
	searchEdit = createControl("EDIT", "", WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL, 265, 19, 380, 26, parent, idSearch)
	user32.NewProc("SendMessageW").Call(searchEdit, 0x1501, 0, uintptr(unsafe.Pointer(utf16Ptr("搜索设置"))))
	scaleCombo = createControl("COMBOBOX", "", WS_CHILD|WS_TABSTOP|CBS_DROPDOWNLIST|themedComboStyle, 0, 0, 340, 200, parent, idScale)
	fpsCombo = createControl("COMBOBOX", "", WS_CHILD|WS_TABSTOP|CBS_DROPDOWNLIST|themedComboStyle, 0, 0, 340, 200, parent, idFPS)
	qualityCombo = createControl("COMBOBOX", "", WS_CHILD|WS_TABSTOP|CBS_DROPDOWNLIST|themedComboStyle, 0, 0, 340, 200, parent, idQuality)
	accentCombo = createControl("COMBOBOX", "", WS_CHILD|WS_TABSTOP|CBS_DROPDOWNLIST|themedComboStyle, 0, 0, 340, 200, parent, idAccent)
	for _, item := range []struct {
		hwnd   uintptr
		labels []string
	}{
		{scaleCombo, []string{"保持比例", "铺满屏幕"}},
		{fpsCombo, []string{"自动 · 当前分辨率最高", "自动 · 跟随 Windows 当前", "10 FPS · 节能", "15 FPS", "30 FPS", "60 FPS"}},
		{qualityCombo, []string{"像素优先", "清晰缩放"}},
		{accentCombo, []string{"蓝色", "紫色", "青色"}},
	} {
		for _, label := range item.labels {
			procSendMessageW.Call(item.hwnd, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(label))))
		}
	}
	for _, c := range []uintptr{searchEdit, scaleCombo, fpsCombo, accentCombo, qualityCombo} {
		procSendMessageW.Call(c, WM_SETFONT, fontRow, 1)
		procSetWindowTheme.Call(c, uintptr(unsafe.Pointer(utf16Ptr("DarkMode_CFD"))), 0)
	}
	for _, c := range []uintptr{cbSource, cbOutput, sourceWindowCombo, scaleCombo, fpsCombo, accentCombo, qualityCombo} {
		procSendMessageW.Call(c, 0x0153, ^uintptr(0), 30) // CB_SETITEMHEIGHT: selection
		procSendMessageW.Call(c, 0x0153, 0, 28)
		procSendMessageW.Call(c, 0x0160, 350, 0) // CB_SETDROPPEDWIDTH
		styleControl(c)
		styleDropdown(c)
	}
	styleControl(searchEdit)
	refreshSourceWindows()
	syncPreferenceControls()
}

func syncPreferenceControls() {
	fill := uintptr(0)
	if preferences.Fill {
		fill = 1
	}
	fps := uintptr(0)
	for i, rate := range fpsChoices {
		if rate == preferences.FPS {
			fps = uintptr(i)
		}
	}
	quality := uintptr(0)
	if preferences.Smooth {
		quality = 1
	}
	procSendMessageW.Call(qualityCombo, CB_SETCURSEL, quality, 0)
	procSendMessageW.Call(scaleCombo, CB_SETCURSEL, fill, 0)
	procSendMessageW.Call(fpsCombo, CB_SETCURSEL, fps, 0)
	procSendMessageW.Call(accentCombo, CB_SETCURSEL, uintptr(preferences.Accent), 0)
}

func handleSettingsCommand(id, notify int) bool {
	if loadingSettings {
		return true
	}
	if id == idSearch && notify == 0x0300 { // EN_CHANGE
		query := getText(searchEdit)
		matches := searchPages(query)
		searchMessage = ""
		if len(matches) > 0 {
			selectPage(matches[0])
			names := []string{}
			for _, i := range matches {
				names = append(names, pageNames[i])
			}
			searchMessage = "匹配页面：" + strings.Join(names, "、")
		} else if strings.TrimSpace(query) != "" {
			searchMessage = "没有匹配项，试试“鼠标”“帧率”或“快捷键”"
		}
		procInvalidateRect.Call(hwndMain, 0, 0)
		return true
	}
	if notify != CBN_SELCHANGE {
		return false
	}
	switch id {
	case idScale:
		preferences.Fill = comboIndex(scaleCombo) == 1
		if hwndOutput != 0 {
			procInvalidateRect.Call(hwndOutput, 0, 0)
		}
	case idQuality:
		preferences.Smooth = comboIndex(qualityCombo) == 1
		if hwndOutput != 0 {
			procInvalidateRect.Call(hwndOutput, 0, 0)
		}
	case idFPS:
		idx := comboIndex(fpsCombo)
		if idx < 0 || idx >= len(fpsChoices) {
			return true
		}
		preferences.FPS = fpsChoices[idx]
		if outputRunning {
			if note := configureOutputRate(); note != "" {
				setStatus(note)
			}
			if !frozen {
				startFramePump()
			}
		}
	case idAccent:
		idx := comboIndex(accentCombo)
		if idx < 0 || idx > 2 {
			return true
		}
		preferences.Accent = idx
		applyAccent()
	default:
		return false
	}
	updateGPUSettings()
	saveConfig()
	procInvalidateRect.Call(hwndMain, 0, 0)
	return true
}

func selectPage(page int) {
	if page < 0 || page >= len(pageNames) {
		return
	}
	currentPage = page
	pageScroll = 0
	lastHoverID = 0
	actions = nil
	layoutControls(hwndMain)
	pollAudiencePreview()
	procInvalidateRect.Call(hwndMain, 0, 0)
	procUpdateWindow.Call(hwndMain)
}

func navRect(i int) RECT {
	right := int32(208)
	if contentShift() != 0 {
		right = 54
	}
	return RECT{12, 64 + int32(i)*46, right, 104 + int32(i)*46}
}

func handleClick(p POINT) {
	for i := range pageNames {
		if ptInRect(p, navRect(i)) {
			setText(searchEdit, "")
			searchMessage = ""
			selectPage(i)
			return
		}
	}
	for _, a := range actions {
		if a.enabled && ptInRect(p, a.rect) {
			a.run()
			procInvalidateRect.Call(hwndMain, 0, 0)
			return
		}
	}
}

func updateHover(p POINT) {
	if p == hoverPoint {
		return
	}
	hoverPoint = p
	refreshChromeHover(p)
	tme := struct {
		Size, Flags uint32
		Hwnd        uintptr
		Time        uint32
	}{Flags: 2, Hwnd: hwndMain}
	tme.Size = uint32(unsafe.Sizeof(tme))
	if p.X >= 0 {
		user32.NewProc("TrackMouseEvent").Call(uintptr(unsafe.Pointer(&tme)))
	}
	id, r := hoverRegion(p)
	if id != lastHoverID {
		invalidateLogical(RECT{0, footerTop(), logicalWidth, logicalHeight})
		if lastHoverID != 0 {
			invalidateLogical(lastHoverRect)
		}
		if id != 0 {
			invalidateLogical(r)
		}
		lastHoverID = id
		lastHoverRect = r
	}
}
func layoutControls(parent uintptr) {
	if parent == 0 {
		return
	}
	var rc RECT
	procGetClientRect.Call(parent, uintptr(unsafe.Pointer(&rc)))
	logicalWidth, logicalHeight = dip(rc.Right), dip(rc.Bottom)
	if pageScroll > maxPageScroll() {
		pageScroll = maxPageScroll()
	}
	if controlFont == 0 || controlFontDPI != windowDPI {
		old := controlFont
		controlFont = createFont(int(px(14)), 400, "Microsoft YaHei UI")
		controlFontDPI = windowDPI
		for _, c := range []uintptr{searchEdit, cbSource, cbOutput, sourceWindowCombo, editFreeze, editResume, editBlack, editStop, scaleCombo, fpsCombo, qualityCombo, accentCombo} {
			if c != 0 {
				procSendMessageW.Call(c, WM_SETFONT, controlFont, 1)
			}
		}
		if old != 0 {
			procDeleteObject.Call(old)
		}
	}
	search := searchBounds(logicalWidth)
	procSetWindowPos.Call(searchEdit, 0, uintptr(px(search.Left+14)), uintptr(px(search.Top+6)), uintptr(px(search.Right-search.Left-28)), uintptr(px(23)), SWP_NOZORDER|SWP_NOACTIVATE)
	controlX := logicalWidth - 398
	place := func(hwnd uintptr, y int32, show, combo bool) {
		y -= pageScroll
		if y < 58 || y+32 > footerTop()-8 {
			show = false
		}
		height := int32(32)
		if combo {
			height = 220
		}
		x, width := controlX, int32(350)
		if !combo {
			x += 12
			y += 7
			width -= 24
			height = 20
		}
		procSetWindowPos.Call(hwnd, 0, uintptr(int64(px(x))), uintptr(int64(px(y))), uintptr(px(width)), uintptr(px(height)), SWP_NOZORDER|SWP_NOACTIVATE)
		if combo {
			procSendMessageW.Call(hwnd, 0x0153, ^uintptr(0), uintptr(px(28)))
			procSendMessageW.Call(hwnd, 0x0153, 0, uintptr(px(28)))
			procSendMessageW.Call(hwnd, 0x0160, uintptr(px(350)), 0)
		}
		v := uintptr(0)
		if show {
			v = SW_SHOW
		}
		procShowWindow.Call(hwnd, v)
	}
	place(cbSource, 181, currentPage == 1 && !preferences.WindowMode, true)
	place(sourceWindowCombo, 181, currentPage == 1 && preferences.WindowMode, true)
	for _, c := range []uintptr{cbSource, cbOutput, sourceWindowCombo} {
		v := uintptr(1)
		if outputRunning {
			v = 0
		}
		user32.NewProc("EnableWindow").Call(c, v)
	}
	place(cbOutput, 261, currentPage == 1, true)
	place(editFreeze, 181, currentPage == 2, false)
	place(editResume, 261, currentPage == 2, false)
	place(editBlack, 341, currentPage == 2, false)
	place(editStop, 421, currentPage == 2, false)
	place(scaleCombo, 181, currentPage == 3, true)
	place(fpsCombo, 281, currentPage == 3, true)
	place(qualityCombo, 381, currentPage == 3, true)
	place(accentCombo, 281, currentPage == 4, true)
}
func openLocation(target string) {
	r, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(hwndMain, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(utf16Ptr(target))), 0, 0, SW_SHOW)
	if r <= 32 {
		setStatus("无法打开，请手动访问：" + target)
	}
}

func resetHotkeys() {
	loadingSettings = true
	setText(editFreeze, "Ctrl+Alt+F8")
	setText(editResume, "Ctrl+Alt+F9")
	setText(editBlack, "Ctrl+Alt+F10")
	setText(editStop, "Ctrl+Alt+F12")
	loadingSettings = false
	applyHotkeys(false)
	saveConfig()
}

func resetSettings() {
	r, _, _ := procMessageBoxW.Call(hwndMain, uintptr(unsafe.Pointer(utf16Ptr("将停止投影并恢复全部默认设置。是否继续？"))), uintptr(unsafe.Pointer(utf16Ptr("恢复默认设置"))), 0x24)
	if r != 6 {
		return
	} // IDYES
	stopOutput()
	setPrivacyEnabled(false)
	preferences = Preferences{FPS: 0}
	showCursor = true
	refreshMonitors()
	procSendMessageW.Call(cbSource, CB_SETCURSEL, uintptr(primaryIndex()), 0)
	dst := nonPrimaryIndex()
	if dst < 0 {
		dst = primaryIndex()
	}
	procSendMessageW.Call(cbOutput, CB_SETCURSEL, uintptr(dst), 0)
	syncPreferenceControls()
	applyOpacity(hwndMain)
	applyAccent()
	resetHotkeys()
}

func textLine(hdc uintptr, text string, r RECT, font uintptr, muted bool) {
	c := uiTextColor()
	if muted {
		c = uiMutedColor()
	}
	drawText(hdc, text, r, font, c, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX|DT_END_ELLIPSIS)
}

func uiButton(hdc uintptr, r RECT, label string, primary, enabled bool, run func()) {
	b, p := brushControl, penBorder
	if primary && enabled {
		b, p = brushAccent, penAccent
	}
	if enabled && ptInRect(actionHoverPoint(), r) {
		p = penAccent
		if !primary {
			b = brushActive
		}
	}
	drawRounded(hdc, r, 8, b, p)
	c := uiTextColor()
	if primary && enabled {
		c = rgb(255, 255, 255)
	}
	if !enabled {
		c = uiDisabledColor()
	}
	drawText(hdc, label, r, fontButton, c, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)
	addAction(r, run, enabled)
}

func uiToggle(hdc uintptr, r RECT, on bool, run func()) {
	b := brushMuted
	if on {
		b = brushAccent
	}
	if !smoothPill(hdc, r, b) {
		drawRounded(hdc, r, 13, b, 0)
	}
	x := r.Left + 3
	if on {
		x = r.Right - 23
	}
	knob := RECT{x, r.Top + 3, x + 20, r.Bottom - 3}
	if !smoothPill(hdc, knob, brushWhite) {
		drawRounded(hdc, knob, 10, brushWhite, 0)
	}
	addAction(r, run, true)
}

func settingCard(hdc uintptr, right, top int32, title, description string, hasControl bool) {
	drawRounded(hdc, RECT{244, top, right, top + 72}, 8, brushCard, penBorder)
	end := right - 20
	if hasControl {
		end = right - 374
	}
	if description == "" {
		textLine(hdc, title, RECT{264, top, end, top + 72}, fontRow, false)
		return
	}
	textLine(hdc, title, RECT{264, top + 12, end, top + 39}, fontRow, false)
	textLine(hdc, description, RECT{264, top + 36, end, top + 60}, fontDesc, true)
}

func drawMainUI(hdc uintptr, client RECT) {
	actions = nil
	w, h := client.Right, client.Bottom
	right := w - 28
	procSetBkMode.Call(hdc, TRANSPARENT)
	paintingPage = false
	shift := contentShift()
	right += shift
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&RECT{0, titleHeight, 220 - shift, h})), brushSidebar)
	drawChrome(hdc, w)
	if shift == 0 {
		textLine(hdc, "v0.7.7", RECT{24, h - 40, 212, h - 12}, fontSmall, true)
	}
	icons := []string{"\uE80F", "\uE7F4", "\uE765", "\uE7F8", "\uE790", "\uE713", "\uE946"}
	for i, name := range pageNames {
		r := navRect(i)
		if i == currentPage || ptInRect(hoverPoint, r) {
			drawRounded(hdc, r, 8, brushActive, 0)
		}
		if i == currentPage {
			procFillRect.Call(hdc, uintptr(unsafe.Pointer(&RECT{12, r.Top + 9, 16, r.Bottom - 9})), brushAccent)
		}
		if shift == 0 {
			textLine(hdc, icons[i], RECT{30, r.Top, 60, r.Bottom}, fontNavIcon, false)
		} else {
			drawText(hdc, icons[i], r, fontNavIcon, uiTextColor(), DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		}
		if shift == 0 {
			textLine(hdc, name, RECT{68, r.Top, 204, r.Bottom}, fontNav, false)
		}
	}
	pageDC := beginBody(hdc, true)
	textLine(hdc, pageNames[currentPage], RECT{244, 76, right, 117}, fontTitle, false)
	desc := pageDescriptions[currentPage]
	if searchMessage != "" {
		desc = searchMessage
	}
	if currentPage != 1 {
		textLine(hdc, desc, RECT{244, 117, right, 146}, fontSubtitle, true)
	}
	switch currentPage {
	case 0:
		drawHome(hdc, right)
	case 1:
		drawSourceModes(hdc)
		for i, title := range []string{"我的屏幕", "观众屏幕"} {
			if i == 0 && preferences.WindowMode {
				title = "投影窗口"
			}
			top := int32(158 + i*80)
			drawRounded(hdc, RECT{244, top, right, top + 70}, 8, brushCard, penBorder)
			textLine(hdc, title, RECT{264, top, right - 374, top + 70}, fontRow, false)
		}
		textLine(hdc, sourceRouteLabel(), RECT{264, 314, right - 20, 342}, fontRow, false)
		uiButton(hdc, RECT{264, 350, 410, 386}, "刷新列表", false, !outputRunning, refreshSourceWindows)
		uiButton(hdc, RECT{424, 350, 570, 386}, "刷新屏幕", false, !outputRunning, refreshMonitors)
		drawAudiencePreview(hdc, right, 404)
		drawCursorControl(hdc, right, 352)
	case 2:
		focus, _, _ := user32.NewProc("GetFocus").Call()
		for i, edit := range []uintptr{editFreeze, editResume, editBlack, editStop} {
			top := int32(158 + i*80)
			desc := "未启用：保存检查或换一个组合键"
			if actionHotkeyOK[i] {
				desc = "已启用"
			}
			edited, err := parseHotkey(getText(edit))
			if err != nil || edited.Mods != appliedActionKeys[i].Mods || edited.VK != appliedActionKeys[i].VK {
				desc = "修改后请保存检查"
			}
			if recordingEdit == edit {
				desc = "请按下组合键，Esc 取消"
			}
			settingCard(hdc, right, top, actionHotkeyNames[i]+"快捷键", desc, true)
			border := penBorder
			if focus == edit {
				border = penAccent
			}
			y := top + 23
			drawRounded(hdc, RECT{right - 370, y, right - 20, y + 34}, 6, brushControl, border)
		}
		uiButton(hdc, RECT{264, 486, 410, 528}, "保存快捷键", true, true, func() { applyHotkeys(true); saveConfig() })
		uiButton(hdc, RECT{424, 486, 584, 528}, "恢复默认快捷键", false, true, resetHotkeys)
		textLine(hdc, "停止投影会关闭输出，观众屏返回桌面", RECT{264, 540, right - 20, 568}, fontDesc, true)

	case 3:
		settingCard(hdc, right, 158, "画面缩放", "保持比例可避免画面变形", true)
		settingCard(hdc, right, 258, "帧率", "自动跟随投影屏幕", true)
		settingCard(hdc, right, 358, "缩放质量", "", true)
		textLine(hdc, displayRateSummary(), RECT{264, 446, right - 20, 474}, fontDesc, true)
		textLine(hdc, "投影中显示鼠标", RECT{264, 484, right - 90, 514}, fontRow, false)
		uiToggle(hdc, RECT{right - 76, 486, right - 28, 512}, showCursor, func() { showCursor = !showCursor; updateGPUSettings(); saveConfig() })
		textLine(hdc, projectionBackendSummary(), RECT{264, 526, right - 20, 553}, fontDesc, true)
		textLine(hdc, projectionScaleSummary(), RECT{264, 562, right - 20, 590}, fontDesc, true)
	case 4:
		settingCard(hdc, right, 158, "界面主题", "", true)
		uiButton(hdc, RECT{right - 254, 178, right - 144, 212}, "深色", !preferences.LightTheme, true, func() { setLightTheme(false) })
		uiButton(hdc, RECT{right - 134, 178, right - 24, 212}, "浅色", preferences.LightTheme, true, func() { setLightTheme(true) })
		settingCard(hdc, right, 258, "强调色", "", true)
		settingCard(hdc, right, 358, "背景效果", "", false)
		uiToggle(hdc, RECT{right - 76, 379, right - 28, 405}, preferences.Transparent, func() { preferences.Transparent = !preferences.Transparent; applyOpacity(hwndMain); saveConfig() })
	case 5:
		settingCard(hdc, right, 158, "本地配置", "", false)
		uiButton(hdc, RECT{264, 258, 444, 302}, "打开配置目录", false, true, func() { openLocation(filepath.Dir(configPath())) })
		settingCard(hdc, right, 330, "恢复默认设置", "重置投影、外观和快捷键", false)
		uiButton(hdc, RECT{264, 430, 444, 474}, "恢复默认设置", false, true, resetSettings)
		textLine(hdc, "强制 GDI 兼容模式（下次开始生效）", RECT{264, 500, right - 90, 532}, fontDesc, true)
		uiToggle(hdc, RECT{right - 76, 503, right - 28, 529}, preferences.Compatibility, func() { preferences.Compatibility = !preferences.Compatibility; saveConfig() })
		drawPrivacyControls(hdc, right)
	case 6:
		settingCard(hdc, right, 158, "ProjEZ", "v0.7.7", false)
		textLine(hdc, "Windows 投影控制", RECT{264, 250, right - 20, 282}, fontDesc, true)
		textLine(hdc, "窗口投影、冻结画面与临时黑屏", RECT{264, 286, right - 20, 318}, fontDesc, true)
		uiButton(hdc, RECT{264, 350, 410, 388}, "使用说明", false, true, showUsageGuide)
	}
	endLogicalDC(hdc, pageDC)
	paintingPage = false
	footerDC := beginBody(hdc, false)
	defer endLogicalDC(hdc, footerDC)
	drawControlDock(hdc, right)
}
