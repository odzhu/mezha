package app

import (
	"fmt"
	"time"
)

// progressPulse reports that a long-running operation is still active.
func progressPulse(message string) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Printf("%s...\n", message)
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}
