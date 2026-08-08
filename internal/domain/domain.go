// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package domain is the vocabulary the rest of the control plane agrees about: a
// conference, the participants in it, the role each of them holds, the sources
// they publish, and the subscriptions that decide what each of them receives.
//
// It answers issue #29. It holds no transport type, no storage type and no media
// plane type, and imports_test.go refuses an import of anything outside the
// standard library, so that boundary is a property this package proves about
// itself rather than a sentence about it. The general form of the same assertion
// over the whole tree is issue #98.
//
// Two of the words carry weight the ordinary version does not.
//
// A participant is not a person. The same person joining from a phone and a
// laptop is two participants and one identity, so a Participant carries the
// identity it belongs to, the two identifiers are different types, and
// Conference reports the two counts separately. The capacity model counts
// participants while the permission model answers about identities, and a model
// with one identifier for both would leave that distinction in a comment.
//
// A subscription is a value this package holds and not a consequence of being in
// the room. In a room of three hundred nobody receives everything, so what exists
// and what somebody receives are different sets, and the second one is written
// down rather than derived.
//
// One name here disagrees with a landed decision, and the disagreement is stated
// rather than settled quietly. What issue #29 calls a track,
// docs/decisions/media-plane-port.md calls a source, and both mean one stream of
// one kind offered by one publishing participant. The port is a landed decision
// and #29 is the issue this package answers, so the landed name is the one in the
// code, which is how docs/design/scaling-loop.md resolved the same kind of
// collision over the name of a unit state. The word track appears nowhere in the
// code.
//
// What is deliberately not here. What an identity holds, and how a session or a
// room credential is made, is issue #33. What powers a role bundles is issue #34,
// so a Role here is a name and nothing more. Joining, leaving and rejoining as a
// sequence is issue #36, so a conference here takes admissions and has no
// departure. Whether any of this is written down anywhere is issue #30.
package domain

import (
	"errors"
	"fmt"
	"sort"
)

// The refusals. Every constructor and every method that changes a conference
// returns one of these wrapped with what it was given, so the kind of refusal
// tells the caller the kind of mistake without reading this file.
var (
	// ErrEmpty is an identifier or a name with nothing in it.
	ErrEmpty = errors.New("empty identifier")

	// ErrUnknown is a reference to something this conference does not hold.
	ErrUnknown = errors.New("not in this conference")

	// ErrDuplicate is a second thing under an identifier already held.
	ErrDuplicate = errors.New("identifier already held")

	// ErrElsewhere is a participant made for one conference offered to another.
	ErrElsewhere = errors.New("belongs to another conference")

	// ErrOwnSource is a subscription from a participant to a source it publishes
	// itself.
	ErrOwnSource = errors.New("a publisher does not subscribe to its own source")

	// ErrNoLayer is a layer the source does not offer.
	ErrNoLayer = errors.New("layer outside the arrangement the source offers")

	// ErrNotAKind is a source that is neither audio nor video.
	ErrNotAKind = errors.New("a source is audio or video")
)

// The identifiers. Each is its own type so that a participant identifier cannot
// be passed where a conference identifier is wanted, which is the mistake no
// amount of care catches in a codebase that spells all four as a string.
//
// The invariant is that nothing accepts an empty one. Go has no way to forbid a
// zero value, so IdentityID{} can be written and this package cannot stop it;
// what it does instead is refuse it everywhere it could enter, in the
// constructors below and in every method of Conference. That is a residual of the
// language rather than a property of the model, and it is stated here rather than
// left for a reader to discover.
type (
	// An IdentityID names who somebody is, across every conference and every
	// device.
	IdentityID struct{ v string }

	// A ParticipantID names one endpoint of one conference.
	ParticipantID struct{ v string }

	// A ConferenceID names one conference.
	ConferenceID struct{ v string }

	// A SourceID names one stream published by one participant.
	SourceID struct{ v string }
)

