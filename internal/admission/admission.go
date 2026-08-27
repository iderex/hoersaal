// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package admission runs the exchange in which somebody presenting a room
// credential becomes a participant on a forwarding unit.
//
// It answers issue #35. Every step, every failure and what a client is told in
// each case is argued in docs/decisions/room-admission.md rather than here, so
// the two cannot drift into two different answers.
//
// This is the exchange that runs three hundred times in the two minutes before a
// lecture, so it is written as a function of its inputs. A Desk holds the seams
// it was built with and the admissions it has granted and not yet seen
// completed, and nothing else: the room it is asked about is passed in, the
// clock is passed in, the identifiers it mints come from a seam, and there is no
// connection anywhere in it. A room of three hundred is therefore a test rather
// than a deployment, which is the property internal/presence was built for.
//
// # The order the steps are in, and why it is that order
//
// The credential is read first, because everything after it costs something and
// a stranger must not be able to spend a placement decision. The powers are
// asked second, so that a request to publish which the role does not carry is
// refused before any unit is told anything, which is the condition of issue #35
// that docs/threat-model.md names. The placer is asked third and may refuse,
// which is a normal answer rather than an error. The unit is told last, because
// it is the only step that leaves anything behind.
//
// # The media plane credential carries no power the room credential did not
//
// The transport parameters a participant receives are the ones the unit answered
// with, and which of the two admissions the unit was asked for is decided here
// from the role. A credential whose role may not publish reaches AdmitSubscriber
// and never AdmitPublisher, so the unit is never made ready to accept a
// publication from somebody who holds no floor. That is the whole of the
// mechanism: the unit and not this package is what enforces it at the moment
// media arrives, and this package's part is not to ask for more than was granted.
//
// What a role bundles is issue #34 and is not decided here. Powers is the
// smallest question this exchange has to ask, it is a seam, and a Desk takes the
// answer rather than computing it.
//
// # An admission nobody completes
//
// Every step can fail, and a failure after the unit has been told leaves an
// admission nobody will ever use. The route by which those are reclaimed has two
// halves and only one of them is in this repository.
//
// The half that is here: a granted admission is held with a deadline, Arrived
// takes it out of that set, and Sweep reports the ones whose deadline passed and
// forgets them. So the control plane stops believing in an admission that never
// completed, and a client that reappears afterwards is a new admission rather
// than a claim on an old one.
//
// The half that is not: the unit still holds its side of it. The port in
// docs/decisions/media-plane-port.md has eight operations and none of them
// releases one participant, so nothing here can shorten the life of the unit's
// copy. What releases it is CloseConference, which releases every participant in
// the conference, so an abandoned admission costs the unit until the conference
// ends. That bound is real and it is stated rather than described as a reclaim:
// Sweep answers with the unit each abandoned admission is on, so the day the
// port grows an operation for it, the call site is already here and named.
package admission

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/domain"
	"github.com/iderex/hoersaal/internal/mediaport"
	"github.com/iderex/hoersaal/internal/placement"
	"github.com/iderex/hoersaal/internal/roomcred"
	"github.com/iderex/hoersaal/internal/wire"
)

// The message types this exchange owns. internal/wire carries the envelope and
// gives a type no meaning; these are the names inside it, prefixed the way
// internal/presence prefixes its own.
const (
	// TypeRequest is what a client sends to join a room.
	TypeRequest = "admission.request"

	// TypeGranted is what it receives when it may.
	TypeGranted = "admission.granted"

	// TypeRefused is what it receives when it may not, carrying one of Reasons
	// and nothing else.
	TypeRefused = "admission.refused"
)

// DefaultWindow is how long a granted admission is held before Sweep reports it
// abandoned. It is a duration rather than a number of attempts, because what is
// being waited for is a client connecting to a unit over a path this process
// cannot see.
//
// No decision document fixes it. It is longer than the twenty seconds
// docs/decisions/signalling-transport.md allows a client to answer a ping,
// because a participant that has to carry transport parameters to a media stack,
// gather candidates and connect is doing more than answering; and it is far
// shorter than a lecture, because an admission that outlives the room it was for
// has stopped being a bound at all. A Desk takes the value as an argument, so a
// deployment that measures a better one on issue #71 sets it there.
const DefaultWindow = 2 * time.Minute

