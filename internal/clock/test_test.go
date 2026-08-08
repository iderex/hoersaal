// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

package clock

import (
	"testing"
	"time"
)

// epoch is a fixed instant so that nothing in these tests depends on when they
// were run.
var epoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// TestATwoMinuteWindowCostsNoTime is the demonstration issue #27 asks for. The
// window is two minutes of the code under test's time, and it finishes in the
// same run as everything else because nothing waits.
func TestATwoMinuteWindowCostsNoTime(t *testing.T) {
	c := NewTest(epoch)
	const window = 2 * time.Minute

	fired := c.After(window)

	if c.Waiting() != 1 {
		t.Fatalf("want 1 wait registered, got %d", c.Waiting())
	}
	select {
	case <-fired:
		t.Fatal("the window closed before the clock moved")
	default:
	}

	c.Advance(window - time.Nanosecond)
	select {
	case <-fired:
		t.Fatal("the window closed a nanosecond early")
	default:
	}

	c.Advance(time.Nanosecond)
	select {
	case at := <-fired:
		if want := epoch.Add(window); !at.Equal(want) {
			t.Errorf("delivered at %v, want %v", at, want)
		}
	default:
		t.Fatal("the window did not close when the clock reached it")
	}

	if c.Waiting() != 0 {
		t.Errorf("want the wait retired, got %d still registered", c.Waiting())
	}
	if want := epoch.Add(window); !c.Now().Equal(want) {
		t.Errorf("clock reads %v, want %v", c.Now(), want)
	}
}

func TestAWindowThatHasAlreadyPassedIsDeliveredAtOnce(t *testing.T) {
	c := NewTest(epoch)
	for _, d := range []time.Duration{0, -time.Second} {
		select {
		case <-c.After(d):
		default:
			t.Errorf("a wait of %v was not delivered", d)
		}
	}
}

func TestWaitsAreDeliveredInTheOrderTheyComeDue(t *testing.T) {
	c := NewTest(epoch)
	late := c.After(90 * time.Second)
	early := c.After(30 * time.Second)

	c.Advance(2 * time.Minute)

	first := <-early
	second := <-late
	if !first.Before(second) {
		t.Errorf("delivered %v then %v, which is out of order", first, second)
	}
}

func TestAdvanceRefusesToGoBackwards(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("moving the clock backwards was allowed")
		}
	}()
	NewTest(epoch).Advance(-time.Second)
}
