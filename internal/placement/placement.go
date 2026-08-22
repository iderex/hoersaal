// SPDX-FileCopyrightText: 2026 Nils Lehnen
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

	// ErrNotACount is a number of participants that is negative.
	ErrNotACount = errors.New("a participant count is not negative")

	// ErrNotABitrate is a bitrate that is negative or is not a number.
	ErrNotABitrate = errors.New("a bitrate is a number and is not negative")

	// ErrNoCeiling is a unit ceiling below one, which is a conference that may
	// live nowhere.
	ErrNoCeiling = errors.New("a conference occupies at least one unit")

	// ErrNotAnArrival is a participant that publishes and offers no source, or
	// one that does not publish and offers some.
	ErrNotAnArrival = errors.New("a publisher offers at least one source and a subscriber offers none")
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

// A Reason is why a placement was refused. docs/decisions/placement-seam.md
// names three and all three are here.
type Reason struct{ v string }

// NoUnits is a pool with no rows at all.
func NoUnits() Reason { return Reason{"the pool holds no units at all"} }

// NoEligibleUnit is a pool whose rows are all draining, gone, or at or above the
// eligibility ceiling.
func NoEligibleUnit() Reason { return Reason{"the pool holds no eligible unit"} }

// ReachedUnitCeiling is a conference that occupies as many units as its ceiling
// allows and no unit already carrying it can take more.
//
// It is a different answer from NoEligibleUnit and the difference is what the
// caller does next. docs/design/scaling-loop.md says a refusal for the
// conference ceiling does not grow the pool, because another unit would not be
// allowed to carry that conference anyway, so a deployment that collapsed the
// two would buy a machine for a room that cannot use it.
func ReachedUnitCeiling() Reason { return Reason{"the conference has reached its unit ceiling"} }

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
	best, found := lowest(pool, anyUnit)
	if !found {
		return refuse(NoEligibleUnit())
	}
	return place(best.id)
}

// lowest is the order the naive policy applies twice: the eligible unit with the
// lowest effective load out of those the caller is willing to consider, ties
// broken by the smallest identifier.
//
// The two questions differ only in which units they will consider, so the search
// is written once. A second copy of this loop is a second place for the tie
// break to be forgotten, and the tie break is what makes the answer total rather
// than merely deterministic.
func lowest(pool Pool, consider func(Unit) bool) (Unit, bool) {
	var best Unit
	found := false
	for _, u := range pool.units {
		if !eligible(u) || !consider(u) {
			continue
		}
		if !found || prefer(u, best) {
			best, found = u, true
		}
	}
	return best, found
}

// anyUnit is the conference case: a conference on no unit has no preference
// between units.
func anyUnit(Unit) bool { return true }

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

// A Carrying is one row of the conference record: a unit the conference is
// already on, how many participants of this conference that unit holds, and the
// bitrate of this conference's sources published there.
//
// That last figure is B(i) in docs/decisions/room-topology.md, and the seam says
// it is passed so that a placer can see what adding a unit to a conference would
// cost the units already carrying it. The naive policy does not read either
// figure, which is said here rather than left to be inferred from a policy that
// happens not to mention them: it reads only the load, so it does not know that
// the participant arriving will cost more than the last one, and it does not
// account for what a publisher on a new unit costs the rest of the mesh. Both
// are named as what is naive about it.
type Carrying struct {
	unit             UnitID
	participants     int
	publishedBitrate float64
}

// NewCarrying refuses a row no control plane could honestly produce: a row about
// no unit, a negative number of participants, and a bitrate that is negative or
// is not a number. The bitrate refusal is the same one NewUnit makes about a
// load and it is made for the same reason, so that a NaN cannot reach an
// ordering and make the answer depend on the order the rows arrived in.
func NewCarrying(unit UnitID, participants int, publishedBitrate float64) (Carrying, error) {
	switch {
	case unit.v == "":
		return Carrying{}, fmt.Errorf("carrying unit: %w", ErrEmpty)
	case participants < 0:
		return Carrying{}, fmt.Errorf("unit %s carrying %d participant(s): %w", unit.v, participants, ErrNotACount)
	case math.IsNaN(publishedBitrate) || publishedBitrate < 0:
		return Carrying{}, fmt.Errorf("unit %s publishing %v: %w", unit.v, publishedBitrate, ErrNotABitrate)
	}
	return Carrying{unit: unit, participants: participants, publishedBitrate: publishedBitrate}, nil
}

