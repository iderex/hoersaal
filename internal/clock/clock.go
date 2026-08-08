// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package clock is the only place in this repository that reads the machine's
// clock. Everything else takes a Clock and is handed one.
//
// The reason is issue #27. Almost everything this service does is a duration:
// reconnect windows, observation windows, cooldowns, and the timeout on a unit
// that stopped answering. A suite that covers those by waiting is slow and is
// flaky on a loaded runner, and a flaky suite gets rerun until it is green,
// which is the same as not having one. A test that covers a two minute window
// advances a Test clock by two minutes and finishes in the same instant as the
// rest of the run.
//
// The guard that refuses a direct call to the machine's clock anywhere else is
// in the guard package, and it names this file as the one place allowed to make
// one.
package clock

import "time"

// A Clock reports the time and says when a duration has passed.
type Clock interface {
	// Now is the current time as this clock sees it.
	Now() time.Time

	// After delivers one value once d has passed. A duration that has already
	// passed delivers immediately. The channel is never closed, and nothing is
	// sent on it a second time.
	After(d time.Duration) <-chan time.Time
}

// System returns the clock that reads the machine. It is what a running service
// is given, and it is the only Clock that can be slow.
func System() Clock { return systemClock{} }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
