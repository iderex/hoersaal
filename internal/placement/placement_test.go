// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// The suite is in the package rather than beside it as placement_test, which is
// the shape a caller would have. internal/arch refuses it: the allow-list for
// this directory is internal/domain and nothing else in this repository, it
// covers _test.go files deliberately, and an external test package imports the
// package it tests. Nothing below reaches for an unexported name, so what the
// cases exercise is still only what a caller can reach.
package placement

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/domain"
)

// naive is the placer under test, held as the seam rather than as the concrete
// type, so every case below goes through the interface issue #57 asks the policy
// to sit behind and the assignment itself is the assertion that it does.
var naive ConferencePlacer = Naive{}

// reported is the one instant every scripted row carries. The placer does not
// read it, which is what this test file demonstrates by using a single value
// everywhere and still getting different answers out of different loads. It is a
// literal rather than a clock read, because internal/guard refuses a clock read
// outside internal/clock and the placer is not allowed to hold one either way.
var reported = time.Date(2026, time.August, 8, 9, 0, 0, 0, time.UTC)

// unit builds a scripted row and fails the test rather than returning an error,
// so that a case reads as the pool it is about rather than as error handling.
func unit(t *testing.T, id string, load float64, state State) Unit {
	t.Helper()
	uid, err := NewUnitID(id)
	if err != nil {
		t.Fatalf("unit id %q: %v", id, err)
	}
	u, err := NewUnit(uid, load, reported, state)
	if err != nil {
		t.Fatalf("unit %q at %v: %v", id, load, err)
	}
	return u
}

func pool(t *testing.T, units ...Unit) Pool {
	t.Helper()
	p, err := NewPool(units...)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return p
}

func conference(t *testing.T, id string) domain.ConferenceID {
	t.Helper()
	c, err := domain.NewConferenceID(id)
	if err != nil {
		t.Fatalf("conference id %q: %v", id, err)
	}
	return c
}

// placedOn reads the answer as a placement and fails saying what it got instead.
func placedOn(t *testing.T, a Answer) string {
	t.Helper()
	u, ok := a.Placed()
	if !ok {
		t.Fatalf("wanted a placement, got %s", a)
	}
	return u.String()
}

// TestTheScriptedPoolsAndTheUnitEachOneChooses is the first condition of issue
// #57: a scripted pool per case with the chosen unit asserted, rather than a run
// checked for not crashing.
func TestTheScriptedPoolsAndTheUnitEachOneChooses(t *testing.T) {
	cases := []struct {
		name  string
		units []Unit
		want  string
	}{
		{
			name:  "one admitting unit takes it",
			units: []Unit{unit(t, "b", 0.10, Admitting())},
			want:  "b",
		},
		{
			name: "the lowest effective load wins, whatever order the pool returns",
			units: []Unit{
				unit(t, "a", 0.80, Admitting()),
				unit(t, "b", 0.20, Admitting()),
				unit(t, "c", 0.50, Admitting()),
			},
			want: "b",
		},
		{
			name: "a tie is broken by the smallest identifier",
			units: []Unit{
				unit(t, "c", 0.30, Admitting()),
				unit(t, "a", 0.30, Admitting()),
				unit(t, "b", 0.30, Admitting()),
			},
			want: "a",
		},
		{
			name: "a draining unit is skipped however empty it is",
			units: []Unit{
				unit(t, "a", 0.00, Draining()),
				unit(t, "b", 0.70, Admitting()),
			},
			want: "b",
		},
		{
			name: "a gone unit is skipped however empty it is",
			units: []Unit{
				unit(t, "a", 0.00, Gone()),
				unit(t, "b", 0.70, Admitting()),
			},
			want: "b",
		},
		{
			name: "a unit above the ceiling is skipped and a fuller eligible one is not",
			units: []Unit{
				unit(t, "a", 0.95, Admitting()),
				unit(t, "b", 0.89, Admitting()),
			},
			want: "b",
		},
		{
			name: "a unit exactly one step below the ceiling is still eligible",
			units: []Unit{
				unit(t, "a", math.Nextafter(0.90, 0), Admitting()),
			},
			want: "a",
		},
		{
			name: "a unit past its calibrated capacity is skipped",
			units: []Unit{
				unit(t, "a", 1.40, Admitting()),
				unit(t, "b", 0.85, Admitting()),
			},
			want: "b",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := naive.PlaceConference(pool(t, c.units...), conference(t, "lecture"))
			if on := placedOn(t, got); on != c.want {
				t.Errorf("placed on %s, wanted %s", on, c.want)
			}
		})
	}
}

