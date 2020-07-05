package grouped_test

import (
	"github.com/devnev/go-grouped"
	"testing"
)

func TestCalls_Do_CallsCallbackOnce(t *testing.T) {
	var calls grouped.Calls
	called := 0
	calls.Do("", nil, func() (interface{}, bool) {
		called++
		return nil, true
	})
	if called != 1 {
		t.Fatalf("Expected 1 call to callback, got %d", called)
	}
}
