package grouped

import (
	"sync"
	"sync/atomic"
)

// RefCache caches and shares the result of calls with the same key until the result is removed from
// the cache. The cached items are explicitly reference-counted and closed when all references have
// been closed. As an alternative to reference-counting of RefCache, the Cache type may be used in
// combination with SetFinalizer to run a cleanup when items are garbage-collected.
type RefCache struct {
	Valid func(interface{}) bool

	mu    sync.RWMutex
	items map[string]*refCacheItem
}

// Get retrieves the value for the key, calling the fetch method if necessary to retrieve the value.
// The fetch method is only called if no existing value is in the cache. If the cache contains a
// value that is in the process of being fetched, the result of the ongoing fetch is used instead of
// beginning a new fetch. However, if the fetch fails or is canceled, one new fetch call is
// initiated for all Get calls that were waiting for result of that call.
// The successfully cached items are reference-counted, so if the Get call is successful it returns
// a callback that must be called to free the returned reference.
// If an entry is removed from the cache or considered invalid, a new entry for the key is created
// in the cache. However, the previous entry's value is only cleaned up once all references have
// been closed.
func (p *RefCache) Get(key string, cancel <-chan struct{}, fetch func() (interface{}, func())) (interface{}, func()) {
	// This defer prevents leaking reference-counts when we panic. A successful return will set
	// filled=true before returning to disable the cleanup.
	var item *refCacheItem
	var filled bool
	defer func() {
		if item != nil && !filled {
			item.close()
		}
	}()

	for {
		// Try to get a valid reference with just a read lock
		p.mu.RLock()
		item = p.items[key]
		if item != nil {
			// The item is in the map and we still have the read lock, so we know the reference
			// count is at least 1, and can increment it safely.
			item.ref()
		}
		p.mu.RUnlock()

		// Slow path, get the write lock and possibly create the map and/or entry.
		if item == nil {
			p.mu.Lock()
			if p.items == nil {
				p.items = make(map[string]*refCacheItem)
			}
			item = p.items[key]
			if item == nil {
				item = newCacheItem()
				// This reference count tracks the reference in the map
				item.ref()
				p.items[key] = item
			}
			// The item is in the map and we still have the read lock, so we know the reference
			// count is at least 1, and can increment it safely.
			item.ref()
			p.mu.Unlock()
		}

		{
			// Make sure the item is filled
			result, status := item.fill(cancel, fetch)
			if status == Canceled {
				return result, nil
			} else if status == Exclusive {
				// We (ab)use the status Exclusive to indicate that this this call did the fetch,
				// and can skip the validation callback as the item should be valid for this call
				filled = true
				return item.value, item.close
			}
		}

		// If we have a valid item, we can return it
		if p.Valid == nil || p.Valid(item.value) {
			filled = true
			return item.value, item.close
		}

		// Clear out the invalid item before we try again
		p.mu.Lock()
		if p.items[key] != item {
			// Another caller has already done the cleanup
			p.mu.Unlock()
			continue
		}
		delete(p.items, key)
		p.mu.Unlock()
		item.close()
	}
}

// Delete removes the given key from the pool's entries if present, forcing the removed entry to be
// re-built the next time it is retrieved. The item's closer will be called once all references to
// the item have been closed
func (p *RefCache) Delete(key string) {
	p.mu.RLock()
	item := p.items[key]
	p.mu.RUnlock()
	if item == nil {
		return
	}
	p.mu.Lock()
	item = p.items[key]
	delete(p.items, key)
	p.mu.Unlock()
	if item == nil {
		return
	}
	item.closer()
}

func (p *RefCache) Purge(keep func(interface{}) bool) {
	if keep == nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		for key, item := range p.items {
			delete(p.items, key)
			item.close()
		}
		return
	}

	type record struct {
		key  string
		item *refCacheItem
	}
	var invalid []record
	{
		p.mu.RLock()
		defer p.mu.RUnlock()
		invalid = make([]record, 0, len(p.items))
		for key, item := range p.items {
			if !item.filled() {
				continue
			}
			if p.Valid(item.value) {
				continue
			}
			invalid = append(invalid, record{key: key, item: item})
		}
	}

	if len(invalid) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, rec := range invalid {
		if p.items[rec.key] == rec.item {
			delete(p.items, rec.key)
			rec.item.close()
		}
	}
}

type refCacheItem struct {
	refs      int32
	fillCalls atomic.Value

	value  interface{}
	closer func()
}

func newCacheItem() *refCacheItem {
	item := new(refCacheItem)
	item.fillCalls.Store(new(Calls))
	return item
}

func (i *refCacheItem) fill(cancel <-chan struct{}, get func() (interface{}, func())) (interface{}, Status) {
	grp := i.fillCalls.Load().(*Calls)
	if grp == nil {
		// The item was already filled by a previous call to the group.
		// We return status Shared to indicate that this routine didn't do the fetch.
		return nil, Shared
	}
	filled := false
	result, shared := grp.Do("", cancel, func() (interface{}, bool) {
		if i.filled() {
			return nil, true
		}
		value, valCloser := get()
		if valCloser == nil {
			return value, false
		}
		i.value = value
		i.closer = valCloser
		i.fillCalls.Store((*Calls)(nil))
		filled = true
		return nil, true
	})
	if shared == Canceled {
		return result, Canceled
	} else if filled {
		// We return status Exclusive to indicate that this call did the fetch, and can skip the
		// validation callback.
		return nil, Exclusive
	}
	return nil, Shared
}

func (i *refCacheItem) filled() bool {
	return i.fillCalls.Load().(*Calls) == nil
}

func (i *refCacheItem) ref() {
	atomic.AddInt32(&i.refs, 1)
}

func (i *refCacheItem) close() {
	refs := atomic.AddInt32(&i.refs, -1)
	if refs != 0 {
		return
	}
	if i.closer != nil {
		i.closer()
	}
}
