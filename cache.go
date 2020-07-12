package grouped

import (
	"sync"
)

// Cache shares the results of all calls with the same key, executing only one of the callbacks
// in the group to build the result if necessary.
type Cache struct {
	callgroup Calls

	mu     sync.RWMutex
	values map[string]interface{}
}

// Get retrieves the existing value for the key if present. If not, it starts or joins the call
// group for the given key, waiting for a member of the group to complete its callback and return a
// result that should be accepted by the group. If the executed callback panics or indicates the
// result should not be accepted, a different member's callback will be invoked for the group, and
// so on until an invoked callback completes successfully. A cancel channel may be provided,
// allowing a caller to leave the group before the result is ready.
func (p *Cache) Get(key string, cancel <-chan struct{}, get func() (interface{}, bool)) (interface{}, Status) {
	p.mu.RLock()
	if val, ok := p.values[key]; ok {
		p.mu.RUnlock()
		return val, Shared
	}
	p.mu.RUnlock()

	return p.callgroup.Do(key, cancel, func() (interface{}, bool) {
		p.mu.RLock()
		if val, ok := p.values[key]; ok {
			p.mu.RUnlock()
			return val, true
		}
		p.mu.RUnlock()
		val, accept := get()
		if !accept {
			return val, false
		}
		p.mu.Lock()
		if p.values == nil {
			p.values = make(map[string]interface{})
		}
		p.values[key] = val
		p.mu.Unlock()
		return val, true
	})
}

// Delete removes the given key from the cache's entries if present, forcing the removed entry to be
// re-built the next time it is retrieved.
func (p *Cache) Delete(key string) {
	p.mu.Lock()
	delete(p.values, key)
	p.mu.Unlock()
}

// Delete removes the given key from the cache's entries if present and the callback returns false.
// If removed, the key will be rebuilt the next time it is retrieved.
func (p *Cache) DeleteUnless(key string, keep func(interface{}) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if val, ok := p.values[key]; ok && !keep(val) {
		delete(p.values, key)
	}
}

// Purge removes any items from the cache where the callback returns false, forcing the removed
// entries to be re-built the next time they are retrieved.
func (p *Cache) Purge(keep func(interface{}) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, val := range p.values {
		if !keep(val) {
			delete(p.values, key)
		}
	}
}
