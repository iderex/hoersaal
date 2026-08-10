// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package pool is the authoritative record of which forwarding units exist,
// what state each is in, and what each last said about its load.
//
// It answers issue #56. Everything else on the scaling milestone reads it and
// nothing else writes it, which is the property docs/design/scaling-loop.md
// fixes under "What is authoritative and what is a view": two components
// disagreeing about which machines exist is how a room comes to be believed to
// be on a machine that no longer runs.
//
// # The five states, and the one name this package settles
//
// Requested, Starting, Admitting, Draining, Gone. Who may move a unit between
// them is as much of the answer as the names are, and each move is a method
// here rather than an assignment somewhere else.
//
// docs/design/scaling-loop.md records a disagreement over one of the names:
// docs/decisions/placement-seam.md calls the placeable state admitting, issue
// #56 called it serving, and the note follows the landed document "until #56
// lands and says otherwise". This package is #56 landing, and it says
// admitting. The landed name wins because two documents already carry it and
// the issue body is the only place the other one appears, so choosing the
// issue's word would have cost two document edits to buy nothing.
//
// # The three inputs, and what each failure costs
//
// Health, reachability and the capacity signal are three separate inputs, and
// this package holds them apart rather than folding them into one verdict. A
// unit can answer the port perfectly and be unreachable from the participants
// it is supposed to serve, which is issue #56's own sentence and is the case a
// single health flag cannot express.
//
// What each verdict is, and how it is reached, is not decided here. What
// reachable means is issue #52, what the signal is and how often it arrives is
// issue #55, and what a health check runs is issue #84. This package owns that
// they are three, that each is recorded separately so the pool can say which
// one failed, and that failing any of them takes a unit out of Admitting.
//
// Where each failure sends it differs, and the difference is the point:
//
// Health failing sends the unit to Gone. That is docs/design/scaling-loop.md's
// own cause, "when it stops answering", and a unit that is not answering has
// already lost whatever it was holding.
//
// The signal going stale sends the unit to Gone, for the same reason read one
// step further out. The report is the answer, docs/decisions/media-plane-port.md
// leaves the pool to decide when one is too old, and a unit that has stopped
// saying what it holds is a unit nothing can place against.
//
// Reachability failing sends the unit to Draining instead, and this is a third
// cause of Draining beyond the two the note names. A unit participants cannot
// reach still answers the port and still holds live conferences, so Gone would
// throw away lectures that are running on a machine that is running. Draining
// stops it being chosen, which is what issue #52 asks for, and keeps what it
// has until those conferences end. docs/design/scaling-loop.md carries the
// added cause rather than leaving the code to be the only place it is written.
//
// # What a registration proves, and what it does not
//
// A unit joining the pool is telling the control plane where to send other
// people's media, so it authenticates. A pool that admits whatever registers is
// a media interception service, and that is not a hardening pass for later.
//
// The instrument is not internal/roomcred. That package mints a bearer
// credential carrying a conference, a role and a subject, and a unit has none
// of the three; carrying one under an invented conference identifier would be
// the weaker of the two designs and would tie the pool to a document about
// people. What a unit presents instead is a proof that it holds the key the
// operator gave it: a keyed digest over its own identifier and the moment it
// was made, checked here against the same key with a constant-time compare.
//
// The negative half, stated as a negative and not softened. This refuses a
// stranger who does not hold the key. It does not refuse somebody who captured
// a live proof and replays it inside MaxProofAge, and nothing in this package
// can: a proof is bytes, and bytes travel. What bounds that is the transport
// the registration arrives over, which is issue #35, and the window below.
// Nothing here has measured either.
//
// # What this package deliberately does not hold
//
// The durable record of provisioning requests, and the reconciliation against
// the provisioner at startup, which docs/decisions/control-plane-state.md
// places here and which issue #30's fourth condition waits on. Both need a
// provisioner to reconcile against and a store to write to, and neither exists:
// issue #63 owes the first and the store on issue #30 owes the second. This
// pool is in memory, one process, and says so rather than reading as though a
// restart kept anything.
//
// Which units carry which conference. That record is the control plane's, and
// docs/design/scaling-loop.md says why the two are not one: the pool answers
// which machines exist and the control plane answers which of them a conference
// is on.
package pool

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/mediaport"
	"github.com/iderex/hoersaal/internal/placement"
	"github.com/iderex/hoersaal/internal/secret"
)

