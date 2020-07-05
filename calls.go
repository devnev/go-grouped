package grouped

import "sync"

// Calls allows batching together calls with the same key to share the result of executing only
// one of the callbacks in the batch.
type Calls struct {
	mu     sync.Mutex
	groups map[string]*callGroupInner
}

// Do starts or joins the call group for the given key, waiting for a member of the group to complete
// its callback and return a result that should be accepted by the group. If the executed callback
// panics or indicates the result should not be accepted, a different member's callback will be
// invoked for the group, and so on until an invoked callback completes successfully.
// A cancel channel may be provided, allowing a caller to leave the group before the result is ready.
func (g *Calls) Do(key string, cancel <-chan struct{}, do func() (result interface{}, accept bool)) (interface{}, Status) {
	g.mu.Lock()
	if g.groups == nil {
		g.groups = make(map[string]*callGroupInner)
	}
	if g.groups[key] == nil {
		g.groups[key] = &callGroupInner{
			leader: make(chan struct{}, 1),
			done:   make(chan struct{}),
		}
		g.groups[key].leader <- struct{}{}
	}
	inner := g.groups[key]
	inner.monitors++
	g.mu.Unlock()

	select {
	case <-cancel:
		g.mu.Lock()
		defer g.mu.Unlock()
		if inner != g.groups[key] {
			return inner.result, Shared
		} else {
			inner.monitors--
			return nil, Canceled
		}
	case <-inner.done:
		return inner.result, Shared
	case <-inner.leader:
	}

	accepted := false
	defer func() {
		if !accepted {
			inner.leader <- struct{}{}
		}
	}()
	if result, accept := do(); accept {
		inner.result = result
	} else {
		return nil, Canceled
	}
	accepted = true

	g.mu.Lock()
	delete(g.groups, key)
	g.mu.Unlock()

	close(inner.done)
	if inner.monitors > 1 {
		return inner.result, Shared
	} else {
		return inner.result, Exclusive
	}
}

type callGroupInner struct {
	leader   chan struct{}
	done     chan struct{}
	result   interface{}
	monitors int
}
