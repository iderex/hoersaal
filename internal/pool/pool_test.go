// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package pool

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/mediafake"
	"github.com/iderex/hoersaal/internal/placement"
	"github.com/iderex/hoersaal/internal/secret"
)

// Two keys of the minimum length, written out rather than generated, for the
// reason internal/roomcred's suite gives at the same place: a suite that makes
// its own keys needs a source of randomness, and nothing here has one. They are
// test material and nothing registers anything real with them.
var (
	keyA = []byte("0123456789abcdef0123456789abcdef")
	keyB = []byte("fedcba9876543210fedcba9876543210")
)

// base is the instant every test starts from. time.Date is arithmetic on the
// calendar and reads no clock, which is what internal/guard refuses.
var base = time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC)

func unitID(t *testing.T, name string) placement.UnitID {
	t.Helper()
	id, err := placement.NewUnitID(name)
	if err != nil {
		t.Fatalf("NewUnitID(%q): %v", name, err)
	}
	return id
}

// newPool builds a pool on a clock the test controls, which is what lets a test
// covering the five minute proof window advance five minutes and finish with
// the rest of the run.
func newPool(t *testing.T) (*Pool, *clock.Test) {
	t.Helper()
	c := clock.NewTest(base)
	p, err := New(secret.Bytes(keyA), c)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, c
}

// registration is a proof made with keyA at the pool's own now.
func registration(t *testing.T, id placement.UnitID, at time.Time) Registration {
	t.Helper()
	pr, err := Prove(secret.Bytes(keyA), id, at)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	return Registration{Unit: id, IssuedAt: at, Proof: pr}
}

// port is a unit that answers. The pool holds a handle per registered unit and
// asks it what it is holding, so a registration needs one; the fake is what
// this repository reaches for, and it needs no media, no device and no network.
func port(t *testing.T, name string, c clock.Clock) *mediafake.Unit {
	t.Helper()
	u, err := mediafake.NewFabric().Add(name, c)
	if err != nil {
		t.Fatalf("mediafake Add(%q): %v", name, err)
	}
	return u
}

// admitted is a pool holding one unit that has walked the whole way in:
// requested, started, verified reachable, and registered with a good proof.
func admitted(t *testing.T, name string) (*Pool, *clock.Test, placement.UnitID, *mediafake.Unit) {
	t.Helper()
	p, c := newPool(t)
	id := unitID(t, name)
	u := port(t, name, c)
	if err := p.Request(id); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if err := p.Started(id); err != nil {
		t.Fatalf("Started: %v", err)
	}
	if err := p.Reachable(id, true); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if err := p.Register(registration(t, id, c.Now()), u); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := state(t, p, id); got != Admitting() {
		t.Fatalf("after registering, the unit is %s, want %s", got, Admitting())
	}
	return p, c, id, u
}

func state(t *testing.T, p *Pool, id placement.UnitID) State {
	t.Helper()
	u, held := p.Unit(id)
	if !held {
		t.Fatalf("the pool does not hold unit %s", id)
	}
	return u.State()
}

// The state machine.
//
// The moves are named here rather than reached through the methods directly, so
// that the legal table and the illegal table below are written against one list
// and cannot drift into two.
type move struct {
	name string
	do   func(p *Pool, id placement.UnitID, u *mediafake.Unit, r Registration) error
}

func moves() []move {
	return []move{
		{"Started", func(p *Pool, id placement.UnitID, _ *mediafake.Unit, _ Registration) error {
			return p.Started(id)
		}},
		{"Register", func(p *Pool, _ placement.UnitID, u *mediafake.Unit, r Registration) error {
			return p.Register(r, u)
		}},
		{"Drain", func(p *Pool, id placement.UnitID, _ *mediafake.Unit, _ Registration) error {
			return p.Drain(id, TheScaleInConditionHeld())
		}},
		{"Retire", func(p *Pool, id placement.UnitID, _ *mediafake.Unit, _ Registration) error {
			return p.Retire(id, TheOperatorAsked())
		}},
	}
}

// legal is the whole machine, written as the pairs that are in it. Everything
// not here is refused, which is what the second table asserts.
var legal = map[string]map[string]State{
	"requested": {"Started": Starting(), "Retire": Gone()},
	"starting":  {"Register": Admitting(), "Retire": Gone()},
	"admitting": {"Drain": Draining(), "Retire": Gone()},
	"draining":  {"Retire": Gone()},
	"gone":      {},
}

