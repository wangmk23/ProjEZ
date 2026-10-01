package main

import "time"

func frameDelay(fps int, elapsed time.Duration) time.Duration {
	if fps < 1 {
		fps = 60
	}
	if fps > 1000 {
		fps = 1000
	}
	remaining := time.Second/time.Duration(fps) - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}
