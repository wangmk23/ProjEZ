package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// ABI: Microsoft Windows.Graphics.Capture metadata; HWND interop requires Windows 10 1903+.
var combase = syscall.NewLazyDLL("combase.dll")

func captureGUID(s string) syscall.GUID {
	var g syscall.GUID
	hr, _, _ := syscall.NewLazyDLL("ole32.dll").NewProc("CLSIDFromString").Call(uintptr(unsafe.Pointer(utf16Ptr(s))), uintptr(unsafe.Pointer(&g)))
	if int32(hr) < 0 {
		panic("invalid capture IID")
	}
	return g
}

var iidCaptureItem = captureGUID("{79c3f95b-31f7-4ec2-a464-632ef5d30760}")
var iidCaptureInterop = captureGUID("{3628e81b-3cac-4c60-b7f4-23ce0e0c3356}")
var iidFramePoolStatics = captureGUID("{589b103f-6bbc-5df5-a991-02e28b3b66d5}")
var iidClosable = captureGUID("{30d5a829-7fa4-4026-83bb-d75bae4ea99e}")
var iidSurfaceAccess = captureGUID("{a9b3d012-3df2-4ee3-b8d1-8695f457d3c1}")
var iidWinRTDevice = captureGUID("{a37624ab-8d5f-4650-9d3e-9eae3d9bc670}")
var iidDXGIDevice = captureGUID("{54ec77fa-1377-44e6-8c32-88fd5f44c84c}")
var iidMultithread = captureGUID("{9b7e4e00-342c-4106-a19f-4f2704f689f0}")
var iidCaptureCursor = captureGUID("{2c39ae40-7d2e-5044-804e-8b6799d4cf9e}")

type captureSize struct{ W, H int32 }

func (s captureSize) packed() uintptr { return uintptr(uint32(s.W)) | uintptr(uint32(s.H))<<32 }
func (s captureSize) valid() bool     { return s.W > 0 && s.H > 0 && s.W <= 16384 && s.H <= 16384 }

type windowCapture struct {
	cursorSet, cursorEnabled           bool
	hwnd                               uintptr
	pid                                uint32
	item, pool, session, device, frame comPtr
	size                               captureSize
	initialized                        bool
	after                              int64
}

