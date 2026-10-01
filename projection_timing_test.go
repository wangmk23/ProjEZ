package main

import (
	"testing"
	"time"
)

func TestFrameDelayAccountsForWork(t *testing.T) {
	if got := frameDelay(200, 2*time.Millisecond); got != 3*time.Millisecond {
		t.Fatal(got)
	}
	if got := frameDelay(200, 6*time.Millisecond); got != 0 {
		t.Fatal("catchup delay", got)
	}
}