// A Reason is why an admission was refused. A client acts on which one it
// received, so each is a distinct string on the wire, and Reasons below is the
// whole set: Admit answers with one of them or with a grant.
type Reason struct{ v string }

func (r Reason) String() string { return r.v }

// CredentialRefused is a credential that did not verify: unreadable, signed with
// another key, naming another room, or outside its window.
//
// It is one reason and not four on purpose. Which of the four failed is a fact
// about the credential a forger holds, and answering it turns the refusal into
// an oracle that says how close an attempt was. What an honest client loses is
// nothing it can act on, because every one of the four is repaired by asking
// whoever sent the link for another one.
func CredentialRefused() Reason { return Reason{"credential-refused"} }

// NotPermitted is a credential that verified and a request its role does not
// carry, which today is a request to publish from a role that may not.
func NotPermitted() Reason { return Reason{"not-permitted"} }

// ConferenceNotOpen is a room that is not taking participants yet. It is its own
// answer because a client can act on it by waiting, which is true of no other
// refusal here.
func ConferenceNotOpen() Reason { return Reason{"conference-not-open"} }

// RoomFull is the placer finding no unit eligible to take another participant.
func RoomFull() Reason { return Reason{"room-full"} }

// NoCapacity is the placer finding no units at all. It is separate from RoomFull
// because it is the deployment's problem rather than the room's, and issue #64 is
// where the deployment refuses rather than degrades.
func NoCapacity() Reason { return Reason{"no-capacity"} }

// ConferenceAtItsUnitCeiling is the conference already occupying as many units as
// docs/decisions/room-topology.md allows one conference to occupy.
func ConferenceAtItsUnitCeiling() Reason { return Reason{"conference-at-its-unit-ceiling"} }

// UnitRefused is the unit answering that it could admit this participant and
// will not, because of what it is already holding.
func UnitRefused() Reason { return Reason{"unit-refused"} }

// UnitUnavailable is the unit not answering, or answering that the state this
// process believed in is gone. The two are one refusal here because what a client
// does about either is the same and is immediate, while the difference between
// them belongs to the pool, which is what decides the unit's fate.
func UnitUnavailable() Reason { return Reason{"unit-unavailable"} }

// MalformedRequest is a message that is not an admission request: the wrong type,
// a payload that is not the object this exchange reads, or a field it cannot use.
func MalformedRequest() Reason { return Reason{"malformed-request"} }

// Reasons is the whole set, in the order the steps that produce them run. A
// client that wants to enumerate what it may be told reads this rather than a
// list in a document, and the suite refuses a reason the code can produce that is
// not in it.
var Reasons = []Reason{
	MalformedRequest(),
	CredentialRefused(),
	NotPermitted(),
	ConferenceNotOpen(),
	NoCapacity(),
	RoomFull(),
	ConferenceAtItsUnitCeiling(),
	UnitRefused(),
	UnitUnavailable(),
}

// The refusals this package returns as errors rather than as answers. A refusal a
// client can act on is a Reason and travels in a message; these are faults in the
// caller or in a seam, and they are the only way Admit returns an error.
var (
	// ErrNoSeam is a Desk built without a part it cannot work without.
	ErrNoSeam = errors.New("admission: the desk is missing a seam it cannot work without")

	// ErrRoom is a room record this exchange cannot use.
	ErrRoom = errors.New("admission: the room record is not usable")

	// ErrNames is the identifier source failing. It is a fault rather than a
	// refusal because a control plane that cannot mint an identifier is serving
	// nobody, and telling one client the room is full would hide that.
	ErrNames = errors.New("admission: no identifier could be minted")
)

// Powers answers what a role may do. It is the smallest question this exchange
// has to ask, and it is a seam rather than a table here because what a role
// bundles is issue #34 and a bundle written in this package would decide that
// issue by accident.
type Powers interface {
	// MayPublish answers whether a participant holding this role may offer
	// sources at all. It is asked once per admission, before any unit is told
	// anything.
	MayPublish(role domain.Role) bool
}

// Names mints the identifiers an admission creates. It is a seam for the reason
// the clock is one: a test that cannot say what the next identifier will be is a
// test nobody can assert against.
//
// Nothing in this repository satisfies it yet, and that absence is deliberate
// rather than an omission. A participant identifier is sent to everybody in the
// room by internal/presence, so it is not a secret; what it must not be is
// guessable in a way that lets one deployment's admissions be predicted, and
// where those bytes come from depends on whether the control plane runs as one
// instance or several, which is issue #39 and is unanswered. A Desk takes the
// answer rather than choosing it.
type Names interface {
	// New answers a fresh identifier, or an error where it cannot.
	New() (string, error)
}

