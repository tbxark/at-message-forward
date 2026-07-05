package telegram

import (
	"context"
	"errors"
	"sync"
)

type MutexExecutor struct {
	mu sync.Mutex
	fn func(context.Context, string) ([]string, error)
}

func NewMutexExecutor(fn func(context.Context, string) ([]string, error)) *MutexExecutor {
	return &MutexExecutor{fn: fn}
}

func (e *MutexExecutor) ExecuteAT(ctx context.Context, cmd string) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.fn == nil {
		return nil, errors.New("telegram AT executor function is required")
	}
	return e.fn(ctx, cmd)
}