// NewIdentityID refuses an empty identifier.
func NewIdentityID(v string) (IdentityID, error) {
	if v == "" {
		return IdentityID{}, fmt.Errorf("identity: %w", ErrEmpty)
	}
	return IdentityID{v}, nil
}

// NewParticipantID refuses an empty identifier.
func NewParticipantID(v string) (ParticipantID, error) {
	if v == "" {
		return ParticipantID{}, fmt.Errorf("participant: %w", ErrEmpty)
	}
	return ParticipantID{v}, nil
}

// NewConferenceID refuses an empty identifier.
func NewConferenceID(v string) (ConferenceID, error) {
	if v == "" {
		return ConferenceID{}, fmt.Errorf("conference: %w", ErrEmpty)
	}
	return ConferenceID{v}, nil
}

// NewSourceID refuses an empty identifier.
func NewSourceID(v string) (SourceID, error) {
	if v == "" {
		return SourceID{}, fmt.Errorf("source: %w", ErrEmpty)
	}
	return SourceID{v}, nil
}

func (i IdentityID) String() string    { return i.v }
func (p ParticipantID) String() string { return p.v }
func (c ConferenceID) String() string  { return c.v }
func (s SourceID) String() string      { return s.v }

// A Kind is what a source carries. The set is closed at two: a source that is
// neither is refused when it is made rather than stored and interpreted later by
// whoever reads it.
type Kind struct{ v string }

// Audio and Video are the two kinds. They are functions rather than variables so
// that nothing can reassign the set.
func Audio() Kind { return Kind{"audio"} }

// Video is the other one.
func Video() Kind { return Kind{"video"} }

func (k Kind) String() string { return k.v }

func (k Kind) known() bool { return k == Audio() || k == Video() }

// A Role is the name of the bundle of powers a participant holds in one
// conference. This package holds the name and nothing else: what a role permits
// is issue #34, and a Role that carried permissions here would decide that issue
// by accident.
type Role struct{ name string }

// NewRole refuses an empty name.
func NewRole(name string) (Role, error) {
	if name == "" {
		return Role{}, fmt.Errorf("role: %w", ErrEmpty)
	}
	return Role{name}, nil
}

func (r Role) String() string { return r.name }

// A Participant is one endpoint of one conference. It is not a person, and
// Conference.IdentityCount is where that shows: two participants carrying one
// IdentityID are one person on two devices.
type Participant struct {
	id         ParticipantID
	identity   IdentityID
	conference ConferenceID
	role       Role
}

// NewParticipant refuses an empty value in any of the four, so a Participant
// that exists is one every field of which was given.
func NewParticipant(id ParticipantID, identity IdentityID, conference ConferenceID, role Role) (Participant, error) {
	switch {
	case id.v == "":
		return Participant{}, fmt.Errorf("participant identifier: %w", ErrEmpty)
	case identity.v == "":
		return Participant{}, fmt.Errorf("participant identity: %w", ErrEmpty)
	case conference.v == "":
		return Participant{}, fmt.Errorf("participant conference: %w", ErrEmpty)
	case role.name == "":
		return Participant{}, fmt.Errorf("participant role: %w", ErrEmpty)
	}
	return Participant{id: id, identity: identity, conference: conference, role: role}, nil
}

// ID is which endpoint this is.
func (p Participant) ID() ParticipantID { return p.id }

// Identity is who this endpoint belongs to. Two participants may return the same
// one.
func (p Participant) Identity() IdentityID { return p.identity }

// Conference is the conference this participant was made for, and the only one
// that will admit it.
func (p Participant) Conference() ConferenceID { return p.conference }

// Role is the name of what this participant may do, resolved elsewhere.
func (p Participant) Role() Role { return p.role }

// A Source is one stream of one kind offered by one publishing participant.
// Layers is how many encodings of it the publisher sends, in rising order of
// cost, so layer zero is the cheapest and Layers-1 is the highest. Audio has one.
type Source struct {
	id        SourceID
	publisher ParticipantID
	kind      Kind
	layers    int
}