// TestTheCeilingIsBelowAndNotAt is the near miss. Changing the comparison in
// eligible from < to <= is one character and reds only this test, which is what
// makes the ceiling a boundary rather than a number in a comment.
func TestTheCeilingIsBelowAndNotAt(t *testing.T) {
	at := pool(t, unit(t, "a", EligibilityCeiling, Admitting()))
	if r, refused := naive.PlaceConference(at, conference(t, "lecture")).Refused(); !refused {
		t.Errorf("a unit at the ceiling took a conference; the ceiling is the load at which a unit takes nothing new")
	} else if r != NoEligibleUnit() {
		t.Errorf("refused with %q, wanted %q", r, NoEligibleUnit())
	}

	below := pool(t, unit(t, "a", math.Nextafter(EligibilityCeiling, 0), Admitting()))
	if on := placedOn(t, naive.PlaceConference(below, conference(t, "lecture"))); on != "a" {
		t.Errorf("placed on %s, wanted a; the unit is below the ceiling", on)
	}
}

// TestANewConferenceIsRefusedRatherThanPlacedOntoASaturatedUnit is the second
// condition of issue #57. Each case is a pool that cannot take a new conference,
// and the reason is asserted rather than only the refusal, because a caller acts
// on the reason: docs/decisions/placement-seam.md has an empty pool and a full
// one leading to the same next step and a client seeing different words.
func TestANewConferenceIsRefusedRatherThanPlacedOntoASaturatedUnit(t *testing.T) {
	cases := []struct {
		name  string
		units []Unit
		want  Reason
	}{
		{
			name:  "a pool with no rows at all",
			units: nil,
			want:  NoUnits(),
		},
		{
			name:  "every unit saturated",
			units: []Unit{unit(t, "a", 0.90, Admitting()), unit(t, "b", 0.99, Admitting())},
			want:  NoEligibleUnit(),
		},
		{
			name:  "every unit draining, none of them full",
			units: []Unit{unit(t, "a", 0.10, Draining()), unit(t, "b", 0.20, Draining())},
			want:  NoEligibleUnit(),
		},
		{
			name:  "every unit gone",
			units: []Unit{unit(t, "a", 0.10, Gone())},
			want:  NoEligibleUnit(),
		},
		{
			name:  "the one empty unit is draining and the one admitting unit is saturated",
			units: []Unit{unit(t, "a", 0.00, Draining()), unit(t, "b", 0.94, Admitting())},
			want:  NoEligibleUnit(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := naive.PlaceConference(pool(t, c.units...), conference(t, "lecture"))
			if u, placed := got.Placed(); placed {
				t.Fatalf("placed on %s; the pool cannot take a new conference", u)
			}
			r, _ := got.Refused()
			if r != c.want {
				t.Errorf("refused with %q, wanted %q", r, c.want)
			}
		})
	}
}

// TestAnEmptyPoolAndAFullOneRefuseDifferently is the distinction the two reasons
// exist for, asserted on its own so that collapsing them into one reason reds a
// test that says why they are two.
func TestAnEmptyPoolAndAFullOneRefuseDifferently(t *testing.T) {
	empty, _ := naive.PlaceConference(pool(t), conference(t, "lecture")).Refused()
	full, _ := naive.PlaceConference(
		pool(t, unit(t, "a", 0.95, Admitting())), conference(t, "lecture")).Refused()

	if empty == full {
		t.Errorf("a pool with no units and a pool with no eligible unit refuse with the same reason %q; "+
			"one of them is answered by growing the pool and the other is not", empty)
	}
}

// TestNaiveCarriesNothingBetweenCalls is the third condition of issue #57. The
// policy holds no state, so this asserts the two things that would make that
// false: a field to hold it in, and a call that changed what a later call reads.
func TestNaiveCarriesNothingBetweenCalls(t *testing.T) {
	if n := reflect.TypeOf(Naive{}).NumField(); n != 0 {
		t.Errorf("Naive carries %d field(s); a placer that holds anything between calls is one whose "+
			"answer depends on what it was asked before, and docs/decisions/placement-seam.md refuses that", n)
	}

	p := pool(t,
		unit(t, "a", 0.50, Admitting()),
		unit(t, "b", 0.10, Admitting()),
	)
	first := naive.PlaceConference(p, conference(t, "one"))
	for i := 0; i < 8; i++ {
		again := naive.PlaceConference(p, conference(t, fmt.Sprintf("call-%d", i)))
		if again != first {
			t.Fatalf("call %d answered %s where the first answered %s, from the same pool", i, again, first)
		}
	}
}

// TestPlacingDoesNotChangeThePoolItWasHanded is the other half of holding no
// state. A placer that committed a load against the row it chose would make the
// caller's pool a thing that changes underneath them, and effective load is the
// pool's to move on issue #56.
func TestPlacingDoesNotChangeThePoolItWasHanded(t *testing.T) {
	p := pool(t,
		unit(t, "a", 0.50, Admitting()),
		unit(t, "b", 0.10, Admitting()),
	)
	before := p.Units()
	naive.PlaceConference(p, conference(t, "lecture"))
	after := p.Units()

	if len(before) != len(after) {
		t.Fatalf("the pool held %d rows before the call and %d after", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("row %d was %v at load %v and is %v at load %v after one placement",
				i, before[i].ID(), before[i].EffectiveLoad(), after[i].ID(), after[i].EffectiveLoad())
		}
	}
}

