package main

import (
	"strings"
	"testing"
)

func TestGPUBackendPreference(t *testing.T) {
	for _, v := range []string{"", "auto", "invalid", "gdi"} {
		p := parsePreferences("Backend=" + v)
		if p.Compatibility != (v == "gdi") {
			t.Fatal(v, p)
		}
		if !strings.Contains(strings.Join(preferenceLines(p), "\n"), "Backend=") {
			t.Fatal("not persisted")
		}
	}
}
func TestGPUStopWaitsForCompletion(t *testing.T) {
	old := outputRunning
	defer func() { outputRunning = old; gpuActive = nil; gpuOwnsOutput = false }()
	outputRunning = true
	gpuOwnsOutput = true
	id := gpuController.begin()
	gpuActive = newGPUMailbox(projectionConfig{Generation: id})
	stopOutput()
	if gpuActive == nil || !outputRunning {
		t.Fatal("destroyed before GPU completion")
	}
	cmd, ok := gpuActive.nextCommand()
	if !ok || cmd.State != projectionStopping {
		t.Fatal("no stop request")
	}
	close(gpuActive.done)
	gpuActive.publish(projectionEvent{Generation: id, Sequence: cmd.Sequence, State: projectionStopped})
	pollGPUEvents()
	if gpuActive != nil || outputRunning {
		t.Fatal("not stopped after completion")
	}
}
func TestGPUOldEventCannotChangeSession(t *testing.T) {
	id := gpuController.begin()
	gpuActive = newGPUMailbox(projectionConfig{Generation: id})
	defer func() { gpuActive = nil }()
	gpuActive.publish(projectionEvent{Generation: id - 1, State: projectionFrozen})
	old := frozen
	pollGPUEvents()
	if frozen != old {
		t.Fatal("stale event")
	}
}

func TestDisconnectStopsWithoutAutomaticRestart(t *testing.T) {
	oldMonitors, oldRates := monitors, monitorRates
	oldSource, oldTarget := srcMonitor, dstMonitor
	defer func() {
		monitors, monitorRates = oldMonitors, oldRates
		srcMonitor, dstMonitor = oldSource, oldTarget
		gpuActive = nil
		gpuOwnsOutput = false
		outputRunning = false
	}()
	outputRunning = true
	gpuOwnsOutput = true
	srcMonitor.Device = "missing-source-test"
	dstMonitor.Device = "missing-target-test"
	id := gpuController.begin()
	gpuActive = newGPUMailbox(projectionConfig{Generation: id})
	handleDisplayRefresh()
	c, ok := gpuActive.nextCommand()
	if !ok || c.State != projectionStopping {
		t.Fatal("disconnect did not stop")
	}
	close(gpuActive.done)
	gpuActive.publish(projectionEvent{Generation: id, Sequence: c.Sequence, State: projectionStopped})
	pollGPUEvents()
	handleDisplayRefresh()
	if outputRunning || gpuActive != nil || livePump != nil {
		t.Fatal("refresh restarted projection")
	}
}
