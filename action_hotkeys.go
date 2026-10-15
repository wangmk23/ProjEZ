package main

import (
	"fmt"
	"syscall"
)

const hotkeyStop = 1005

var actionHotkeyIDs = []int{HOTKEY_FREEZE, HOTKEY_RESUME, hotkeyBlack, hotkeyStop}
var actionHotkeyNames = []string{"冻结", "恢复", "黑屏", "停止投影"}
var actionHotkeyOK [4]bool
var appliedActionKeys [4]Hotkey
var actionHotkeyErrors [4]uint32
var savedActionHotkeyTexts = [4]string{"Ctrl+Alt+F8", "Ctrl+Alt+F9", "Ctrl+Alt+F10", "Ctrl+Alt+F12"}
var registerActionHotkey = func(id int, key Hotkey) bool {
	ok, _, err := procRegisterHotKey.Call(hwndMain, uintptr(id), uintptr(key.Mods|0x4000), uintptr(key.VK))
	for i, actionID := range actionHotkeyIDs {
		if id == actionID {
			actionHotkeyErrors[i] = 0
			if code, isErrno := err.(syscall.Errno); ok == 0 && isErrno {
				actionHotkeyErrors[i] = uint32(code)
			}
		}
	}
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
		if reason := actionHotkeyInputError(i, texts); reason != "" {
			return nil, fmt.Errorf("%s快捷键（%s）：%s", actionHotkeyNames[i], text, reason)
		}
		keys[i], _ = parseHotkey(text)
	}
	return keys, nil
}

func tryActionRegistrations(keys []Hotkey) (results [4]bool) {
	for i, id := range actionHotkeyIDs {
		actionHotkeyErrors[i] = 0
		results[i] = registerActionHotkey(id, keys[i])
	}
	return
}

// Input validation is shared by live row hints and the save dialog.
func actionHotkeyInputError(index int, texts []string) string {
	if len(texts) != 4 || index < 0 || index >= 4 {
		return "需要设置四项快捷键"
	}
	key, err := parseHotkey(texts[index])
	if err != nil {
		return "格式错误，请重新按组合键"
	}
	if key.Mods&^MOD_NOREPEAT == MOD_CONTROL|MOD_ALT && key.VK == 0x7a {
		return "用于释放鼠标，请更换组合键"
	}
	for j, text := range texts {
		other, err := parseHotkey(text)
		if j != index && err == nil && key.Mods == other.Mods && key.VK == other.VK {
			return "与" + actionHotkeyNames[j] + "快捷键重复，请更换"
		}
	}
	return ""
}
func actionHotkeyRegistrationMessage(index int, text string) string {
	prefix := actionHotkeyNames[index] + "快捷键（" + text + "）："
	if actionHotkeyErrors[index] == 1409 {
		return prefix + "已被其他程序或旧版 ProjEZ 占用，请更换组合键"
	}
	if actionHotkeyErrors[index] != 0 {
		return fmt.Sprintf("%s注册失败（错误 %d），请更换组合键后重试", prefix, actionHotkeyErrors[index])
	}
	return prefix + "注册失败，请更换组合键后重试"
}
func actionHotkeyHint(index int, texts []string) (string, bool) {
	if reason := actionHotkeyInputError(index, texts); reason != "" {
		return reason, true
	}
	key, _ := parseHotkey(texts[index])
	if key.Mods != appliedActionKeys[index].Mods || key.VK != appliedActionKeys[index].VK {
		return "修改后请保存检查", false
	}
	if actionHotkeyOK[index] {
		return "已启用", false
	}
	if actionHotkeyErrors[index] == 1409 {
		return "已被占用，请更换组合键", true
	}
	return "注册失败，请更换组合键后重试", true
}
func actionHotkeyTexts() []string {
	return []string{getText(editFreeze), getText(editResume), getText(editBlack), getText(editStop)}
}
func hotkeyTextsForSave(texts []string) [4]string {
	keys, err := validateActionHotkeys(texts)
	if err != nil || recordingEdit != 0 {
		return savedActionHotkeyTexts
	}
	for i, key := range keys {
		if !actionHotkeyOK[i] || key.Mods != appliedActionKeys[i].Mods || key.VK != appliedActionKeys[i].VK {
			return savedActionHotkeyTexts
		}
	}
	return [4]string{texts[0], texts[1], texts[2], texts[3]}
}