// TestTheAnswerDoesNotDependOnTheOrderTheRowsArriveIn is what "total rather than
// merely deterministic" means, and it is checked over every permutation of a
// three-row pool rather than over one shuffle, so it does not depend on a source
// of randomness the placer is not allowed to reach for.
func TestTheAnswerDoesNotDependOnTheOrderTheRowsArriveIn(t *testing.T) {
	rows := []Unit{
		unit(t, "a", 0.40, Admitting()),
		unit(t, "b", 0.40, Admitting()),
		unit(t, "c", 0.40, Draining()),
	}

	orders := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, order := range orders {
		permuted := make([]Unit, 0, len(order))
		for _, i := range order {
			permuted = append(permuted, rows[i])
		}
		got := naive.PlaceConference(pool(t, permuted...), conference(t, "lecture"))
		if on := placedOn(t, got); on != "a" {
			t.Errorf("order %v placed on %s, wanted a", order, on)
		}
	}
}

// TestTheRecordRefusesWhatNoPoolCouldHaveProduced covers the refusals in the
// constructors. They are refusals of a record rather than of a placement, and
// they are here because each one is a way the placer would stop being a total
// function of its inputs.
func TestTheRecordRefusesWhatNoPoolCouldHaveProduced(t *testing.T) {
	good, err := NewUnitID("a")
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}

	if _, err := NewUnitID(""); !errors.Is(err, ErrEmpty) {
		t.Errorf("an empty unit identifier gave %v, wanted %v", err, ErrEmpty)
	}
	if _, err := NewUnit(UnitID{}, 0.1, reported, Admitting()); !errors.Is(err, ErrEmpty) {
		t.Errorf("a row with no identifier gave %v, wanted %v", err, ErrEmpty)
	}
	if _, err := NewUnit(good, -0.01, reported, Admitting()); !errors.Is(err, ErrNotALoad) {
		t.Errorf("a negative load gave %v, wanted %v", err, ErrNotALoad)
	}
	if _, err := NewUnit(good, math.NaN(), reported, Admitting()); !errors.Is(err, ErrNotALoad) {
		t.Errorf("a load that is not a number gave %v, wanted %v", err, ErrNotALoad)
	}
	if _, err := NewUnit(good, 0.1, reported, State{}); !errors.Is(err, ErrNotAState) {
		t.Errorf("a state outside the three gave %v, wanted %v", err, ErrNotAState)
	}

	a := unit(t, "a", 0.10, Admitting())
	b := unit(t, "a", 0.80, Admitting())
	if _, err := NewPool(a, b); !errors.Is(err, ErrDuplicate) {
		t.Errorf("two rows for one unit gave %v, wanted %v; there is no answer to which load a placement is weighed against",
			err, ErrDuplicate)
	}
}

// TestReadingThePoolCannotChangeIt is why Units returns a copy. A caller holding
// the slice the pool holds could move a load between building the record and
// asking the question, which is the one way a value type stops being one.
func TestReadingThePoolCannotChangeIt(t *testing.T) {
	p := pool(t,
		unit(t, "a", 0.80, Admitting()),
		unit(t, "b", 0.10, Admitting()),
	)
	read := p.Units()
	read[0] = unit(t, "a", 0.00, Admitting())

	if on := placedOn(t, naive.PlaceConference(p, conference(t, "lecture"))); on != "b" {
		t.Errorf("placed on %s after a caller edited the slice Units returned; wanted b", on)
	}
}

// TestBuildingThePoolCopiesTheRowsItWasGiven is the other end of the same
// property, and it exists because the first version of this file did not have it
// and the copy in NewPool survived being deleted with the suite still green. A
// caller that builds a pool from a slice it keeps is the ordinary shape, since
// the pool on issue #56 will hold its rows in one, and without the copy that
// caller can move a load after the record was built.
func TestBuildingThePoolCopiesTheRowsItWasGiven(t *testing.T) {
	rows := []Unit{
		unit(t, "a", 0.80, Admitting()),
		unit(t, "b", 0.10, Admitting()),
	}
	p, err := NewPool(rows...)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	rows[0] = unit(t, "a", 0.00, Admitting())

	if on := placedOn(t, naive.PlaceConference(p, conference(t, "lecture"))); on != "b" {
		t.Errorf("placed on %s after the caller edited the slice the pool was built from; wanted b", on)
	}
}

// TestNaiveSatisfiesTheSeam is the interface the policy sits behind. It is a
// compile-time assertion written as a test so that it carries the reason.
func TestNaiveSatisfiesTheSeam(t *testing.T) {
	var p ConferencePlacer = Naive{}
	if _, ok := p.PlaceConference(pool(t, unit(t, "a", 0.10, Admitting())), conference(t, "lecture")).Placed(); !ok {
		t.Errorf("the seam did not place onto the one eligible unit in the pool")
	}
}

