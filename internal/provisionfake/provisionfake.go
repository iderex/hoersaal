// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package provisionfake holds the drivers a test of the scaling loop reaches
// for, which is the third condition of issue #63 as far as a driver can carry
// it. Each one is a provision.Driver that behaves badly in one named way and
// in no other, so a scenario built on it is a scenario about that one thing.
//
// Failing refuses every Ask with the error it was built with, which is the
// driver that cannot make a machine: a provider that is down, a quota that
// was hit, a credential that expired. The error is transient by the
// classification in docs/decisions/provisioning-driver.md, which is what
// separates it from provision.None, whose refusal is terminal.
//
// Slow hands out a machine at once and reports it as existing only after a
// delay has passed on the clock it was handed, which is the driver that takes
// minutes to make a machine. Against internal/clock's test clock that wait is
// an Advance rather than a sleep, so a scenario covering a long provisioning
// time finishes with the rest of the suite.
//
// A unit that never becomes healthy is not a driver. It is Slow with no
// delay, whose machine is there at once, and a pool at which nothing ever
// registers from it: whether a unit answers is the pool's question, and a
// driver that faked the answer would be faking a fact it is not the source
// of. So the scenario is built from this package and internal/pool together,
// and this comment is where that division is written down.
package provisionfake

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/placement"
	"github.com/iderex/hoersaal/internal/provision"
)

// ErrCannotProvision is what Failing answers when it was built with no error
// of its own.
var ErrCannotProvision = errors.New("the provider could not make a machine")

// ErrNoClock is a Slow built without a clock, which would have nothing to
// measure its delay on.
var ErrNoClock = errors.New("a slow driver needs a clock")

// ErrNegativeDelay is a Slow built with a delay before the ask.
var ErrNegativeDelay = errors.New("a delay cannot be negative")

// Failing refuses every Ask and counts how often it was asked, so a scenario
// can assert how many times the loop tried before it reported.
type Failing struct {
	mu    sync.Mutex
	err   error
	asked int
}

// NewFailing builds a driver that refuses every Ask with err, or with
// ErrCannotProvision where err is nil.
func NewFailing(err error) *Failing {
	if err == nil {
		err = ErrCannotProvision
	}
	return &Failing{err: err}
}

// Ask fails, every time, with the error this driver was built with.
func (f *Failing) Ask(provision.Size) (provision.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked++
	return provision.Handle{}, f.err
}

// Exists holds nothing, because nothing was ever made.
func (f *Failing) Exists() ([]provision.Machine, error) { return nil, nil }

// Remove refuses, because nothing was ever handed out.
func (f *Failing) Remove(h provision.Handle) error {
	return fmt.Errorf("%q: %w", h.Machine, provision.ErrUnknown)
}

// Asked is how many times the loop asked.
func (f *Failing) Asked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// Slow hands out a machine at once and reports it as existing once the delay
// has passed on its clock.
type Slow struct {
	mu      sync.Mutex
	clock   clock.Clock
	delay   time.Duration
	next    int
	order   []string
	appears map[string]time.Time
}

// NewSlow builds a driver whose machines appear delay after they are asked
// for, read on c. A delay of zero is a machine that is there at once.
func NewSlow(c clock.Clock, delay time.Duration) (*Slow, error) {
	if c == nil {
		return nil, ErrNoClock
	}
	if delay < 0 {
		return nil, fmt.Errorf("%s: %w", delay, ErrNegativeDelay)
	}
	return &Slow{clock: c, delay: delay, appears: map[string]time.Time{}}, nil
}

// Ask hands out the next machine and records when it will appear.
func (s *Slow) Ask(provision.Size) (provision.Handle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	name := fmt.Sprintf("slow-%d", s.next)
	s.order = append(s.order, name)
	s.appears[name] = s.clock.Now().Add(s.delay)
	return s.handle(name), nil
}

// Exists reports every machine handed out and not given back, in the order
// they were asked for, each existing once its moment has passed on the clock.
func (s *Slow) Exists() ([]provision.Machine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	var out []provision.Machine
	for _, name := range s.order {
		out = append(out, provision.Machine{
			Handle: s.handle(name),
			Exists: !now.Before(s.appears[name]),
		})
	}
	return out, nil
}

// Remove gives a machine back, whether or not it had appeared yet.
func (s *Slow) Remove(h provision.Handle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, held := s.appears[h.Machine]; !held {
		return fmt.Errorf("%q: %w", h.Machine, provision.ErrUnknown)
	}
	delete(s.appears, h.Machine)
	kept := s.order[:0]
	for _, name := range s.order {
		if name != h.Machine {
			kept = append(kept, name)
		}
	}
	s.order = kept
	return nil
}

// handle builds the handle for a name this driver made, which is never empty.
func (s *Slow) handle(name string) provision.Handle {
	id, _ := placement.NewUnitID(name)
	return provision.Handle{Unit: id, Machine: name}
}
