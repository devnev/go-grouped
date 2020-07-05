package grouped_test

import (
	"context"
	"github.com/devnev/go-grouped"
	"testing"
)

func TestCtxCalls_Do_CallsCallbackOnce(t *testing.T) {
	var calls grouped.CtxCalls
	called := 0
	calls.Do(context.Background(), "", func() (interface{}, error) {
		called++
		return nil, nil
	})
	if called != 1 {
		t.Fatalf("Expected 1 call to callback, got %d", called)
	}
}
