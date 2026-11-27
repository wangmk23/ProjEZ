package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	sceneList      = 540
	sceneAddText   = 541
	sceneAddImage  = 542
	sceneSave      = 543
	sceneDelete    = 544
	sceneRaise     = 545
	sceneLower     = 546
	sceneBrowse    = 547
	sceneClose     = 548
	sceneBold      = 549
	sceneItalic    = 550
	sceneTry       = 551
	sceneFieldBase = 560
)

var sceneEditor uintptr
var sceneEditorFont uintptr
var sceneEditorList, sceneEditorStatus uintptr
var sceneEditorFields [10]uintptr
var sceneEditorBold, sceneEditorItalic uintptr
var sceneEditorSelection = -1
var sceneEditorDraft sceneLayer
var sceneEditorPreview *sceneRaster
var sceneEditorDPI int32
var sceneEditorCallback = syscall.NewCallback(sceneEditorProc)
var sceneEditorFieldLabels = []string{"文字", "图片路径", "字体", "颜色", "字号", "横向位置 %", "纵向位置 %", "宽度 %", "高度 %", "不透明度 %"}

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
		sceneEditorDPI = min(sceneEditorDPI, max(60, min((m.RcWork.Right-m.RcWork.Left-16)*96/690, (m.RcWork.Bottom-m.RcWork.Top-16)*96/539)))
	}
	name := utf16Ptr("ProjEZSceneEditor")
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), LpfnWndProc: sceneEditorCallback, HInstance: hInstance, LpszClassName: name, HbrBackground: 6, HIcon: appIcon}
	wc.HCursor, _, _ = procLoadCursorW.Call(0, IDC_ARROW)
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	var rect RECT
	user32.NewProc("GetWindowRect").Call(hwndMain, uintptr(unsafe.Pointer(&rect)))
	w, h := scenePixel(690), scenePixel(539)
	x, y := rect.Left+24, rect.Top+30
	if m, ok := windowMonitor(hwndMain); ok {
		x = max(m.RcWork.Left, min(x, m.RcWork.Right-w))
		y = max(m.RcWork.Top, min(y, m.RcWork.Bottom-h))
	}
	sceneEditor, _, _ = procCreateWindowExW.Call(0x00000001, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(utf16Ptr("演示图层"))), 0x00c80000, uintptr(int64(x)), uintptr(int64(y)), uintptr(w), uintptr(h), hwndMain, 0, hInstance, 0)
	if sceneEditor == 0 {
		setStatus("无法打开图层编辑器")
		return
	}
	sceneEditorFont = createFont(int(scenePixel(14)), 400, "Microsoft YaHei UI")
	if l := len(sceneLayers); l > 0 {
		sceneEditorSelection = l - 1
		sceneEditorDraft = sceneLayers[l-1]
	} else {
		sceneEditorSelection = -1
		sceneEditorDraft = defaultLayer()
	}
	buildSceneEditor()
	refreshSceneLayerList()
	setSceneEditorFields(sceneEditorDraft)
	renderSceneEditorPreview()
	user32.NewProc("EnableWindow").Call(hwndMain, 0)
	procShowWindow.Call(sceneEditor, SW_SHOW)
}
func scenePixel(v int32) int32 { return scaleAt(v, sceneEditorDPI) }
func editorControl(class, text string, style uintptr, x, y, w, h, id int) uintptr {
	d := int(sceneEditorDPI)
	scale := func(n int) int { return (n*d + 48) / 96 }
	c := createControl(class, text, style|WS_CHILD|WS_VISIBLE, scale(x), scale(y), scale(w), scale(h), sceneEditor, id)
	procSendMessageW.Call(c, WM_SETFONT, sceneEditorFont, 1)
	return c
}
func buildSceneEditor() {
	editorControl("STATIC", "图层（从底层到顶层）", 0, 18, 16, 198, 20, 0)
	sceneEditorList = editorControl("LISTBOX", "", 0x00800000|0x00200000|1|WS_TABSTOP, 18, 42, 206, 128, sceneList)
	button := func(label string, x, y, w, id int) { editorControl("BUTTON", label, WS_TABSTOP, x, y, w, 28, id) }
	button("添加文字", 18, 178, 98, sceneAddText)
	button("添加图片", 124, 178, 100, sceneAddImage)
	button("后移", 18, 214, 60, sceneLower)
	button("前移", 86, 214, 60, sceneRaise)
	button("删除", 154, 214, 70, sceneDelete)
	editorControl("STATIC", "图层预览", 0, 18, 260, 206, 22, 0)
	for i, label := range sceneEditorFieldLabels {
		y := 18 + i*36
		editorControl("STATIC", label, 0, 244, y+4, 92, 22, 0)
		w := 316
		if i == 1 {
			w = 240
		}
		field := editorControl("EDIT", "", 0x00800000|WS_TABSTOP|ES_AUTOHSCROLL, 340, y, w, 26, sceneFieldBase+i)
		sceneEditorFields[i] = field
		limits := []uintptr{1024, 2048, 96, 7, 3, 3, 3, 3, 3, 3}
		procSendMessageW.Call(field, 0x00c5, limits[i], 0)
	}
	button("选择图片", 586, 54, 70, sceneBrowse)
	sceneEditorBold = editorControl("BUTTON", "粗体", 3|WS_TABSTOP, 340, 378, 75, 24, sceneBold)
	sceneEditorItalic = editorControl("BUTTON", "斜体", 3|WS_TABSTOP, 420, 378, 75, 24, sceneItalic)
	sceneEditorStatus = editorControl("STATIC", "", 0, 18, 416, 638, 28, 0)
	button("预览修改", 354, 450, 96, sceneTry)
	button("保存图层", 458, 450, 96, sceneSave)
	button("关闭", 562, 450, 94, sceneClose)
}
func refreshSceneLayerList() {
	procSendMessageW.Call(sceneEditorList, 0x0184, 0, 0)
	for i, l := range sceneLayers {
		name := l.Text
		if l.File != "" {
			name = filepath.Base(l.File)
		}
		if len([]rune(name)) > 28 {
			name = string([]rune(name)[:28]) + "…"
		}
		procSendMessageW.Call(sceneEditorList, 0x0180, 0, uintptr(unsafe.Pointer(utf16Ptr(fmt.Sprintf("%d. %s", i+1, name)))))
	}
	if sceneEditorSelection >= 0 {
		procSendMessageW.Call(sceneEditorList, 0x0186, uintptr(sceneEditorSelection), 0)
	}
}
func setSceneEditorFields(l sceneLayer) {
	texts := []string{l.Text, l.File, l.Font, l.Color, strconv.Itoa(l.Size), strconv.Itoa(l.X), strconv.Itoa(l.Y), strconv.Itoa(l.W), strconv.Itoa(l.H), strconv.Itoa(l.Opacity)}
	for i, c := range sceneEditorFields {
		setText(c, texts[i])
	}
	for _, check := range []struct {
		h uintptr
		v bool
	}{{sceneEditorBold, l.Bold}, {sceneEditorItalic, l.Italic}} {
		value := uintptr(0)
		if check.v {
			value = 1
		}
		procSendMessageW.Call(check.h, 0xf1, value, 0)
	}
}
func readSceneEditorFields() (sceneLayer, error) {
	l := sceneEditorDraft
	l.Text, l.File, l.Font, l.Color = getText(sceneEditorFields[0]), strings.TrimSpace(getText(sceneEditorFields[1])), strings.TrimSpace(getText(sceneEditorFields[2])), strings.TrimSpace(getText(sceneEditorFields[3]))
	for j, v := range []*int{&l.Size, &l.X, &l.Y, &l.W, &l.H, &l.Opacity} {
		n, err := strconv.Atoi(strings.TrimSpace(getText(sceneEditorFields[4+j])))
		if err != nil {
			return l, fmt.Errorf("%s须填写整数", sceneEditorFieldLabels[4+j])
		}
		*v = n
	}
	bold, _, _ := procSendMessageW.Call(sceneEditorBold, 0xf0, 0, 0)
	italic, _, _ := procSendMessageW.Call(sceneEditorItalic, 0xf0, 0, 0)
	l.Bold, l.Italic = bold == 1, italic == 1
	return l, validateLayer(l)
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
	w, h := editorSceneSize()
	preview, err := rasterScene(sceneLayers, w, h)
	if err != nil {
		setText(sceneEditorStatus, err.Error())
		return
	}
	sceneEditorPreview = preview
	procInvalidateRect.Call(sceneEditor, 0, 0)
}
func saveEditorLayers(layers []sceneLayer, selection int) {
	w, h := editorSceneSize()
	preview, err := rasterScene(layers, w, h)
	if err == nil {
		err = saveScene(layers)
	}
	if err != nil {
		setText(sceneEditorStatus, err.Error())
		return
	}
	sceneLayers, sceneEditorSelection, sceneEditorPreview = layers, selection, preview
	refreshSceneLayerList()
	if selection >= 0 {
		sceneEditorDraft = sceneLayers[selection]
	} else {
		sceneEditorDraft = defaultLayer()
	}
	setSceneEditorFields(sceneEditorDraft)
	setText(sceneEditorStatus, "图层已保存，开始投影时生效")
	procInvalidateRect.Call(sceneEditor, 0, 0)
}

