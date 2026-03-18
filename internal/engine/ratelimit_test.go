package engine

import (
	"context"
	"testing"
	"time"
)

func TestNewLimiter_Positive(t *testing.T) {
	l := NewLimiter(100)
	defer l.Close()
	if l.tokens == nil {
		t.Error("expected non-nil tokens channel")
	}
}

func TestNewLimiter_Zero(t *testing.T) {
	l := NewLimiter(0)
	defer l.Close()
	if l.tokens != nil {
		t.Error("expected nil tokens for rps=0")
	}
}

func TestNewLimiter_Negative(t *testing.T) {
	l := NewLimiter(-5)
	defer l.Close()
	if l.tokens != nil {
		t.Error("expected nil tokens for negative rps")
	}
}

func TestLimiter_AcquireNoLimit(t *testing.T) {
	l := NewLimiter(0)
	defer l.Close()
	for i := 0; i < 100; i++ {
		if err := l.Acquire(context.Background()); err != nil {
			t.Fatalf("Acquire: %v", err)
		}
	}
}

func TestLimiter_AcquireWithLimit(t *testing.T) {
	l := NewLimiter(1000)
	defer l.Close()
	// Should be able to acquire up to rps tokens immediately (pre-filled)
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := l.Acquire(ctx)
		cancel()
		if err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
	}
}

func TestLimiter_AcquireCancelled(t *testing.T) {
	l := NewLimiter(1)
	defer l.Close()
	// Drain the single pre-filled token
	_ = l.Acquire(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	// Next acquire should block and then fail when context expires
	err := l.Acquire(ctx)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestLimiter_Close(t *testing.T) {
	l := NewLimiter(100)
	l.Close()
	// Close should be safe to call
}

func TestLimiter_CloseNoLimit(t *testing.T) {
	l := NewLimiter(0)
	l.Close()
	// Should not panic
}

func TestLimiter_Refill(t *testing.T) {
	l := NewLimiter(1000)
	defer l.Close()
	// Drain all pre-filled tokens
	for i := 0; i < 1000; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		_ = l.Acquire(ctx)
		cancel()
	}
	// Wait for refill
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := l.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire after refill: %v", err)
	}
}