// at builds a pool holding one reachable unit already in the state named, so
// that both tables below start from the same place for a state.
func at(t *testing.T, s State) (*Pool, placement.UnitID, *mediafake.Unit, Registration) {
	t.Helper()
	p, c := newPool(t)
	id := unitID(t, "unit-a")
	u := port(t, "unit-a", c)
	r := registration(t, id, c.Now())

	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("reaching %s, %s: %v", s, what, err)
		}
	}
	must("Request", p.Request(id))
	must("Reachable", p.Reachable(id, true))
	if s == Requested() {
		return p, id, u, r
	}
	must("Started", p.Started(id))
	if s == Starting() {
		return p, id, u, r
	}
	must("Register", p.Register(r, u))
	if s == Admitting() {
		return p, id, u, r
	}
	must("Drain", p.Drain(id, TheOperatorAsked()))
	if s == Draining() {
		return p, id, u, r
	}
	must("Retire", p.Retire(id, TheOperatorAsked()))
	return p, id, u, r
}

func states() []State {
	return []State{Requested(), Starting(), Admitting(), Draining(), Gone()}
}

func TestEveryLegalTransitionMovesTheUnit(t *testing.T) {
	for _, from := range states() {
		for _, m := range moves() {
			want, ok := legal[from.String()][m.name]
			if !ok {
				continue
			}
			t.Run(from.String()+"/"+m.name, func(t *testing.T) {
				p, id, u, r := at(t, from)
				if err := m.do(p, id, u, r); err != nil {
					t.Fatalf("%s from %s: %v", m.name, from, err)
				}
				if got := state(t, p, id); got != want {
					t.Fatalf("%s from %s left the unit %s, want %s", m.name, from, got, want)
				}
			})
		}
	}
}

func TestEveryIllegalTransitionIsRefusedRatherThanLogged(t *testing.T) {
	for _, from := range states() {
		for _, m := range moves() {
			if _, ok := legal[from.String()][m.name]; ok {
				continue
			}
			t.Run(from.String()+"/"+m.name, func(t *testing.T) {
				p, id, u, r := at(t, from)
				err := m.do(p, id, u, r)
				if !errors.Is(err, ErrIllegalTransition) {
					t.Fatalf("%s from %s = %v, want %v", m.name, from, err, ErrIllegalTransition)
				}
				if got := state(t, p, id); got != from {
					t.Fatalf("a refused %s moved the unit from %s to %s", m.name, from, got)
				}
			})
		}
	}
}

func TestEveryMoveOnAUnitThePoolDoesNotHoldIsRefused(t *testing.T) {
	for _, m := range moves() {
		t.Run(m.name, func(t *testing.T) {
			p, c := newPool(t)
			id := unitID(t, "unit-nobody-asked-for")
			u := port(t, "unit-nobody-asked-for", c)
			err := m.do(p, id, u, registration(t, id, c.Now()))
			if !errors.Is(err, ErrUnknownUnit) {
				t.Fatalf("%s on an absent unit = %v, want %v", m.name, err, ErrUnknownUnit)
			}
		})
	}
}

func TestAUnitEntersOnceUnderOneIdentifier(t *testing.T) {
	p, _ := newPool(t)
	id := unitID(t, "unit-a")
	if err := p.Request(id); err != nil {
		t.Fatalf("Request: %v", err)
	}
	for _, entry := range []struct {
		name string
		do   func() error
	}{
		{"Request", func() error { return p.Request(id) }},
		{"Listed", func() error { return p.Listed(id) }},
	} {
		if err := entry.do(); !errors.Is(err, ErrDuplicate) {
			t.Fatalf("a second %s = %v, want %v", entry.name, err, ErrDuplicate)
		}
	}
}

func TestAListedMachineEntersAtStarting(t *testing.T) {
	p, _ := newPool(t)
	id := unitID(t, "unit-the-operator-listed")
	if err := p.Listed(id); err != nil {
		t.Fatalf("Listed: %v", err)
	}
	if got := state(t, p, id); got != Starting() {
		t.Fatalf("a listed machine is %s, want %s", got, Starting())
	}
}

// The registration.

