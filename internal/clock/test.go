// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package clock

import (
	"sort"
	"sync"
	"time"
)

// Test is a Clock whose time only moves when a test moves it. It lives in the
// package rather than in a test file because every other package's tests need
// it.
type Test struct {
	mu      sync.Mutex
	now     time.Time
	waiting []*waiter
}

type waiter struct {
	due time.Time
	ch  chan time.Time
}

// NewTest returns a Test clock reading start.
func NewTest(start time.Time) *Test { return &Test{now: start} }

// Now is the time this clock was last set or advanced to.
func (c *Test) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After registers a wait of d and returns the channel it will be delivered on.
// A duration that has already passed is delivered before After returns, so a
// caller that only ever receives cannot deadlock on one. The channel holds one
// value, so nothing is lost if the caller receives later than the delivery.
func (c *Test) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	due := c.now.Add(d)
	if !due.After(c.now) {
		ch <- c.now
		return ch
	}
	c.waiting = append(c.waiting, &waiter{due: due, ch: ch})
	return ch
}

// Advance moves the clock forward by d and delivers every wait that d reached,
// in the order the waits came due. A negative duration is refused rather than
// silently moving the clock backwards, because a clock that can go back is one
// no window can be reasoned about against.
func (c *Test) Advance(d time.Duration) {
	if d < 0 {
		panic("clock: Advance with a negative duration")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)

	due := make([]*waiter, 0, len(c.waiting))
	keep := c.waiting[:0]
	for _, w := range c.waiting {
		if w.due.After(c.now) {
			keep = append(keep, w)
			continue
		}
		due = append(due, w)
	}
	c.waiting = keep

	sort.SliceStable(due, func(i, j int) bool { return due[i].due.Before(due[j].due) })
	for _, w := range due {
		w.ch <- w.due
	}
}

// Waiting is how many registered waits have not come due. A test that wants to
// know the code under test actually asked for a window asks this rather than
// waiting to see whether anything happens.
func (c *Test) Waiting() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiting)
}
