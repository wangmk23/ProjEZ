//go:build windows && amd64

package main

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

type comObject struct{ vtable *[128]uintptr }
type comPtr = *comObject

//go:uintptrescapes
func comCall(obj comPtr, slot int, args ...uintptr) uintptr {
	if obj == nil {
		return 0x80004003
	}
	a := append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)
	r, _, _ := syscall.SyscallN(obj.vtable[slot], a...)
	runtime.KeepAlive(obj)
	return r
}
func releaseCOM(p *comPtr) {
	if *p != nil {
		comCall(*p, 2)
		*p = nil
	}
}
func queryCOM(p comPtr, id *syscall.GUID) (comPtr, error) {
	var out comPtr
	hr := comCall(p, 0, uintptr(unsafe.Pointer(id)), uintptr(unsafe.Pointer(&out)))
	return out, hrError("QueryInterface", hr)
}
func hrError(op string, hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("%s: HRESULT 0x%08X", op, uint32(hr))
	}
	return nil
}

var (
	iidFactory1  = syscall.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
	iidFactory2  = syscall.GUID{Data1: 0x50c83a1c, Data2: 0xe072, Data3: 0x4c48, Data4: [8]byte{0x87, 0xb0, 0x36, 0x30, 0xfa, 0x36, 0xa6, 0xd0}}
	iidOutput1   = syscall.GUID{Data1: 0x00cddea8, Data2: 0x939b, Data3: 0x4b83, Data4: [8]byte{0xa3, 0x40, 0xa6, 0x85, 0x22, 0x66, 0x66, 0xcc}}
	iidOutput6   = syscall.GUID{Data1: 0x068346e8, Data2: 0xaaec, Data3: 0x4b84, Data4: [8]byte{0xad, 0xd7, 0x13, 0x7f, 0x51, 0x3f, 0x77, 0xa1}}
	iidTexture2D = syscall.GUID{Data1: 0x6f15aaf2, Data2: 0xd208, Data3: 0x4e89, Data4: [8]byte{0x9a, 0xb4, 0x48, 0x95, 0x35, 0xd3, 0x4f, 0x9c}}
)

type dxgiOutputDesc struct {
	Name     [32]uint16
	Rect     RECT
	Attached int32
	Rotation uint32
	Monitor  uintptr
}
type outputDesc1 struct {
	dxgiOutputDesc
	Bits, ColorSpace uint32
	Primaries        [8]float32
	Min, Max, Full   float32
	_                uint32
}
type adapterDesc struct {
	Name                  [128]uint16
	IDs                   [4]uint32
	Video, System, Shared uintptr
	LUID                  uint64
}
type pointerShapeInfo struct {
	Type, Width, Height, Pitch uint32
	Hotspot                    POINT
}
type dupFrameInfo struct {
	PresentTime, MouseTime  int64
	Accumulated             uint32
	Coalesced, Protected    int32
	Position                POINT
	Visible                 int32
	MetadataSize, ShapeSize uint32
}
type textureDesc struct{ Width, Height, Mips, ArraySize, Format, SampleCount, SampleQuality, Usage, Bind, CPUAccess, Misc uint32 }
type gpuOutput struct {
	Device      string
	AdapterLUID uint64
	Rect        RECT
	Rotation    uint32
	HDR         bool
}
type desktopFrame struct {
	Texture        comPtr
	Width, Height  uint32
	DesktopChanged bool
	Info           dupFrameInfo
}
type desktopDuplication struct {
	window                                          *windowCapture
	factory, adapter, device, context, dup, texture comPtr
	output                                          gpuOutput
	held                                            bool
	shape                                           pointerShapeInfo
	pointer                                         []byte
}

