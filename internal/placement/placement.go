// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package placement

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/iderex/hoersaal/internal/domain"
)

// The refusals. These are refusals of a record that could not have come from a
// pool, and they are separate from a placement refusal, which is an answer and
// not an error.
var (
	// ErrEmpty is an identifier with nothing in it.
	ErrEmpty = errors.New("empty identifier")

	// ErrDuplicate is a second row in one pool under an identifier already held.
	ErrDuplicate = errors.New("identifier already held")

	// ErrNotALoad is a load that is negative or is not a number.
	ErrNotALoad = errors.New("a load is a number and is not negative")

	// ErrNotAState is a unit state outside the three the seam names.
	ErrNotAState = errors.New("a unit is admitting, draining or gone")
)

// EligibilityCeiling is the load at which a unit takes nothing new, neither a
// conference nor a participant. It is the third decision point in
// docs/decisions/capacity-signal.md and it is the eligibility test the naive
// placer in docs/decisions/placement-seam.md is written against.
//
// The number is written here because the code has to hold one somewhere, and it
// is a named constant rather than a literal in the comparison so that the one
// place it is written can be found. Where it should sit is not decided here:
// capacity-signal.md derives it from a rate and an interval neither of which has
// been measured, and moving it is a change to that document first.
const EligibilityCeiling = 0.90

// A UnitID names one forwarding unit.
//
// It is declared here rather than in internal/domain because the model holds no
// unit and no placement, which its own package comment fixes, and it is not
// taken from the pool because docs/repository-layout.md refuses the placer every
// import in this repository except the model. So the placer declares the record
// it reads and the pool on issue #56 fills it in, which is the direction that
// keeps the placer a function of what it was handed.
type UnitID struct{ v string }

// NewUnitID refuses an empty identifier.
func NewUnitID(v string) (UnitID, error) {
	if v == "" {
		return UnitID{}, fmt.Errorf("unit: %w", ErrEmpty)
	}
	return UnitID{v}, nil
}

func (u UnitID) String() string { return u.v }

// A State is what the pool says about a unit. The set is closed at the three
// docs/decisions/placement-seam.md names, and the placer reads them without
// interpreting them: a draining unit is not eligible and that is the whole of
// what draining means here.
type State struct{ v string }

// Admitting is a unit that may be placed onto.
func Admitting() State { return State{"admitting"} }

// Draining is a unit that keeps what it holds and takes nothing new.
func Draining() State { return State{"draining"} }

// Gone is a unit that is no longer serving.
func Gone() State { return State{"gone"} }

func (s State) String() string { return s.v }

func (s State) known() bool { return s == Admitting() || s == Draining() || s == Gone() }

// A Unit is one row of the pool record: its identifier, its effective load, the
// time the load that figure is derived from was reported, and its state.
//
// Effective load is the load the unit last reported plus the load of every
// placement made against it since, which docs/decisions/placement-seam.md fixes
// as the number the placer reads. Taking the reported number instead is how two
// placements in the same second stop being visible to each other.
type Unit struct {
	id            UnitID
	effectiveLoad float64
	reportedAt    time.Time
	state         State
}

// NewUnit refuses a row no pool could honestly produce: an empty identifier, a
// state outside the three, and a load that is negative or is not a number.
//
// The load refusal is the one worth reading. A NaN compares false against every
// other value including itself, so a single NaN row makes the lowest-load answer
// depend on the order the pool happened to hand its rows over, and the placer
// stops being the total function docs/decisions/placement-seam.md says it is.
// Refusing it where the record is built is what keeps that property inside the
// placer rather than inside whoever calls it.
//
// reportedAt is carried because the seam names it as part of the record. Nothing
// in this file reads it: whether a unit that has stopped reporting is idle or
// unknown is the pool's answer on issues #55 and #56, and it reaches the placer
// as a state rather than as a timestamp the placer would have to interpret
// against a clock it is not allowed to hold.
func NewUnit(id UnitID, effectiveLoad float64, reportedAt time.Time, state State) (Unit, error) {
	switch {
	case id.v == "":
		return Unit{}, fmt.Errorf("unit identifier: %w", ErrEmpty)
	case math.IsNaN(effectiveLoad) || effectiveLoad < 0:
		return Unit{}, fmt.Errorf("unit %s at load %v: %w", id.v, effectiveLoad, ErrNotALoad)
	case !state.known():
		return Unit{}, fmt.Errorf("unit %s in state %q: %w", id.v, state.v, ErrNotAState)
	}
	return Unit{id: id, effectiveLoad: effectiveLoad, reportedAt: reportedAt, state: state}, nil
}

// ID is which unit this row is about.
func (u Unit) ID() UnitID { return u.id }

// EffectiveLoad is what the unit reported plus what has been committed against
// it since.
func (u Unit) EffectiveLoad() float64 { return u.effectiveLoad }

// ReportedAt is when the unit reported the figure the effective load is derived
// from.
func (u Unit) ReportedAt() time.Time { return u.reportedAt }

// State is what the pool says about this unit.
func (u Unit) State() State { return u.state }

// A Pool is the pool record, which is the first of the three inputs.
//
// It is a value and the placer never keeps one, so a caller may build a fresh
// pool per call and two callers may hold different views of the same deployment
// without either of them being able to disturb the other.
type Pool struct{ units []Unit }

