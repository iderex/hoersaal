// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package mediaport

import (
	"errors"
	"fmt"

	"github.com/iderex/hoersaal/internal/domain"
)

// The six errors, from the "## The errors" section of
// docs/decisions/media-plane-port.md. No operation invents a seventh, and the
// sentence beside each is that document's own.
//
// The difference between ErrUnavailable and ErrLost is the one worth keeping in
// view: not knowing against knowing the bad answer. A caller that treats them
// alike either tears down live conferences on a slow network or leaves dead ones
// standing.
var (
	// ErrInvalid is arguments that are wrong, so the same call fails the same
	// way.
	ErrInvalid = errors.New("the arguments are wrong and the same call will fail the same way")

	// ErrUnknown is a thing named that is not on this unit.
	ErrUnknown = errors.New("not on this unit")

	// ErrConflict is an identifier in use by something that is not what the
	// caller described.
	ErrConflict = errors.New("identifier in use by something else")

	// ErrRefused is a unit that could do this and will not, because of what it
	// is already holding.
	ErrRefused = errors.New("the unit will not, because of what it already holds")

	// ErrUnavailable is a unit that did not answer, so the caller does not know
	// whether the operation happened.
	ErrUnavailable = errors.New("the unit did not answer")

	// ErrLost is a unit that answered and reported that state the caller
	// believed in is gone, which is the answer that follows a unit restart.
	ErrLost = errors.New("state the caller believed in is gone")
)

// The refusals of a value that could not have come from a control plane. They
// are separate from the six above, which are what a unit answers.
var (
	// ErrEmpty is an identifier or a codec set with nothing in it.
	ErrEmpty = errors.New("empty identifier")

	// ErrNoLayer is a layer arrangement of fewer than one layer, which is a
	// conference nobody can receive anything in.
	ErrNoLayer = errors.New("a profile carries at least one layer")

	// ErrNotAFault is a fault notice that names nothing, or that names something
	// other than what its kind says it names.
	ErrNotAFault = errors.New("a fault names the thing that was lost")
)

// A Profile is the media profile of a conference: the codecs it uses and the
// layer arrangement its video sources are sent in. It is the second argument to
// OpenConference.
//
// The codec set is a name and this package does not read it. Which names exist,
// in what order they are preferred, and what each one costs is issue #48 and is
// not decided, so a type here that enumerated them would decide it by accident.
// What the port does with the name is compare it: a unit answers ErrInvalid for
// a profile it cannot serve and ErrConflict for an identifier already held by a
// conference with a different profile, and both are comparisons rather than
// readings.
//
// The layer arrangement is read, because AdmitPublisher answers ErrInvalid for a
// source whose arrangement is outside the conference profile and that comparison
// cannot be made against an opaque value. Layers is the highest number of layers
// a video source in this conference may offer. An audio source offers one, which
// is internal/domain's sentence about a Source rather than a rule invented here.
type Profile struct {
	codecs string
	layers int
}

// NewProfile refuses an empty codec set and an arrangement of fewer than one
// layer.
func NewProfile(codecs string, layers int) (Profile, error) {
	switch {
	case codecs == "":
		return Profile{}, fmt.Errorf("profile codecs: %w", ErrEmpty)
	case layers < 1:
		return Profile{}, fmt.Errorf("profile with %d layers: %w", layers, ErrNoLayer)
	}
	return Profile{codecs: codecs, layers: layers}, nil
}

// Codecs is the name of the codec set, which nothing in this package reads.
func (p Profile) Codecs() string { return p.codecs }

// Layers is the highest number of layers a video source in this conference may
// offer.
func (p Profile) Layers() int { return p.layers }

// Transport is what the two admissions answer with: the parameters the
// participant needs in order to connect.
//
// It is opaque to the control plane, which carries it to the client and does not
// read it. That opacity is a promise of the port rather than an accident of this
// type, and it is what makes a fake possible at all: nothing above the port can
// tell a real unit's parameters from anything else, so a bookkeeper may answer
// whatever it likes.
//
// Bytes copies, and NewTransport copies, so a value handed across the port
// cannot be edited afterwards by whoever handed it over.
type Transport struct{ opaque []byte }

// NewTransport takes a copy of b.
func NewTransport(b []byte) Transport {
	c := make([]byte, len(b))
	copy(c, b)
	return Transport{opaque: c}
}

// Bytes is the parameters, to be carried and not read.
func (t Transport) Bytes() []byte {
	c := make([]byte, len(t.opaque))
	copy(c, t.opaque)
	return c
}

// A Reference is what one unit produces and another consumes when a conference
// is linked across the two, passed through by the control plane without reading.
// It has the same opacity as Transport and for the same reason.
type Reference struct{ opaque []byte }

// NewReference takes a copy of b.
func NewReference(b []byte) Reference {
	c := make([]byte, len(b))
	copy(c, b)
	return Reference{opaque: c}
}

