package main

import (
	"sync"
	"time"
)

type State int

const (
	StateClosed   State = iota // normal — requests flow through
	StateOpen                  // tripped — all requests rejected
	StateHalfOpen              // testing — one request allowed through
)

type CircuitBreaker struct {
	mu          sync.Mutex
	state       State
	failures    int
	threshold   int
	cooldown    time.Duration
	lastTripped time.Time
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		cooldown:  cooldown,
	}
}

// Allow checks whether a request should be permitted.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastTripped) > cb.cooldown {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		return false
	}
	return false
}

// RecordSuccess resets the breaker back to closed.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = StateClosed
}

// RecordFailure increments the failure count and trips the breaker if threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	if cb.failures >= cb.threshold {
		cb.state = StateOpen
		cb.lastTripped = time.Now()
	}
}