// The numbers this package refuses against.
const (
	// MinKeyBytes is the shortest registration key this pool will verify with.
	//
	// It is the digest size rather than a figure chosen here. A key shorter
	// than the digest it feeds makes the key the bound on the work an attacker
	// does instead of the digest, and writing the number as sha256.Size means
	// the floor moves with the primitive rather than drifting against it.
	MinKeyBytes = sha256.Size

	// ProofVersion is the one registration proof layout that exists. It is the
	// first byte of what is signed, so a later layout is told apart rather than
	// verified against the wrong reading.
	ProofVersion = 1

	// MaxProofAge is how far either side of now a registration's stated moment
	// may sit and still be accepted.
	//
	// It is a judgement and not a measurement, and it is the replay window: a
	// captured proof is usable for this long. It is two-sided because the unit
	// and the control plane are two machines with two clocks, and a bound that
	// only looked backwards would refuse a unit whose clock runs a second fast.
	// It is wider than the difference between two clocks an operator keeps and
	// narrower than a lecture, neither of which has been measured on this
	// board. It is not an operator setting:
	// docs/decisions/what-an-operator-may-set.md admits a key only with an
	// issue against the derivation, and there is none here yet.
	MaxProofAge = 5 * time.Minute
)

// The refusals. Every method that moves a unit or records an input returns one
// of these wrapped with what it was given, so the kind of refusal says what
// kind of mistake was made without reading this file.
var (
	// ErrEmpty is an identifier with nothing in it.
	ErrEmpty = errors.New("pool: empty identifier")

	// ErrKeyTooShort is a registration key shorter than MinKeyBytes.
	ErrKeyTooShort = errors.New("pool: registration key is shorter than the minimum")

	// ErrNoClock is a pool built without one. Nothing in this tree reads the
	// machine's clock directly, so a pool with no clock cannot tell a fresh
	// report from a stale one and is refused where it is built.
	ErrNoClock = errors.New("pool: a pool is handed a clock")

	// ErrDuplicate is a unit entering the pool under an identifier it already
	// holds.
	ErrDuplicate = errors.New("pool: identifier already held")

	// ErrUnknownUnit is a move or an input naming a unit this pool does not
	// hold.
	ErrUnknownUnit = errors.New("pool: not in this pool")

	// ErrIllegalTransition is a move the state machine does not have. It is
	// returned rather than logged, which is this issue's first condition: a
	// transition nobody may make is a refusal and not a note.
	ErrIllegalTransition = errors.New("pool: the state machine has no such move")

	// ErrProof is a registration whose proof this key did not make, or one
	// whose stated moment is outside MaxProofAge.
	ErrProof = errors.New("pool: the registration proof does not verify")

	// ErrNoPort is a registration carrying no handle to the unit. A unit that
	// registered and cannot be asked anything is a row nothing can read.
	ErrNoPort = errors.New("pool: a registration carries the port handle of the unit")

	// ErrNotALoad is a load that is negative or is not a number, which
	// internal/placement refuses for the reason its own comment gives.
	ErrNotALoad = errors.New("pool: a load is a number and is not negative")

	// ErrNotAnAge is a staleness bound of zero or less, which would retire
	// every unit the moment it was applied.
	ErrNotAnAge = errors.New("pool: a staleness bound is a positive duration")
)

// A State is what the pool says about a unit. The set is closed at five and
// each value is a function rather than a variable so that nothing can reassign
// the set.
type State struct{ v string }

// Requested is a machine the provisioner has been asked for that does not exist
// yet. It is a state rather than a gap because it is the interval the whole
// scale-out inequality is about, and a pool that cannot see its own outstanding
// request will ask twice.
func Requested() State { return State{"requested"} }