// Units resolves what the placer answered into something that can be asked. The
// placer answers with a placement.UnitID because docs/decisions/placement-seam.md
// hands it a record and not a connection; this is where that identifier becomes
// the port.
type Units interface {
	// Unit answers the port for this unit, and whether this deployment holds one.
	Unit(id placement.UnitID) (mediaport.Unit, bool)
}

// A Room is what a Desk is told about the conference at the moment somebody asks
// to join it. It is passed in rather than held, so a Desk carries no room state
// and two rooms are two values rather than two desks.
type Room struct {
	// ID is the conference.
	ID domain.ConferenceID

	// Open is whether the room is taking participants. A room that is not open
	// refuses with ConferenceNotOpen, which is the answer a client can wait on.
	Open bool

	// Pool is the units this deployment holds, as the placer reads them.
	Pool placement.Pool

	// Placement is where this conference already is and how many units it may
	// occupy, as the placer reads it.
	Placement placement.Conference
}

// A Grant is an admission that was made: who the participant is, where they go,
// and what they need in order to connect.
type Grant struct {
	// Conference is the room this admits to.
	Conference domain.ConferenceID

	// Participant is the identifier this admission minted. It names one endpoint
	// of one conference and is new every time, so a client that drops and comes
	// back is a second participant rather than a claim on the first.
	Participant domain.ParticipantID

	// Identity is who the credential said this is. It is the zero identity for a
	// credential minted into a link that was sent to a group.
	Identity domain.IdentityID

	// Role is the name the credential carried. What it bundles is issue #34.
	Role domain.Role

	// Unit is where the placer put them.
	Unit placement.UnitID

	// Publishing is whether the unit was told to accept sources from them.
	Publishing bool

	// Sources are the sources the unit was told to accept. It is empty where
	// Publishing is false.
	Sources []domain.Source

	// Transport is what the unit answered with. It is opaque here and is carried
	// to the client unread.
	Transport mediaport.Transport

	// GrantedAt and Deadline bound the wait for the client to arrive.
	GrantedAt time.Time
	Deadline  time.Time
}

// An Outcome is what one attempt produced: a grant or a refusal, and the message
// to send back either way. There is no third state and no error in it, for the
// reason placement.Answer has none: a refusal is the normal way this exchange
// says no, and a caller reading it out of an error would be reading a policy
// statement as a failure.
type Outcome struct {
	grant   *Grant
	reason  Reason
	message wire.Message
}

// Granted answers the grant, and whether there was one.
func (o Outcome) Granted() (Grant, bool) {
	if o.grant == nil {
		return Grant{}, false
	}
	return *o.grant, true
}

// Refused answers why nothing was granted, and whether that is what happened.
func (o Outcome) Refused() (Reason, bool) {
	if o.grant != nil {
		return Reason{}, false
	}
	return o.reason, true
}

// Message is what to send the client. It is built here rather than by the caller,
// so that a refusal cannot reach a client in a shape this exchange never
// described.
func (o Outcome) Message() wire.Message { return o.message }

// An Abandoned is a granted admission whose client never arrived. Unit is in it
// because the unit still holds its side and the port has no operation that
// releases one participant, so whoever eventually has one needs to know which
// unit to ask.
type Abandoned struct {
	Conference  domain.ConferenceID
	Participant domain.ParticipantID
	Unit        placement.UnitID
	GrantedAt   time.Time
	Deadline    time.Time
}

// A Desk runs the exchange. It is safe for concurrent use, because three hundred
// people join at once and the set of outstanding admissions is the one thing here
// that is shared between them.
type Desk struct {
	verifier *roomcred.Verifier
	powers   Powers
	placer   placement.ParticipantPlacer
	units    Units
	names    Names
	clock    clock.Clock
	window   time.Duration

	mu          sync.Mutex
	outstanding map[domain.ParticipantID]Abandoned
}