func TestAnUnauthenticatedRegistrationIsRefused(t *testing.T) {
	good := func(t *testing.T, id placement.UnitID, at time.Time) []byte {
		t.Helper()
		return registration(t, id, at).Proof
	}

	for _, c := range []struct {
		name  string
		proof func(t *testing.T, id placement.UnitID, at time.Time) []byte
	}{
		{"no proof at all", func(*testing.T, placement.UnitID, time.Time) []byte { return nil }},
		{"an empty proof", func(*testing.T, placement.UnitID, time.Time) []byte { return []byte{} }},
		{"a proof made with another key", func(t *testing.T, id placement.UnitID, at time.Time) []byte {
			t.Helper()
			pr, err := Prove(secret.Bytes(keyB), id, at)
			if err != nil {
				t.Fatalf("Prove: %v", err)
			}
			return pr
		}},
		{"a proof made for another unit", func(t *testing.T, _ placement.UnitID, at time.Time) []byte {
			t.Helper()
			return good(t, unitID(t, "unit-somebody-else"), at)
		}},
		{"a proof made for another moment", func(t *testing.T, id placement.UnitID, at time.Time) []byte {
			t.Helper()
			return good(t, id, at.Add(time.Second))
		}},
		{"a proof with one byte changed", func(t *testing.T, id placement.UnitID, at time.Time) []byte {
			t.Helper()
			pr := good(t, id, at)
			pr[0] ^= 0x01
			return pr
		}},
		{"a proof one byte short", func(t *testing.T, id placement.UnitID, at time.Time) []byte {
			t.Helper()
			return good(t, id, at)[:MinKeyBytes-1]
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, clk := newPool(t)
			id := unitID(t, "unit-a")
			u := port(t, "unit-a", clk)
			if err := p.Listed(id); err != nil {
				t.Fatalf("Listed: %v", err)
			}
			if err := p.Reachable(id, true); err != nil {
				t.Fatalf("Reachable: %v", err)
			}
			r := Registration{Unit: id, IssuedAt: clk.Now(), Proof: c.proof(t, id, clk.Now())}
			if err := p.Register(r, u); !errors.Is(err, ErrProof) {
				t.Fatalf("Register with %s = %v, want %v", c.name, err, ErrProof)
			}
			if got := state(t, p, id); got != Starting() {
				t.Fatalf("a refused registration left the unit %s, want %s", got, Starting())
			}
		})
	}
}

func TestAProofOutsideTheWindowIsRefusedInBothDirections(t *testing.T) {
	for _, c := range []struct {
		name  string
		skew  time.Duration
		admit bool
	}{
		{"made now", 0, true},
		{"made at the far edge of the window", -MaxProofAge, true},
		{"made a moment past the far edge", -MaxProofAge - time.Second, false},
		{"made at the near edge by a clock that runs fast", MaxProofAge, true},
		{"made a moment past the near edge", MaxProofAge + time.Second, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, clk := newPool(t)
			id := unitID(t, "unit-a")
			u := port(t, "unit-a", clk)
			if err := p.Listed(id); err != nil {
				t.Fatalf("Listed: %v", err)
			}
			if err := p.Reachable(id, true); err != nil {
				t.Fatalf("Reachable: %v", err)
			}
			err := p.Register(registration(t, id, clk.Now().Add(c.skew)), u)
			switch {
			case c.admit && err != nil:
				t.Fatalf("a proof %s = %v, want it admitted", c.name, err)
			case !c.admit && !errors.Is(err, ErrProof):
				t.Fatalf("a proof %s = %v, want %v", c.name, err, ErrProof)
			}
		})
	}
}

func TestAProofGoesStaleWhileTheUnitWaits(t *testing.T) {
	p, clk := newPool(t)
	id := unitID(t, "unit-a")
	u := port(t, "unit-a", clk)
	if err := p.Listed(id); err != nil {
		t.Fatalf("Listed: %v", err)
	}
	if err := p.Reachable(id, true); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	r := registration(t, id, clk.Now())
	clk.Advance(MaxProofAge + time.Second)
	if err := p.Register(r, u); !errors.Is(err, ErrProof) {
		t.Fatalf("a proof held past the window = %v, want %v", err, ErrProof)
	}
}

func TestARegistrationWithNoReachabilityVerdictIsRefused(t *testing.T) {
	p, clk := newPool(t)
	id := unitID(t, "unit-a")
	u := port(t, "unit-a", clk)
	if err := p.Listed(id); err != nil {
		t.Fatalf("Listed: %v", err)
	}
	err := p.Register(registration(t, id, clk.Now()), u)
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("registering an unverified unit = %v, want %v", err, ErrIllegalTransition)
	}
	if got := state(t, p, id); got != Starting() {
		t.Fatalf("the unit is %s, want %s", got, Starting())
	}
}