// TestTheReportedTimeIsCarriedAndNotRead is the disclosure in NewUnit's comment,
// asserted rather than left as a sentence. The record carries the time because
// the seam names it, and two pools differing only in it answer the same, because
// whether a silent unit is idle or unknown is the pool's answer and not the
// placer's.
func TestTheReportedTimeIsCarriedAndNotRead(t *testing.T) {
	id, err := NewUnitID("a")
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	old := reported.Add(-72 * time.Hour)

	fresh, err := NewUnit(id, 0.10, reported, Admitting())
	if err != nil {
		t.Fatalf("fresh row: %v", err)
	}
	stale, err := NewUnit(id, 0.10, old, Admitting())
	if err != nil {
		t.Fatalf("stale row: %v", err)
	}
	if !stale.ReportedAt().Equal(old) {
		t.Errorf("the row reports %v, wanted %v; the record carries the time even though the placer does not read it",
			stale.ReportedAt(), old)
	}

	c := conference(t, "lecture")
	if a, b := naive.PlaceConference(pool(t, fresh), c), naive.PlaceConference(pool(t, stale), c); a != b {
		t.Errorf("a row reported three days ago answered %s where the same row reported now answered %s; "+
			"a unit that has stopped reporting reaches the placer as a state, on issues #55 and #56", b, a)
	}
}

// carrying builds one row of the conference record and fails the test rather
// than returning an error, for the reason unit does.
func carrying(t *testing.T, id string, participants int, published float64) Carrying {
	t.Helper()
	uid, err := NewUnitID(id)
	if err != nil {
		t.Fatalf("unit id %q: %v", id, err)
	}
	c, err := NewCarrying(uid, participants, published)
	if err != nil {
		t.Fatalf("carrying row %q: %v", id, err)
	}
	return c
}

// running builds the conference record for a room that is already somewhere.
func running(t testing.TB, id string, ceiling int, rows ...Carrying) Conference {
	t.Helper()
	cid, err := domain.NewConferenceID(id)
	if err != nil {
		t.Fatalf("conference id %q: %v", id, err)
	}
	c, err := NewConference(cid, ceiling, rows...)
	if err != nil {
		t.Fatalf("conference %q: %v", id, err)
	}
	return c
}

// subscriber is an arrival that publishes nothing, which is what all but a few
// of three hundred people in a lecture are.
func subscriber(t testing.TB) Arrival {
	t.Helper()
	a, err := NewArrival(false)
	if err != nil {
		t.Fatalf("arrival: %v", err)
	}
	return a
}

// publisher is an arrival offering one video source in three layers, which is
// the other shape and the one a cost-reading policy would answer differently.
func publisher(t *testing.T) Arrival {
	t.Helper()
	pid, err := domain.NewParticipantID("arriving")
	if err != nil {
		t.Fatalf("participant id: %v", err)
	}
	sid, err := domain.NewSourceID("camera")
	if err != nil {
		t.Fatalf("source id: %v", err)
	}
	s, err := domain.NewSource(sid, pid, domain.Video(), 3)
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	a, err := NewArrival(true, s)
	if err != nil {
		t.Fatalf("arrival: %v", err)
	}
	return a
}

// joiner is the placer under test, held as the second seam rather than as the
// concrete type, so every case below goes through the interface issue #58 asks
// the policy to sit behind.
var joiner ParticipantPlacer = Naive{}

// TestTheScriptedPoolsAndTheUnitAnArrivingParticipantGoesOn is the first
// condition of issue #58: the policy behind the same kind of interface as
// conference placement, asserted per scripted pool rather than run and checked
// for not crashing.
func TestTheScriptedPoolsAndTheUnitAnArrivingParticipantGoesOn(t *testing.T) {
	cases := []struct {
		name  string
		units []Unit
		room  Conference
		want  string
	}{
		{
			name:  "the one unit already carrying it takes them",
			units: []Unit{unit(t, "a", 0.40, Admitting()), unit(t, "b", 0.10, Admitting())},
			room:  running(t, "lecture", 4, carrying(t, "a", 120, 2.5)),
			want:  "a",
		},
		{
			name: "the emptiest of the units already carrying it takes them",
			units: []Unit{
				unit(t, "a", 0.70, Admitting()),
				unit(t, "b", 0.30, Admitting()),
				unit(t, "c", 0.50, Admitting()),
			},
			room: running(t, "lecture", 4,
				carrying(t, "a", 200, 2.5), carrying(t, "b", 60, 0.0), carrying(t, "c", 90, 1.0)),
			want: "b",
		},
		{
			name: "a tie between two carrying units is broken by the smallest identifier",
			units: []Unit{
				unit(t, "c", 0.30, Admitting()),
				unit(t, "a", 0.30, Admitting()),
			},
			room: running(t, "lecture", 4, carrying(t, "c", 80, 1.0), carrying(t, "a", 80, 1.0)),
			want: "a",
		},
		{
			name:  "a carrying unit that is draining is skipped and the room reaches for another",
			units: []Unit{unit(t, "a", 0.10, Draining()), unit(t, "b", 0.60, Admitting())},
			room:  running(t, "lecture", 4, carrying(t, "a", 40, 1.0)),
			want:  "b",
		},
		{
			name:  "a carrying unit at the eligibility ceiling is skipped and the room reaches for another",
			units: []Unit{unit(t, "a", EligibilityCeiling, Admitting()), unit(t, "b", 0.80, Admitting())},
			room:  running(t, "lecture", 4, carrying(t, "a", 300, 2.5)),
			want:  "b",
		},
		{
			name:  "a carrying unit the pool no longer lists is not eligible",
			units: []Unit{unit(t, "b", 0.75, Admitting())},
			room:  running(t, "lecture", 4, carrying(t, "a", 300, 2.5)),
			want:  "b",
		},
		{
			name: "the second unit is the emptiest of the ones not carrying it",
			units: []Unit{
				unit(t, "a", 0.95, Admitting()),
				unit(t, "b", 0.60, Admitting()),
				unit(t, "c", 0.20, Admitting()),
			},
			room: running(t, "lecture", 4, carrying(t, "a", 300, 2.5)),
			want: "c",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := joiner.PlaceParticipant(pool(t, c.units...), c.room, subscriber(t))
			if on := placedOn(t, got); on != c.want {
				t.Errorf("placed on %s, wanted %s", on, c.want)
			}
		})
	}
}

