package sync2

import (
	"sync"
)

type Pool struct {
	callgroup CallGroup

	mu     sync.Mutex
	values map[string]interface{}
}

func (p *Pool) Get(key string, cancel <-chan struct{}, get func() (interface{}, bool)) (interface{}, GroupResult) {
	return p.callgroup.Do(key, cancel, func() (interface{}, bool) {
		p.mu.Lock()
		if val, ok := p.values[key]; ok {
			p.mu.Unlock()
			return val, true
		}
		p.mu.Unlock()
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

func (p *Pool) Purge(keep func(string, interface{}) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, val := range p.values {
		if !keep(key, val) {
			delete(p.values, key)
		}
	}
}
