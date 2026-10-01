//go:build windows && amd64

package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

const wmGPUEvent = 0x8003
const gpuPollTimer = 0x4750

type gpuSession struct {
	previewEnabled   bool
	previewFPS       int
	preview          *previewFrame
	previewError     string
	mu               sync.Mutex
	cfg              projectionConfig
	command          projectionCommand
	blackCommand     projectionCommand
	hasBlack         bool
	hasCommand, stop bool
	event            projectionEvent
	hasEvent         bool
	wake             chan struct{}
	done             chan struct{}
}

func newGPUMailbox(cfg projectionConfig) *gpuSession {
	return &gpuSession{cfg: cfg, wake: make(chan struct{}, 1), done: make(chan struct{})}
}
func (s *gpuSession) request(c projectionCommand) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Generation != s.cfg.Generation || s.stop {
		return false
	}
	if c.State == projectionStopping {
		s.stop = true
	}
	if c.State == projectionBlack {
		s.blackCommand = c
		s.hasBlack = true
		s.hasCommand = false
	} else {
		s.command = c
		s.hasCommand = true
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return true
}
func (s *gpuSession) nextCommand() (projectionCommand, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop {
		return projectionCommand{Generation: s.cfg.Generation, Sequence: s.command.Sequence, State: projectionStopping}, true
	}
	if s.hasBlack {
		s.hasBlack = false
		return s.blackCommand, true
	}
	c, ok := s.command, s.hasCommand
	s.hasCommand = false
	return c, ok
}
func (s *gpuSession) publish(e projectionEvent) {
	s.mu.Lock()
	s.event = e
	s.hasEvent = true
	s.mu.Unlock()
	// UI also polls the retained event, so a failed PostMessage cannot lose terminal state.
	procPostFrame.Call(s.cfg.HWND, wmGPUEvent, uintptr(s.cfg.Generation), 0)
}
func (s *gpuSession) drain() (projectionEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.event, s.hasEvent
	s.hasEvent = false
	return e, ok
}
func (s *gpuSession) wait(d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-s.wake:
	}
}
func startGPUSession(cfg projectionConfig) *gpuSession { s := newGPUMailbox(cfg); go s.run(); return s }
func (s *gpuSession) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cfg := s.cfg
	var seq uint64
	var d *desktopDuplication
	var r *gpuRenderer
	state := projectionStarting
	frozenState := false
	blackState, blackPending := cfg.StartBlack, cfg.StartBlack
	terminal := projectionEvent{Generation: cfg.Generation, Backend: "DirectX GPU", State: projectionStopped}
	cleanup := func() {
		if r != nil {
			r.close()
			r = nil
		}
		if d != nil {
			d.close()
			d = nil
		}
	}
	defer func() {
		cleanup()
		s.mu.Lock()
		s.stop = true
		s.mu.Unlock()
		terminal.Sequence = seq
		close(s.done)
		s.publish(terminal)
	}()
	fail := func(err error) {
		terminal.State = projectionFailed
		terminal.Reason = err.Error()
		terminal.Fallback = cfg.SourceWindow == 0 && !frozenState && !blackState
	}
	open := func() error {
		if cfg.SourceWindow != 0 {
			var err error
			d, err = openWindowCapture(cfg.SourceWindow, cfg.SourcePID, cfg.Target.Device, cfg.Cursor)
			if err != nil {
				return err
			}
			r, err = newGPURenderer(cfg.HWND, d, cfg)
			return err
		}
		outs, err := enumerateGPUOutputs()
		if err != nil {
			return err
		}
		src, err := selectGPUOutput(outs, cfg.Source.Device)
		if err != nil {
			return err
		}
		dst, err := selectGPUOutput(outs, cfg.Target.Device)
		if err != nil {
			return err
		}
		if src.AdapterLUID != dst.AdapterLUID {
			return fmt.Errorf("来源与目标连接到不同显卡，使用 GDI 兼容模式")
		}
		if src.HDR || dst.HDR {
			return fmt.Errorf("检测到 HDR，当前 GPU 核心仅支持 SDR；兼容模式不保证 HDR 色彩")
		}
		d, err = openDuplication(cfg.Source.Device)
		if err != nil {
			return err
		}
		r, err = newGPURenderer(cfg.HWND, d, cfg)
		return err
	}
	if err := open(); err != nil {
		fail(err)
		return
	}
	if _, err := r.presentBlack(); err != nil {
		fail(err)
		return
	}
	state = projectionStarting
	var captured, presented uint64
	start := time.Now()
	lastStat := start
	var countC, countP uint64
	dirty := false
	retries := 0
	lastFrame := time.Now()
	var frozenSnapshot []byte
	var snapW, snapH uint32
	announce := func(reason string) {
		now := time.Now()
		elapsed := now.Sub(lastStat).Seconds()
		e := projectionEvent{Generation: cfg.Generation, Sequence: seq, State: state, Backend: "DirectX GPU", Reason: reason, Captured: captured, Presented: presented}
		if elapsed > 0 {
			e.CaptureFPS = float64(captured-countC) / elapsed
			e.PresentFPS = float64(presented-countP) / elapsed
		}
		s.publish(e)
		lastStat = now
		countC = captured
		countP = presented
	}
	announce("")
	for {
		frameStart := time.Now()
		s.mu.Lock()
		stopping := s.stop
		s.mu.Unlock()
		var c projectionCommand
		var ok bool
		if !blackPending || stopping {
			c, ok = s.nextCommand()
		}
		if ok {
			seq = c.Sequence
			if c.State == projectionStopping {
				return
			}
			if c.Config.HWND != 0 {
				cfg = c.Config
				r.cfg = cfg
				dirty = true
				r.data.Options[0] = 0
				if cfg.Cursor {
					r.data.Options[0] = r.data.Options[1]
				}
			}
			switch c.State {
			case projectionBlack:
				blackState, blackPending = true, true
				frozenState = false
				frozenSnapshot = nil
				r.snapshot = nil
				state = projectionStarting
			case projectionFreezing:
				if blackState {
					state = projectionBlack
					announce("")
					break
				}
				if !r.lastValid {
					blackState, blackPending = true, true
					state = projectionStarting
					break
				}
				frozenState = true
				state = projectionFrozen
				if err := r.freeze(); err != nil {
					announce("已冻结；故障恢复快照不可用：" + err.Error())
				} else {
					frozenSnapshot = r.snapshot
					snapW = r.snapshotW
					snapH = r.snapshotH
					announce("")
				}
			case projectionLive:
				blackState = false
				frozenState = false
				state = projectionStarting
				frozenSnapshot = nil
				r.snapshot = nil
				// Resume waits for a fresh duplication frame, never an old unpresented capture.
				r.valid = false
				if d.window != nil {
					d.window.after = captureClock()
					procInvalidateRect.Call(d.window.hwnd, 0, 0)
				}
				dirty = false
				announce("")
			}
		}
		var err error
		if blackState {
			var shown bool
			shown, err = r.presentBlack()
			if shown && state != projectionBlack {
				blackPending = false
				state = projectionBlack
				announce("")
			}
			s.wait(16 * time.Millisecond)
		} else if frozenState {
			_, err = r.presentFrozen()
			s.wait(100 * time.Millisecond)
		} else {
			var f desktopFrame
			var got bool
			f, got, err = d.acquire(5)
			if err == nil && got {
				err = r.copyFrame(f)
				if err == nil {
					if d.window == nil {
						err = r.updatePointer(d, f)
					} else {
						r.data.Options[0] = 0
						r.cfg.Source.Rect = RECT{0, 0, int32(f.Width), int32(f.Height)}
						d.window.setCursor(cfg.Cursor)
					}
				}
				releaseErr := d.releaseFrame()
				if err == nil {
					err = releaseErr
				}
				if f.DesktopChanged {
					captured++
				}
				dirty = true
				lastFrame = time.Now()
			}
			if err == nil && dirty && r.valid {
				var ok bool
				ok, err = r.present()
				if ok {
					presented++
					if state != projectionLive {
						state = projectionLive
						announce("")
					}
					dirty = false
				}
			}
			s.wait(frameDelay(cfg.TargetFPS, time.Since(frameStart)))
		}
		if err != nil {
			if cfg.SourceWindow != 0 {
				r.presentBlack()
				fail(err)
				return
			}
			if frozenState && len(frozenSnapshot) == 0 {
				fail(fmt.Errorf("冻结恢复失败，已停止：%w", err))
				return
			}
			recovered := false
			for retries < 2 {
				retries++
				state = projectionRecovering
				announce(err.Error())
				cleanup()
				s.wait(time.Duration(retries*retries) * 250 * time.Millisecond)
				s.mu.Lock()
				stopped := s.stop
				s.mu.Unlock()
				if stopped {
					return
				}
				err = open()
				if err == nil && frozenState {
					err = r.restoreSnapshot(frozenSnapshot, snapW, snapH)
				}
				if err == nil {
					recovered = true
					break
				}
			}
			if !recovered {
				fail(err)
				return
			}
			state = projectionStarting
			if blackState {
				blackPending = true
			}
			if frozenState {
				state = projectionFrozen
			}
			dirty = false // a rebuilt renderer must first acquire a complete frame
			announce("")
		}
		s.mu.Lock()
		previewEnabled := s.previewEnabled && s.previewError == ""
		previewFPS := s.previewFPS
		s.mu.Unlock()
		if previewEnabled && (state == projectionLive || state == projectionFrozen || state == projectionBlack) {
			f, previewErr := r.thumbnail(cfg.Generation, seq, state, previewFPS)
			s.mu.Lock()
			if s.previewEnabled {
				if previewErr != nil {
					s.previewError = previewErr.Error()
					s.preview = nil
				} else if f != nil {
					s.preview = f
				}
			}
			s.mu.Unlock()
		} else if !previewEnabled && r.preview != nil {
			r.preview.close()
			r.preview = nil
		}
		if time.Since(lastStat) >= time.Second {
			reason := ""
			if !cfg.Cursor {
				reason = "隐藏鼠标：驱动若已合成鼠标则无法移除"
			}
			if time.Since(lastFrame) > time.Second && !frozenState && !blackState {
				reason = "静态画面 / 等待桌面更新"
			}
			announce(reason)
		}
	}
}