// TestAFullerCarryingUnitBeatsAnEmptierOneThatIsNotCarryingIt is the second
// condition of issue #58: the preference between an extra relay hop and a fuller
// unit, asserted in the case where the two disagree as loudly as they can. The
// carrying unit is one step below the load at which it takes nothing new and the
// alternative is empty, so a policy that ordered by load alone would answer the
// other way here.
func TestAFullerCarryingUnitBeatsAnEmptierOneThatIsNotCarryingIt(t *testing.T) {
	p := pool(t,
		unit(t, "a", math.Nextafter(EligibilityCeiling, 0), Admitting()),
		unit(t, "b", 0.00, Admitting()),
	)
	room := running(t, "lecture", 4, carrying(t, "a", 280, 2.5))

	for _, arriving := range []Arrival{subscriber(t), publisher(t)} {
		got := joiner.PlaceParticipant(p, room, arriving)
		if on := placedOn(t, got); on != "a" {
			t.Errorf("placed on %s, wanted a; a second unit costs a link to every other unit carrying the "+
				"conference and one added hop for every subscriber on it, and the room pays that for as long "+
				"as it lasts, where the hop this policy declines to avoid is paid by one person for one session", on)
		}
	}

	// The same pool with the room on neither unit is the control. Nothing there
	// prefers a fuller unit, so the emptiest wins, and the preference above is
	// the conference being kept together rather than an order over loads.
	if on := placedOn(t, naive.PlaceConference(p, conference(t, "lecture"))); on != "b" {
		t.Errorf("a new conference went to %s, wanted b", on)
	}
}

// TestTheCeilingRefusalIsGivenRatherThanASecondUnit is the third refusal reason,
// which arrives with this issue. The pool has an empty unit in it and the answer
// is still a refusal, because another unit would not be allowed to carry this
// conference and growing the pool on that refusal would buy a machine for a room
// that cannot use it.
func TestTheCeilingRefusalIsGivenRatherThanASecondUnit(t *testing.T) {
	p := pool(t,
		unit(t, "a", 0.95, Admitting()),
		unit(t, "b", 0.95, Admitting()),
		unit(t, "c", 0.00, Admitting()),
	)
	at := running(t, "lecture", 2, carrying(t, "a", 300, 2.5), carrying(t, "b", 300, 2.5))

	got := joiner.PlaceParticipant(p, at, subscriber(t))
	if u, placed := got.Placed(); placed {
		t.Fatalf("placed on %s; the conference is on as many units as its ceiling allows", u)
	}
	if r, _ := got.Refused(); r != ReachedUnitCeiling() {
		t.Errorf("refused with %q, wanted %q; a caller reads the reason and only one of the two grows the pool",
			r, ReachedUnitCeiling())
	}
}

// TestTheCeilingIsReachedAtAndNotPast is the near miss. Writing the comparison
// in PlaceParticipant as > rather than >= is one character, it lets every
// conference occupy one unit more than its ceiling, and it reds only this test.
func TestTheCeilingIsReachedAtAndNotPast(t *testing.T) {
	p := pool(t,
		unit(t, "a", 0.95, Admitting()),
		unit(t, "b", 0.95, Admitting()),
		unit(t, "c", 0.00, Admitting()),
	)
	rows := []Carrying{carrying(t, "a", 300, 2.5), carrying(t, "b", 300, 2.5)}

	at := running(t, "lecture", 2, rows...)
	if _, refused := joiner.PlaceParticipant(p, at, subscriber(t)).Refused(); !refused {
		t.Errorf("a conference on two units with a ceiling of two took a third; the ceiling is the number of " +
			"units it may occupy and not the number it may exceed by one")
	}

	below := running(t, "lecture", 3, rows...)
	if on := placedOn(t, joiner.PlaceParticipant(p, below, subscriber(t))); on != "c" {
		t.Errorf("a conference on two units with a ceiling of three was placed on %s, wanted c", on)
	}
}

