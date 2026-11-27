package main

import (
	"syscall"
	"unsafe"
)

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