// Starting is a machine that exists and does not yet answer the port.
func Starting() State { return State{"starting"} }

// Admitting is a unit that has registered and may be placed onto. It is the
// only state in which the placer may choose it.
func Admitting() State { return State{"admitting"} }

// Draining is a unit that takes nothing new and keeps what it holds until those
// conferences end on their own.
func Draining() State { return State{"draining"} }

// Gone is a unit that is not there. A unit in this state never returns: a
// machine that comes back registers as a new unit, because the port's ErrLost
// is what a restarted unit answers and a control plane that treated it as the
// same unit would go on believing in conferences that are not there.
func Gone() State { return State{"gone"} }

func (s State) String() string { return s.v }

// A Cause is why a unit left the state it was in. It is carried so that the
// pool can answer which of the three inputs failed rather than only that
// something did, which is what an operator asking why a machine was retired
// needs and what issue #66 reports on.
type Cause struct{ v string }

// TheOperatorAsked is a drain or a retirement somebody asked for.
func TheOperatorAsked() Cause { return Cause{"the operator asked"} }

// TheScaleInConditionHeld is the drain the pool decides on its own.
func TheScaleInConditionHeld() Cause { return Cause{"the scale-in condition held"} }

// HealthFailed is the health input saying the unit is not answering.
func HealthFailed() Cause { return Cause{"health failed"} }

// ReachabilityFailed is the reachability input saying participants cannot reach
// the unit.
func ReachabilityFailed() Cause { return Cause{"reachability failed"} }

// TheSignalWentStale is no load answer inside the bound the caller applied.
func TheSignalWentStale() Cause { return Cause{"the capacity signal went stale"} }

// TheUnitRestarted is the port's ErrLost, which says the state the caller
// believed in is gone.
func TheUnitRestarted() Cause { return Cause{"the unit restarted"} }

// TheDrainFinished is a drained unit the provisioner has released.
func TheDrainFinished() Cause { return Cause{"the drain finished"} }

func (c Cause) String() string { return c.v }

// A Unit is one row of the pool, as the pool holds it. It carries more than the
// placer's row does, because the placer reads three states and this record
// holds five, and because the three inputs are separate here and are one
// eligibility question there.
type Unit struct {
	id           placement.UnitID
	state        State
	cause        Cause
	reachable    bool
	healthy      bool
	reported     float64
	reportedAt   time.Time
	everReported bool
	committed    float64
	port         mediaport.Unit
}

// ID is which unit this row is about.
func (u Unit) ID() placement.UnitID { return u.id }

// State is what the pool says about it.
func (u Unit) State() State { return u.state }

// Cause is why it last left a state, and is empty for a unit that has not.
func (u Unit) Cause() Cause { return u.cause }

// Reachable is the reachability input, as last recorded.
func (u Unit) Reachable() bool { return u.reachable }

// Healthy is the health input, as last recorded.
func (u Unit) Healthy() bool { return u.healthy }

// Reported is the last load the unit itself answered with, which is what the
// unit believes about itself. It is zero for a unit that has not answered yet,
// and Reported and ReportedAt are read together: a zero with a zero time is an
// absence and a zero with a time is a unit holding nothing.
func (u Unit) Reported() float64 { return u.reported }

// ReportedAt is when that answer arrived, by the pool's clock. It is the zero
// time for a unit that has never answered.
func (u Unit) ReportedAt() time.Time { return u.reportedAt }

// EffectiveLoad is the reported load plus every placement committed against
// this unit since that report. It is what the placer reads, and
// docs/decisions/placement-seam.md gives the reason: taking the reported number
// instead is how two placements in the same second stop being visible to each
// other.
func (u Unit) EffectiveLoad() float64 { return u.reported + u.committed }

// A Registration is what a unit presents when it joins. The proof is over the
// version, the identifier and the moment, so a proof made for one unit does not
// admit another and a proof made an hour ago does not admit anybody.
type Registration struct {
	// Unit is which unit is registering.
	Unit placement.UnitID

	// IssuedAt is the moment the unit says it made the proof.
	IssuedAt time.Time

	// Proof is the keyed digest over the three above.
	Proof []byte
}

