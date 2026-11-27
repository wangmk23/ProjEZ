package main

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const maxSceneLayers = 16

type sceneLayer struct {
	Text, File, Font, Color   string
	Size, X, Y, W, H, Opacity int
	Bold, Italic              bool
}
type sceneRaster struct {
	W, H   int32
	Pixels []byte
}

var sceneLayers []sceneLayer
var sceneLoaded bool
var sceneLoadError error
var activeScene *sceneRaster
var sceneDC, sceneBitmap, sceneOld uintptr
var alphaBlend = syscall.NewLazyDLL("msimg32.dll").NewProc("AlphaBlend")

func defaultLayer() sceneLayer {
	return sceneLayer{Font: "Microsoft YaHei UI", Color: "#FFFFFF", Size: 42, X: 5, Y: 5, W: 60, H: 20, Opacity: 100}
}
func scenePath() string { return filepath.Join(filepath.Dir(configPath()), "scene.json") }
func loadScene() {
	if sceneLoaded {
		return
	}
	sceneLoaded = true
	data, err := os.ReadFile(scenePath())
	if os.IsNotExist(err) {
		return
	}
	if err == nil && len(data) > 256*1024 {
		err = fmt.Errorf("图层配置超过大小限制")
	}
	if err == nil {
		err = json.Unmarshal(data, &sceneLayers)
	}
	if err == nil && len(sceneLayers) > maxSceneLayers {
		err = fmt.Errorf("最多支持 %d 个图层", maxSceneLayers)
	}
	sceneLoadError = err
	if err != nil {
		sceneLayers = nil
	}
}
func saveScene(layers []sceneLayer) error {
	data, err := json.MarshalIndent(layers, "", "  ")
	if err != nil {
		return err
	}
	path := scenePath()
	tmp, err := os.CreateTemp(filepath.Dir(path), "scene-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}
func layerColor(text string) (uint32, error) {
	if len(text) != 7 || !strings.HasPrefix(text, "#") {
		return 0, fmt.Errorf("颜色请填写 #RRGGBB，例如 #FFFFFF")
	}
	n, err := strconv.ParseUint(text[1:], 16, 24)
	return uint32(n), err
}
func validateLayer(l sceneLayer) error {
	if l.Text == "" && l.File == "" {
		return fmt.Errorf("请输入文字或选择图片")
	}
	if len(l.Text) > 2048 || len(l.Font) > 120 || len(l.File) > 4096 {
		return fmt.Errorf("图层内容过长")
	}
	if l.X < 0 || l.Y < 0 || l.W <= 0 || l.H <= 0 || l.X >= 100 || l.Y >= 100 || l.W > 100-l.X || l.H > 100-l.Y || l.Opacity < 0 || l.Opacity > 100 || l.Size < 8 || l.Size > 200 {
		return fmt.Errorf("位置和尺寸须在画面内，字号 8–200，透明度 0–100")
	}
	_, err := layerColor(l.Color)
	return err
}
func createSceneDIB(w, h int32) (dc, bitmap, old uintptr, bits []byte, err error) {
	dc, _, _ = procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return 0, 0, 0, nil, fmt.Errorf("无法创建图层画布")
	}
	var ptr unsafe.Pointer
	header := bitmapHeader{Size: 40, Width: w, Height: -h, Planes: 1, Bits: 32}
	bitmap, _, _ = gdi32.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&ptr)), 0, 0)
	if bitmap != 0 {
		old, _, _ = procSelectObject.Call(dc, bitmap)
	}
	if bitmap == 0 || ptr == nil || old == 0 || old == ^uintptr(0) {
		if bitmap != 0 {
			procDeleteObject.Call(bitmap)
		}
		procDeleteDC.Call(dc)
		return 0, 0, 0, nil, fmt.Errorf("图层画布内存不足")
	}
	return dc, bitmap, old, unsafe.Slice((*byte)(ptr), int(w*h*4)), nil
}
func releaseSceneDIB(dc, bitmap, old uintptr) {
	if dc == 0 {
		return
	}
	procSelectObject.Call(dc, old)
	procDeleteObject.Call(bitmap)
	procDeleteDC.Call(dc)
}
func blendScenePixel(dst []byte, r, g, b, a uint32) {
	inverse := 255 - a
	dst[0] = byte(b + (uint32(dst[0])*inverse+127)/255)
	dst[1] = byte(g + (uint32(dst[1])*inverse+127)/255)
	dst[2] = byte(r + (uint32(dst[2])*inverse+127)/255)
	dst[3] = byte(a + (uint32(dst[3])*inverse+127)/255)
}
func paintSceneText(canvas *sceneRaster, l sceneLayer, rect RECT) error {
	w, h := rect.Right-rect.Left, rect.Bottom-rect.Top
	dc, bmp, old, mask, err := createSceneDIB(w, h)
	if err != nil {
		return err
	}
	defer releaseSceneDIB(dc, bmp, old)
	weight := uintptr(400)
	if l.Bold {
		weight = 700
	}
	italic := uintptr(0)
	if l.Italic {
		italic = 1
	}
	size := max(1, int32(l.Size)*canvas.H/720)
	font, _, _ := gdi32.NewProc("CreateFontW").Call(uintptr(int64(-size)), 0, 0, 0, weight, italic, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(utf16Ptr(l.Font))))
	if font == 0 {
		return fmt.Errorf("无法创建字体")
	}
	defer procDeleteObject.Call(font)
	gdi32.NewProc("SetTextAlign").Call(dc, 0)
	procSetBkMode.Call(dc, TRANSPARENT)
	black, _, _ := procGetStockObject.Call(4)
	local := RECT{0, 0, w, h}
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&local)), black)
	drawText(dc, l.Text, local, font, 0xffffff, 0x10|DT_NOPREFIX)
	gdi32.NewProc("GdiFlush").Call()
	color, _ := layerColor(l.Color)
	red, green, blue := color>>16, (color>>8)&255, color&255
	for y := int32(0); y < h; y++ {
		for x := int32(0); x < w; x++ {
			a := uint32(mask[(y*w+x)*4]) * uint32(l.Opacity) / 100
			if a == 0 {
				continue
			}
			off := ((y+rect.Top)*canvas.W + x + rect.Left) * 4
			blendScenePixel(canvas.Pixels[off:off+4], red*a/255, green*a/255, blue*a/255, a)
		}
	}
	return nil
}
func loadSceneImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > 32*1024*1024 {
		return nil, fmt.Errorf("图片须为 32 MB 以内的本地文件")
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return nil, fmt.Errorf("图片像素过大")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	return img, err
}
func paintSceneImage(canvas *sceneRaster, l sceneLayer, fill RECT) error {
	img, err := loadSceneImage(l.File)
	if err != nil {
		return err
	}
	b := img.Bounds()
	w, h := fitRect(int32(b.Dx()), int32(b.Dy()), fill.Right-fill.Left, fill.Bottom-fill.Top)
	if w <= 0 || h <= 0 {
		return nil
	}
	x0, y0 := fill.Left+(fill.Right-fill.Left-w)/2, fill.Top+(fill.Bottom-fill.Top-h)/2
	opacity := uint32(l.Opacity)
	for y := int32(0); y < h; y++ {
		for x := int32(0); x < w; x++ {
			r, g, blue, a := img.At(b.Min.X+int(x)*b.Dx()/int(w), b.Min.Y+int(y)*b.Dy()/int(h)).RGBA()
			off := ((y0+y)*canvas.W + x0 + x) * 4
			blendScenePixel(canvas.Pixels[off:off+4], (r>>8)*opacity/100, (g>>8)*opacity/100, (blue>>8)*opacity/100, (a>>8)*opacity/100)
		}
	}
	return nil
}
func rasterScene(layers []sceneLayer, w, h int32) (*sceneRaster, error) {
	if len(layers) == 0 {
		return nil, nil
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("无效画布尺寸")
	}
	if w > 1920 || h > 1080 {
		w, h = fitRect(w, h, 1920, 1080)
	}
	canvas := &sceneRaster{w, h, make([]byte, int(w*h*4))}
	for i, l := range layers {
		if err := validateLayer(l); err != nil {
			return nil, fmt.Errorf("第 %d 层：%w", i+1, err)
		}
		if l.Opacity == 0 {
			continue
		}
		r := RECT{w * int32(l.X) / 100, h * int32(l.Y) / 100, w * int32(l.X+l.W) / 100, h * int32(l.Y+l.H) / 100}
		if r.Right <= r.Left || r.Bottom <= r.Top {
			continue
		}
		var err error
		if l.File != "" {
			err = paintSceneImage(canvas, l, r)
		} else {
			err = paintSceneText(canvas, l, r)
		}
		if err != nil {
			return nil, fmt.Errorf("第 %d 层：%w", i+1, err)
		}
	}
	return canvas, nil
}
func releaseActiveScene() {
	releaseSceneDIB(sceneDC, sceneBitmap, sceneOld)
	sceneDC, sceneBitmap, sceneOld, activeScene = 0, 0, 0, nil
}
func prepareActiveScene(width, height int32) error {
	loadScene()
	if sceneLoadError != nil {
		return fmt.Errorf("无法读取图层配置：%w", sceneLoadError)
	}
	raster, err := rasterScene(sceneLayers, width, height)
	if err != nil || raster == nil {
		return err
	}
	dc, bmp, old, pixels, err := createSceneDIB(raster.W, raster.H)
	if err != nil {
		return err
	}
	copy(pixels, raster.Pixels)
	sceneDC, sceneBitmap, sceneOld, activeScene = dc, bmp, old, raster
	return nil
}
func blendActiveScene(dc uintptr, w, h int32) bool {
	if activeScene == nil {
		return true
	}
	ok, _, _ := alphaBlend.Call(dc, 0, 0, uintptr(w), uintptr(h), sceneDC, 0, 0, uintptr(activeScene.W), uintptr(activeScene.H), 0x01ff0000)
	return ok != 0
}