// Unit is which unit this row is about.
func (c Carrying) Unit() UnitID { return c.unit }

// Participants is how many participants of this conference that unit holds.
func (c Carrying) Participants() int { return c.participants }

// PublishedBitrate is B(i): the bitrate of this conference's sources published
// on that unit.
func (c Carrying) PublishedBitrate() float64 { return c.publishedBitrate }

// A Conference is the second of the three records, in the form a participant
// placement takes: which units are already carrying it, and how many units it
// may occupy at all.
//
// It is not domain.Conference and it is not a smaller copy of one. The model
// holds the people in a room and this holds where the room is, and the seam is
// explicit that nothing about the people is passed, because a placer that could
// read them would be a second place where personal data lives.
//
// The unit ceiling is passed rather than derived, and that is the one addition
// this record makes to the set the seam first fixed. The bound is over f, the
// fraction of a unit's egress the deployment spends on links, and E, the unit's
// egress denominator; neither is in any record the placer is handed and neither
// has a value on this board yet. So a placer that derived it would be inventing
// the figure issue #59 exists to measure. It is read the way a unit state is
// read, without being interpreted, so the day f is measured is a change to
// whoever fills this record in rather than a change to the policy.
type Conference struct {
	id          domain.ConferenceID
	unitCeiling int
	carrying    []Carrying
}

// NewConference refuses a conference with no identifier, a ceiling below one,
// and two rows for one unit.
//
// A ceiling below one is a conference that may live nowhere, which no arithmetic
// over room-topology.md produces and which would make every placement a ceiling
// refusal. Two rows for one unit is the same refusal NewPool makes and it is
// made here for a second reason: the number of rows is U, so a duplicate would
// spend the ceiling twice on one machine.
//
// A record holding more rows than its ceiling is not refused. The ceiling is
// derived from figures that move, and a conference already on three units when
// the bound falls to two is a real state; the placer answers it by refusing to
// reach for a fourth, which is what the ceiling is for.
func NewConference(id domain.ConferenceID, unitCeiling int, carrying ...Carrying) (Conference, error) {
	if id.String() == "" {
		return Conference{}, fmt.Errorf("conference identifier: %w", ErrEmpty)
	}
	if unitCeiling < 1 {
		return Conference{}, fmt.Errorf("conference %s with a ceiling of %d: %w", id, unitCeiling, ErrNoCeiling)
	}
	held := make(map[UnitID]struct{}, len(carrying))
	for _, c := range carrying {
		if c.unit.v == "" {
			return Conference{}, fmt.Errorf("carrying row: %w", ErrEmpty)
		}
		if _, dup := held[c.unit]; dup {
			return Conference{}, fmt.Errorf("unit %s: %w", c.unit.v, ErrDuplicate)
		}
		held[c.unit] = struct{}{}
	}
	return Conference{id: id, unitCeiling: unitCeiling, carrying: append([]Carrying(nil), carrying...)}, nil
}

// ID is which conference this is.
func (c Conference) ID() domain.ConferenceID { return c.id }

// UnitCeiling is how many units this conference may occupy.
func (c Conference) UnitCeiling() int { return c.unitCeiling }

// Carrying is the units already carrying it, in the order the caller gave them.
// It is a copy, for the reason Pool.Units is one.
func (c Conference) Carrying() []Carrying { return append([]Carrying(nil), c.carrying...) }

// Units is U in docs/decisions/room-topology.md: how many units this conference
// is on. It is the number of rows the caller put in the record, so a unit that
// has died leaves this record when whoever maintains it says so and not when the
// pool stops listing the unit. The placer does not reconcile the two, because a
// placer that decided a conference was no longer somewhere would be deciding
// liveness, which is the pool's answer on issue #56.
func (c Conference) Units() int { return len(c.carrying) }

