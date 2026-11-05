package core

import (
	"context"
	"fmt"
	"sync"
)

// GoroutineSupervisor owns lifecycle resilience for long-lived internal workers.
// A worker panic is recovered at this boundary, reported, and restarted once.
// A second consecutive panic is reported as terminal to avoid a restart storm.
// Normal return is a clean worker termination.
type GoroutineSupervisor struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	failures map[string]int
	reports  chan GoroutineFailure
}

type GoroutineFailure struct {
	Name        string
	Panic       string
	Restarted   bool
	Consecutive int
}

func NewGoroutineSupervisor() *GoroutineSupervisor {
	return &GoroutineSupervisor{
		failures: make(map[string]int),
		reports:  make(chan GoroutineFailure, 16),
	}
}

func (s *GoroutineSupervisor) Reports() <-chan GoroutineFailure {
	if s == nil {
		return nil
	}
	return s.reports
}

func (s *GoroutineSupervisor) FailureCount(name string) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures[name]
}

func (s *GoroutineSupervisor) Start(ctx context.Context, name string, worker func(context.Context)) {
	if s == nil || worker == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		consecutive := 0
		for {
			if ctx.Err() != nil {
				return
			}

			panicked := false
			panicText := ""
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
						panicText = fmt.Sprint(r)
					}
				}()
				worker(ctx)
			}()

			if !panicked {
				return
			}

			consecutive++
			s.mu.Lock()
			s.failures[name] = consecutive
			s.mu.Unlock()

			restarted := consecutive == 1
			s.report(GoroutineFailure{
				Name:        name,
				Panic:       panicText,
				Restarted:   restarted,
				Consecutive: consecutive,
			})

			if !restarted {
				return
			}
		}
	}()
}

func (s *GoroutineSupervisor) report(failure GoroutineFailure) {
	select {
	case s.reports <- failure:
	default:
	}
}

func (s *GoroutineSupervisor) Wait() {
	if s == nil {
		return
	}
	s.wg.Wait()
}
