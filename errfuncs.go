package grouped

import (
	"context"
	"errors"
)

// ErrFuncs helps starting and waiting for a group of error-returning functions.
type ErrFuncs struct {
	// Sets cancellation context for any functions added after this is set.
	// If a function has a context set, the function result is ignored if
	// the context is done and the result matches the context's error.
	Ctx context.Context
	// Sets a recovery callback for any functions added after this is set.
	// If a function has a recovery set, and the function panics, the recovery will be called with
	// the panic value and the returned error used as the result.
	Recover func(interface{}) error
	// Sets a monitoring callback for any functions added after this is set.
	// If set, the monitoring callback is called for any function result except if ignored due to
	// the context setting or if the result is `IgnoreResult`.
	Monitor func(error)

	addedFuncs []errFuncParams
	lastSpawn  *spawnedErrFuncs
}

// Add some callbacks to be executed as part of the group.
// The callbacks will not be started until one of FirstDone/FirstOK/FirstError/AllDone are called.
func (s *ErrFuncs) Add(fn ...func() error) {
	for _, f := range fn {
		s.addedFuncs = append(s.addedFuncs, errFuncParams{
			fn:  f,
			ctx: s.Ctx,
			rec: s.Recover,
			mon: s.Monitor,
		})
	}
}

// Launch all previously added functions, and return a channel that signals when any function completes.
func (s *ErrFuncs) FirstDone() (<-chan struct{}, *error) {
	spawned := s.start()
	return spawned.firstDone, &spawned.firstResult
}

// Launch all previously added functions, and return a channel that signals when any function completes successfully.
func (s *ErrFuncs) FirstOK() <-chan struct{} {
	spawned := s.start()
	return spawned.anyOK
}

// Launch all previously added functions, and return a channel that signals when any function completes with an error.
func (s *ErrFuncs) FirstError() (<-chan struct{}, *error) {
	spawned := s.start()
	return spawned.anyNot, &spawned.firstError
}

// Launch all previously added functions, and return a channel that signals when they all complete.
func (s *ErrFuncs) AllDone() <-chan struct{} {
	spawned := s.start()
	return spawned.allDone
}

// Return IgnoreResult from a callback to have the returned value ignored by all reporting.
// A function that returns this error will not trigger the First, FirstOK or FirstError signals, and
// its monitoring callback will not be called.
var IgnoreResult = new(ignoreResultErr)

type ignoreResultErr int

func (*ignoreResultErr) Error() string { return "ignored result" }

func (s *ErrFuncs) start() *spawnedErrFuncs {
	if s.lastSpawn == nil && len(s.addedFuncs) == 0 {
		done := make(chan struct{})
		close(done)
		return &spawnedErrFuncs{
			allDone: done,
		}
	}
	if s.lastSpawn != nil && len(s.addedFuncs) == 0 {
		return s.lastSpawn
	}

	spawned := &spawnedErrFuncs{
		prevSpawn:   s.lastSpawn,
		funcs:       s.addedFuncs,
		firstDone:   make(chan struct{}),
		anyOK:       make(chan struct{}),
		anyNot:      make(chan struct{}),
		allDone:     make(chan struct{}),
		funcResults: make(chan error, len(s.addedFuncs)),
	}
	s.addedFuncs = nil
	s.lastSpawn = spawned

	go spawned.run()

	for i := range spawned.funcs {
		go func(fn errFuncParams) {
			var returned bool
			var err error
			{
				if fn.rec != nil {
					defer func() {
						v := recover()
						if !returned {
							err = fn.rec(v)
						}
					}()
				}
				err = fn.fn()
				returned = true
			}
			if fn.ctx != nil && err != IgnoreResult {
				if ctxErr := fn.ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
					err = IgnoreResult
				}
			}
			if fn.mon != nil && err != IgnoreResult {
				fn.mon(err)
			}
			spawned.funcResults <- err
		}(spawned.funcs[i])
	}

	return spawned
}

type spawnedErrFuncs struct {
	prevSpawn *spawnedErrFuncs

	funcs       []errFuncParams
	funcResults chan error

	firstDone, anyOK, anyNot, allDone chan struct{}
	firstResult                       error
	firstError                        error
	aggResult                         struct{ anyOK, anyNot bool }
}

type errFuncParams struct {
	fn  func() error
	ctx context.Context
	rec func(interface{}) error
	mon func(error)
}

func (s *spawnedErrFuncs) run() {
	var prevFirstDone, prevAnyOK, prevAnyNot, prevAllDone chan struct{}
	if s.prevSpawn != nil {
		prevFirstDone, prevAnyOK, prevAnyNot, prevAllDone = s.prevSpawn.firstDone, s.prevSpawn.anyOK, s.prevSpawn.anyNot, s.prevSpawn.allDone
	}

	var seen struct{ first, anyOK, anyNot bool }

	onFirst := func(val error) {
		if seen.first {
			return
		}
		seen.first = true
		s.firstResult = val
		prevFirstDone = nil
		close(s.firstDone)
	}
	onAnyOK := func() {
		if seen.anyOK {
			return
		}
		seen.anyOK = true
		prevAnyOK = nil
		close(s.anyOK)
	}
	onAnyNot := func(err error) {
		if seen.anyNot {
			return
		}
		seen.anyNot = true
		s.firstError = err
		prevAnyNot = nil
		close(s.anyNot)
	}

	received := 0
	for received < len(s.funcs) || prevAllDone != nil {
		select {
		case res := <-s.funcResults:
			received++
			if res == IgnoreResult {
				break
			}
			onFirst(res)
			if res == nil {
				onAnyOK()
			} else {
				onAnyNot(res)
			}
		case <-prevFirstDone:
			onFirst(s.prevSpawn.firstResult)
			if s.firstResult == nil {
				onAnyOK()
			} else {
				onAnyNot(s.firstResult)
			}
		case <-prevAnyOK:
			onAnyOK()
		case <-prevAnyNot:
			onAnyNot(s.prevSpawn.firstError)
		case <-prevAllDone:
			prevAllDone = nil
		}
	}
	close(s.allDone)
}
