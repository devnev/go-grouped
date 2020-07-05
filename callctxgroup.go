package sync2

import (
	"context"
	"sync"
)

type CallCtxGroup struct {
	mu     sync.Mutex
	groups map[string]*callCtxGroupInner
}

type callCtxGroupInner struct {
	leader   chan struct{}
	done     chan struct{}
	result   interface{}
	err      error
	monitors int
}

func (g *CallCtxGroup) Do(ctx context.Context, key string, do func() (interface{}, error)) (interface{}, GroupResult, error) {
	g.mu.Lock()
	if g.groups == nil {
		g.groups = make(map[string]*callCtxGroupInner)
	}
	if g.groups[key] == nil {
		g.groups[key] = &callCtxGroupInner{
			leader: make(chan struct{}, 1),
			done:   make(chan struct{}),
		}
		g.groups[key].leader <- struct{}{}
	}
	inner := g.groups[key]
	inner.monitors++
	g.mu.Unlock()

	select {
	case <-ctx.Done():
		g.mu.Lock()
		defer g.mu.Unlock()
		if inner != g.groups[key] {
			return inner.result, GroupShared, inner.err
		} else {
			inner.monitors--
			return nil, GroupCanceled, ctx.Err()
		}
	case <-inner.done:
		return inner.result, GroupShared, inner.err
	case <-inner.leader:
	}

	// pass on leadership unless we accept the result
	accepted := false
	defer func() {
		if !accepted {
			inner.leader <- struct{}{}
		}
	}()
	if result, err := do(); ctx.Err() != nil {
		// if the context is done we assume the result was affected by this and so the result is
		// exclusive to this call rather than relevant to the entire group.
		return result, GroupExclusive, err
	} else {
		inner.result = result
		inner.err = err
	}
	accepted = true

	g.mu.Lock()
	delete(g.groups, key)
	g.mu.Unlock()
	close(inner.done)
	if inner.monitors > 1 {
		return inner.result, GroupShared, inner.err
	} else {
		return inner.result, GroupExclusive, inner.err
	}
}