type sceneFileDialog struct {
	Size                         uint32
	Owner, Instance              uintptr
	Filter, CustomFilter         *uint16
	MaxCustomFilter, FilterIndex uint32
	File                         *uint16
	MaxFile                      uint32
	FileTitle                    *uint16
	MaxFileTitle                 uint32
	InitialDir, Title            *uint16
	Flags                        uint32
	FileOffset, Extension        uint16
	DefaultExt                   *uint16
	CustomData, Hook             uintptr
	Template                     *uint16
	Reserved                     uintptr
	ReservedFlags, FlagsEx       uint32
}

func sceneChooseImage() string {
	var buf [2048]uint16
	filter := []uint16{'P', 'N', 'G', ' ', '/', ' ', 'J', 'P', 'E', 'G', 0, '*', '.', 'p', 'n', 'g', ';', '*', '.', 'j', 'p', 'g', ';', '*', '.', 'j', 'p', 'e', 'g', 0, 0}
	d := sceneFileDialog{Owner: sceneEditor, File: &buf[0], MaxFile: uint32(len(buf)), Filter: &filter[0], FilterIndex: 1, Flags: 0x1808}
	d.Size = uint32(unsafe.Sizeof(d))
	ok, _, _ := syscall.NewLazyDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&d)))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}
func sceneEditorCommand(id, notify uint16) {
	switch id {
	case sceneList:
		if notify != 1 {
			return
		}
		i, _, _ := procSendMessageW.Call(sceneEditorList, 0x0188, 0, 0)
		if i < uintptr(len(sceneLayers)) {
			sceneEditorSelection = int(i)
			sceneEditorDraft = sceneLayers[i]
			setSceneEditorFields(sceneEditorDraft)
		}
	case sceneAddText, sceneAddImage:
		if len(sceneLayers) >= maxSceneLayers {
			setText(sceneEditorStatus, "最多支持 16 个图层")
			return
		}
		draft := defaultLayer()
		if id == sceneAddImage {
			draft.File = sceneChooseImage()
			if draft.File == "" {
				return
			}
			draft.W, draft.H = 30, 30
		} else {
			draft.Text = "演示标题"
		}
		sceneEditorSelection, sceneEditorDraft = -1, draft
		procSendMessageW.Call(sceneEditorList, 0x0186, ^uintptr(0), 0)
		setSceneEditorFields(draft)
		setText(sceneEditorStatus, "编辑后点击保存图层")
	case sceneBrowse:
		if path := sceneChooseImage(); path != "" {
			setText(sceneEditorFields[1], path)
		}
	case sceneSave, sceneTry:
		l, err := readSceneEditorFields()
		if err != nil {
			setText(sceneEditorStatus, err.Error())
			return
		}
		layers := append([]sceneLayer(nil), sceneLayers...)
		i := sceneEditorSelection
		if i < 0 {
			if len(layers) >= maxSceneLayers {
				setText(sceneEditorStatus, "最多支持 16 个图层")
				return
			}
			i = len(layers)
			layers = append(layers, l)
		} else {
			layers[i] = l
		}
		if id == sceneTry {
			w, h := editorSceneSize()
			preview, err := rasterScene(layers, w, h)
			if err != nil {
				setText(sceneEditorStatus, err.Error())
				return
			}
			sceneEditorPreview = preview
			setText(sceneEditorStatus, "当前为修改预览，点击保存图层应用")
			procInvalidateRect.Call(sceneEditor, 0, 0)
			return
		}
		saveEditorLayers(layers, i)
	case sceneDelete, sceneRaise, sceneLower:
		i := sceneEditorSelection
		if i < 0 || i >= len(sceneLayers) {
			return
		}
		layers := append([]sceneLayer(nil), sceneLayers...)
		if id == sceneDelete {
			layers = append(layers[:i], layers[i+1:]...)
			saveEditorLayers(layers, min(i, len(layers)-1))
			return
		}
		j := i + 1
		if id == sceneLower {
			j = i - 1
		}
		if j >= 0 && j < len(layers) {
			layers[i], layers[j] = layers[j], layers[i]
			saveEditorLayers(layers, j)
		}
	case sceneClose:
		procDestroyWindow.Call(sceneEditor)
	}
}
func sceneEditorProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		sceneEditorCommand(uint16(wp), uint16(wp>>16))
		return 0
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		procDeleteObject.Call(sceneEditorFont)
		sceneEditor, sceneEditorFont, sceneEditorPreview = 0, 0, nil
		user32.NewProc("EnableWindow").Call(hwndMain, 1)
		user32.NewProc("SetForegroundWindow").Call(hwndMain)
		procInvalidateRect.Call(hwndMain, 0, 0)
		return 0
	case WM_PAINT, 0x0318: // WM_PRINTCLIENT uses the supplied DC for screenshots.
		dc := wp
		if msg == WM_PAINT {
			var ps PAINTSTRUCT
			dc, _, _ = procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		if dc == 0 {
			return 0
		}
		r := RECT{scenePixel(18), scenePixel(284), scenePixel(224), scenePixel(408)}
		black, _, _ := procGetStockObject.Call(4)
		procFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), black)
		if f := sceneEditorPreview; f != nil {
			w, h := fitRect(f.W, f.H, r.Right-r.Left, r.Bottom-r.Top)
			x, y := r.Left+(r.Right-r.Left-w)/2, r.Top+(r.Bottom-r.Top-h)/2
			procSetStretchBltMode.Call(dc, HALFTONE)
			procSetBrushOrgEx.Call(dc, 0, 0, 0)
			b := bitmapHeader{Size: 40, Width: f.W, Height: -f.H, Planes: 1, Bits: 32}
			gdi32.NewProc("StretchDIBits").Call(dc, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, uintptr(f.W), uintptr(f.H), uintptr(unsafe.Pointer(&f.Pixels[0])), uintptr(unsafe.Pointer(&b)), 0, SRCCOPY)
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
