// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package provisionfake

import (
	"errors"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/provision"
)

var base = time.Date(2026, 9, 3, 4, 0, 0, 0, time.UTC)

func TestAFailingDriverFailsEveryAskWithItsOwnErrorAndCountsThem(t *testing.T) {
	quota := errors.New("the quota was hit")
	f := NewFailing(quota)
	for i := 1; i <= 3; i++ {
		if _, err := f.Ask(provision.Size{}); !errors.Is(err, quota) {
			t.Fatalf("Ask %d = %v, want %v", i, err, quota)
		}
		if got := f.Asked(); got != i {
			t.Fatalf("after %d asks Asked = %d", i, got)
		}
	}
	if held, err := f.Exists(); err != nil || len(held) != 0 {
		t.Fatalf("Exists = %v, %v; want nothing", held, err)
	}
	if err := f.Remove(provision.Handle{Machine: "any"}); !errors.Is(err, provision.ErrUnknown) {
		t.Fatalf("Remove = %v, want %v", err, provision.ErrUnknown)
	}
}

func TestAFailingDriverBuiltWithNoErrorStillFails(t *testing.T) {
	if _, err := NewFailing(nil).Ask(provision.Size{}); !errors.Is(err, ErrCannotProvision) {
		t.Fatalf("Ask = %v, want %v", err, ErrCannotProvision)
	}
}

// The failing driver's refusal is not the fixed pool's refusal, and a loop
// that treated them alike would either retry a pool that can never grow or
// give up on a provider that is merely down. This holds the two apart.
func TestAFailingDriverIsNotTheFixedPool(t *testing.T) {
	if _, err := NewFailing(nil).Ask(provision.Size{}); errors.Is(err, provision.ErrNoCapacity) {
		t.Fatalf("a failing driver answered %v, which is the fixed pool's terminal answer", err)
	}
}

func TestASlowDriverReportsAMachineOnlyOnceTheDelayHasPassed(t *testing.T) {
	c := clock.NewTest(base)
	s, err := NewSlow(c, 2*time.Minute)
	if err != nil {
		t.Fatalf("NewSlow: %v", err)
	}
	h, err := s.Ask(provision.Size{})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if h.Machine == "" || h.Unit.String() != h.Machine {
		t.Fatalf("the handle is %+v, want a named machine and the same name as its unit", h)
	}

	exists := func() bool {
		t.Helper()
		held, err := s.Exists()
		if err != nil || len(held) != 1 || held[0].Handle != h {
			t.Fatalf("Exists = %+v, %v; want the one machine asked for", held, err)
		}
		return held[0].Exists
	}
	if exists() {
		t.Fatal("the machine exists the moment it was asked for, and this driver is the slow one")
	}
	c.Advance(2*time.Minute - time.Nanosecond)
	if exists() {
		t.Fatal("the machine exists one nanosecond before its delay has passed")
	}
	c.Advance(time.Nanosecond)
	if !exists() {
		t.Fatal("the machine does not exist once its delay has passed")
	}
}

func TestASlowDriverWithNoDelayIsAMachineThatIsThereAtOnce(t *testing.T) {
	s, err := NewSlow(clock.NewTest(base), 0)
	if err != nil {
		t.Fatalf("NewSlow: %v", err)
	}
	if _, err := s.Ask(provision.Size{}); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	held, _ := s.Exists()
	if len(held) != 1 || !held[0].Exists {
		t.Fatalf("Exists = %+v, want one machine that is there", held)
	}
}

func TestASlowDriverHandsOutDistinctMachinesAndGivesOneBack(t *testing.T) {
	s, _ := NewSlow(clock.NewTest(base), time.Second)
	first, _ := s.Ask(provision.Size{})
	second, _ := s.Ask(provision.Size{})
	if first == second {
		t.Fatalf("two asks handed out the same machine %+v", first)
	}
	if err := s.Remove(first); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := s.Remove(first); !errors.Is(err, provision.ErrUnknown) {
		t.Fatalf("a second Remove = %v, want %v", err, provision.ErrUnknown)
	}
	held, _ := s.Exists()
	if len(held) != 1 || held[0].Handle != second {
		t.Fatalf("after Remove, Exists = %+v, want only %+v", held, second)
	}
}

func TestASlowDriverRefusesWhatItCannotMeasureOn(t *testing.T) {
	if _, err := NewSlow(nil, time.Second); !errors.Is(err, ErrNoClock) {
		t.Fatalf("NewSlow(nil clock) = %v, want %v", err, ErrNoClock)
	}
	if _, err := NewSlow(clock.NewTest(base), -time.Second); !errors.Is(err, ErrNegativeDelay) {
		t.Fatalf("NewSlow(negative delay) = %v, want %v", err, ErrNegativeDelay)
	}
}

// Both fakes are drivers, which is what lets a loop's test swap one in for the
// real thing; a fake that drifted from the interface would compile the suite
// that uses it into a suite that cannot.
func TestBothFakesAreDrivers(t *testing.T) {
	var _ provision.Driver = NewFailing(nil)
	s, _ := NewSlow(clock.NewTest(base), 0)
	var _ provision.Driver = s
}