// NewDesk refuses a desk that is missing anything it cannot work without, rather
// than answering ErrNoSeam three hundred times at the start of a lecture.
//
// window is how long a grant is held before Sweep reports it abandoned. Zero
// takes DefaultWindow.
func NewDesk(v *roomcred.Verifier, p Powers, pl placement.ParticipantPlacer, u Units, n Names, c clock.Clock, window time.Duration) (*Desk, error) {
	switch {
	case v == nil:
		return nil, fmt.Errorf("%w: no verifier", ErrNoSeam)
	case p == nil:
		return nil, fmt.Errorf("%w: no powers", ErrNoSeam)
	case pl == nil:
		return nil, fmt.Errorf("%w: no placer", ErrNoSeam)
	case u == nil:
		return nil, fmt.Errorf("%w: no units", ErrNoSeam)
	case n == nil:
		return nil, fmt.Errorf("%w: no names", ErrNoSeam)
	case c == nil:
		return nil, fmt.Errorf("%w: no clock", ErrNoSeam)
	case window < 0:
		return nil, fmt.Errorf("%w: a negative window", ErrNoSeam)
	}
	if window == 0 {
		window = DefaultWindow
	}
	return &Desk{
		verifier:    v,
		powers:      p,
		placer:      pl,
		units:       u,
		names:       n,
		clock:       c,
		window:      window,
		outstanding: map[domain.ParticipantID]Abandoned{},
	}, nil
}

// requestJSON is the shape a client sends, with the wire names on it.
//
// It carries no participant identifier and no identity. Both are the control
// plane's to decide: a client that named itself would be naming somebody else on
// the attempt that mattered, and the identity is inside the signed bytes of the
// credential rather than beside them.
type requestJSON struct {
	Conference string      `json:"conference"`
	Credential string      `json:"credential"`
	Publishing bool        `json:"publishing"`
	Offers     []offerJSON `json:"offers"`
}

// offerJSON is one source a publisher intends to send. It names a kind and how
// many encodings of it the publisher will produce, and it does not name the
// source, because a source identifier is minted here.
type offerJSON struct {
	Kind   string `json:"kind"`
	Layers int    `json:"layers"`
}

type grantedJSON struct {
	Conference  string `json:"conference"`
	Participant string `json:"participant"`
	Role        string `json:"role"`
	Publishing  bool   `json:"publishing"`
	Transport   []byte `json:"transport"`
}

type refusedJSON struct {
	Reason string `json:"reason"`
}

// Admit runs the whole exchange for one message and answers what to send back.
//
// It returns an error only where this process is at fault: a room record it
// cannot read, or an identifier source that failed. Everything a client did
// wrong, and everything the deployment could not do for them, is a Reason in the
// Outcome.
func (d *Desk) Admit(room Room, m wire.Message) (Outcome, error) {
	if room.ID.String() == "" {
		return Outcome{}, fmt.Errorf("%w: no conference", ErrRoom)
	}
	if room.Placement.ID() != room.ID {
		return Outcome{}, fmt.Errorf("%w: the placement record names %q and the room is %q", ErrRoom, room.Placement.ID().String(), room.ID.String())
	}

	req, ok := readRequest(m)
	if !ok {
		return d.refuse(MalformedRequest())
	}
	if req.Conference != room.ID.String() {
		// The room the message names and the room this connection is in
		// disagree. It is malformed rather than a credential refusal: nothing
		// about the credential has been read yet, and refusing it as one would
		// tell a prober that the room they named exists.
		return d.refuse(MalformedRequest())
	}

	claims, err := d.verifier.Verify(req.Credential, room.ID.String())
	if err != nil {
		return d.refuse(CredentialRefused())
	}
	role, err := domain.NewRole(claims.Role)
	if err != nil {
		// A credential that verified and carries no role is this installation's
		// own fault rather than the holder's, and it is refused as a credential
		// because the credential is the thing that is wrong.
		return d.refuse(CredentialRefused())
	}

	if req.Publishing && !d.powers.MayPublish(role) {
		// Before any unit is told anything, which is the condition of issue #35
		// that docs/threat-model.md names under a participant publishing when
		// they hold no floor.
		return d.refuse(NotPermitted())
	}
	if !room.Open {
		return d.refuse(ConferenceNotOpen())
	}

	participant, sources, err := d.mint(req)
	if err != nil {
		return Outcome{}, err
	}
	if req.Publishing && len(sources) != len(req.Offers) {
		// An offer the model refuses: a kind that is neither audio nor video, or
		// a layer arrangement outside what a source may have. Nothing has been
		// told to any unit at this point.
		return d.refuse(MalformedRequest())
	}

	arrival, err := placement.NewArrival(req.Publishing, sources...)
	if err != nil {
		return d.refuse(MalformedRequest())
	}

	answer := d.placer.PlaceParticipant(room.Pool, room.Placement, arrival)
	unitID, placed := answer.Placed()
	if !placed {
		reason, _ := answer.Refused()
		return d.refuse(placementReason(reason))
	}

	unit, held := d.units.Unit(unitID)
	if !held {
		// The placer named a unit this deployment cannot reach, which is the
		// pool and the placer disagreeing about which machines exist.
		// docs/design/scaling-loop.md names that as the failure the authority
		// rule is arranged against. The client is told the same thing as for a
		// unit that did not answer, because from where they stand it is the same
		// thing.
		return d.refuse(UnitUnavailable())
	}

	var transport mediaport.Transport
	if req.Publishing {
		transport, err = unit.AdmitPublisher(room.ID, participant, sources)
	} else {
		transport, err = unit.AdmitSubscriber(room.ID, participant)
	}
	if err != nil {
		return d.refuse(unitReason(err))
	}

	now := d.clock.Now()
	grant := Grant{
		Conference:  room.ID,
		Participant: participant,
		Identity:    identityOf(claims),
		Role:        role,
		Unit:        unitID,
		Publishing:  req.Publishing,
		Sources:     sources,
		Transport:   transport,
		GrantedAt:   now,
		Deadline:    now.Add(d.window),
	}

	payload, err := json.Marshal(grantedJSON{
		Conference:  room.ID.String(),
		Participant: participant.String(),
		Role:        role.String(),
		Publishing:  req.Publishing,
		Transport:   transport.Bytes(),
	})
	if err != nil {
		return Outcome{}, fmt.Errorf("admission: the grant could not be encoded: %w", err)
	}

	d.mu.Lock()
	d.outstanding[participant] = Abandoned{
		Conference:  room.ID,
		Participant: participant,
		Unit:        unitID,
		GrantedAt:   now,
		Deadline:    grant.Deadline,
	}
	d.mu.Unlock()

	return Outcome{grant: &grant, message: wire.Message{Type: TypeGranted, Payload: payload}}, nil
}

