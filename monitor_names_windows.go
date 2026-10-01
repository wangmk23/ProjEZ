package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

type displayLUID struct {
	Low  uint32
	High int32
}
type displayNameHeader struct {
	Type, Size uint32
	Adapter    displayLUID
	ID         uint32
}
type displaySourceName struct {
	Header displayNameHeader
	Name   [32]uint16
}
type displayTargetName struct {
	Header                displayNameHeader
	Flags, Technology     uint32
	Manufacturer, Product uint16
	Connector             uint32
	Name                  [64]uint16
	Path                  [128]uint16
}
type displayPath struct {
	SourceAdapter                      displayLUID
	SourceID, SourceMode, SourceStatus uint32
	TargetAdapter                      displayLUID
	TargetID                           uint32
	TargetRest                         [9]uint32
	Flags                              uint32
}

// Query the connected target's EDID/driver name, mapped to its GDI display ID.
// https://learn.microsoft.com/windows/win32/api/wingdi/ns-wingdi-displayconfig_target_device_name
func connectedMonitorNames() map[string]string {
	result := map[string]string{}
	for attempt := 0; attempt < 3; attempt++ {
		var pathsCount, modesCount uint32
		code, _, _ := user32.NewProc("GetDisplayConfigBufferSizes").Call(2, uintptr(unsafe.Pointer(&pathsCount)), uintptr(unsafe.Pointer(&modesCount)))
		if code != 0 || pathsCount == 0 || pathsCount > 1024 || modesCount > 4096 {
			return result
		}
		paths := make([]displayPath, pathsCount)
		// DISPLAYCONFIG_MODE_INFO is 64 bytes with 8-byte alignment.
		modes := make([][8]uint64, modesCount+1)
		code, _, _ = user32.NewProc("QueryDisplayConfig").Call(2, uintptr(unsafe.Pointer(&pathsCount)), uintptr(unsafe.Pointer(&paths[0])), uintptr(unsafe.Pointer(&modesCount)), uintptr(unsafe.Pointer(&modes[0])), 0)
		if code == 122 {
			continue
		} // A display was connected during enumeration.
		if code != 0 {
			return result
		}
		for _, p := range paths[:pathsCount] {
			source := displaySourceName{Header: displayNameHeader{1, uint32(unsafe.Sizeof(displaySourceName{})), p.SourceAdapter, p.SourceID}}
			target := displayTargetName{Header: displayNameHeader{2, uint32(unsafe.Sizeof(displayTargetName{})), p.TargetAdapter, p.TargetID}}
			a, _, _ := user32.NewProc("DisplayConfigGetDeviceInfo").Call(uintptr(unsafe.Pointer(&source)))
			b, _, _ := user32.NewProc("DisplayConfigGetDeviceInfo").Call(uintptr(unsafe.Pointer(&target)))
			if a != 0 || b != 0 {
				continue
			}
			name := strings.TrimSpace(syscall.UTF16ToString(target.Name[:]))
			if name != "" {
				result[syscall.UTF16ToString(source.Name[:])] = name
			}
		}
		return result
	}
	return result
}

func monitorChoiceLabel(m Monitor) string {
	id := strings.TrimPrefix(m.Device, `\\.\`)
	name := strings.TrimSpace(m.Name)
	if name == "" {
		name = id
	} else {
		name += " (" + id + ")"
	}
	role := "副屏"
	if m.Primary {
		role = "主屏"
	}
	return fmt.Sprintf("%s · %d × %d · %s", name, m.Rect.Right-m.Rect.Left, m.Rect.Bottom-m.Rect.Top, role)
}
