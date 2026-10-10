package resilience

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Breaker opens after threshold consecutive failures and permits a probe after cooldown.
type Breaker struct {
	mu                  sync.Mutex
	failures, threshold int
	openedAt            time.Time
	cooldown            time.Duration
}

func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	return &Breaker{threshold: threshold, cooldown: cooldown}
}
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < b.threshold {
		return true
	}
	if time.Since(b.openedAt) >= b.cooldown {
		b.failures = 0
		b.openedAt = time.Time{}
		return true
	}
	return false
}
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openedAt = time.Time{}
}
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold && b.openedAt.IsZero() {
		b.openedAt = time.Now()
	}
}

func Do[T any](parent context.Context, timeout time.Duration, attempts int, b *Breaker, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if !b.Allow() {
		return zero, errors.New("circuit breaker is open; product service temporarily unavailable")
	}
	var last error
	for i := 0; i < attempts; i++ {
		ctx, cancel := context.WithTimeout(parent, timeout)
		value, err := fn(ctx)
		cancel()
		if err == nil {
			b.Success()
			return value, nil
		}
		last = err
		b.Failure()
		if i+1 < attempts {
			delay := time.Duration(100*(1<<i))*time.Millisecond + time.Duration(rand.Intn(50))*time.Millisecond
			select {
			case <-parent.Done():
				return zero, parent.Err()
			case <-time.After(delay):
			}
		}
	}
	return zero, fmt.Errorf("product service request failed after %d attempts: %w", attempts, last)
}