func TestARegistrationWithNoPortHandleIsRefused(t *testing.T) {
	p, clk := newPool(t)
	id := unitID(t, "unit-a")
	if err := p.Listed(id); err != nil {
		t.Fatalf("Listed: %v", err)
	}
	if err := p.Reachable(id, true); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if err := p.Register(registration(t, id, clk.Now()), nil); !errors.Is(err, ErrNoPort) {
		t.Fatalf("registering with no handle = %v, want %v", err, ErrNoPort)
	}
}

func TestProveRefusesWhatItCannotProve(t *testing.T) {
	if _, err := Prove(secret.Bytes(keyA[:MinKeyBytes-1]), unitID(t, "unit-a"), base); !errors.Is(err, ErrKeyTooShort) {
		t.Fatalf("Prove with a short key = %v, want %v", err, ErrKeyTooShort)
	}
	if _, err := Prove(secret.Bytes(keyA), placement.UnitID{}, base); !errors.Is(err, ErrEmpty) {
		t.Fatalf("Prove for no unit = %v, want %v", err, ErrEmpty)
	}
}

// The three inputs.

func TestFailingHealthTakesAUnitOutOfAdmitting(t *testing.T) {
	p, _, id, _ := admitted(t, "unit-a")
	if err := p.Healthy(id, false); err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	u, _ := p.Unit(id)
	if u.State() != Gone() {
		t.Fatalf("a unit that stopped answering is %s, want %s", u.State(), Gone())
	}
	if u.Cause() != HealthFailed() {
		t.Fatalf("the cause is %q, want %q", u.Cause(), HealthFailed())
	}
}

func TestFailingReachabilityTakesAUnitOutOfAdmittingWithoutLosingWhatItHolds(t *testing.T) {
	p, _, id, _ := admitted(t, "unit-a")
	if err := p.Reachable(id, false); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	u, _ := p.Unit(id)
	if u.State() != Draining() {
		t.Fatalf("a unit participants cannot reach is %s, want %s", u.State(), Draining())
	}
	if u.Cause() != ReachabilityFailed() {
		t.Fatalf("the cause is %q, want %q", u.Cause(), ReachabilityFailed())
	}
	if !u.Healthy() {
		t.Fatal("an unreachable unit was recorded as unhealthy, and the two inputs are separate")
	}
}

func TestReachabilityComingBackDoesNotUndoTheDrain(t *testing.T) {
	p, _, id, _ := admitted(t, "unit-a")
	if err := p.Reachable(id, false); err != nil {
		t.Fatalf("Reachable(false): %v", err)
	}
	if err := p.Reachable(id, true); err != nil {
		t.Fatalf("Reachable(true): %v", err)
	}
	if got := state(t, p, id); got != Draining() {
		t.Fatalf("the unit is %s, want it still %s", got, Draining())
	}
}

func TestAStaleSignalTakesAUnitOutOfAdmitting(t *testing.T) {
	const window = 30 * time.Second
	p, clk, id, u := admitted(t, "unit-a")

	u.SetLoad(0.4)
	p.Collect()
	clk.Advance(window)
	retired, err := p.Sweep(window)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(retired) != 0 {
		t.Fatalf("a report exactly at the bound retired %v, want nothing", retired)
	}

	clk.Advance(time.Second)
	retired, err = p.Sweep(window)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(retired) != 1 || retired[0] != id {
		t.Fatalf("Sweep past the bound retired %v, want [%s]", retired, id)
	}
	row, _ := p.Unit(id)
	if row.State() != Gone() || row.Cause() != TheSignalWentStale() {
		t.Fatalf("the unit is %s because %q, want %s because %q", row.State(), row.Cause(), Gone(), TheSignalWentStale())
	}
}

func TestAUnitThatRegisteredAndNeverReportedGoesStaleToo(t *testing.T) {
	const window = 30 * time.Second
	p, clk, id, _ := admitted(t, "unit-a")
	clk.Advance(window + time.Second)
	retired, err := p.Sweep(window)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(retired) != 1 || retired[0] != id {
		t.Fatalf("Sweep retired %v, want [%s]", retired, id)
	}
}