// An Arrival is the third record: the participant who is joining, as much of
// them as is known at that moment.
//
// The seam passes whether they will publish and, if so, the sources they offer
// with their layer arrangement, and says that is all that is passed. A
// domain.Source names its own publisher, so this record carries the arriving
// participant's identifier as a property of that type rather than because the
// seam asks for it. Nothing here reads it, and the alternative was a second
// vocabulary for a source beside the model's.
//
// The participant's network location is deliberately absent, which is the seam's
// own sentence: it would matter for a pool spread across sites, no decision on
// this board has described one, and a field that exists before a policy reads it
// is a field the first placer will use for something else.
type Arrival struct {
	publishes bool
	sources   []domain.Source
}

// NewArrival refuses the two records that contradict themselves: a participant
// that publishes and offers no source, and one that does not publish and offers
// some. Either would leave a later policy weighing a cost against a flag that
// disagrees with it.
func NewArrival(publishes bool, sources ...domain.Source) (Arrival, error) {
	if publishes != (len(sources) > 0) {
		return Arrival{}, fmt.Errorf("publishes=%v with %d source(s): %w", publishes, len(sources), ErrNotAnArrival)
	}
	return Arrival{publishes: publishes, sources: append([]domain.Source(nil), sources...)}, nil
}

// Publishes is whether this participant will send anything.
func (a Arrival) Publishes() bool { return a.publishes }

// Sources is what they offer, and it is a copy.
func (a Arrival) Sources() []domain.Source { return append([]domain.Source(nil), a.sources...) }

// A ParticipantPlacer answers where a participant joining a conference that is
// already running goes. It is the second form of the one question in
// docs/decisions/placement-seam.md.
//
// It is a separate call from ConferencePlacer because the inputs differ and
// because this is the only one of the two that has a conference to keep
// together. It is a separate interface for the reason the first one is one: the
// policy is the component most likely to be replaced, and a caller that holds
// the seam rather than the type pays nothing on the day it is.
type ParticipantPlacer interface {
	PlaceParticipant(pool Pool, conference Conference, arriving Arrival) Answer
}

// PlaceParticipant prefers the units already carrying the conference, and only
// reaches for another when none of them can take the arrival and the conference
// is below its unit ceiling.
//
// That preference is this policy's answer to the trade the issue names: an extra
// relay hop for this person against a fuller unit for the room. It prefers the
// fuller unit, and the reason is in docs/decisions/room-topology.md rather than
// in a judgement made here. A second unit is not one participant's cost. It is a
// link to every other unit carrying that conference, the media of every forwarded
// speaker crossing it, and one added hop of delay for every subscriber on it and
// not only for the arrival, and that cost is committed for as long as the room
// lasts because nothing moves a conference that is already placed. The hop is
// paid by one person for one session; the mesh is paid by the room. So the
// second unit is what the placer reaches for last, which is the same sentence as
// a conference occupying a second unit at the moment a participant cannot be
// placed on the unit already carrying it, and for no other reason.
//
// The arriving participant is not read, and the parameter name says so. This
// policy reads only the load, so what the arrival offers cannot change its
// answer; a policy that weighed a publisher differently from a subscriber would
// be the one docs/decisions/placement-seam.md describes as what a real placer
// has to do, and it would read the record this one is handed and carries.
func (Naive) PlaceParticipant(pool Pool, conference Conference, _ Arrival) Answer {
	if pool.Len() == 0 {
		return refuse(NoUnits())
	}

	on := make(map[UnitID]struct{}, len(conference.carrying))
	for _, c := range conference.carrying {
		on[c.unit] = struct{}{}
	}
	carrying := func(u Unit) bool { _, held := on[u.id]; return held }

	if best, found := lowest(pool, carrying); found {
		return place(best.id)
	}

	// The ceiling is asked before the rest of the pool and not after, because the
	// two refusals lead the caller to different places: a conference at its
	// ceiling is not a reason to grow the pool, and a conference below it with
	// nothing eligible anywhere is.
	if conference.Units() >= conference.unitCeiling {
		return refuse(ReachedUnitCeiling())
	}

	if best, found := lowest(pool, func(u Unit) bool { return !carrying(u) }); found {
		return place(best.id)
	}
	return refuse(NoEligibleUnit())
}