func captureFactory(name string, iid *syscall.GUID) (comPtr, error) {
	chars := syscall.StringToUTF16(name)
	var hs uintptr
	hr, _, _ := combase.NewProc("WindowsCreateString").Call(uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)-1), uintptr(unsafe.Pointer(&hs)))
	if e := hrError("WindowsCreateString", hr); e != nil {
		return nil, e
	}
	defer combase.NewProc("WindowsDeleteString").Call(hs)
	var factory comPtr
	hr, _, _ = combase.NewProc("RoGetActivationFactory").Call(hs, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&factory)))
	return factory, hrError("窗口捕获接口不可用", hr)
}
func closeInspectable(p *comPtr) {
	if *p != nil {
		if c, e := queryCOM(*p, &iidClosable); e == nil {
			comCall(c, 6)
			releaseCOM(&c)
		}
		releaseCOM(p)
	}
}
func (w *windowCapture) close() {
	closeInspectable(&w.frame)
	closeInspectable(&w.session)
	closeInspectable(&w.pool)
	releaseCOM(&w.item)
	releaseCOM(&w.device)
	if w.initialized {
		combase.NewProc("RoUninitialize").Call()
		w.initialized = false
	}
}
func (w *windowCapture) setCursor(on bool) {
	if w.cursorSet && w.cursorEnabled == on {
		return
	}
	w.cursorSet, w.cursorEnabled = true, on
	if c, e := queryCOM(w.session, &iidCaptureCursor); e == nil {
		v := uintptr(0)
		if on {
			v = 1
		}
		comCall(c, 7, v)
		releaseCOM(&c)
	}
}
func captureClock() int64 {
	var count, freq int64
	kernel32.NewProc("QueryPerformanceCounter").Call(uintptr(unsafe.Pointer(&count)))
	kernel32.NewProc("QueryPerformanceFrequency").Call(uintptr(unsafe.Pointer(&freq)))
	if freq <= 0 {
		return 0
	}
	return count/freq*10000000 + (count%freq)*10000000/freq
}
func openWindowCapture(hwnd uintptr, pid uint32, targetDevice string, cursor bool) (d *desktopDuplication, err error) {
	d = &desktopDuplication{window: &windowCapture{hwnd: hwnd, pid: pid}}
	defer func() {
		if err != nil {
			d.close()
		}
	}()
	w := d.window
	if hwnd == 0 || windowPID(hwnd) != pid {
		return d, fmt.Errorf("所选窗口已关闭，请重新选择")
	}
	hr, _, _ := combase.NewProc("RoInitialize").Call(1)
	if err = hrError("RoInitialize", hr); err != nil {
		return
	}
	w.initialized = true
	err = walkOutputs(func(a, o comPtr, info gpuOutput) error {
		if info.Device != targetDevice {
			return nil
		}
		if info.HDR {
			return fmt.Errorf("窗口投影暂不支持HDR输出")
		}
		comCall(a, 1)
		d.adapter = a
		d.output = info
		d.output.Rotation = 1
		var e error
		d.device, d.context, e = createD3DDevice(a)
		return e
	})
	if err != nil {
		return
	}
	if d.device == nil {
		return d, fmt.Errorf("找不到观众屏幕显卡")
	}
	d.factory, err = makeDXGIFactory()
	if err != nil {
		return
	}
	var multi comPtr
	multi, err = queryCOM(d.context, &iidMultithread)
	if err != nil {
		return
	}
	comCall(multi, 5, 1)
	releaseCOM(&multi)
	var dxgi comPtr
	dxgi, err = queryCOM(d.device, &iidDXGIDevice)
	if err != nil {
		return
	}
	defer releaseCOM(&dxgi)
	var inspect comPtr
	hr, _, _ = syscall.NewLazyDLL("d3d11.dll").NewProc("CreateDirect3D11DeviceFromDXGIDevice").Call(uintptr(unsafe.Pointer(dxgi)), uintptr(unsafe.Pointer(&inspect)))
	if err = hrError("CreateDirect3D11DeviceFromDXGIDevice", hr); err != nil {
		return
	}
	defer releaseCOM(&inspect)
	w.device, err = queryCOM(inspect, &iidWinRTDevice)
	if err != nil {
		return
	}
	var interop comPtr
	interop, err = captureFactory("Windows.Graphics.Capture.GraphicsCaptureItem", &iidCaptureInterop)
	if err != nil {
		return
	}
	defer releaseCOM(&interop)
	err = hrError("无法捕获此窗口", comCall(interop, 3, hwnd, uintptr(unsafe.Pointer(&iidCaptureItem)), uintptr(unsafe.Pointer(&w.item))))
	if err != nil {
		return
	}
	err = hrError("窗口尺寸", comCall(w.item, 7, uintptr(unsafe.Pointer(&w.size))))
	if err != nil {
		return
	}
	if !w.size.valid() {
		return d, fmt.Errorf("请先还原所选窗口")
	}
	var factory comPtr
	factory, err = captureFactory("Windows.Graphics.Capture.Direct3D11CaptureFramePool", &iidFramePoolStatics)
	if err != nil {
		return
	}
	defer releaseCOM(&factory)
	err = hrError("窗口帧池", comCall(factory, 6, uintptr(unsafe.Pointer(w.device)), 87, 2, w.size.packed(), uintptr(unsafe.Pointer(&w.pool))))
	if err != nil {
		return
	}
	err = hrError("窗口捕获会话", comCall(w.pool, 10, uintptr(unsafe.Pointer(w.item)), uintptr(unsafe.Pointer(&w.session))))
	if err != nil {
		return
	}
	w.setCursor(cursor)
	err = hrError("开始窗口捕获", comCall(w.session, 6))
	return
}
func (d *desktopDuplication) acquireWindow() (desktopFrame, bool, error) {
	w := d.window
	if windowPID(w.hwnd) != w.pid {
		return desktopFrame{}, false, fmt.Errorf("演示窗口已关闭")
	}
	if minimized, _, _ := user32.NewProc("IsIconic").Call(w.hwnd); minimized != 0 {
		return desktopFrame{}, false, fmt.Errorf("演示窗口已最小化，请还原后重新开始")
	}
	if e := hrError("读取窗口画面", comCall(w.pool, 7, uintptr(unsafe.Pointer(&w.frame)))); e != nil {
		return desktopFrame{}, false, e
	}
	if w.frame == nil {
		return desktopFrame{}, false, nil
	}
	d.held = true
	fail := func(e error) (desktopFrame, bool, error) { d.releaseFrame(); return desktopFrame{}, false, e }
	var size captureSize
	if e := hrError("窗口画面尺寸", comCall(w.frame, 8, uintptr(unsafe.Pointer(&size)))); e != nil {
		return fail(e)
	}
	if !size.valid() {
		return fail(fmt.Errorf("窗口尺寸不可用"))
	}
	if size != w.size {
		d.releaseFrame()
		w.size = size
		e := hrError("调整窗口画面", comCall(w.pool, 6, uintptr(unsafe.Pointer(w.device)), 87, 2, size.packed()))
		if e == nil {
			procInvalidateRect.Call(w.hwnd, 0, 0)
		}
		return desktopFrame{}, false, e
	}
	var timestamp int64
	if e := hrError("窗口画面时间", comCall(w.frame, 7, uintptr(unsafe.Pointer(&timestamp)))); e != nil {
		return fail(e)
	}
	if timestamp < w.after {
		return fail(nil)
	}
	var surface comPtr
	if e := hrError("窗口纹理", comCall(w.frame, 6, uintptr(unsafe.Pointer(&surface)))); e != nil {
		return fail(e)
	}
	defer releaseCOM(&surface)
	access, e := queryCOM(surface, &iidSurfaceAccess)
	if e != nil {
		return fail(e)
	}
	defer releaseCOM(&access)
	e = hrError("窗口D3D纹理", comCall(access, 3, uintptr(unsafe.Pointer(&iidTexture2D)), uintptr(unsafe.Pointer(&d.texture))))
	if e != nil {
		return fail(e)
	}
	return desktopFrame{Texture: d.texture, Width: uint32(size.W), Height: uint32(size.H), DesktopChanged: true}, true, nil
}
