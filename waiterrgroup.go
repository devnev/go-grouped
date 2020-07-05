package sync2

import (
	"context"
	"errors"
)

type WaitErrGroup struct {
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

	addedFuncs []waitErrParams
	lastSpawn  *spawnedErrGroup
}

type ignoreResultErr int

func (*ignoreResultErr) Error() string { return "ignored result" }

// Return IgnoreResult to have the returned value ignored by all reporting.
var IgnoreResult = new(ignoreResultErr)

type spawnedErrGroup struct {
	prevSpawn *spawnedErrGroup

	funcs       []waitErrParams
	funcResults chan error

	firstDone, anyOK, anyNot, allDone chan struct{}
	firstResult                       error
	firstError                        error
	aggResult                         struct{ anyOK, anyNot bool }
}

type waitErrParams struct {
	fn  func() error
	ctx context.Context
	rec func(interface{}) error
	mon func(error)
}

func (s *WaitErrGroup) Add(fn ...func() error) {
	for _, f := range fn {
		s.addedFuncs = append(s.addedFuncs, waitErrParams{
			fn:  f,
			ctx: s.Ctx,
			rec: s.Recover,
			mon: s.Monitor,
		})
	}
}

func (s *WaitErrGroup) FirstDone() (<-chan struct{}, *error) {
	spawned := s.start()
	return spawned.firstDone, &spawned.firstResult
}

func (s *WaitErrGroup) FirstOK() <-chan struct{} {
	spawned := s.start()
	return spawned.anyOK
}

func (s *WaitErrGroup) FirstError() (<-chan struct{}, *error) {
	spawned := s.start()
	return spawned.anyNot, &spawned.firstError
}

func (s *WaitErrGroup) AllDone() <-chan struct{} {
	spawned := s.start()
	return spawned.allDone
}

func (s *WaitErrGroup) start() *spawnedErrGroup {
	if s.lastSpawn == nil && len(s.addedFuncs) == 0 {
		done := make(chan struct{})
		close(done)
		return &spawnedErrGroup{
			allDone: done,
		}
	}
	if s.lastSpawn != nil && len(s.addedFuncs) == 0 {
		return s.lastSpawn
	}

	spawned := &spawnedErrGroup{
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
		go func(fn waitErrParams) {
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
			if fn.ctx != nil {
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

func (s *spawnedErrGroup) run() {
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