// Bytes is the reference, to be carried and not read.
func (r Reference) Bytes() []byte {
	c := make([]byte, len(r.opaque))
	copy(c, r.opaque)
	return c
}

// A LinkID names one path this unit carries a conference over. It is chosen by
// the unit and answered by LinkConference, unlike every other identifier in this
// port, which the control plane chooses.
type LinkID struct{ v string }

// NewLinkID refuses an empty identifier.
func NewLinkID(v string) (LinkID, error) {
	if v == "" {
		return LinkID{}, fmt.Errorf("link: %w", ErrEmpty)
	}
	return LinkID{v}, nil
}

func (l LinkID) String() string { return l.v }

// A FaultKind is what a notice on the fault stream says was lost. The set is
// closed at the four docs/decisions/media-plane-port.md names.
type FaultKind struct{ v string }

// UnitGone is the whole unit.
func UnitGone() FaultKind { return FaultKind{"unit"} }

// ConferenceGone is one conference on it.
func ConferenceGone() FaultKind { return FaultKind{"conference"} }

// ParticipantGone is one participant in one conference.
func ParticipantGone() FaultKind { return FaultKind{"participant"} }

// LinkGone is one link carrying one conference.
func LinkGone() FaultKind { return FaultKind{"link"} }

func (k FaultKind) String() string { return k.v }

// A Fault is one notice: what was lost, with the identifier of the thing named.
//
// A notice means the thing is gone and is not coming back on its own. Silence
// promises nothing at all, so a control plane that waits for a notice to decide
// a unit has died waits forever on the one case that matters. Delivery is at
// least once, which costs the caller nothing, because every reaction to a death
// is the removal of something.
type Fault struct {
	kind        FaultKind
	conference  domain.ConferenceID
	participant domain.ParticipantID
	link        LinkID
}

// UnitFault is the notice that the whole unit is gone. It names nothing else,
// because there is nothing else left to name.
func UnitFault() Fault { return Fault{kind: UnitGone()} }

// ConferenceFault refuses an empty conference identifier.
func ConferenceFault(c domain.ConferenceID) (Fault, error) {
	if c.String() == "" {
		return Fault{}, fmt.Errorf("conference fault: %w", ErrNotAFault)
	}
	return Fault{kind: ConferenceGone(), conference: c}, nil
}

// ParticipantFault names the conference as well, because a participant
// identifier is only unique inside one.
func ParticipantFault(c domain.ConferenceID, p domain.ParticipantID) (Fault, error) {
	if c.String() == "" || p.String() == "" {
		return Fault{}, fmt.Errorf("participant fault: %w", ErrNotAFault)
	}
	return Fault{kind: ParticipantGone(), conference: c, participant: p}, nil
}

// LinkFault names the conference the link carried.
func LinkFault(c domain.ConferenceID, l LinkID) (Fault, error) {
	if c.String() == "" || l.v == "" {
		return Fault{}, fmt.Errorf("link fault: %w", ErrNotAFault)
	}
	return Fault{kind: LinkGone(), conference: c, link: l}, nil
}

// Kind is what was lost.
func (f Fault) Kind() FaultKind { return f.kind }

// Conference is the conference the loss is in. It is empty on a unit fault.
func (f Fault) Conference() domain.ConferenceID { return f.conference }

// Participant is who was lost. It is empty on every kind but a participant
// fault.
func (f Fault) Participant() domain.ParticipantID { return f.participant }

// Link is the path that was lost. It is empty on every kind but a link fault.
func (f Fault) Link() LinkID { return f.link }

func (f Fault) String() string {
	switch f.kind {
	case ConferenceGone():
		return fmt.Sprintf("conference %s is gone", f.conference)
	case ParticipantGone():
		return fmt.Sprintf("participant %s of conference %s is gone", f.participant, f.conference)
	case LinkGone():
		return fmt.Sprintf("link %s of conference %s is gone", f.link, f.conference)
	default:
		return "the unit is gone"
	}
}

// Faults is the stream ReportFaults answers with.
//
// Notices is closed when the stream ends, and Err is why: ErrUnavailable where
// it broke, and nil where the caller stopped it. A broken stream is not itself a
// fault notice about the unit, because a broken stream and a dead unit are
// different things and collapsing them retires healthy units.
type Faults interface {
	// Notices delivers one value per thing lost, and is closed when the stream
	// ends.
	Notices() <-chan Fault

	// Err is why Notices closed, and is nil while it is open or where Stop
	// closed it.
	Err() error

	// Stop ends the stream from the caller's side. It is safe to call twice.
	Stop()
}