// Arrived says the client connected to the unit and the admission is complete. It
// answers whether this desk was still holding one for that participant, so a
// caller can tell a first arrival from a second one and from one already swept.
func (d *Desk) Arrived(p domain.ParticipantID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, held := d.outstanding[p]
	delete(d.outstanding, p)
	return held
}

// Sweep forgets every admission whose deadline has passed and answers them in the
// order they were granted, so that what a caller records reads in the order the
// admissions were made rather than in map order.
//
// What it does not do is release the unit's side, because the port has no
// operation that releases one participant. The Unit in each answer is what
// whoever eventually has one will need.
func (d *Desk) Sweep() []Abandoned {
	now := d.clock.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	var out []Abandoned
	for p, a := range d.outstanding {
		if now.Before(a.Deadline) {
			continue
		}
		out = append(out, a)
		delete(d.outstanding, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GrantedAt.Equal(out[j].GrantedAt) {
			return out[i].Participant.String() < out[j].Participant.String()
		}
		return out[i].GrantedAt.Before(out[j].GrantedAt)
	})
	return out
}

// Outstanding is how many admissions have been granted and neither completed nor
// swept, so that a test and an operator ask the same question.
func (d *Desk) Outstanding() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.outstanding)
}

// refuse builds the one shape a refusal reaches a client in.
func (d *Desk) refuse(r Reason) (Outcome, error) {
	payload, err := json.Marshal(refusedJSON{Reason: r.String()})
	if err != nil {
		return Outcome{}, fmt.Errorf("admission: the refusal could not be encoded: %w", err)
	}
	return Outcome{reason: r, message: wire.Message{Type: TypeRefused, Payload: payload}}, nil
}

