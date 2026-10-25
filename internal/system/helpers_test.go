package system

import (
	"context"
	"sync"
	"testing"
)

type commandCall struct {
	name string
	args []string
}

type fakeRunner struct {
	mu    sync.Mutex
	calls []commandCall
	runFn func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (r *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	copiedArgs := append([]string(nil), args...)
	r.calls = append(r.calls, commandCall{name: name, args: copiedArgs})
	runFn := r.runFn
	r.mu.Unlock()

	if runFn == nil {
		return nil, nil
	}
	return runFn(ctx, name, args...)
}

func (r *fakeRunner) Calls() []commandCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]commandCall, len(r.calls))
	copy(result, r.calls)
	return result
}

func requireEqual[T comparable](t *testing.T, label string, got, want T) {
	t.Helper()
	if got == want {
		return
	}
	t.Fatalf("%s = %v, want %v", label, got, want)
}

func floatPtr(value float64) *float64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func sameFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
