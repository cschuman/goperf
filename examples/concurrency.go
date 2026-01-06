// Package examples contains sample code demonstrating performance anti-patterns.
package examples

import (
	"sync"
	"time"
)

// MutexInLoop demonstrates acquiring a lock on every iteration.
// This serializes all goroutines and defeats concurrency.
func MutexInLoop(items []int) int {
	var mu sync.Mutex
	var sum int

	for _, item := range items {
		mu.Lock() // BAD: lock acquired every iteration
		sum += item
		mu.Unlock()
	}
	return sum
}

// MutexBatched shows a better approach - batch updates.
func MutexBatched(items []int) int {
	var mu sync.Mutex
	var sum int

	// Calculate locally first
	localSum := 0
	for _, item := range items {
		localSum += item
	}

	// Single lock for final update - GOOD
	mu.Lock()
	sum += localSum
	mu.Unlock()

	return sum
}

// UnboundedGoroutines spawns goroutines without limits.
// This can exhaust memory and overwhelm the scheduler.
func UnboundedGoroutines(items []string) {
	for _, item := range items { // BAD: unbounded goroutine creation
		go processAsync(item)
	}
}

// BoundedGoroutines uses a worker pool pattern.
func BoundedGoroutines(items []string) {
	const maxWorkers = 10
	sem := make(chan struct{}, maxWorkers)

	var wg sync.WaitGroup
	for _, item := range items {
		sem <- struct{}{} // Acquire semaphore - GOOD: bounded
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			defer func() { <-sem }()
			processAsync(s)
		}(item)
	}
	wg.Wait()
}

// GoroutineLeak demonstrates a goroutine that may never terminate.
func GoroutineLeak(ch chan int) {
	go func() { // BAD: no way to cancel this goroutine
		for {
			val := <-ch // Blocks forever if ch is never closed
			_ = val
		}
	}()
}

// GoroutineWithContext shows proper cancellation.
func GoroutineWithContext(ch chan int, done chan struct{}) {
	go func() { // GOOD: can be cancelled via done channel
		for {
			select {
			case val := <-ch:
				_ = val
			case <-done:
				return
			}
		}
	}()
}

// TimeTickerLeak creates a ticker that's never stopped.
func TimeTickerLeak() {
	ticker := time.NewTicker(time.Second) // BAD: ticker never stopped
	go func() {
		for range ticker.C {
			// do something
		}
	}()
}

func processAsync(s string) {
	// placeholder
}