func selectGPUOutput(outputs []gpuOutput, device string) (gpuOutput, error) {
	for _, o := range outputs {
		if strings.EqualFold(o.Device, device) {
			return o, nil
		}
	}
	return gpuOutput{}, fmt.Errorf("未找到屏幕 %s", device)
}
func makeDXGIFactory() (comPtr, error) {
	proc := syscall.NewLazyDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
	if err := proc.Find(); err != nil {
		return nil, err
	}
	var f comPtr
	hr, _, _ := proc.Call(uintptr(unsafe.Pointer(&iidFactory1)), uintptr(unsafe.Pointer(&f)))
	return f, hrError("CreateDXGIFactory1", hr)
}
func walkOutputs(visit func(comPtr, comPtr, gpuOutput) error) error {
	f, err := makeDXGIFactory()
	if err != nil {
		return err
	}
	defer releaseCOM(&f)
	for ai := uintptr(0); ; ai++ {
		var a comPtr
		hr := comCall(f, 7, ai, uintptr(unsafe.Pointer(&a)))
		if uint32(hr) == 0x887a0002 {
			return nil
		}
		if err := hrError("EnumAdapters", hr); err != nil {
			return err
		}
		err = func() error {
			defer releaseCOM(&a)
			var ad adapterDesc
			if err := hrError("Adapter.GetDesc", comCall(a, 8, uintptr(unsafe.Pointer(&ad)))); err != nil {
				return err
			}
			for oi := uintptr(0); ; oi++ {
				var o comPtr
				hr := comCall(a, 7, oi, uintptr(unsafe.Pointer(&o)))
				if uint32(hr) == 0x887a0002 {
					return nil
				}
				if err := hrError("EnumOutputs", hr); err != nil {
					return err
				}
				err := func() error {
					defer releaseCOM(&o)
					var desc dxgiOutputDesc
					if err := hrError("Output.GetDesc", comCall(o, 7, uintptr(unsafe.Pointer(&desc)))); err != nil {
						return err
					}
					if desc.Attached == 0 {
						return nil
					}
					info := gpuOutput{Device: syscall.UTF16ToString(desc.Name[:]), AdapterLUID: ad.LUID, Rect: desc.Rect, Rotation: desc.Rotation}
					if o6, e := queryCOM(o, &iidOutput6); e == nil {
						var d outputDesc1
						if hrError("GetDesc1", comCall(o6, 27, uintptr(unsafe.Pointer(&d)))) == nil {
							info.HDR = d.ColorSpace != 0
						}
						releaseCOM(&o6)
					}
					return visit(a, o, info)
				}()
				if err != nil {
					return err
				}
			}
		}()
		if err != nil {
			return err
		}
	}
}
func enumerateGPUOutputs() ([]gpuOutput, error) {
	var out []gpuOutput
	err := walkOutputs(func(a, o comPtr, info gpuOutput) error { out = append(out, info); return nil })
	return out, err
}
func createD3DDevice(adapter comPtr) (comPtr, comPtr, error) {
	var device, ctx comPtr
	var feature uint32
	levels := []uint32{0xb000, 0xa100, 0xa000}
	proc := syscall.NewLazyDLL("d3d11.dll").NewProc("D3D11CreateDevice")
	if err := proc.Find(); err != nil {
		return nil, nil, err
	}
	hr, _, _ := proc.Call(uintptr(unsafe.Pointer(adapter)), 0, 0, 0x20, uintptr(unsafe.Pointer(&levels[0])), uintptr(len(levels)), 7, uintptr(unsafe.Pointer(&device)), uintptr(unsafe.Pointer(&feature)), uintptr(unsafe.Pointer(&ctx)))
	runtime.KeepAlive(levels)
	if err := hrError("D3D11CreateDevice", hr); err != nil {
		releaseCOM(&ctx)
		releaseCOM(&device)
		return nil, nil, err
	}
	return device, ctx, nil
}
func openDuplication(device string) (*desktopDuplication, error) {
	d := &desktopDuplication{}
	err := walkOutputs(func(a, o comPtr, info gpuOutput) error {
		if !strings.EqualFold(info.Device, device) {
			return nil
		}
		d.output = info
		if info.HDR {
			return fmt.Errorf("来源为 HDR，当前 GPU 核心仅支持 SDR")
		}
		comCall(a, 1)
		d.adapter = a
		var err error
		d.device, d.context, err = createD3DDevice(a)
		if err != nil {
			return err
		}
		out1, err := queryCOM(o, &iidOutput1)
		if err != nil {
			return err
		}
		defer releaseCOM(&out1)
		return hrError("DuplicateOutput", comCall(out1, 22, uintptr(unsafe.Pointer(d.device)), uintptr(unsafe.Pointer(&d.dup))))
	})
	if err == nil && d.dup == nil {
		err = fmt.Errorf("来源屏幕不可用于 DXGI 捕获")
	}
	if err != nil {
		d.close()
		return nil, err
	}
	d.factory, err = makeDXGIFactory()
	if err != nil {
		d.close()
		return nil, err
	}
	return d, nil
}
func (d *desktopDuplication) acquire(timeoutMS uint32) (desktopFrame, bool, error) {
	if d.window != nil {
		return d.acquireWindow()
	}
	if d.held {
		return desktopFrame{}, false, fmt.Errorf("上一帧尚未释放")
	}
	var info dupFrameInfo
	var resource comPtr
	hr := comCall(d.dup, 8, uintptr(timeoutMS), uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&resource)))
	if uint32(hr) == 0x887a0027 {
		return desktopFrame{}, false, nil
	}
	if err := hrError("AcquireNextFrame", hr); err != nil {
		return desktopFrame{}, false, err
	}
	d.held = true
	defer releaseCOM(&resource)
	tex, err := queryCOM(resource, &iidTexture2D)
	if err != nil {
		d.releaseFrame()
		return desktopFrame{}, false, err
	}
	d.texture = tex
	var desc textureDesc
	comCall(tex, 10, uintptr(unsafe.Pointer(&desc)))
	if info.ShapeSize > 0 {
		if info.ShapeSize > 16*1024*1024 {
			d.releaseFrame()
			return desktopFrame{}, false, fmt.Errorf("鼠标形状过大")
		}
		d.pointer = make([]byte, info.ShapeSize)
		var written uint32
		hr = comCall(d.dup, 11, uintptr(len(d.pointer)), uintptr(unsafe.Pointer(&d.pointer[0])), uintptr(unsafe.Pointer(&written)), uintptr(unsafe.Pointer(&d.shape)))
		if err := hrError("GetFramePointerShape", hr); err != nil {
			d.releaseFrame()
			return desktopFrame{}, false, err
		}
	}
	return desktopFrame{Texture: tex, Width: desc.Width, Height: desc.Height, DesktopChanged: info.PresentTime != 0, Info: info}, true, nil
}
func (d *desktopDuplication) releaseFrame() error {
	releaseCOM(&d.texture)
	if d.window != nil {
		closeInspectable(&d.window.frame)
		d.held = false
		return nil
	}
	if !d.held {
		return nil
	}
	d.held = false
	return hrError("ReleaseFrame", comCall(d.dup, 14))
}
func (d *desktopDuplication) close() {
	d.releaseFrame()
	if d.window != nil {
		d.window.close()
		d.window = nil
	}
	releaseCOM(&d.dup)
	if d.context != nil {
		comCall(d.context, 110)
		comCall(d.context, 111)
	}
	releaseCOM(&d.context)
	releaseCOM(&d.device)
	releaseCOM(&d.adapter)
	releaseCOM(&d.factory)
}
