package sync2

import "sync"

type CallGroup struct {
	mu     sync.Mutex
	groups map[string]*callGroupInner
}

type callGroupInner struct {
	leader   chan struct{}
	done     chan struct{}
	result   interface{}
	monitors int
}

func (g *CallGroup) Do(key string, cancel <-chan struct{}, do func() (interface{}, bool)) (interface{}, GroupResult) {
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
			return inner.result, GroupShared
		} else {
			inner.monitors--
			return nil, GroupCanceled
		}
	case <-inner.done:
		return inner.result, GroupShared
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
		return nil, GroupCanceled
	}
	accepted = true

	g.mu.Lock()
	delete(g.groups, key)
	g.mu.Unlock()

	close(inner.done)
	if inner.monitors > 1 {
		return inner.result, GroupShared
	} else {
		return inner.result, GroupExclusive
	}
}