// TestAParticipantIsRefusedWithTheReasonTheCallerActsOn covers the other two
// reasons. Each leads to a different next step in docs/design/scaling-loop.md,
// so the reason is asserted and not only the refusal.
func TestAParticipantIsRefusedWithTheReasonTheCallerActsOn(t *testing.T) {
	cases := []struct {
		name  string
		units []Unit
		room  Conference
		want  Reason
	}{
		{
			name:  "a pool with no rows at all",
			units: nil,
			room:  running(t, "lecture", 4, carrying(t, "a", 300, 2.5)),
			want:  NoUnits(),
		},
		{
			name:  "the carrying unit is full, the ceiling is not reached, and nothing else is eligible",
			units: []Unit{unit(t, "a", 0.99, Admitting()), unit(t, "b", 0.00, Draining())},
			room:  running(t, "lecture", 4, carrying(t, "a", 300, 2.5)),
			want:  NoEligibleUnit(),
		},
		{
			name:  "the conference is on nothing the pool lists and the pool is saturated",
			units: []Unit{unit(t, "b", 0.94, Admitting())},
			room:  running(t, "lecture", 4, carrying(t, "a", 300, 2.5)),
			want:  NoEligibleUnit(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := joiner.PlaceParticipant(pool(t, c.units...), c.room, subscriber(t))
			if u, placed := got.Placed(); placed {
				t.Fatalf("placed on %s; nothing in this pool can take the arrival", u)
			}
			if r, _ := got.Refused(); r != c.want {
				t.Errorf("refused with %q, wanted %q", r, c.want)
			}
		})
	}
}

// TestTheArrivingParticipantIsCarriedAndNotRead is the disclosure in
// PlaceParticipant's comment, asserted rather than left as a sentence. The naive
// policy reads only the load, so two arrivals that differ in everything the
// record carries answer the same, and the day a policy weighs a publisher
// differently this test is what says so.
func TestTheArrivingParticipantIsCarriedAndNotRead(t *testing.T) {
	p := pool(t, unit(t, "a", 0.40, Admitting()), unit(t, "b", 0.10, Admitting()))
	room := running(t, "lecture", 4, carrying(t, "a", 120, 2.5))

	quiet, loud := subscriber(t), publisher(t)
	if quiet.Publishes() || !loud.Publishes() || len(loud.Sources()) != 1 {
		t.Fatalf("the two arrivals are not the two shapes this test is about")
	}
	if a, b := joiner.PlaceParticipant(p, room, quiet), joiner.PlaceParticipant(p, room, loud); a != b {
		t.Errorf("a publisher answered %s where a subscriber answered %s, from one pool and one conference; "+
			"the naive policy reads only the load and does not know that this arrival costs more than the last", b, a)
	}
}

// TestReadingTheArrivalCannotChangeIt is why NewArrival and Sources copy. The
// record is a value the caller may keep, and a caller editing the slice it
// handed over would be changing what a later placement is decided against.
func TestReadingTheArrivalCannotChangeIt(t *testing.T) {
	loud := publisher(t)
	read := loud.Sources()
	if len(read) != 1 {
		t.Fatalf("the arrival offers %d source(s), wanted 1", len(read))
	}
	read[0] = domain.Source{}

	if again := loud.Sources(); again[0].ID().String() != "camera" {
		t.Errorf("the arrival offers %q after a caller edited the slice Sources returned, wanted camera",
			again[0].ID())
	}
}

// TestPlacingAParticipantDoesNotChangeTheRecordsItWasHanded is the property
// TestPlacingDoesNotChangeThePoolItWasHanded holds for the first question,
// extended to the second record. The effective load of the unit that was chosen
// is the pool's to move on issue #56, and adding that unit to the conference is
// the caller's once the port has answered, so a placer doing either would make
// the caller's records change underneath them.
func TestPlacingAParticipantDoesNotChangeTheRecordsItWasHanded(t *testing.T) {
	p := pool(t, unit(t, "a", 0.50, Admitting()), unit(t, "b", 0.10, Admitting()))
	room := running(t, "lecture", 4, carrying(t, "a", 120, 2.5))

	poolBefore, roomBefore := p.Units(), room.Carrying()
	joiner.PlaceParticipant(p, room, publisher(t))
	poolAfter, roomAfter := p.Units(), room.Carrying()

	if !reflect.DeepEqual(poolBefore, poolAfter) {
		t.Errorf("the pool held %v before the call and %v after", poolBefore, poolAfter)
	}
	if !reflect.DeepEqual(roomBefore, roomAfter) {
		t.Errorf("the conference held %v before the call and %v after", roomBefore, roomAfter)
	}
	if room.Units() != 1 {
		t.Errorf("the conference is on %d unit(s) after one placement, wanted 1", room.Units())
	}
}

