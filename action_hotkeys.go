package main

import "fmt"

const hotkeyStop = 1005

var actionHotkeyIDs = []int{HOTKEY_FREEZE, HOTKEY_RESUME, hotkeyBlack, hotkeyStop}
var actionHotkeyNames = []string{"冻结", "恢复", "黑屏", "停止投影"}
var actionHotkeyOK [4]bool
var appliedActionKeys [4]Hotkey
var registerActionHotkey = func(id int, key Hotkey) bool {
	ok, _, _ := procRegisterHotKey.Call(hwndMain, uintptr(id), uintptr(key.Mods|0x4000), uintptr(key.VK))
	return ok != 0
}

func isHotkeyControl(id int) bool {
	return id == IDC_FREEZE_HOTKEY || id == IDC_RESUME_HOTKEY || id == IDC_BLACK_HOTKEY || id == IDC_STOP_HOTKEY
}
func validateActionHotkeys(texts []string) ([]Hotkey, error) {
	if len(texts) != 4 {
		return nil, fmt.Errorf("需要设置四项快捷键")
	}
	keys := make([]Hotkey, 4)
	for i, text := range texts {
		key, err := parseHotkey(text)
		if err != nil {
			return nil, fmt.Errorf("%s快捷键格式错误", actionHotkeyNames[i])
		}
		if key.Mods&^MOD_NOREPEAT == 3 && key.VK == 0x7a {
			return nil, fmt.Errorf("Ctrl+Alt+F11 用于释放鼠标，请换一个组合键")
		}
		for j := 0; j < i; j++ {
			if key.Mods == keys[j].Mods && key.VK == keys[j].VK {
				return nil, fmt.Errorf("%s与%s快捷键相同", actionHotkeyNames[i], actionHotkeyNames[j])
			}
		}
		keys[i] = key
	}
	return keys, nil
}

func tryActionRegistrations(keys []Hotkey) (results [4]bool) {
	for i, id := range actionHotkeyIDs {
		results[i] = registerActionHotkey(id, keys[i])
	}
	return
}
