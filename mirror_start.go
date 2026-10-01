package main

// Keep a valid audience selection, but always mirror the Windows primary screen.
func mirrorPair(screens []Monitor, preferred string) (int, int) {
	source, target := -1, -1
	for i, m := range screens {
		if m.Primary {
			source = i
			break
		}
	}
	if source < 0 {
		return -1, -1
	}
	for i, m := range screens {
		if outputPairError(screens[source], m) != "" {
			continue
		}
		if target < 0 {
			target = i
		}
		if m.Device == preferred {
			return source, i
		}
	}
	return source, target
}

func startPrimaryMirror() {
	if outputRunning || gpuActive != nil {
		return
	}
	refreshMonitors()
	source, target := mirrorPair(monitors, selectedDevice(cbOutput))
	if source < 0 || target < 0 {
		setStatus("请连接投影屏幕，并在 Win+P 中选择扩展")
		return
	}
	preferences.WindowMode = false
	preferences.Fill = false // Preserve the complete image without stretching.
	preferences.FreeCursor = false
	showCursor = true
	procSendMessageW.Call(cbSource, CB_SETCURSEL, uintptr(source), 0)
	procSendMessageW.Call(cbOutput, CB_SETCURSEL, uintptr(target), 0)
	procSendMessageW.Call(scaleCombo, CB_SETCURSEL, 0, 0)
	layoutControls(hwndMain)
	startOutput()
}