// NewPool refuses two rows for one unit. Two rows means two effective loads for
// one machine, and there is no answer to which of them a placement should be
// weighed against, so it is refused where the record is built rather than
// resolved by whichever row the loop reached first.
func NewPool(units ...Unit) (Pool, error) {
	held := make(map[UnitID]struct{}, len(units))
	for _, u := range units {
		if u.id.v == "" {
			return Pool{}, fmt.Errorf("pool row: %w", ErrEmpty)
		}
		if _, dup := held[u.id]; dup {
			return Pool{}, fmt.Errorf("unit %s: %w", u.id.v, ErrDuplicate)
		}
		held[u.id] = struct{}{}
	}
	return Pool{units: append([]Unit(nil), units...)}, nil
}

// Units is the rows this pool holds, in the order it was given them. It is a
// copy, so a caller reading the pool cannot change what a later placement is
// decided against.
func (p Pool) Units() []Unit { return append([]Unit(nil), p.units...) }

// Len is how many rows the pool holds, which is zero for a deployment whose pool
// has not been filled in yet.
func (p Pool) Len() int { return len(p.units) }

// A Reason is why a placement was refused.
//
// docs/decisions/placement-seam.md names three, and two of them are here. The
// third is the conference reaching the unit ceiling in
// docs/decisions/room-topology.md, which a conference that is on no unit cannot
// have reached, so it belongs to participant placement on issue #58 and is added
// there rather than declared here and never returned.
type Reason struct{ v string }

// NoUnits is a pool with no rows at all.
func NoUnits() Reason { return Reason{"the pool holds no units at all"} }

// NoEligibleUnit is a pool whose rows are all draining, gone, or at or above the
// eligibility ceiling.
func NoEligibleUnit() Reason { return Reason{"the pool holds no eligible unit"} }

func (r Reason) String() string { return r.v }

// An Answer is what the placer returns: a unit, or a refusal carrying its
// reason. There is no third answer and there is no error, because a refusal is
// the normal way a full pool says so and a caller that had to read it out of an
// error would be reading a policy statement as a failure.
type Answer struct {
	placed bool
	unit   UnitID
	reason Reason
}

// Placed is the unit chosen, and whether one was.
func (a Answer) Placed() (UnitID, bool) { return a.unit, a.placed }

// Refused is why nothing was chosen, and whether that is what happened.
func (a Answer) Refused() (Reason, bool) { return a.reason, !a.placed }

func (a Answer) String() string {
	if a.placed {
		return "place on " + a.unit.v
	}
	return "refused: " + a.reason.v
}

// A ConferencePlacer answers where a conference that is on no unit goes. It is
// the seam docs/decisions/placement-seam.md describes, in the form the first of
// its two questions takes.
//
// It is an interface so that replacing the policy is one type rather than a
// rewrite, which is the reason the seam exists. It is not a service and not a
// process: an implementation is a function of the two records it is handed, and
// an implementation that needed anything else would have to take it as an
// argument and change this signature, which is a change somebody has to argue
// for.
type ConferencePlacer interface {
	PlaceConference(pool Pool, conference domain.ConferenceID) Answer
}

// Naive is the placer specified under "The naive placer" in
// docs/decisions/placement-seam.md, and the policy is argued in
// docs/decisions/new-conference-placement.md.
//
// It carries no fields, so there is nothing for it to hold between calls. That
// is the property issue #57 asks for and TestNaiveCarriesNothingBetweenCalls is
// what holds it: a later placer that wants history reads it from an input, which
// means adding a field to the pool record and writing down what it means.
type Naive struct{}

// PlaceConference chooses the eligible unit with the lowest effective load, ties
// broken by the smallest unit identifier, and refuses when none is eligible.
//
// The conference identifier is part of the record the seam passes and this
// policy does not read it, which the parameter name says rather than leaving a
// reader to work out. Nothing about a conference that exists on no unit
// distinguishes it from another, so a policy reading the identifier here would
// be reading a name for something that is not yet a property of the room. It is
// in the signature because the seam puts it there and because the next policy
// may have a use for it.
//
// The 0.60 decision point in docs/decisions/capacity-signal.md, which says a
// unit stops being a preferred home for a new conference while staying eligible,
// is not a second comparison. Ordering by lowest effective load already prefers
// every unit below that point to every unit above it, so the preference is a
// consequence of the order rather than a rule beside it, and a second rule would
// be a second place to get it wrong.
func (Naive) PlaceConference(pool Pool, _ domain.ConferenceID) Answer {
	if pool.Len() == 0 {
		return refuse(NoUnits())
	}

	var best Unit
	found := false
	for _, u := range pool.units {
		if !eligible(u) {
			continue
		}
		if !found || prefer(u, best) {
			best, found = u, true
		}
	}
	if !found {
		return refuse(NoEligibleUnit())
	}
	return place(best.id)
}

// eligible is the seam's own sentence: a unit is eligible when the pool says it
// is admitting and its effective load is below the ceiling. Below and not at,
// because the ceiling is the load at which the unit takes nothing new.
func eligible(u Unit) bool {
	return u.state == Admitting() && u.effectiveLoad < EligibilityCeiling
}

// prefer is the order over eligible units. It is total rather than merely
// deterministic: every tie is broken by the identifier, so the answer does not
// become an accident of the order the pool view happened to return its rows in.
func prefer(a, b Unit) bool {
	if a.effectiveLoad != b.effectiveLoad {
		return a.effectiveLoad < b.effectiveLoad
	}
	return a.id.v < b.id.v
}

func place(u UnitID) Answer { return Answer{placed: true, unit: u} }

func refuse(r Reason) Answer { return Answer{reason: r} }