// NewSource refuses an empty identifier, an empty publisher, a kind that is
// neither audio nor video, and an arrangement of fewer than one layer. A source
// nobody can receive at any layer is not a source.
func NewSource(id SourceID, publisher ParticipantID, kind Kind, layers int) (Source, error) {
	switch {
	case id.v == "":
		return Source{}, fmt.Errorf("source identifier: %w", ErrEmpty)
	case publisher.v == "":
		return Source{}, fmt.Errorf("source publisher: %w", ErrEmpty)
	case !kind.known():
		return Source{}, fmt.Errorf("source kind %q: %w", kind.v, ErrNotAKind)
	case layers < 1:
		return Source{}, fmt.Errorf("source with %d layers: %w", layers, ErrNoLayer)
	}
	return Source{id: id, publisher: publisher, kind: kind, layers: layers}, nil
}

// ID is which stream this is.
func (s Source) ID() SourceID { return s.id }

// Publisher is the participant offering it.
func (s Source) Publisher() ParticipantID { return s.publisher }

// Kind is what it carries.
func (s Source) Kind() Kind { return s.kind }

// Layers is how many encodings the publisher sends.
func (s Source) Layers() int { return s.layers }

// A Subscription is one participant receiving one source at up to one layer.
// Nothing about being in a conference creates one.
type Subscription struct {
	subscriber ParticipantID
	source     SourceID
	layer      int
}

// NewSubscription takes the source itself rather than its identifier, because two
// of the three invariants are about the source: a participant does not subscribe
// to a source it publishes, and the layer has to be one the source offers.
func NewSubscription(subscriber ParticipantID, source Source, layer int) (Subscription, error) {
	switch {
	case subscriber.v == "":
		return Subscription{}, fmt.Errorf("subscriber: %w", ErrEmpty)
	case source.id.v == "":
		return Subscription{}, fmt.Errorf("subscribed source: %w", ErrEmpty)
	case subscriber == source.publisher:
		return Subscription{}, fmt.Errorf("%s and source %s: %w", subscriber.v, source.id.v, ErrOwnSource)
	case layer < 0 || layer >= source.layers:
		return Subscription{}, fmt.Errorf("layer %d of source %s, which offers %d: %w", layer, source.id.v, source.layers, ErrNoLayer)
	}
	return Subscription{subscriber: subscriber, source: source.id, layer: layer}, nil
}

// Subscriber is who receives.
func (s Subscription) Subscriber() ParticipantID { return s.subscriber }

// Source is what they receive.
func (s Subscription) Source() SourceID { return s.source }

// Layer is the highest encoding of it they are to be sent.
func (s Subscription) Layer() int { return s.layer }

// A Conference is the set of participants whose media may reach each other, what
// they publish, and what each of them receives. It is the aggregate: a
// participant, a source and a subscription all enter through it, and each entry
// is refused unless what it refers to is already held.
//
// It carries no unit and no placement. Which unit a conference is on is the
// pool's and the control plane's, in issues #56 and #57, and a field for it here
// would make this package depend on a decision it is not part of.
type Conference struct {
	id            ConferenceID
	participants  map[ParticipantID]Participant
	sources       map[SourceID]Source
	subscriptions map[reception]Subscription
}

// A reception is the pair a subscription is unique in. A subscriber receives at
// most one layer of one source, which docs/decisions/media-plane-port.md fixes,
// so a second subscription for the same pair replaces the first rather than
// adding to it.
type reception struct {
	subscriber ParticipantID
	source     SourceID
}

// NewConference refuses an empty identifier and returns an empty conference.
func NewConference(id ConferenceID) (*Conference, error) {
	if id.v == "" {
		return nil, fmt.Errorf("conference identifier: %w", ErrEmpty)
	}
	return &Conference{
		id:            id,
		participants:  map[ParticipantID]Participant{},
		sources:       map[SourceID]Source{},
		subscriptions: map[reception]Subscription{},
	}, nil
}

// ID is which conference this is.
func (c *Conference) ID() ConferenceID { return c.id }

