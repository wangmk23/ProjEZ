package main

import "testing"

func TestSessionGenerationsAndStop(t *testing.T) {
	var c projectionController
	for i := 0; i < 1000; i++ {
		old := c.generation
		id := c.begin()
		if c.accept(projectionEvent{Generation: old, State: projectionLive}) {
			t.Fatal("old event")
		}
		if !c.accept(projectionEvent{Generation: id, State: projectionLive}) {
			t.Fatal("new event")
		}
		if c.desired != projectionLive {
			t.Fatal("startup indicator never acknowledged")
		}
		cmd := c.request(projectionFreezing)
		if c.observed == projectionFrozen {
			t.Fatal("premature freeze")
		}
		if c.accept(projectionEvent{Generation: id, Sequence: cmd.Sequence - 1, State: projectionFrozen}) {
			t.Fatal("old command")
		}
		if !c.accept(projectionEvent{Generation: id, Sequence: cmd.Sequence, State: projectionFrozen}) {
			t.Fatal("freeze ack")
		}
		stop := c.request(projectionStopping)
		again := c.request(projectionLive)
		if again.State != projectionStopping || again.Sequence != stop.Sequence {
			t.Fatal("stop overwritten")
		}
	}
}