// mint makes the identifiers this admission creates, the participant first,
// because a source names its publisher. An offer the model refuses produces
// fewer sources than there were offers, which Admit reads as a malformed request
// rather than silently admitting a publisher with less than it asked for.
func (d *Desk) mint(req requestJSON) (domain.ParticipantID, []domain.Source, error) {
	name, err := d.names.New()
	if err != nil {
		return domain.ParticipantID{}, nil, fmt.Errorf("%w: %s", ErrNames, err)
	}
	participant, err := domain.NewParticipantID(name)
	if err != nil {
		return domain.ParticipantID{}, nil, fmt.Errorf("%w: %s", ErrNames, err)
	}
	if !req.Publishing {
		return participant, nil, nil
	}

	sources := make([]domain.Source, 0, len(req.Offers))
	for _, o := range req.Offers {
		kind, ok := kindOf(o.Kind)
		if !ok {
			return participant, sources, nil
		}
		name, err := d.names.New()
		if err != nil {
			return domain.ParticipantID{}, nil, fmt.Errorf("%w: %s", ErrNames, err)
		}
		id, err := domain.NewSourceID(name)
		if err != nil {
			return domain.ParticipantID{}, nil, fmt.Errorf("%w: %s", ErrNames, err)
		}
		s, err := domain.NewSource(id, participant, kind, o.Layers)
		if err != nil {
			return participant, sources, nil
		}
		sources = append(sources, s)
	}
	return participant, sources, nil
}

// readRequest refuses everything about a message that is not an admission
// request. The payload is decoded strictly, so a member this exchange does not
// have is refused rather than dropped, which is the rule internal/wire applies to
// the envelope and this applies to what the envelope carried.
func readRequest(m wire.Message) (requestJSON, bool) {
	if m.Type != TypeRequest || len(m.Payload) == 0 {
		return requestJSON{}, false
	}
	var req requestJSON
	dec := json.NewDecoder(bytes.NewReader(m.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return requestJSON{}, false
	}
	if dec.More() {
		return requestJSON{}, false
	}
	if req.Conference == "" || req.Credential == "" {
		return requestJSON{}, false
	}
	if !req.Publishing && len(req.Offers) > 0 {
		// Sources offered by somebody who says they are not publishing. It is
		// refused rather than ignored, because the two readings of that message
		// differ in what the unit is told, and a client is entitled to know
		// which of them it got.
		return requestJSON{}, false
	}
	if req.Publishing && len(req.Offers) == 0 {
		return requestJSON{}, false
	}
	return req, true
}

// placementReason maps the three refusals docs/decisions/placement-seam.md names
// onto what a client is told. They stay three answers because what a person does
// about a full room, about a deployment with no units, and about a conference at
// its ceiling are three different things.
func placementReason(r placement.Reason) Reason {
	switch r {
	case placement.NoUnits():
		return NoCapacity()
	case placement.ReachedUnitCeiling():
		return ConferenceAtItsUnitCeiling()
	case placement.NoEligibleUnit():
		return RoomFull()
	}
	// The seam names three and the placer answers with one of them. A fourth
	// would be a change to that document, and until it is made this is the
	// honest answer rather than a guess at what the new one means.
	return RoomFull()
}

// unitReason maps the port's errors onto what a client is told. Each is either
// something the client can act on or something only the deployment can.
func unitReason(err error) Reason {
	switch {
	case errors.Is(err, mediaport.ErrRefused):
		return UnitRefused()
	case errors.Is(err, mediaport.ErrUnavailable), errors.Is(err, mediaport.ErrLost):
		return UnitUnavailable()
	case errors.Is(err, mediaport.ErrUnknown), errors.Is(err, mediaport.ErrConflict):
		// The conference is not on the unit the placer chose, or the identifier
		// this desk minted is in use there. Both are the deployment disagreeing
		// with itself rather than anything the client did, and both are answered
		// the way a unit that did not answer is, because the client's move is
		// identical and naming the difference would describe this deployment's
		// internals to a stranger.
		return UnitUnavailable()
	case errors.Is(err, mediaport.ErrInvalid):
		// The unit will fail the same call the same way, and what is wrong with
		// it came out of the client's own offer.
		return MalformedRequest()
	}
	return UnitUnavailable()
}

func identityOf(c roomcred.Claims) domain.IdentityID {
	id, err := domain.NewIdentityID(c.Subject)
	if err != nil {
		// A credential minted into a link that was sent to a group carries no
		// subject, which docs/decisions/accounts-and-room-credentials.md allows
		// and says the cost of. The zero identity is that absence rather than a
		// failure.
		return domain.IdentityID{}
	}
	return id
}

func kindOf(s string) (domain.Kind, bool) {
	switch s {
	case domain.Audio().String():
		return domain.Audio(), true
	case domain.Video().String():
		return domain.Video(), true
	}
	return domain.Kind{}, false
}
