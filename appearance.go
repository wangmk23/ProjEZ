package main

import "unsafe"

var fontNavIcon uintptr

func uiTextColor() uint32 {
	if preferences.LightTheme {
		return rgb(30, 30, 30)
	}
	return rgb(240, 240, 240)
}
func uiMutedColor() uint32 {
	if preferences.LightTheme {
		return rgb(94, 94, 94)
	}
	return rgb(180, 180, 180)
}
func uiDisabledColor() uint32 {
	if preferences.LightTheme {
		return rgb(150, 150, 150)
	}
	return rgb(112, 112, 112)
}
func uiControlColor() uint32 {
	if preferences.LightTheme {
		return rgb(246, 246, 246)
	}
	return rgb(57, 57, 57)
}
func applyPalette() {
	values := [][3]byte{{36, 36, 36}, {31, 31, 31}, {45, 45, 45}, {57, 57, 57}, {66, 66, 66}}
	if preferences.LightTheme {
		values = [][3]byte{{243, 243, 243}, {237, 237, 237}, {255, 255, 255}, {246, 246, 246}, {213, 213, 213}}
	}
	for i, p := range []*uintptr{&brushBG, &brushSidebar, &brushCard, &brushControl, &brushMuted} {
		old := *p
		c := values[i]
		*p = makeBrush(c[0], c[1], c[2])
		if old != 0 {
			procDeleteObject.Call(old)
		}
	}
	old := penBorder
	if preferences.LightTheme {
		penBorder = makePen(220, 220, 220)
	} else {
		penBorder = makePen(58, 58, 58)
	}
	if old != 0 {
		procDeleteObject.Call(old)
	}
	colors := [][3]byte{{45, 128, 237}, {128, 91, 224}, {13, 148, 136}}
	c := colors[preferences.Accent]
	base := values[2]
	old = brushActive
	brushActive = makeBrush(byte((int(c[0])+4*int(base[0]))/5), byte((int(c[1])+4*int(base[1]))/5), byte((int(c[2])+4*int(base[2]))/5))
	if old != 0 {
		procDeleteObject.Call(old)
	}
	if hwndMain != 0 {
		dark := int32(1)
		if preferences.LightTheme {
			dark = 0
		}
		procDwmSetWindowAttribute.Call(hwndMain, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&dark)), 4)
		user32.NewProc("RedrawWindow").Call(hwndMain, 0, 0, 0x485)
		for popup := range dropdownProcs {
			user32.NewProc("RedrawWindow").Call(popup, 0, 0, 0x485)
		}
	}
}
func setLightTheme(light bool) { preferences.LightTheme = light; applyPalette(); saveConfig() }
