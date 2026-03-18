package engine

import (
	"context"
	"time"
)

// Limiter implements a token-bucket rate limiter for RPS control.
type Limiter struct {
	tokens chan struct{}
	cancel context.CancelFunc
}

// NewLimiter creates a token-bucket limiter that allows rps requests per second.
// If rps <= 0, no rate limiting is applied.
func NewLimiter(rps int) *Limiter {
	if rps <= 0 {
		return &Limiter{} // no limiting
	}

	ctx, cancel := context.WithCancel(context.Background())
	tokens := make(chan struct{}, rps)

	// Pre-fill the bucket
	for i := 0; i < rps; i++ {
		tokens <- struct{}{}
	}

	// Refill goroutine
	go func() {
		interval := time.Second / time.Duration(rps)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case tokens <- struct{}{}:
				default:
					// bucket full, drop token
				}
			}
		}
	}()

	return &Limiter{tokens: tokens, cancel: cancel}
}

// Acquire blocks until a token is available or ctx is cancelled.
// Returns nil on success, ctx.Err() on cancellation.
func (l *Limiter) Acquire(ctx context.Context) error {
	if l.tokens == nil {
		return nil // no rate limiting
	}
	select {
	case <-l.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops the token refill goroutine.
func (l *Limiter) Close() {
	if l.cancel != nil {
		l.cancel()
	}
}
