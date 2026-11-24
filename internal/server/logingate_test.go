package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Slowed sign-in attempts from this machine are let through one at a time, so
// parallel guesses through a loopback tunnel are no faster than serial ones.
func TestLoginGateSerializesSlowedAttempts(t *testing.T) {
	var g loginGate
	const n, d = 5, 40 * time.Millisecond
	start := time.Now()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !g.wait(context.Background(), d) {
				t.Error("an attempt was dropped")
			}
		}()
	}
	wg.Wait()
	if elapsed := time.Since(start); elapsed < n*d {
		t.Fatalf("%d parallel attempts finished in %v; they must take at least %v", n, elapsed, n*d)
	}

	// A request that ends while waiting leaves the gate free for the next.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if g.wait(ctx, time.Hour) {
		t.Fatal("a cancelled request was let through")
	}
	if !g.wait(context.Background(), time.Millisecond) {
		t.Fatal("the gate stayed closed after a cancelled request")
	}
}