// Prove makes the proof a unit presents. It is here rather than in whatever
// runs on a unit so that the two ends cannot drift into two layouts, and it is
// exported for the same reason internal/roomcred exports its issuer: the thing
// that mints and the thing that verifies are one decision.
func Prove(key secret.Bytes, unit placement.UnitID, issuedAt time.Time) ([]byte, error) {
	switch {
	case key.Len() < MinKeyBytes:
		return nil, fmt.Errorf("proving %d bytes: %w", key.Len(), ErrKeyTooShort)
	case unit.String() == "":
		return nil, fmt.Errorf("proving a registration: %w", ErrEmpty)
	}
	return proof(key, unit, issuedAt), nil
}

func proof(key secret.Bytes, unit placement.UnitID, issuedAt time.Time) []byte {
	m := hmac.New(sha256.New, key.Reveal())
	m.Write(payload(unit, issuedAt))
	return m.Sum(nil)
}

// payload is the one place the covered bytes are laid out, so the end that
// makes a proof and the end that checks one cannot disagree about what is
// covered. The version is first, then the identifier with its length in front
// of it, then the moment as eight bytes of seconds since the epoch in UTC.
//
// The length in front of the identifier is not what separates two identifiers
// today, and that is worth writing down rather than leaving somebody to assume
// otherwise. One variable-length field followed by a fixed-width one is already
// unambiguous, and deleting the length leaves the suite green, which was run
// rather than reasoned about. It is there for the field after this one: two
// adjacent variable-length fields are where a proof for one thing starts
// admitting another, and the change that adds the second field is the one
// nobody re-derives the layout for.
func payload(unit placement.UnitID, issuedAt time.Time) []byte {
	out := make([]byte, 0, 16+len(unit.String()))
	out = append(out, ProofVersion)
	// #nosec G115 -- Prove and Register both refuse an empty identifier, and a
	// unit identifier long enough to overflow a uint16 is one no pool could
	// hold, since the same string is a map key in this process.
	out = binary.BigEndian.AppendUint16(out, uint16(len(unit.String())))
	out = append(out, unit.String()...)
	// #nosec G115 -- the moment is written as a count of seconds since the
	// epoch and is never read back out of these bytes: the check recomputes the
	// whole payload from a time it already holds and compares digests.
	out = binary.BigEndian.AppendUint64(out, uint64(issuedAt.UTC().Unix()))
	return out
}

// A Pool is the record. It is safe for several goroutines, because
// docs/decisions/placement-seam.md says the serialisation the pool view uses
// for its own writes is the serialisation for placement, and a record that
// needed a second lock outside it would be a second mechanism to keep in step
// with.
type Pool struct {
	mu    sync.Mutex
	key   secret.Bytes
	clock clock.Clock
	units map[placement.UnitID]*Unit
	order []placement.UnitID
}

// New refuses a key shorter than MinKeyBytes and a pool with no clock.
func New(key secret.Bytes, c clock.Clock) (*Pool, error) {
	switch {
	case key.Len() < MinKeyBytes:
		return nil, fmt.Errorf("a pool key of %d bytes: %w", key.Len(), ErrKeyTooShort)
	case c == nil:
		return nil, ErrNoClock
	}
	return &Pool{key: key, clock: c, units: map[placement.UnitID]*Unit{}}, nil
}

// Format writes a placeholder under every verb, because a Pool holds the
// registration key in a field and fmt reaches an unexported field by reflection
// without calling the methods on its type. internal/secret's own comment says
// why the type cannot answer for itself there and that the holder has to.
func (p *Pool) Format(f fmt.State, verb rune) {
	// #nosec G104 -- a Format method has nowhere to return an error to, and the
	// write failing is reported to whoever started the printing, for the reason
	// secret.Bytes.Format gives at the same call.
	io.WriteString(f, "pool.Pool{key: "+secret.Placeholder+"}")
}

