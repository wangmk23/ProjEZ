package main

import (
	"fmt"
	"image"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	sceneList = 540 + iota
	sceneAddText
	sceneAddImage
	sceneAddClock
	sceneSave
	sceneClose
	sceneDelete
	sceneRaise
	sceneLower
	sceneBrowse
	sceneText
	sceneFont
	sceneSize
	sceneOpacity
	sceneStatus
	sceneTextLabel
	sceneFontLabel
	sceneStyleLabel
	sceneColorLabel
	sceneLayerLabel
	sceneHint
	sceneMoreColor
)
const scenePaintTimer = 0x4820

var sceneEditor uintptr
var sceneEditorDPI int32
var sceneEditorFont uintptr
var editorBackground, editorCanvasBrush uintptr
var sceneEditorSelection = -1
var sceneEditorPreview *sceneRaster
var sceneEditorCallback = syscall.NewCallback(sceneEditorProc)
var editorControls map[int]uintptr
var editorLayers []sceneLayer
var editorCanvas RECT
var editorBuffer paintBuffer
var editorImages map[string]image.Image
var editorFontNames []string
var editorDirty, editorSyncing, editorNeedsPaint bool
var editorClockText string
var editorDragging, editorResizing bool
var editorDragStart POINT
var editorDragLayer sceneLayer
var editorDragIndex int

var sceneFontChoices = []string{"Microsoft YaHei UI", "微软雅黑", "宋体", "黑体", "楷体", "仿宋", "Segoe UI", "Arial", "Consolas", "Times New Roman"}
var sceneSizes = []int{16, 22, 28, 36, 42, 48, 56, 64, 80, 96, 120, 144, 180}
var sceneColors = []string{"#FFFFFF", "#222222", "#63BCFF", "#38D799", "#FFD45A", "#FF7191"}
var editorPalette = [16]uint32{}

type editorLogFont struct {
	Height, Width, Escape, Orientation, Weight                                               int32
	Italic, Underline, StrikeOut, Charset, OutPrecision, ClipPrecision, Quality, PitchFamily byte
	Face                                                                                     [32]uint16
}

var sceneFontCatalog []string
var enumSceneFontCallback = syscall.NewCallback(func(logFont, _ uintptr, _ uint32, _ uintptr) uintptr {
	name := syscall.UTF16ToString(nativeMessagePointer[editorLogFont](logFont).Face[:])
	if name != "" && !strings.HasPrefix(name, "@") {
		sceneFontCatalog = append(sceneFontCatalog, name)
	}
	return 1
})

func installedSceneFonts() []string {
	if sceneFontCatalog == nil {
		sceneFontCatalog = make([]string, 0, 64)
		dc, _, _ := procGetDC.Call(hwndMain)
		if dc != 0 {
			filter := editorLogFont{Charset: 1}
			gdi32.NewProc("EnumFontFamiliesExW").Call(dc, uintptr(unsafe.Pointer(&filter)), enumSceneFontCallback, 0, 0)
			procReleaseDC.Call(hwndMain, dc)
			slices.Sort(sceneFontCatalog)
		}
	}
	fonts := append([]string(nil), sceneFontChoices...)
	seen := make(map[string]bool, len(fonts))
	for _, f := range fonts {
		seen[strings.ToLower(f)] = true
	}
	for _, f := range sceneFontCatalog {
		key := strings.ToLower(f)
		if !seen[key] {
			seen[key] = true
			fonts = append(fonts, f)
		}
	}
	return fonts
}

