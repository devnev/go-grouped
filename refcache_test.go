package grouped_test

import (
	"github.com/devnev/go-grouped"
	"testing"
)

func TestRefCache_Get(t *testing.T) {
	var pool grouped.RefCache
	called := 0
	pool.Get("", nil, func() (interface{}, func()) {
		called++
		return nil, nil
	})
	if called != 1 {
		t.Fatalf("Expected 1 call to callback, got %d", called)
	}
}