// A Unit is the port: the whole of what the control plane may ask of a
// forwarding unit. It is the transcription of the eight operations in
// docs/decisions/media-plane-port.md, and that document rather than this
// interface is where each one is argued.
//
// Every method is synchronous with the media plane. When one returns, the state
// is on the unit and not only in a queue, which is what lets a caller act on the
// answer rather than on a hope.
//
// There is no context argument, and that is a decision rather than an omission.
// The port document specifies no cancellation and gives every operation
// ErrUnavailable for a unit that did not answer, so a deadline is the adapter's
// business on issue #43 and belongs where the transport is, not in an interface
// a bookkeeper also satisfies. Changing that is a change to the document first.
type Unit interface {
	// OpenConference makes the unit hold a conference under this identifier with
	// this profile. Opening one that already exists with the same profile
	// succeeds and changes nothing, so a control plane that lost the answer may
	// ask again.
	//
	// Errors: ErrInvalid where the profile is not one this unit can serve,
	// ErrConflict where the identifier is held by a conference with a different
	// profile, ErrRefused where the unit will not take another conference,
	// ErrUnavailable.
	OpenConference(conference domain.ConferenceID, profile Profile) error

	// CloseConference releases the conference, every participant in it and every
	// link that carried it on this unit's side. It does not promise the
	// participants have been told, because telling them is the control plane's
	// work over its own transport.
	//
	// Closing a conference that is not there succeeds, because the state the
	// caller wanted is the state that exists.
	//
	// Errors: ErrUnavailable.
	CloseConference(conference domain.ConferenceID) error

	// AdmitPublisher makes the unit ready to receive a connection for this
	// participant and to accept the sources named. It does not promise the
	// participant has connected, that any packet has arrived, or that any
	// subscriber can see them.
	//
	// Errors: ErrUnknown where the conference is not on this unit, ErrConflict
	// where the participant identifier is in use, ErrRefused where the unit will
	// not admit another publisher, ErrInvalid where a source names a kind or an
	// arrangement outside the conference profile, ErrUnavailable.
	AdmitPublisher(conference domain.ConferenceID, participant domain.ParticipantID, sources []domain.Source) (Transport, error)

	// AdmitSubscriber makes the unit ready to receive a connection for this
	// participant. The participant receives nothing until SetReception says
	// what.
	//
	// The two admissions are separate because they fail for different reasons
	// and cost different things, and because a room where almost everybody only
	// receives is the room this project is built for. A participant that both
	// publishes and receives holds both.
	//
	// Errors: as for AdmitPublisher, without the source errors.
	AdmitSubscriber(conference domain.ConferenceID, participant domain.ParticipantID) (Transport, error)

	// SetReception replaces what this subscriber receives with the set given,
	// and answers with the set the unit accepted, which may be smaller. A source
	// absent from the set is not received.
	//
	// The whole set is given every time. There is no add and no remove, because
	// two operations that must be applied in order across a lossy path produce a
	// state nobody can reconstruct, and reconstructing it is exactly what the
	// control plane does after a reconnection.
	//
	// The accepted set is what the unit will try to send until the next call for
	// this subscriber. It is not a promise that the subscriber receives it,
	// because what arrives depends on the path and on the publisher.
	//
	// Errors: ErrUnknown for an absent conference or participant, ErrInvalid for
	// an entry naming a source that is not in the conference or a layer outside
	// the profile, ErrUnavailable. ErrRefused is not among them: a unit that
	// cannot serve the set it was given answers with the set it accepted, so the
	// failure is a smaller picture rather than an error the caller has to invent
	// a picture for.
	SetReception(conference domain.ConferenceID, subscriber domain.ParticipantID, entries []domain.Subscription) ([]domain.Subscription, error)

	// LinkConference makes this unit accept and send the conference's media over
	// a path to the unit the reference names. It does not promise the other side
	// is ready, so the control plane calls it on both units and treats the
	// conference as spanning only once both have answered.
	//
	// Closing a link is not an operation. A link ends when the conference ends
	// on either side, because an operation that could remove a link under a live
	// conference is an operation that can split a room in half.
	//
	// Errors: ErrUnknown where the conference is not here, ErrRefused where the
	// unit will not carry another link, ErrInvalid where the reference is not
	// one this unit can use, ErrUnavailable.
	LinkConference(conference domain.ConferenceID, reference Reference) (LinkID, error)

	// ReportCapacity is the load, one number and nothing else, defined in
	// docs/decisions/capacity-signal.md. Zero is a unit holding nothing, one is
	// a unit at the capacity calibrated for it, and values above one are
	// reported rather than clipped.
	//
	// It says nothing about when it was computed. The pool holds the time the
	// answer arrived and decides for itself when an answer is too old to use.
	//
	// Errors: ErrUnavailable. There is no error for a unit that cannot compute
	// its load, because a unit that cannot say what it is holding is a unit that
	// should not be holding anything, and it reports that through ReportFaults.
	ReportCapacity() (float64, error)

	// ReportFaults opens the stream of notices. It is an optimisation that
	// shortens the delay rather than the mechanism that detects death: a unit
	// that has died sends no notice, so the pool decides liveness by asking.
	//
	// Errors: ErrUnavailable, where the stream could not be established.
	ReportFaults() (Faults, error)
}