func openSceneEditor() {
	if outputRunning || regionPicker != 0 {
		return
	}
	if sceneEditor != 0 {
		user32.NewProc("SetForegroundWindow").Call(sceneEditor)
		return
	}
	loadScene()
	if sceneLoadError != nil {
		messageBox(hwndMain, sceneLoadError.Error(), "无法读取图层设置", MB_ICONWARNING)
		return
	}
	sceneEditorDPI = queryWindowDPI(hwndMain)
	if m, ok := windowMonitor(hwndMain); ok {
		maxDPI := min((m.RcWork.Right-m.RcWork.Left-20)*96/760, (m.RcWork.Bottom-m.RcWork.Top-20)*96/610)
		sceneEditorDPI = min(sceneEditorDPI, max(60, maxDPI))
	}
	name := utf16Ptr("ProjEZSceneEditor")
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), LpfnWndProc: sceneEditorCallback, HInstance: hInstance, LpszClassName: name, HbrBackground: 6, HIcon: appIcon}
	wc.HCursor, _, _ = procLoadCursorW.Call(0, IDC_ARROW)
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	var rect RECT
	user32.NewProc("GetWindowRect").Call(hwndMain, uintptr(unsafe.Pointer(&rect)))
	w, h := scenePixel(1080), scenePixel(650)
	x, y := rect.Left+24, rect.Top+24
	if m, ok := windowMonitor(hwndMain); ok {
		w = min(w, m.RcWork.Right-m.RcWork.Left-20)
		h = min(h, m.RcWork.Bottom-m.RcWork.Top-20)
		x = max(m.RcWork.Left, min(x, m.RcWork.Right-w))
		y = max(m.RcWork.Top, min(y, m.RcWork.Bottom-h))
	}
	// A resizable owned editor can fill the control screen without crossing to the audience display.
	sceneEditor, _, _ = procCreateWindowExW.Call(0x00000001, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(utf16Ptr("图文图层"))), 0x02cf0000, uintptr(int64(x)), uintptr(int64(y)), uintptr(w), uintptr(h), hwndMain, 0, hInstance, 0)
	if sceneEditor == 0 {
		setStatus("无法打开图层编辑器")
		return
	}
	editorLayers = append([]sceneLayer(nil), sceneLayers...)
	editorImages = make(map[string]image.Image)
	editorDirty = false
	editorFontNames = installedSceneFonts()
	for _, l := range editorLayers {
		found := false
		for _, font := range editorFontNames {
			if font == l.Font {
				found = true
				break
			}
		}
		if !found && l.Font != "" {
			editorFontNames = append(editorFontNames, l.Font)
		}
	}
	sceneEditorSelection = len(editorLayers) - 1
	buildVisualEditor()
	layoutVisualEditor()
	syncVisualEditor()
	renderSceneEditorPreview()
	user32.NewProc("EnableWindow").Call(hwndMain, 0)
	procSetTimer.Call(sceneEditor, scenePaintTimer, 50, 0)
	procShowWindow.Call(sceneEditor, SW_SHOW)
}
func scenePixel(v int32) int32 { return scaleAt(v, sceneEditorDPI) }
func editorChild(id int, class, text string, style uintptr) uintptr {
	c := createControl(class, text, style|WS_CHILD|WS_VISIBLE, 0, 0, 0, 0, sceneEditor, id)
	editorControls[id] = c
	procSendMessageW.Call(c, WM_SETFONT, sceneEditorFont, 1)
	return c
}
func buildVisualEditor() {
	editorControls = make(map[int]uintptr)
	editorBackground = makeBrush(245, 247, 250)
	editorCanvasBrush = makeBrush(24, 29, 39)
	sceneEditorFont = createFont(int(scenePixel(14)), 400, "Microsoft YaHei UI")
	for _, b := range []struct {
		id   int
		text string
	}{
		{sceneAddText, "＋ 文字"}, {sceneAddImage, "＋ 图片"}, {sceneAddClock, "＋ 当前时间"},
		{sceneRaise, "前移"}, {sceneLower, "后移"}, {sceneDelete, "删除"}, {sceneBrowse, "更换图片"},
		{sceneMoreColor, "更多颜色"}, {sceneSave, "保存"}, {sceneClose, "关闭"},
	} {
		editorChild(b.id, "BUTTON", b.text, WS_TABSTOP)
	}
	for _, l := range []struct {
		id   int
		text string
	}{
		{sceneLayerLabel, "图层"}, {sceneTextLabel, "文字"}, {sceneFontLabel, "字体与字号"},
		{sceneStyleLabel, "样式"}, {sceneColorLabel, "颜色"}, {sceneHint, "拖动画面中的图层，拖右下角调整大小"},
		{sceneStatus, ""},
	} {
		editorChild(l.id, "STATIC", l.text, 0)
	}
	editorChild(sceneList, "LISTBOX", "", WS_TABSTOP|0x00800000|0x00200000|1)
	editorChild(sceneText, "EDIT", "", WS_TABSTOP|0x00800000|ES_AUTOHSCROLL)
	procSendMessageW.Call(editorControls[sceneText], 0xc5, 512, 0)
	editorChild(sceneFont, "COMBOBOX", "", WS_TABSTOP|0x00200000|CBS_DROPDOWNLIST)
	editorChild(sceneSize, "COMBOBOX", "", WS_TABSTOP|0x00200000|CBS_DROPDOWNLIST)
	editorChild(sceneOpacity, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST)
	for _, name := range editorFontNames {
		label := name
		if name == "Microsoft YaHei UI" {
			label = "微软雅黑 UI"
		}
		editorAddCombo(sceneFont, label)
	}
	for _, size := range sceneSizes {
		editorAddCombo(sceneSize, strconv.Itoa(size))
	}
	for _, label := range []string{"不透明", "半透明", "淡化"} {
		editorAddCombo(sceneOpacity, label)
	}
}
func editorAddCombo(id int, text string) {
	procSendMessageW.Call(editorControls[id], CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(text))))
}
func editorPlace(id int, x, y, w, h int32) {
	if c := editorControls[id]; c != 0 {
		procSetWindowPos.Call(c, 0, uintptr(x), uintptr(y), uintptr(max(1, w)), uintptr(max(1, h)), SWP_NOZORDER|SWP_NOACTIVATE)
	}
}
func editorRightPanel() int32 {
	var r RECT
	procGetClientRect.Call(sceneEditor, uintptr(unsafe.Pointer(&r)))
	return r.Right - scenePixel(296)
}
func layoutVisualEditor() {
	if sceneEditor == 0 || editorControls == nil {
		return
	}
	var r RECT
	procGetClientRect.Call(sceneEditor, uintptr(unsafe.Pointer(&r)))
	s := scenePixel
	p := editorRightPanel()
	place := func(id int, x, y, w, h int32) { editorPlace(id, x, s(y), s(w), s(h)) }
	for i, id := range []int{sceneAddText, sceneAddImage, sceneAddClock} {
		place(id, s(16+int32(i)*105), 13, 96, 32)
	}
	place(sceneLayerLabel, p, 15, 266, 22)
	editorPlace(sceneList, p, s(40), s(272), s(102))
	listTop := int32(148)
	for i, id := range []int{sceneRaise, sceneLower, sceneDelete} {
		place(id, p+s(int32(i)*91), listTop, 84, 28)
	}
	place(sceneTextLabel, p, 187, 266, 22)
	place(sceneText, p, 214, 272, 28)
	place(sceneBrowse, p, 214, 132, 28)
	place(sceneFontLabel, p, 257, 272, 20)
	place(sceneFont, p, 284, 178, 230)
	place(sceneSize, p+s(188), 284, 84, 220)
	place(sceneStyleLabel, p, 328, 272, 20)
	place(sceneColorLabel, p, 410, 272, 20)
	place(sceneMoreColor, p+s(199), 438, 73, 28)
	place(sceneOpacity, p, 485, 155, 160)
	bottom := r.Bottom - s(48)
	editorPlace(sceneHint, s(16), bottom, s(343), s(21))
	editorPlace(sceneStatus, s(16), bottom+s(22), max(s(100), p-s(28)), s(21))
	editorPlace(sceneSave, r.Right-s(191), bottom, s(82), s(32))
	editorPlace(sceneClose, r.Right-s(99), bottom, s(82), s(32))
	area := RECT{s(16), s(58), p - s(18), bottom - s(11)}
	w, h := editorSceneSize()
	w, h = fitRect(w, h, area.Right-area.Left, area.Bottom-area.Top)
	x, y := area.Left+(area.Right-area.Left-w)/2, area.Top+(area.Bottom-area.Top-h)/2
	editorCanvas = RECT{x, y, x + w, y + h}
	editorNeedsPaint = true
}
func updateEditorFontSize(size int) {
	for i, v := range sceneSizes {
		if size == v {
			procSendMessageW.Call(editorControls[sceneSize], CB_SETCURSEL, uintptr(i), 0)
			return
		}
	}
	for i := len(sceneSizes) - 1; i >= 0; i-- {
		if sceneSizes[i] <= size {
			procSendMessageW.Call(editorControls[sceneSize], CB_SETCURSEL, uintptr(i), 0)
			return
		}
	}
	procSendMessageW.Call(editorControls[sceneSize], CB_SETCURSEL, 0, 0)
}
func editorLayerName(l sceneLayer) string {
	if l.Clock {
		return "当前时间"
	}
	if l.File != "" {
		return filepath.Base(l.File)
	}
	if l.Text == "" {
		return "文字"
	}
	return l.Text
}
func refreshSceneLayerList() {
	list := editorControls[sceneList]
	procSendMessageW.Call(list, 0x0184, 0, 0)
	for _, l := range editorLayers {
		label := []rune(editorLayerName(l))
		if len(label) > 28 {
			label = append(label[:28], '…')
		}
		procSendMessageW.Call(list, 0x0180, 0, uintptr(unsafe.Pointer(utf16Ptr(string(label)))))
	}
	if sceneEditorSelection >= 0 {
		procSendMessageW.Call(list, 0x0186, uintptr(sceneEditorSelection), 0)
	}
}
func selectedEditorLayer() *sceneLayer {
	if sceneEditorSelection < 0 || sceneEditorSelection >= len(editorLayers) {
		return nil
	}
	return &editorLayers[sceneEditorSelection]
}
func syncVisualEditor() {
	editorSyncing = true
	defer func() { editorSyncing = false }()
	refreshSceneLayerList()
	l := selectedEditorLayer()
	text, icon := l != nil && l.File == "", l != nil && l.File != ""
	for _, id := range []int{sceneFont, sceneSize, sceneOpacity, sceneDelete, sceneRaise, sceneLower, sceneMoreColor} {
		enabled := uintptr(0)
		if l != nil && (id == sceneOpacity || id == sceneDelete || id == sceneRaise && !l.Clock || id == sceneLower && !l.Clock || text) {
			enabled = 1
		}
		user32.NewProc("EnableWindow").Call(editorControls[id], enabled)
	}
	show := func(id int, v bool) {
		flag := uintptr(0)
		if v {
			flag = SW_SHOWNOACTIVATE
		}
		procShowWindow.Call(editorControls[id], flag)
	}
	show(sceneText, text && !l.Clock)
	show(sceneBrowse, icon)
	if text && l.Clock {
		setText(editorControls[sceneTextLabel], "当前时间 · 冻结时继续走")
	} else if icon {
		setText(editorControls[sceneTextLabel], "图片 · "+filepath.Base(l.File))
	} else {
		setText(editorControls[sceneTextLabel], "文字")
	}
	if l != nil {
		setText(editorControls[sceneText], l.Text)
		for i, name := range editorFontNames {
			if name == l.Font {
				procSendMessageW.Call(editorControls[sceneFont], CB_SETCURSEL, uintptr(i), 0)
				break
			}
		}
		updateEditorFontSize(l.Size)
		opacity := uintptr(0)
		if l.Opacity < 40 {
			opacity = 2
		} else if l.Opacity < 90 {
			opacity = 1
		}
		procSendMessageW.Call(editorControls[sceneOpacity], CB_SETCURSEL, opacity, 0)
	}
	editorNeedsPaint = true
}
func editorSceneSize() (int32, int32) {
	i := comboIndex(cbOutput)
	if i >= 0 && i < len(monitors) {
		r := monitors[i].Rect
		return r.Right - r.Left, r.Bottom - r.Top
	}
	return 1280, 720
}
func renderSceneEditorPreview() {
	editorNeedsPaint = false
	editorClockText = clockText(time.Now())
	w, h := editorCanvas.Right-editorCanvas.Left, editorCanvas.Bottom-editorCanvas.Top
	if w <= 0 || h <= 0 {
		return
	}
	for _, l := range editorLayers {
		if l.File == "" || editorImages[l.File] != nil {
			continue
		}
		img, err := loadSceneImage(l.File)
		if err != nil {
			setText(editorControls[sceneStatus], "找不到图片，可选中该层后更换图片")
			return
		}
		editorImages[l.File] = img
	}
	preview, err := rasterSceneAt(editorLayers, w, h, time.Now(), editorImages)
	if err != nil {
		setText(editorControls[sceneStatus], err.Error())
		return
	}
	sceneEditorPreview = preview
	procInvalidateRect.Call(sceneEditor, 0, 0)
}
func markEditorChanged() {
	editorDirty = true
	editorNeedsPaint = true
	setText(editorControls[sceneStatus], "有未保存的修改")
}
func addEditorLayer(id int) {
	if id == sceneAddClock {
		for i, l := range editorLayers {
			if l.Clock {
				sceneEditorSelection = i
				syncVisualEditor()
				return
			}
		}
	}
	if len(editorLayers) >= maxSceneLayers {
		setText(editorControls[sceneStatus], "最多可添加 16 层")
		return
	}
	l := defaultLayer()
	switch id {
	case sceneAddText:
		l.Text = "演示标题"
		l.W, l.H = 45, 18
	case sceneAddImage:
		l.File = sceneChooseImage()
		if l.File == "" {
			return
		}
		l.W, l.H = 30, 30
	case sceneAddClock:
		l.Clock = true
		l.Font = "Consolas"
		l.Size = 48
		l.Bold = true
		l.Style = "label"
		l.X, l.Y, l.W, l.H = 72, 5, 24, 10
	}
	i := len(editorLayers)
	if i > 0 && editorLayers[i-1].Clock && !l.Clock {
		i--
	}
	editorLayers = append(editorLayers, sceneLayer{})
	copy(editorLayers[i+1:], editorLayers[i:])
	editorLayers[i] = l
	sceneEditorSelection = i
	syncVisualEditor()
	markEditorChanged()
	if id == sceneAddText {
		user32.NewProc("SetFocus").Call(editorControls[sceneText])
		procSendMessageW.Call(editorControls[sceneText], 0x00b1, 0, ^uintptr(0))
	}
}
func applyEditorSave() {
	w, h := editorSceneSize()
	w, h = fitRect(w, h, 960, 540)
	if _, err := rasterSceneAt(editorLayers, w, h, time.Now(), editorImages); err != nil {
		setText(editorControls[sceneStatus], err.Error())
		return
	}
	if err := saveScene(editorLayers); err != nil {
		setText(editorControls[sceneStatus], err.Error())
		return
	}
	sceneLayers = append([]sceneLayer(nil), editorLayers...)
	editorDirty = false
	refreshSceneLayerList()
	setText(editorControls[sceneStatus], "图层已保存")
}
func closeEditor() {
	if editorDirty {
		choice, _, _ := procMessageBoxW.Call(sceneEditor, uintptr(unsafe.Pointer(utf16Ptr("还有未保存的图层修改。要保存吗？"))), uintptr(unsafe.Pointer(utf16Ptr("图层编辑"))), 0x23)
		if choice == 6 {
			applyEditorSave()
			if editorDirty {
				return
			}
		} else if choice != 7 {
			return
		}
	}
	procDestroyWindow.Call(sceneEditor)
}
func sceneEditorCommand(id, notify uint16) {
	if editorSyncing {
		return
	}
	l := selectedEditorLayer()
	switch int(id) {
	case sceneList:
		if notify != 1 {
			return
		}
		n, _, _ := procSendMessageW.Call(editorControls[sceneList], 0x0188, 0, 0)
		if n < uintptr(len(editorLayers)) {
			sceneEditorSelection = int(n)
			syncVisualEditor()
		}
	case sceneAddText, sceneAddImage, sceneAddClock:
		addEditorLayer(int(id))
	case sceneBrowse:
		if l == nil || l.File == "" {
			return
		}
		if path := sceneChooseImage(); path != "" {
			l.File = path
			syncVisualEditor()
			markEditorChanged()
		}
	case sceneText:
		if notify == EN_KILLFOCUS {
			refreshSceneLayerList()
			return
		}
		if l == nil || l.Clock || notify != 0x0300 {
			return
		}
		l.Text = getText(editorControls[sceneText])
		markEditorChanged()
	case sceneFont, sceneSize, sceneOpacity:
		if l == nil || notify != CBN_SELCHANGE {
			return
		}
		choice := comboIndex(editorControls[int(id)])
		if choice < 0 {
			return
		}
		switch int(id) {
		case sceneFont:
			if choice < len(editorFontNames) {
				l.Font = editorFontNames[choice]
			}
		case sceneSize:
			if choice < len(sceneSizes) {
				l.Size = sceneSizes[choice]
			}
		case sceneOpacity:
			if choice < 3 {
				l.Opacity = []int{100, 65, 25}[choice]
			}
		}
		markEditorChanged()
	case sceneMoreColor:
		if l != nil && l.File == "" {
			if c := chooseSceneColor(l.Color); c != "" {
				l.Color = c
				markEditorChanged()
			}
		}
	case sceneDelete:
		if l == nil {
			return
		}
		i := sceneEditorSelection
		editorLayers = append(editorLayers[:i], editorLayers[i+1:]...)
		sceneEditorSelection = min(i, len(editorLayers)-1)
		syncVisualEditor()
		markEditorChanged()
	case sceneRaise, sceneLower:
		if l == nil || l.Clock {
			return
		}
		i := sceneEditorSelection
		j := i + 1
		if id == sceneLower {
			j = i - 1
		}
		if j >= 0 && j < len(editorLayers) && !editorLayers[j].Clock {
			editorLayers[i], editorLayers[j] = editorLayers[j], editorLayers[i]
			sceneEditorSelection = j
			syncVisualEditor()
			markEditorChanged()
		}
	case sceneSave:
		applyEditorSave()
	case sceneClose:
		closeEditor()
	}
}
func layerCanvasBounds(l sceneLayer, canvas RECT) RECT {
	w, h := canvas.Right-canvas.Left, canvas.Bottom-canvas.Top
	return RECT{canvas.Left + w*int32(l.X)/100, canvas.Top + h*int32(l.Y)/100, canvas.Left + w*int32(l.X+l.W)/100, canvas.Top + h*int32(l.Y+l.H)/100}
}
func visualLayerDrag(l sceneLayer, dx, dy int, resize bool) sceneLayer {
	if resize {
		l.W = min(100-l.X, max(3, l.W+dx))
		l.H = min(100-l.Y, max(3, l.H+dy))
	} else {
		l.X = max(0, min(100-l.W, l.X+dx))
		l.Y = max(0, min(100-l.H, l.Y+dy))
	}
	return l
}
func editorResizeHandle(l sceneLayer) RECT {
	b := layerCanvasBounds(l, editorCanvas)
	size := scenePixel(9)
	return RECT{b.Right - size, b.Bottom - size, b.Right + size/2, b.Bottom + size/2}
}
func styleOptionRect(i int) RECT {
	p := editorRightPanel() + scenePixel(int32(i)*68)
	return RECT{p, scenePixel(354), p + scenePixel(61), scenePixel(394)}
}
func editorColorRect(i int) RECT {
	p := editorRightPanel() + scenePixel(int32(i)*33)
	return RECT{p, scenePixel(439), p + scenePixel(26), scenePixel(465)}
}
func editorBeginDrag(p POINT) {
	l := selectedEditorLayer()
	if l != nil && ptInRect(p, editorResizeHandle(*l)) {
		editorResizing = true
	} else {
		editorResizing = false
		selected := -1
		for i := len(editorLayers) - 1; i >= 0; i-- {
			if ptInRect(p, layerCanvasBounds(editorLayers[i], editorCanvas)) {
				selected = i
				break
			}
		}
		if selected < 0 {
			return
		}
		sceneEditorSelection = selected
		syncVisualEditor()
		l = selectedEditorLayer()
	}
	editorDragIndex, editorDragLayer, editorDragStart = sceneEditorSelection, *l, p
	editorDragging = true
	user32.NewProc("SetCapture").Call(sceneEditor)
}
func editCanvasAt(p POINT) {
	if l := selectedEditorLayer(); l != nil && l.File == "" {
		for i, color := range sceneColors {
			if ptInRect(p, editorColorRect(i)) {
				l.Color = color
				markEditorChanged()
				return
			}
		}
		for i := 0; i < 4; i++ {
			if ptInRect(p, styleOptionRect(i)) {
				l.Bold = i == 1 || i == 3
				l.Italic = i == 2
				l.Style = ""
				if i == 3 {
					l.Style = "label"
				}
				markEditorChanged()
				return
			}
		}
	}
	if l := selectedEditorLayer(); ptInRect(p, editorCanvas) || l != nil && ptInRect(p, editorResizeHandle(*l)) {
		editorBeginDrag(p)
	}
}
func moveEditorLayer(p POINT) {
	if !editorDragging || editorDragIndex >= len(editorLayers) {
		return
	}
	w, h := editorCanvas.Right-editorCanvas.Left, editorCanvas.Bottom-editorCanvas.Top
	if w <= 0 || h <= 0 {
		return
	}
	dx, dy := int((p.X-editorDragStart.X)*100/w), int((p.Y-editorDragStart.Y)*100/h)
	next := visualLayerDrag(editorDragLayer, dx, dy, editorResizing)
	if next != editorLayers[editorDragIndex] {
		editorLayers[editorDragIndex] = next
		markEditorChanged()
	}
}
func paintVisualEditor(dc uintptr) {
	var r RECT
	procGetClientRect.Call(sceneEditor, uintptr(unsafe.Pointer(&r)))
	if !editorBuffer.ensure(dc, r.Right, r.Bottom) {
		return
	}
	b := editorBuffer.dc
	procFillRect.Call(b, uintptr(unsafe.Pointer(&r)), editorBackground)
	procSetBkMode.Call(b, TRANSPARENT)
	procFillRect.Call(b, uintptr(unsafe.Pointer(&editorCanvas)), editorCanvasBrush)
	if f := sceneEditorPreview; f != nil {
		c := editorCanvas
		h := bitmapHeader{Size: 40, Width: f.W, Height: -f.H, Planes: 1, Bits: 32}
		backdrop, bitmap, old, pixels, err := createSceneDIB(f.W, f.H)
		if err == nil {
			copy(pixels, f.Pixels)
			alphaBlend.Call(b, uintptr(c.Left), uintptr(c.Top), uintptr(c.Right-c.Left), uintptr(c.Bottom-c.Top), backdrop, 0, 0, uintptr(f.W), uintptr(f.H), 0x01ff0000)
			releaseSceneDIB(backdrop, bitmap, old)
		} else {
			gdi32.NewProc("StretchDIBits").Call(b, uintptr(c.Left), uintptr(c.Top), uintptr(c.Right-c.Left), uintptr(c.Bottom-c.Top), 0, 0, uintptr(f.W), uintptr(f.H), uintptr(unsafe.Pointer(&f.Pixels[0])), uintptr(unsafe.Pointer(&h)), 0, SRCCOPY)
		}
	} else {
		drawText(b, "添加文字、图片或当前时间", editorCanvas, sceneEditorFont, rgb(184, 193, 208), DT_SINGLELINE|DT_CENTER|DT_VCENTER)
	}
	if l := selectedEditorLayer(); l != nil {
		bounds := layerCanvasBounds(*l, editorCanvas)
		user32.NewProc("FrameRect").Call(b, uintptr(unsafe.Pointer(&bounds)), brushAccent)
		handle := editorResizeHandle(*l)
		procFillRect.Call(b, uintptr(unsafe.Pointer(&handle)), brushAccent)
		if l.File == "" {
			paintStyleOptions(b, *l)
			for i, color := range sceneColors {
				value, _ := layerColor(color)
				box := editorColorRect(i)
				swatch := makeBrush(byte(value>>16), byte(value>>8), byte(value))
				edge := penBorder
				if l.Color == color {
					edge = penAccent
				}
				drawRounded(b, box, 3, swatch, edge)
				procDeleteObject.Call(swatch)
			}
		}
	}
	procBitBlt.Call(dc, 0, 0, uintptr(r.Right), uintptr(r.Bottom), b, 0, 0, SRCCOPY)
}
func paintStyleOptions(dc uintptr, l sceneLayer) {
	for i, label := range []string{"普通", "粗体", "斜体", "标签"} {
		box := styleOptionRect(i)
		wt := uintptr(400)
		if i == 1 || i == 3 {
			wt = 700
		}
		italic := uintptr(0)
		if i == 2 {
			italic = 1
		}
		font, _, _ := gdi32.NewProc("CreateFontW").Call(uintptr(int64(-scenePixel(14))), 0, 0, 0, wt, italic, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(utf16Ptr(l.Font))))
		br := makeBrush(255, 255, 255)
		color := rgb(32, 40, 52)
		if i == 3 {
			procDeleteObject.Call(br)
			br = makeBrush(46, 55, 70)
			color = rgb(255, 255, 255)
		}
		pen := penBorder
		if (i == 0 && !l.Bold && !l.Italic && l.Style != "label") || (i == 1 && l.Bold && l.Style != "label") || (i == 2 && l.Italic) || (i == 3 && l.Style == "label") {
			pen = penAccent
		}
		drawRounded(dc, box, 4, br, pen)
		drawText(dc, label, box, font, color, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		procDeleteObject.Call(font)
		procDeleteObject.Call(br)
	}
}
func sceneEditorProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	p := POINT{int32(int16(lp)), int32(int16(lp >> 16))}
	switch msg {
	case WM_COMMAND:
		sceneEditorCommand(uint16(wp), uint16(wp>>16))
		return 0
	case WM_CLOSE:
		closeEditor()
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_SIZE:
		if hwnd == sceneEditor {
			layoutVisualEditor()
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case 0x0024: // WM_GETMINMAXINFO
		info := nativeMessagePointer[struct{ Reserved, MaxSize, MaxPosition, MinTrack, MaxTrack POINT }](lp)
		minW, minH := scenePixel(740), scenePixel(590)
		if m, ok := windowMonitor(hwnd); ok {
			minW = min(minW, m.RcWork.Right-m.RcWork.Left)
			minH = min(minH, m.RcWork.Bottom-m.RcWork.Top)
		}
		info.MinTrack = POINT{minW, minH}
		return 0
	case 0x0138: // WM_CTLCOLORSTATIC
		procSetBkColor.Call(wp, uintptr(rgb(245, 247, 250)))
		return editorBackground
	case 0x0113:
		if wp == scenePaintTimer {
			if !editorNeedsPaint && editorClockText != clockText(time.Now()) {
				for _, l := range editorLayers {
					if l.Clock {
						editorNeedsPaint = true
						break
					}
				}
			}
			if editorNeedsPaint {
				renderSceneEditorPreview()
			}
			return 0
		}
	case 0x0201:
		editCanvasAt(p)
		return 0
	case 0x0200:
		moveEditorLayer(p)
		return 0
	case WM_LBUTTONUP:
		if editorDragging {
			moveEditorLayer(p)
			editorDragging = false
			user32.NewProc("ReleaseCapture").Call()
		}
		return 0
	case 0x0215:
		editorDragging = false
		return 0 // WM_CAPTURECHANGED
	case WM_DESTROY:
		procKillTimer.Call(hwnd, scenePaintTimer)
		editorBuffer.release()
		procDeleteObject.Call(sceneEditorFont)
		procDeleteObject.Call(editorBackground)
		procDeleteObject.Call(editorCanvasBrush)
		editorBackground, editorCanvasBrush = 0, 0
		sceneEditor, sceneEditorFont, sceneEditorPreview = 0, 0, nil
		editorControls, editorLayers, editorImages = nil, nil, nil
		editorDragging = false
		user32.NewProc("EnableWindow").Call(hwndMain, 1)
		user32.NewProc("SetForegroundWindow").Call(hwndMain)
		procInvalidateRect.Call(hwndMain, 0, 0)
		return 0
	case WM_PAINT, 0x0318:
		dc := wp
		if msg == WM_PAINT {
			var ps PAINTSTRUCT
			dc, _, _ = procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		if dc != 0 {
			paintVisualEditor(dc)
		}
		return 0
	}
	v, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return v
}

type editorColorDialog struct {
	Size                 uint32
	Owner, Instance      uintptr
	Color                uint32
	Custom               *uint32
	Flags                uint32
	Data, Hook, Template uintptr
}

func chooseSceneColor(text string) string {
	c, _ := layerColor(text)
	win := ((c & 255) << 16) | (c & 0xff00) | (c >> 16)
	d := editorColorDialog{Owner: sceneEditor, Color: win, Custom: &editorPalette[0], Flags: 3}
	d.Size = uint32(unsafe.Sizeof(d))
	ok, _, _ := syscall.NewLazyDLL("comdlg32.dll").NewProc("ChooseColorW").Call(uintptr(unsafe.Pointer(&d)))
	if ok == 0 {
		return ""
	}
	c = d.Color
	return fmt.Sprintf("#%02X%02X%02X", c&255, c>>8&255, c>>16&255)
}