// Admit puts a participant in the conference. It refuses a participant made for
// a different conference, because a participant identifier is only unique inside
// one, and it refuses an identifier already held rather than replacing what is
// there.
func (c *Conference) Admit(p Participant) error {
	switch {
	case p.id.v == "":
		return fmt.Errorf("admitting a participant: %w", ErrEmpty)
	case p.conference != c.id:
		return fmt.Errorf("participant %s was made for conference %s: %w", p.id.v, p.conference.v, ErrElsewhere)
	}
	if _, held := c.participants[p.id]; held {
		return fmt.Errorf("participant %s: %w", p.id.v, ErrDuplicate)
	}
	c.participants[p.id] = p
	return nil
}

// Publish records a source. It refuses a source whose publisher has not been
// admitted, so a conference cannot hold a stream from somebody who is not in it.
func (c *Conference) Publish(s Source) error {
	if s.id.v == "" {
		return fmt.Errorf("publishing a source: %w", ErrEmpty)
	}
	if _, held := c.participants[s.publisher]; !held {
		return fmt.Errorf("publisher %s: %w", s.publisher.v, ErrUnknown)
	}
	if _, held := c.sources[s.id]; held {
		return fmt.Errorf("source %s: %w", s.id.v, ErrDuplicate)
	}
	c.sources[s.id] = s
	return nil
}

// Subscribe records that one participant receives one source. The subscriber and
// the source both have to be held here, and the layer is checked again against
// the source this conference holds rather than against the copy the subscription
// was made from, since the two can differ.
func (c *Conference) Subscribe(s Subscription) error {
	if s.subscriber.v == "" || s.source.v == "" {
		return fmt.Errorf("subscribing: %w", ErrEmpty)
	}
	if _, held := c.participants[s.subscriber]; !held {
		return fmt.Errorf("subscriber %s: %w", s.subscriber.v, ErrUnknown)
	}
	held, ok := c.sources[s.source]
	if !ok {
		return fmt.Errorf("source %s: %w", s.source.v, ErrUnknown)
	}
	if s.subscriber == held.publisher {
		return fmt.Errorf("%s and source %s: %w", s.subscriber.v, s.source.v, ErrOwnSource)
	}
	if s.layer < 0 || s.layer >= held.layers {
		return fmt.Errorf("layer %d of source %s, which offers %d: %w", s.layer, s.source.v, held.layers, ErrNoLayer)
	}
	c.subscriptions[reception{s.subscriber, s.source}] = s
	return nil
}

// Participant is the participant under this identifier, and whether it is held.
func (c *Conference) Participant(id ParticipantID) (Participant, bool) {
	p, held := c.participants[id]
	return p, held
}

// Source is the source under this identifier, and whether it is held.
func (c *Conference) Source(id SourceID) (Source, bool) {
	s, held := c.sources[id]
	return s, held
}

// ParticipantCount is how many endpoints are in the conference. This is the
// number the capacity model counts.
func (c *Conference) ParticipantCount() int { return len(c.participants) }

// IdentityCount is how many people those endpoints belong to. It is at most
// ParticipantCount and is smaller as soon as somebody joins twice.
func (c *Conference) IdentityCount() int {
	seen := make(map[IdentityID]struct{}, len(c.participants))
	for _, p := range c.participants {
		seen[p.identity] = struct{}{}
	}
	return len(seen)
}

// Reception is everything one participant receives, ordered by source identifier
// so that two calls on one conference return the same thing in the same order. It
// is empty for a participant who has subscribed to nothing, including one who has
// been in the conference the whole time, and that emptiness is the difference
// between what exists and what somebody receives.
//
// The set this returns is the set a caller hands to SetReception on the port. It
// is a whole set every time and never a change to one, which
// docs/decisions/media-plane-port.md fixes and this ordering exists to make
// reproducible.
func (c *Conference) Reception(subscriber ParticipantID) []Subscription {
	var out []Subscription
	for pair, s := range c.subscriptions {
		if pair.subscriber == subscriber {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].source.v < out[j].source.v })
	return out
}
