package sync2

type WaitOKGroup struct {
	// Sets a recovery callback for any functions added after this is set.
	// If a function has a recovery set, and the function panics, the recovery will be called with
	// the panic value. In the aggregation the function is assumed to have returned false.
	Recover func(interface{})

	funcs []waitOKParams
	last  *spawnedOKGroup
}

// Add some callbacks to be executed as part of the group.
// The callbacks will not be started until one of FirstDone/FirstOK/FirstNot/AllDone are called.
func (s *WaitOKGroup) Add(fn ...func() bool) {
	for _, f := range fn {
		s.funcs = append(s.funcs, waitOKParams{fn: f, rec: s.Recover})
	}
}

// Launch all previously added functions, and return a channel that signals when any function completes.
// The returned pointer should only be used after the signal has been received. It points to the result
// of the function that completed first.
func (s *WaitOKGroup) FirstDone() (<-chan struct{}, *bool) {
	spawned := s.start()
	return spawned.firstDone, &spawned.firstResult
}

// Launch all previously added functions, and return a channel that signals when any function completes ok.
// Note that the channel will never signal if none of the functions complete ok.
func (s *WaitOKGroup) FirstOK() <-chan struct{} {
	spawned := s.start()
	return spawned.anyOK
}

// Launch all previously added functions, and return a channel that signals when any function completes not ok.
// Note that the channel will never signal if all of the functions complete ok.
func (s *WaitOKGroup) FirstNot() <-chan struct{} {
	spawned := s.start()
	return spawned.anyNot
}

// Launch all previously added functions, and return a channel that signals when they all complete.
// The returned pointer should only be used after the signal has been received. It points to the aggregated
// results of all the functions.
func (s *WaitOKGroup) AllDone() (<-chan struct{}, *struct{ anyOK, anyNot bool }) {
	spawned := s.start()
	return spawned.allDone, &spawned.aggResult
}

func (s *WaitOKGroup) start() *spawnedOKGroup {
	if len(s.funcs) == 0 {
		if s.last != nil {
			return s.last
		}
		done := make(chan struct{})
		close(done)
		return &spawnedOKGroup{
			allDone: done,
		}
	}
	spawned := &spawnedOKGroup{
		prev:        s.last,
		funcs:       s.funcs,
		firstDone:   make(chan struct{}),
		anyOK:       make(chan struct{}),
		anyNot:      make(chan struct{}),
		allDone:     make(chan struct{}),
		funcResults: make(chan bool, len(s.funcs)),
	}
	s.funcs = nil
	s.last = spawned

	go spawned.run()

	for i := range spawned.funcs {
		go func(params waitOKParams) {
			var ok, returned bool
			{
				if params.rec != nil {
					defer func() {
						v := recover()
						if !returned {
							params.rec(v)
						}
					}()
				}
				ok = params.fn()
				returned = true
			}
			spawned.funcResults <- ok
		}(spawned.funcs[i])
	}

	return spawned
}

type waitOKParams struct {
	fn  func() bool
	rec func(interface{})
}

type spawnedOKGroup struct {
	prev *spawnedOKGroup

	funcs       []waitOKParams
	funcResults chan bool

	firstDone, anyOK, anyNot, allDone chan struct{}
	firstResult                       bool
	aggResult                         struct{ anyOK, anyNot bool }
}

func (s *spawnedOKGroup) run() {
	var prevFirstDone, prevAnyOK, prevAnyNot, prevAllDone chan struct{}
	if s.prev != nil {
		prevFirstDone, prevAnyOK, prevAnyNot, prevAllDone = s.prev.firstDone, s.prev.anyOK, s.prev.anyNot, s.prev.allDone
	}

	var seen struct{ first, anyOK, anyNot bool }

	onFirst := func(val bool) {
		seen.first = true
		prevFirstDone = nil
		s.firstResult = val
		close(s.firstDone)
	}
	onAnyOK := func() {
		if seen.anyOK {
			return
		}
		prevAnyOK = nil
		seen.anyOK = true
		close(s.anyOK)
	}
	onAnyNot := func() {
		if seen.anyNot {
			return
		}
		prevAnyNot = nil
		seen.anyNot = true
		close(s.anyNot)
	}

	received := 0
	for received < len(s.funcs) || prevAllDone != nil {
		select {
		case res := <-s.funcResults:
			received++
			if !seen.first {
				onFirst(res)
			}
			if res {
				onAnyOK()
			}
			if !res {
				onAnyNot()
			}
		case <-prevFirstDone:
			onFirst(s.prev.firstResult)
			if s.firstResult {
				onAnyOK()
			} else {
				onAnyNot()
			}
		case <-prevAnyOK:
			onAnyOK()
		case <-prevAnyNot:
			onAnyNot()
		case <-prevAllDone:
			prevAllDone = nil
			if !seen.first {
				onFirst(s.prev.firstResult)
			}
			if s.prev.aggResult.anyOK {
				onAnyOK()
			}
			if s.prev.aggResult.anyNot {
				onAnyNot()
			}
		}
	}
	close(s.allDone)
}