// TestTheParticipantAnswerDoesNotDependOnTheOrderTheRowsArriveIn is "total
// rather than merely deterministic" for the second question, over every
// permutation of a three-row pool rather than over one shuffle. Two of the three
// carry the conference and tie on load, so the answer turns on the tie break and
// on nothing about the order.
func TestTheParticipantAnswerDoesNotDependOnTheOrderTheRowsArriveIn(t *testing.T) {
	rows := []Unit{
		unit(t, "a", 0.40, Admitting()),
		unit(t, "b", 0.40, Admitting()),
		unit(t, "c", 0.00, Admitting()),
	}
	room := running(t, "lecture", 4, carrying(t, "b", 100, 1.0), carrying(t, "a", 100, 1.0))

	orders := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, order := range orders {
		permuted := make([]Unit, 0, len(order))
		for _, i := range order {
			permuted = append(permuted, rows[i])
		}
		got := joiner.PlaceParticipant(pool(t, permuted...), room, subscriber(t))
		if on := placedOn(t, got); on != "a" {
			t.Errorf("order %v placed on %s, wanted a", order, on)
		}
	}
}

// TestTheConferenceAndArrivalRecordsRefuseWhatNoCallerCouldHaveProduced covers
// the constructors. Each refusal is a way the placer would stop being a total
// function of its inputs, or a record that contradicts itself.
func TestTheConferenceAndArrivalRecordsRefuseWhatNoCallerCouldHaveProduced(t *testing.T) {
	good, err := NewUnitID("a")
	if err != nil {
		t.Fatalf("unit id: %v", err)
	}
	id := conference(t, "lecture")

	if _, err := NewCarrying(UnitID{}, 10, 1.0); !errors.Is(err, ErrEmpty) {
		t.Errorf("a carrying row about no unit gave %v, wanted %v", err, ErrEmpty)
	}
	if _, err := NewCarrying(good, -1, 1.0); !errors.Is(err, ErrNotACount) {
		t.Errorf("a negative participant count gave %v, wanted %v", err, ErrNotACount)
	}
	if _, err := NewCarrying(good, 10, -0.01); !errors.Is(err, ErrNotABitrate) {
		t.Errorf("a negative bitrate gave %v, wanted %v", err, ErrNotABitrate)
	}
	if _, err := NewCarrying(good, 10, math.NaN()); !errors.Is(err, ErrNotABitrate) {
		t.Errorf("a bitrate that is not a number gave %v, wanted %v", err, ErrNotABitrate)
	}

	if _, err := NewConference(domain.ConferenceID{}, 2); !errors.Is(err, ErrEmpty) {
		t.Errorf("a conference with no identifier gave %v, wanted %v", err, ErrEmpty)
	}
	if _, err := NewConference(id, 0); !errors.Is(err, ErrNoCeiling) {
		t.Errorf("a ceiling of zero gave %v, wanted %v; a conference that may live nowhere is not a record",
			err, ErrNoCeiling)
	}
	if _, err := NewConference(id, -1); !errors.Is(err, ErrNoCeiling) {
		t.Errorf("a negative ceiling gave %v, wanted %v", err, ErrNoCeiling)
	}
	if _, err := NewConference(id, 4, carrying(t, "a", 10, 1.0), carrying(t, "a", 20, 2.0)); !errors.Is(err, ErrDuplicate) {
		t.Errorf("two rows for one unit gave %v, wanted %v; the row count is U and a duplicate spends the ceiling twice",
			err, ErrDuplicate)
	}

	// A record already past its ceiling is a real state and is not refused, for
	// the reason NewConference gives: the ceiling is derived from figures that
	// move, and the placer answers such a record by refusing to reach for one
	// more unit.
	past, err := NewConference(id, 1, carrying(t, "a", 10, 1.0), carrying(t, "b", 10, 1.0))
	if err != nil {
		t.Fatalf("a conference on more units than its ceiling: %v", err)
	}
	if past.Units() != 2 {
		t.Errorf("the record holds %d row(s), wanted 2", past.Units())
	}

	if _, err := NewArrival(true); !errors.Is(err, ErrNotAnArrival) {
		t.Errorf("a publisher offering nothing gave %v, wanted %v", err, ErrNotAnArrival)
	}
	if _, err := NewArrival(false, publisher(t).Sources()...); !errors.Is(err, ErrNotAnArrival) {
		t.Errorf("a non-publisher offering a source gave %v, wanted %v", err, ErrNotAnArrival)
	}
}

// TestNaiveSatisfiesBothSeams is the compile-time assertion that one policy
// answers the one question in its two forms, written as a test so that it
// carries the reason.
func TestNaiveSatisfiesBothSeams(t *testing.T) {
	var both interface {
		ConferencePlacer
		ParticipantPlacer
	} = Naive{}

	p := pool(t, unit(t, "a", 0.10, Admitting()))
	if _, ok := both.PlaceConference(p, conference(t, "lecture")).Placed(); !ok {
		t.Errorf("the conference seam did not place onto the one eligible unit in the pool")
	}
	room := running(t, "lecture", 4, carrying(t, "a", 1, 0.0))
	if _, ok := both.PlaceParticipant(p, room, subscriber(t)).Placed(); !ok {
		t.Errorf("the participant seam did not place onto the one eligible unit in the pool")
	}
}