// Request records that the provisioner has been asked for a machine. It is the
// pool's own move: nothing else may create a Requested unit, because the state
// exists so that the pool can see its own outstanding request and not ask
// twice.
func (p *Pool) Request(id placement.UnitID) error { return p.enter(id, Requested()) }

// Listed records a machine the operator listed rather than one the pool asked
// for. It enters at Starting, because that is what Starting means: the machine
// exists and does not yet answer the port.
//
// It is a separate method from Request rather than a flag, so that a pool
// reading its own rows can tell a machine it bought from one it was given, and
// so that the fixed-pool deployment on issue #63, where asking for another unit
// always fails, has a way in that does not pretend a request was made.
func (p *Pool) Listed(id placement.UnitID) error { return p.enter(id, Starting()) }

func (p *Pool) enter(id placement.UnitID, at State) error {
	if id.String() == "" {
		return fmt.Errorf("entering the pool: %w", ErrEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, held := p.units[id]; held {
		return fmt.Errorf("unit %s: %w", id, ErrDuplicate)
	}
	p.units[id] = &Unit{id: id, state: at, healthy: true}
	p.order = append(p.order, id)
	return nil
}

// Started moves a requested unit to Starting. Only the provisioner makes this
// move, by reporting that the machine exists.
func (p *Pool) Started(id placement.UnitID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.held(id)
	if err != nil {
		return err
	}
	if u.state != Requested() {
		return p.illegal(u, Starting())
	}
	u.state = Starting()
	return nil
}

// Reachable records the reachability input. It is recorded for a unit in any
// state, because issue #52 asks that a unit be verified reachable before it is
// given any participant, which means the verdict has to exist before the unit
// registers rather than after.
//
// A unit that is Admitting and becomes unreachable drains. It answers the port
// and holds live conferences, so retiring it would throw away lectures on a
// machine that is running, and a drain stops it being chosen without doing
// that. A drain is not undone by reachability coming back: a unit on its way
// out stays on its way out, and a machine that recovers registers as a new one.
func (p *Pool) Reachable(id placement.UnitID, ok bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.held(id)
	if err != nil {
		return err
	}
	u.reachable = ok
	if !ok && u.state == Admitting() {
		u.state, u.cause = Draining(), ReachabilityFailed()
	}
	return nil
}

// Healthy records the health input. A unit that stops answering is Gone, which
// is docs/design/scaling-loop.md's own cause for that state.
//
// What counts as not answering is not decided here, and one ErrUnavailable is
// not it: docs/decisions/placement-seam.md says so in those words, and Collect
// below leaves a single unanswered call to age out through the signal rather
// than treating it as a death.
func (p *Pool) Healthy(id placement.UnitID, ok bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.held(id)
	if err != nil {
		return err
	}
	u.healthy = ok
	if !ok && u.state != Gone() {
		u.state, u.cause = Gone(), HealthFailed()
	}
	return nil
}

// Register admits a unit that proves it holds the operator's key. It is the
// only move into Admitting, and the unit itself is the only thing that makes
// it.
//
// Three things have to hold and each refusal says which. The proof has to
// verify against the key and sit inside MaxProofAge of now. The unit has to be
// Starting, because a unit that was never provisioned and a unit that has
// already gone are both things nothing asked for. And reachability has to have
// been recorded as holding, which is issue #56's own sentence about the check
// being part of becoming serving rather than a later formality.
func (p *Pool) Register(r Registration, port mediaport.Unit) error {
	if r.Unit.String() == "" {
		return fmt.Errorf("registering: %w", ErrEmpty)
	}
	if port == nil {
		return fmt.Errorf("unit %s: %w", r.Unit, ErrNoPort)
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	want := proof(p.key, r.Unit, r.IssuedAt)
	if !hmac.Equal(want, r.Proof) {
		return fmt.Errorf("unit %s: %w", r.Unit, ErrProof)
	}
	age := p.clock.Now().Sub(r.IssuedAt)
	if age < 0 {
		age = -age
	}
	if age > MaxProofAge {
		return fmt.Errorf("unit %s, proof made %s from now: %w", r.Unit, age, ErrProof)
	}

	u, err := p.held(r.Unit)
	if err != nil {
		return err
	}
	if u.state != Starting() {
		return p.illegal(u, Admitting())
	}
	if !u.reachable {
		return fmt.Errorf("unit %s has not been verified reachable: %w", r.Unit, ErrIllegalTransition)
	}
	u.state, u.cause, u.port = Admitting(), Cause{}, port
	return nil
}

// Drain moves an admitting unit to Draining. Only the pool makes this move, for
// the two causes docs/design/scaling-loop.md names and for the third this
// package adds; the placer never does, because the placer reads the state and
// does not interpret it.
func (p *Pool) Drain(id placement.UnitID, why Cause) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.held(id)
	if err != nil {
		return err
	}
	if u.state != Admitting() {
		return p.illegal(u, Draining())
	}
	u.state, u.cause = Draining(), why
	return nil
}

// Retire moves a unit to Gone from anywhere but Gone. Retiring one that is
// already Gone is refused rather than accepted quietly, because the second
// call carries a second cause and the first one is the true one.
func (p *Pool) Retire(id placement.UnitID, why Cause) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.held(id)
	if err != nil {
		return err
	}
	if u.state == Gone() {
		return p.illegal(u, Gone())
	}
	u.state, u.cause = Gone(), why
	return nil
}

// Report records what a unit answered about its own load, with the time the
// answer arrived by the pool's clock. It resets the committed load, because the
// number the unit just gave already carries every placement it has noticed.
func (p *Pool) Report(id placement.UnitID, load float64) error {
	if math.IsNaN(load) || load < 0 {
		return fmt.Errorf("unit %s at load %v: %w", id, load, ErrNotALoad)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.held(id)
	if err != nil {
		return err
	}
	u.reported, u.reportedAt, u.everReported, u.committed = load, p.clock.Now(), true, 0
	return nil
}

// Commit adds the load of a placement made against a unit since its last
// report. It refuses a unit that is not Admitting, because a placement against
// a unit the placer may not choose is a placement that should not have been
// made and a pool that recorded it would hide the mistake in an effective load.
func (p *Pool) Commit(id placement.UnitID, load float64) error {
	if math.IsNaN(load) || load < 0 {
		return fmt.Errorf("committing %v against unit %s: %w", load, id, ErrNotALoad)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.held(id)
	if err != nil {
		return err
	}
	if u.state != Admitting() {
		return p.illegal(u, Admitting())
	}
	u.committed += load
	return nil
}

// Collect asks every unit that answers the port what it is holding and records
// the answers. It is how the pool decides liveness, which
// docs/decisions/media-plane-port.md fixes: a unit that has died sends no
// notice, so silence is not a signal and asking is.
//
// Two answers and two responses. A load is recorded. An error leaves the row
// untouched, because one unanswered call is not a death,
// docs/decisions/placement-seam.md says so in those words, and what retires a
// unit that goes on not answering is Sweep below, when its last answer is older
// than the bound the caller applies.
//
// A restart does not reach the pool here, and that is the port document rather
// than an omission. ReportCapacity is declared to answer ErrUnavailable and
// nothing else, so a unit that came back holding nothing answers this call with
// a load like any other. What meets ErrLost is the control plane, on the first
// operation naming a conference the unit held before, and it tells the pool by
// retiring the unit with TheUnitRestarted.
//
// It returns the units it recorded a load for, so a caller has the list without
// reading every row back.
func (p *Pool) Collect() []placement.UnitID {
	type asking struct {
		id   placement.UnitID
		port mediaport.Unit
	}
	p.mu.Lock()
	var ask []asking
	for _, id := range p.order {
		u := p.units[id]
		if u.port != nil && (u.state == Admitting() || u.state == Draining()) {
			ask = append(ask, asking{id: id, port: u.port})
		}
	}
	p.mu.Unlock()

	var answered []placement.UnitID
	for _, a := range ask {
		load, err := a.port.ReportCapacity()
		if err != nil {
			continue
		}
		if p.Report(a.id, load) == nil {
			answered = append(answered, a.id)
		}
	}
	return answered
}

// Sweep retires every unit whose last load answer is older than the bound, and
// every unit that has registered and never answered at all for that long. It
// returns what it retired.
//
// The bound is the caller's rather than a constant here. How often a unit
// reports is issue #55 and no decision document fixes it, which
// docs/design/scaling-loop.md says in its own words, so a number written here
// would be inventing the value that issue exists to produce.
func (p *Pool) Sweep(olderThan time.Duration) ([]placement.UnitID, error) {
	if olderThan <= 0 {
		return nil, fmt.Errorf("a bound of %s: %w", olderThan, ErrNotAnAge)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock.Now()
	var retired []placement.UnitID
	for _, id := range p.order {
		u := p.units[id]
		if u.state != Admitting() && u.state != Draining() {
			continue
		}
		if u.everReported && now.Sub(u.reportedAt) <= olderThan {
			continue
		}
		if !u.everReported && u.port == nil {
			continue
		}
		u.state, u.cause = Gone(), TheSignalWentStale()
		retired = append(retired, id)
	}
	return retired, nil
}

// Units is the whole pool in one call, ordered by unit identifier so that two
// calls return the same thing in the same order. It is a copy of every row, so
// a caller reading the pool cannot change what the next placement is decided
// against.
//
// This is the answer to issue #56's fourth condition, and it is deliberately
// wider than the placer's view below: it carries all five states and the three
// inputs separately, so a caller can ask which input failed rather than only
// that a unit is no longer being placed onto.
func (p *Pool) Units() []Unit {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Unit, 0, len(p.order))
	for _, id := range p.order {
		out = append(out, *p.units[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id.String() < out[j].id.String() })
	return out
}

// Unit is the row under this identifier, and whether the pool holds it.
func (p *Pool) Unit(id placement.UnitID) (Unit, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, held := p.units[id]
	if !held {
		return Unit{}, false
	}
	return *u, true
}

// View is the pool as docs/decisions/placement-seam.md's first input: one row
// per unit the seam has a word for, carrying the effective load and the state.
//
// Requested and Starting units are absent from it rather than mapped onto one
// of the seam's three. The seam names admitting, draining and gone, and the
// placer reads them without interpreting them, so a machine that is on its way
// and has no word there is better left out than reported as draining or as
// gone, both of which say something untrue about it. What that costs is that a
// pool holding only outstanding requests looks empty to the placer, and the
// answer is the right one: there is nowhere to place. A caller that needs to
// know a machine is coming reads Units above, which is where the pool's own
// states live.
func (p *Pool) View() (placement.Pool, error) {
	rows := p.Units()
	var units []placement.Unit
	for _, r := range rows {
		var s placement.State
		switch r.state {
		case Admitting():
			s = placement.Admitting()
		case Draining():
			s = placement.Draining()
		case Gone():
			s = placement.Gone()
		default:
			continue
		}
		u, err := placement.NewUnit(r.id, r.EffectiveLoad(), r.reportedAt, s)
		if err != nil {
			return placement.Pool{}, fmt.Errorf("pool view: %w", err)
		}
		units = append(units, u)
	}
	return placement.NewPool(units...)
}

// held answers the row under an identifier. The caller holds the lock.
func (p *Pool) held(id placement.UnitID) (*Unit, error) {
	if id.String() == "" {
		return nil, fmt.Errorf("naming a unit: %w", ErrEmpty)
	}
	u, ok := p.units[id]
	if !ok {
		return nil, fmt.Errorf("unit %s: %w", id, ErrUnknownUnit)
	}
	return u, nil
}

// illegal is the one refusal every move that is not in the machine goes
// through, so the sentence a caller reads names both ends rather than only that
// something was refused.
func (p *Pool) illegal(u *Unit, to State) error {
	return fmt.Errorf("unit %s is %s and cannot become %s: %w", u.id, u.state, to, ErrIllegalTransition)
}
