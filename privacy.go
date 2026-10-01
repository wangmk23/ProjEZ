package main

// Pure policy: the selected top-level window and its same-process owned dialogs only.
// A foreground return never clears the latch; only an explicit resume may do so.
type privacyGuard struct {
	enabled, latched bool
	target           uintptr
	pid              uint32
}

func (p *privacyGuard) allows(hwnd uintptr, pid uint32, owner uintptr, valid bool) bool {
	return valid && p.target != 0 && pid == p.pid && (hwnd == p.target || owner == p.target)
}
func (p *privacyGuard) observe(hwnd uintptr, pid uint32, owner uintptr, valid bool) bool {
	if !p.enabled || p.latched || p.allows(hwnd, pid, owner, valid) {
		return false
	}
	p.latched = true
	return true
}
func (p *privacyGuard) resume(hwnd uintptr, pid uint32, owner uintptr, valid bool) bool {
	if !p.enabled {
		return true
	}
	if !p.allows(hwnd, pid, owner, valid) {
		return false
	}
	p.latched = false
	return true
}