// storm is the join storm the test and the benchmark below both run: a stated
// number of arrivals into one conference, against a pool the caller rebuilds
// after every placement, answering with where each arrival landed in order.
//
// The rebuild is the point rather than an artefact of writing it this way.
// Effective load is what the unit reported plus the load of every placement made
// against it since, which docs/decisions/placement-seam.md fixes as the number
// the placer reads, so a storm that handed one unchanging pool over three
// hundred times would be three hundred placements none of which could see the
// other two hundred and ninety nine.
func storm(t testing.TB, arrivals int, cost float64) []string {
	t.Helper()

	names := []string{"a", "b", "c"}
	loads := map[string]float64{}
	on := []string{"a"}
	landed := make([]string, 0, arrivals)

	for i := 0; i < arrivals; i++ {
		rows := make([]Unit, 0, len(names))
		for _, n := range names {
			id, err := NewUnitID(n)
			if err != nil {
				t.Fatalf("unit id %q: %v", n, err)
			}
			u, err := NewUnit(id, loads[n], reported, Admitting())
			if err != nil {
				t.Fatalf("unit %q at %v: %v", n, loads[n], err)
			}
			rows = append(rows, u)
		}
		p, err := NewPool(rows...)
		if err != nil {
			t.Fatalf("pool: %v", err)
		}

		held := make([]Carrying, 0, len(on))
		for _, n := range on {
			id, err := NewUnitID(n)
			if err != nil {
				t.Fatalf("unit id %q: %v", n, err)
			}
			c, err := NewCarrying(id, 0, 0.0)
			if err != nil {
				t.Fatalf("carrying row %q: %v", n, err)
			}
			held = append(held, c)
		}

		answer := Naive{}.PlaceParticipant(p, running(t, "lecture", 3, held...), subscriber(t))
		chosen, placed := answer.Placed()
		if !placed {
			t.Fatalf("arrival %d was refused: %s", i, answer)
		}

		name := chosen.String()
		landed = append(landed, name)
		loads[name] += cost
		if !slices.Contains(on, name) {
			on = append(on, name)
		}
	}
	return landed
}

// TestThreeHundredPlacementsIntoOneConference is the fourth condition of issue
// #58, in the half that is an assertion rather than a figure. The figure is
// BenchmarkThreeHundredPlacements below, because internal/guard refuses a clock
// read in a test and the benchmark harness times the run from outside the
// source.
//
// Each arrival costs 0.004 of a unit, so the first one saturates partway through
// and the room reaches for a second under load rather than in a constructed
// case. What is asserted is the shape of the storm: every arrival is placed, the
// room fills one unit before it spans, and the third unit is never touched.
func TestThreeHundredPlacementsIntoOneConference(t *testing.T) {
	landed := storm(t, 300, 0.004)

	counts := map[string]int{}
	for _, name := range landed {
		counts[name]++
	}
	if counts["a"] != 225 || counts["b"] != 75 || counts["c"] != 0 {
		t.Errorf("three hundred arrivals landed a=%d b=%d c=%d, wanted a=225 b=75 c=0",
			counts["a"], counts["b"], counts["c"])
	}

	// The room spans once and does not oscillate. A policy preferring the
	// emptiest unit would alternate between the two from the moment the second
	// one existed, and it would still place three hundred people.
	first := slices.Index(landed, "b")
	if first != 225 {
		t.Fatalf("the room reached a second unit at arrival %d, wanted 225", first)
	}
	for i, name := range landed[first:] {
		if name != "b" {
			t.Fatalf("arrival %d went back to %s after the room had spanned; the conference is kept together",
				first+i, name)
		}
	}
}

// BenchmarkThreeHundredPlacements is the measured number the fourth condition of
// issue #58 asks for: what a join storm of three hundred costs in the placer.
//
// It times the whole of what a caller does per arrival, which is building the
// three records and asking the question, rather than the policy alone. That is
// the wider of the two figures and it is the one the condition is about, since
// the records are rebuilt per placement for the reason storm gives. A figure
// over the policy alone would be smaller and would describe nothing anybody
// runs.
func BenchmarkThreeHundredPlacements(b *testing.B) {
	for i := 0; i < b.N; i++ {
		storm(b, 300, 0.004)
	}
}

// TestTheConferenceRecordAnswersWithWhatItWasGiven reads the figures back. The
// naive policy does not use B(i) or the participant count, so without this the
// two would be write-only fields that a later policy would find untested on the
// day it first read one.
func TestTheConferenceRecordAnswersWithWhatItWasGiven(t *testing.T) {
	row := carrying(t, "a", 137, 2.75)
	if row.Unit().String() != "a" || row.Participants() != 137 || row.PublishedBitrate() != 2.75 {
		t.Errorf("the row reads back as %s carrying %d participant(s) publishing %v, wanted a, 137 and 2.75",
			row.Unit(), row.Participants(), row.PublishedBitrate())
	}

	room := running(t, "lecture", 3, row)
	if room.ID().String() != "lecture" || room.UnitCeiling() != 3 || room.Units() != 1 {
		t.Errorf("the record reads back as %s with a ceiling of %d on %d unit(s), wanted lecture, 3 and 1",
			room.ID(), room.UnitCeiling(), room.Units())
	}
}
