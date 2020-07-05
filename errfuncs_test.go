package grouped_test

import (
	"github.com/devnev/go-grouped"
	"testing"
	"time"
)

func TestErrFuncs_AllDone_CallsAddedFuncOnce(t *testing.T) {
	var funcs grouped.ErrFuncs
	called := 0
	funcs.Add(func() error {
		called++
		return nil
	})
	done := funcs.AllDone()
	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatal("timed out")
	}
	if called != 1 {
		t.Fatalf("Expected 1 call to callback, got %d", called)
	}
}
