package grouped_test

import (
	"github.com/devnev/go-grouped"
	"testing"
)

func TestCache_Get(t *testing.T) {
	var pool grouped.Cache
	called := 0
	pool.Get("", nil, func() (interface{}, bool) {
		called++
		return nil, true
	})
	if called != 1 {
		t.Fatalf("Expected 1 call to callback, got %d", called)
	}
}