func TestSweepLeavesTheStatesItHasNoBusinessWith(t *testing.T) {
	p, clk := newPool(t)
	requested := unitID(t, "unit-requested")
	starting := unitID(t, "unit-starting")
	if err := p.Request(requested); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if err := p.Listed(starting); err != nil {
		t.Fatalf("Listed: %v", err)
	}
	clk.Advance(time.Hour)
	retired, err := p.Sweep(time.Second)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(retired) != 0 {
		t.Fatalf("Sweep retired %v, want nothing: a machine that has not registered has no signal to go stale", retired)
	}
}

func TestSweepRefusesABoundThatWouldRetireEverything(t *testing.T) {
	p, _ := newPool(t)
	for _, d := range []time.Duration{0, -time.Second} {
		if _, err := p.Sweep(d); !errors.Is(err, ErrNotAnAge) {
			t.Fatalf("Sweep(%s) = %v, want %v", d, err, ErrNotAnAge)
		}
	}
}

func TestTheThreeInputsAreRecordedSeparately(t *testing.T) {
	p, _, id, u := admitted(t, "unit-a")
	u.SetLoad(0.25)
	p.Collect()

	row, _ := p.Unit(id)
	switch {
	case !row.Healthy():
		t.Fatal("health is false on a unit nothing said was unhealthy")
	case !row.Reachable():
		t.Fatal("reachability is false on a unit that was verified reachable")
	case row.Reported() != 0.25:
		t.Fatalf("the signal is %v, want 0.25", row.Reported())
	}
}

// The view, and what a unit believes about itself.

func TestThePoolViewAgreesWithWhatEachUnitBelievesAboutItself(t *testing.T) {
	c := clock.NewTest(base)
	p, err := New(secret.Bytes(keyA), c)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fabric := mediafake.NewFabric()

	loads := map[string]float64{"unit-a": 0.10, "unit-b": 0.55, "unit-c": 1.20}
	units := map[string]*mediafake.Unit{}
	for _, name := range []string{"unit-a", "unit-b", "unit-c"} {
		u, err := fabric.Add(name, c)
		if err != nil {
			t.Fatalf("mediafake Add(%q): %v", name, err)
		}
		u.SetLoad(loads[name])
		units[name] = u

		id := unitID(t, name)
		if err := p.Listed(id); err != nil {
			t.Fatalf("Listed(%s): %v", name, err)
		}
		if err := p.Reachable(id, true); err != nil {
			t.Fatalf("Reachable(%s): %v", name, err)
		}
		if err := p.Register(registration(t, id, c.Now()), u); err != nil {
			t.Fatalf("Register(%s): %v", name, err)
		}
	}

	if answered := p.Collect(); len(answered) != 3 {
		t.Fatalf("Collect asked and recorded %d unit(s), want 3", len(answered))
	}

	rows := p.Units()
	if len(rows) != 3 {
		t.Fatalf("the pool holds %d row(s) in one call, want 3", len(rows))
	}
	for _, row := range rows {
		believed, err := units[row.ID().String()].ReportCapacity()
		if err != nil {
			t.Fatalf("asking %s: %v", row.ID(), err)
		}
		if row.Reported() != believed {
			t.Fatalf("the pool holds %v for %s and the unit believes %v", row.Reported(), row.ID(), believed)
		}
		if row.ReportedAt() != base {
			t.Fatalf("the pool timed %s at %s, want %s", row.ID(), row.ReportedAt(), base)
		}
	}

	view, err := p.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	for _, u := range view.Units() {
		if want := loads[u.ID().String()]; u.EffectiveLoad() != want {
			t.Fatalf("the placer would read %v for %s, want %v", u.EffectiveLoad(), u.ID(), want)
		}
	}
}

func TestAUnitThatDoesNotAnswerLeavesItsRowWhereItWas(t *testing.T) {
	p, _, id, u := admitted(t, "unit-a")
	u.SetLoad(0.4)
	p.Collect()

	u.Die()
	if answered := p.Collect(); len(answered) != 0 {
		t.Fatalf("Collect recorded %v from a unit that did not answer", answered)
	}
	row, _ := p.Unit(id)
	switch {
	case row.State() != Admitting():
		t.Fatalf("one unanswered call left the unit %s, and one is not a death", row.State())
	case row.Reported() != 0.4:
		t.Fatalf("the last load is %v, want the 0.4 the unit gave before it stopped answering", row.Reported())
	}
}

