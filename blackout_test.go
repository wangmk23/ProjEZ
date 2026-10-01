package main

import "testing"

func TestBlackoutCommandCannotBeSkipped(t *testing.T) {
	s := newGPUMailbox(projectionConfig{Generation: 7})
	s.request(projectionCommand{Generation: 7, Sequence: 1, State: projectionBlack})
	s.request(projectionCommand{Generation: 7, Sequence: 2, State: projectionLive})
	c, ok := s.nextCommand()
	if !ok || c.State != projectionBlack {
		t.Fatal("blackout was replaced", c)
	}
	c, ok = s.nextCommand()
	if !ok || c.State != projectionLive {
		t.Fatal("explicit resume lost", c)
	}
	s.request(projectionCommand{Generation: 7, State: projectionBlack})
	s.request(projectionCommand{Generation: 7, State: projectionStopping})
	c, _ = s.nextCommand()
	if c.State != projectionStopping {
		t.Fatal("stop must win")
	}
}
