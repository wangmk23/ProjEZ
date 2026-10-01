package main

type projectionState uint8

const (
	projectionStopped projectionState = iota
	projectionStarting
	projectionLive
	projectionFreezing
	projectionFrozen
	projectionRecovering
	projectionStopping
	projectionFailed
	projectionBlack
)

type projectionConfig struct {
	Generation           uint64
	HWND                 uintptr
	Source, Target       Monitor
	TargetFPS            int
	Fill, Smooth, Cursor bool
	StartBlack           bool
	SourceWindow         uintptr
	SourcePID            uint32
}
type projectionCommand struct {
	Generation, Sequence uint64
	State                projectionState
	Config               projectionConfig
}
type projectionEvent struct {
	Generation, Sequence   uint64
	State                  projectionState
	Backend, Reason        string
	Captured, Presented    uint64
	CaptureFPS, PresentFPS float64
	Fallback               bool
}
type projectionController struct {
	generation, sequence uint64
	desired, observed    projectionState
}

func (c *projectionController) begin() uint64 {
	c.generation++
	c.sequence = 0
	c.desired = projectionStarting
	c.observed = projectionStarting
	return c.generation
}
func (c *projectionController) request(s projectionState) projectionCommand {
	if c.desired != projectionStopping {
		c.sequence++
		c.desired = s
	}
	return projectionCommand{Generation: c.generation, Sequence: c.sequence, State: c.desired}
}
func (c *projectionController) accept(e projectionEvent) bool {
	if e.Generation != c.generation || e.Sequence < c.sequence {
		return false
	}
	c.observed = e.State
	if c.desired != projectionStopping && (e.State == projectionLive || e.State == projectionFrozen || e.State == projectionBlack) {
		c.desired = e.State
	}
	return true
}