func TestCollectAsksOnlyTheUnitsThatCanAnswer(t *testing.T) {
	p, clk := newPool(t)
	requested := unitID(t, "unit-requested")
	if err := p.Request(requested); err != nil {
		t.Fatalf("Request: %v", err)
	}
	drained := unitID(t, "unit-draining")
	u := port(t, "unit-draining", clk)
	u.SetLoad(0.7)
	if err := p.Listed(drained); err != nil {
		t.Fatalf("Listed: %v", err)
	}
	if err := p.Reachable(drained, true); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if err := p.Register(registration(t, drained, clk.Now()), u); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Drain(drained, TheScaleInConditionHeld()); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	answered := p.Collect()
	if len(answered) != 1 || answered[0] != drained {
		t.Fatalf("Collect recorded %v, want [%s]: a draining unit still reports and a requested one has nothing to ask", answered, drained)
	}
}

func TestTheViewOmitsTheStatesTheSeamHasNoWordFor(t *testing.T) {
	p, clk := newPool(t)
	for _, name := range []string{"unit-requested", "unit-starting"} {
		id := unitID(t, name)
		if err := p.Request(id); err != nil {
			t.Fatalf("Request(%s): %v", name, err)
		}
		if name == "unit-starting" {
			if err := p.Started(id); err != nil {
				t.Fatalf("Started: %v", err)
			}
		}
	}
	view, err := p.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if view.Len() != 0 {
		t.Fatalf("the placer sees %d row(s), want none: neither state has a word in the seam", view.Len())
	}
	if len(p.Units()) != 2 {
		t.Fatalf("the pool holds %d row(s), want 2: the machines are still on their way", len(p.Units()))
	}

	id := unitID(t, "unit-a")
	u := port(t, "unit-a", clk)
	if err := p.Listed(id); err != nil {
		t.Fatalf("Listed: %v", err)
	}
	if err := p.Reachable(id, true); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if err := p.Register(registration(t, id, clk.Now()), u); err != nil {
		t.Fatalf("Register: %v", err)
	}
	view, err = p.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if view.Len() != 1 || view.Units()[0].State() != placement.Admitting() {
		t.Fatalf("the placer sees %d row(s), want one that is admitting", view.Len())
	}
}

