package failure

import (
	"context"
	"sync"
)

type boundaryKey struct{}
type boundaryState struct {
	mu             sync.Mutex
	infrastructure error
}

// WithBoundary lets a protocol adapter recover failures that a provider may
// otherwise convert into invalid_grant, access_denied, or active:false.
func WithBoundary(ctx context.Context) context.Context {
	return context.WithValue(ctx, boundaryKey{}, new(boundaryState))
}

func Record(ctx context.Context, err error) {
	// 仅记录基础设施类失败，业务拒绝不应掩盖真实故障。
	if err == nil || !IsInfrastructure(err) {
		return
	}
	if state, ok := ctx.Value(boundaryKey{}).(*boundaryState); ok {
		state.mu.Lock()
		defer state.mu.Unlock()
		// 并发下只保留首个故障，后续记录被忽略。
		if state.infrastructure == nil {
			state.infrastructure = err
		}
	}
}

func BoundaryFailure(ctx context.Context) error {
	if state, ok := ctx.Value(boundaryKey{}).(*boundaryState); ok {
		state.mu.Lock()
		defer state.mu.Unlock()
		return state.infrastructure
	}
	return nil
}