func TestEffectiveLoadCarriesEveryCommitmentMadeSinceTheReport(t *testing.T) {
	p, _, id, u := admitted(t, "unit-a")
	u.SetLoad(0.30)
	p.Collect()

	for range 3 {
		if err := p.Commit(id, 0.05); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
	row, _ := p.Unit(id)
	if row.Reported() != 0.30 {
		t.Fatalf("committing moved the reported load to %v, and the unit said 0.30", row.Reported())
	}
	if row.EffectiveLoad() != 0.45 {
		t.Fatalf("the effective load is %v, want 0.45", row.EffectiveLoad())
	}

	p.Collect()
	row, _ = p.Unit(id)
	if row.EffectiveLoad() != 0.30 {
		t.Fatalf("after a fresh report the effective load is %v, want 0.30: the unit's own number already carries what it has noticed", row.EffectiveLoad())
	}
}

func TestCommitIsRefusedAgainstAUnitThePlacerMayNotChoose(t *testing.T) {
	p, _, id, _ := admitted(t, "unit-a")
	if err := p.Drain(id, TheOperatorAsked()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if err := p.Commit(id, 0.1); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("committing against a draining unit = %v, want %v", err, ErrIllegalTransition)
	}
}

func TestALoadThatIsNotOneIsRefusedWhereItEnters(t *testing.T) {
	p, _, id, _ := admitted(t, "unit-a")
	nan := math.NaN()
	for _, c := range []struct {
		name string
		do   func(float64) error
	}{
		{"Report", func(l float64) error { return p.Report(id, l) }},
		{"Commit", func(l float64) error { return p.Commit(id, l) }},
	} {
		for _, l := range []float64{-0.1, nan} {
			if err := c.do(l); !errors.Is(err, ErrNotALoad) {
				t.Fatalf("%s(%v) = %v, want %v", c.name, l, err, ErrNotALoad)
			}
		}
	}
}

func TestUnitsIsOrderedByIdentifierWhateverOrderTheyArrivedIn(t *testing.T) {
	p, _ := newPool(t)
	for _, name := range []string{"unit-c", "unit-a", "unit-b"} {
		if err := p.Listed(unitID(t, name)); err != nil {
			t.Fatalf("Listed(%s): %v", name, err)
		}
	}
	var got []string
	for _, u := range p.Units() {
		got = append(got, u.ID().String())
	}
	if want := "unit-a unit-b unit-c"; strings.Join(got, " ") != want {
		t.Fatalf("Units gave %q, want %q", strings.Join(got, " "), want)
	}
}

func TestReadingThePoolCannotChangeWhatTheNextPlacementSees(t *testing.T) {
	p, _, id, u := admitted(t, "unit-a")
	u.SetLoad(0.2)
	p.Collect()

	rows := p.Units()
	rows[0].reported = 0.99
	again, _ := p.Unit(id)
	if again.Reported() != 0.2 {
		t.Fatalf("editing a row a caller was given moved the pool to %v", again.Reported())
	}
}

func TestNewRefusesAPoolThatCouldNotDoItsJob(t *testing.T) {
	if _, err := New(secret.Bytes(keyA[:MinKeyBytes-1]), clock.NewTest(base)); !errors.Is(err, ErrKeyTooShort) {
		t.Fatalf("New with a short key = %v, want %v", err, ErrKeyTooShort)
	}
	if _, err := New(secret.Bytes(keyA), nil); !errors.Is(err, ErrNoClock) {
		t.Fatalf("New with no clock = %v, want %v", err, ErrNoClock)
	}
}

func TestAnEmptyIdentifierIsRefusedWhereverItEnters(t *testing.T) {
	p, _ := newPool(t)
	empty := placement.UnitID{}
	for _, c := range []struct {
		name string
		do   func() error
	}{
		{"Request", func() error { return p.Request(empty) }},
		{"Listed", func() error { return p.Listed(empty) }},
		{"Started", func() error { return p.Started(empty) }},
		{"Reachable", func() error { return p.Reachable(empty, true) }},
		{"Healthy", func() error { return p.Healthy(empty, true) }},
		{"Register", func() error { return p.Register(Registration{}, port(t, "unit-a", clock.NewTest(base))) }},
	} {
		if err := c.do(); !errors.Is(err, ErrEmpty) {
			t.Fatalf("%s of an empty identifier = %v, want %v", c.name, err, ErrEmpty)
		}
	}
	if _, held := p.Unit(empty); held {
		t.Fatal("the pool holds a unit under an empty identifier")
	}
}

// Printing the pool prints the key, unless the pool answers for itself. fmt
// reaches an unexported field by reflection and does not call the methods on
// its type, which internal/secret's own comment says is a limit of the type
// rather than a defect in it, so a struct holding a secret has to carry a
// Format of its own.
func TestPrintingAPoolDoesNotRevealTheKey(t *testing.T) {
	p, _ := newPool(t)
	for _, verb := range []string{"%v", "%s", "%q", "%d", "%x", "%X", "%#v", "%+v"} {
		out := fmt.Sprintf(verb, p)
		if strings.Contains(out, string(keyA)) {
			t.Fatalf("%s of a pool contains the registration key: %q", verb, out)
		}
		if !strings.Contains(out, secret.Placeholder) {
			t.Fatalf("%s of a pool is %q, want it to carry %q", verb, out, secret.Placeholder)
		}
	}
}

func TestTheProofCoversTheVersionTheUnitAndTheMoment(t *testing.T) {
	id := unitID(t, "unit-a")
	one := payload(id, base)
	for _, c := range []struct {
		name  string
		other []byte
	}{
		{"another unit", payload(unitID(t, "unit-b"), base)},
		{"another moment", payload(id, base.Add(time.Second))},
	} {
		if string(one) == string(c.other) {
			t.Fatalf("the covered bytes are the same for %s, so a proof for one admits the other", c.name)
		}
	}
	if one[0] != ProofVersion {
		t.Fatalf("the covered bytes open with %d, want the version %d", one[0], ProofVersion)
	}
}

// One identifier that is a prefix of another is the pair a layout gets wrong,
// so it gets its own case. It passes without the length in front of the
// identifier as well, because the moment behind it is fixed width, and the
// package comment on payload says so rather than letting this test be read as
// proof of a guard it does not reach.
func TestOneIdentifierThatIsAPrefixOfAnotherIsStillTwoProofs(t *testing.T) {
	short := payload(unitID(t, "unit-a"), base)
	long := payload(unitID(t, "unit-ab"), base)
	if string(short) == string(long) {
		t.Fatal("unit-a and unit-ab cover the same bytes")
	}
}
