// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package mediafake

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/domain"
	"github.com/iderex/hoersaal/internal/mediaport"
)

// StreamBuffer is how many undelivered notices a fault stream holds.
//
// A stream whose reader has fallen this far behind is broken rather than
// blocked, and it is closed with mediaport.ErrUnavailable. That is the port's
// own word for a stream that broke, and it is the choice that keeps Fail from
// ever waiting on a caller: a fake that blocked because nobody was reading would
// be a deadlock a test finds at three in the morning.
const StreamBuffer = 64

// A Fabric is the set of fakes that can link conferences to each other. Every
// unit belongs to one, and a reference produced by one unit is usable only by
// another unit of the same fabric.
//
// It exists because a link is the one thing in the port that is not a property
// of a single unit. Two units that carry one conference between them are one
// bookkeeping problem, and the fabric is where that is admitted: it holds the
// one lock covering every unit in it, so a link can be read from both sides
// without two locks and the deadlock that eventually comes with them.
type Fabric struct {
	mu    sync.Mutex
	units map[string]*Unit
}

// NewFabric returns an empty fabric.
func NewFabric() *Fabric { return &Fabric{units: map[string]*Unit{}} }

// Add returns a new fake in this fabric under name, which is what its reference
// carries and is not part of the port. It refuses an empty name and a name
// already in the fabric.
//
// The clock is what a latency is measured against. A fake with no latency set
// never asks it anything, so a test that does not care about slowness may hand
// it any clock at all.
func (f *Fabric) Add(name string, c clock.Clock) (*Unit, error) {
	if name == "" {
		return nil, fmt.Errorf("unit name: %w", mediaport.ErrEmpty)
	}
	if c == nil {
		return nil, fmt.Errorf("unit %s has no clock: %w", name, mediaport.ErrEmpty)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, held := f.units[name]; held {
		return nil, fmt.Errorf("unit %s: %w", name, mediaport.ErrConflict)
	}
	u := &Unit{
		name:         name,
		fabric:       f,
		clock:        c,
		alive:        true,
		acceptAtMost: -1,
		lost:         map[domain.ConferenceID]bool{},
		refusing:     map[Refusable]bool{},
		conferences:  map[domain.ConferenceID]*conference{},
	}
	f.units[name] = u
	return u, nil
}

// A Refusable is an operation the port gives ErrRefused for. The set is closed
// at four, and that is the point of the type: SetReception answers with the set
// it accepted rather than refusing, and CloseConference, ReportCapacity and
// ReportFaults have no Refused either, so a test cannot ask this fake for an
// error the port does not have.
type Refusable struct{ v string }

// OpeningAConference is OpenConference, refused when the unit will not take
// another conference.
func OpeningAConference() Refusable { return Refusable{"OpenConference"} }

// AdmittingAPublisher is AdmitPublisher, refused when the unit will not admit
// another publisher.
func AdmittingAPublisher() Refusable { return Refusable{"AdmitPublisher"} }

// AdmittingASubscriber is AdmitSubscriber, refused for the same kind of reason.
func AdmittingASubscriber() Refusable { return Refusable{"AdmitSubscriber"} }

// CarryingALink is LinkConference, refused when the unit will not carry another
// link.
func CarryingALink() Refusable { return Refusable{"LinkConference"} }

func (r Refusable) String() string { return r.v }

// A Unit is a fake forwarding unit: a bookkeeper that answers the same questions
// a real one answers and carries no media at all.
//
// It enforces the port's own rules against its own records, so a caller that
// admits a subscriber to a conference that was never opened gets the error a
// real unit would give and a test cannot pass by doing something the real thing
// would refuse.
//
// One arm of one error is not enforceable here and is set by the test instead.
// OpenConference answers ErrInvalid where the profile is not one the unit can
// serve, and which profiles are servable at all is the codec and layer policy on
// issue #48, which is not decided. So a fake serves every profile until Serves
// says otherwise. The neighbouring arm, a source outside the profile the
// conference was opened with, is a comparison against this unit's own record and
// is enforced. That difference is written here rather than left to be found by
// somebody whose test passed for the wrong reason.
type Unit struct {
	name   string
	fabric *Fabric
	clock  clock.Clock

	alive        bool
	load         float64
	latency      time.Duration
	acceptAtMost int
	servable     map[string]bool
	lost         map[domain.ConferenceID]bool
	refusing     map[Refusable]bool
	conferences  map[domain.ConferenceID]*conference
	streams      []*stream
	links        int
}

type conference struct {
	profile     mediaport.Profile
	publishers  map[domain.ParticipantID]bool
	subscribers map[domain.ParticipantID]bool
	sources     map[domain.SourceID]domain.Source
	receptions  map[domain.ParticipantID][]domain.Subscription
	links       map[string]mediaport.LinkID
}

func newConference(p mediaport.Profile) *conference {
	return &conference{
		profile:     p,
		publishers:  map[domain.ParticipantID]bool{},
		subscribers: map[domain.ParticipantID]bool{},
		sources:     map[domain.SourceID]domain.Source{},
		receptions:  map[domain.ParticipantID][]domain.Subscription{},
		links:       map[string]mediaport.LinkID{},
	}
}

// Name is what this unit is called inside its fabric. It is not part of the
// port.
func (u *Unit) Name() string { return u.name }

// The knobs. Everything a test sets rather than leaves to chance is here, and
// nothing in this file decides any of it for itself.

// SetLoad fixes what ReportCapacity answers. It is not derived from what the
// unit holds, because the derivation is docs/decisions/capacity-signal.md over
// denominators the calibration on issue #54 has not produced, and a fake that
// invented them would be the one place a made-up capacity is least visible.
func (u *Unit) SetLoad(l float64) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	u.load = l
}

// SetLatency makes every operation wait d on this unit's clock before it
// answers. Nothing sleeps: with a clock.Test the wait ends when the test
// advances it, which is how a test covering a slow unit finishes in the same
// instant as the rest of the run. Zero, the default, asks the clock nothing.
func (u *Unit) SetLatency(d time.Duration) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	u.latency = d
}

// AcceptAtMost caps how many entries SetReception accepts. A negative number,
// the default, accepts every entry. This is the fake's way of being a unit that
// could not serve the whole set it was given, which the port answers by
// returning a smaller set rather than an error.
func (u *Unit) AcceptAtMost(n int) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	u.acceptAtMost = n
}

// Serves closes the set of profiles this unit can serve to the codec sets named.
// Until it is called every profile is servable, for the reason the type comment
// gives. Calling it with nothing makes the unit serve nothing, which is a unit
// that refuses every conference with ErrInvalid.
func (u *Unit) Serves(codecs ...string) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	u.servable = map[string]bool{}
	for _, c := range codecs {
		u.servable[c] = true
	}
}

// Refuse makes each named operation answer mediaport.ErrRefused until Allow.
func (u *Unit) Refuse(ops ...Refusable) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	for _, op := range ops {
		u.refusing[op] = true
	}
}

// Allow undoes Refuse.
func (u *Unit) Allow(ops ...Refusable) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	for _, op := range ops {
		delete(u.refusing, op)
	}
}

// Die makes every operation answer mediaport.ErrUnavailable and breaks every
// open fault stream. It does not send a fault notice about itself, because a
// unit that has died sends nothing, which is the sentence in the port that makes
// liveness something the pool asks about rather than waits for.
func (u *Unit) Die() {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	u.alive = false
	for _, s := range u.streams {
		s.breakStream(mediaport.ErrUnavailable)
	}
	u.streams = nil
}

// Restart brings the unit back having lost everything it held. The next
// operation naming a conference it held before answers mediaport.ErrLost and
// forgets it, so a control plane is told once that what it believed in is gone
// and the call after that finds a unit that never held it.
//
// The knobs survive a restart. They are the test's settings and not the unit's
// state, and a restart that reset them would make every test that restarts a
// unit set them twice.
func (u *Unit) Restart() {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	u.alive = true
	for id := range u.conferences {
		u.lost[id] = true
	}
	u.conferences = map[domain.ConferenceID]*conference{}
}

// Fail delivers one notice to every open fault stream. Delivery to a stream
// whose reader has fallen StreamBuffer behind breaks that stream rather than
// waiting for it.
func (u *Unit) Fail(f mediaport.Fault) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	keep := u.streams[:0]
	for _, s := range u.streams {
		if !s.deliver(f) {
			continue
		}
		keep = append(keep, s)
	}
	u.streams = keep
}

// Reference is what another unit in this fabric is handed to link a conference
// to this one. Its bytes are this unit's name, which is a shape the two fakes
// agree between themselves; the port promises only that nothing above it reads
// them.
func (u *Unit) Reference() mediaport.Reference {
	return mediaport.NewReference([]byte("mediafake:" + u.name))
}

// What this unit believes. Every one of these is what a test asserts against,
// and none of them is part of the port.

// Conferences is every conference this unit holds, ordered.
func (u *Unit) Conferences() []domain.ConferenceID {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	out := make([]domain.ConferenceID, 0, len(u.conferences))
	for id := range u.conferences {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// Profile is the profile a conference was opened with, and whether it is held.
func (u *Unit) Profile(c domain.ConferenceID) (mediaport.Profile, bool) {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	held, ok := u.conferences[c]
	if !ok {
		return mediaport.Profile{}, false
	}
	return held.profile, true
}

// Publishers is who has been admitted to publish, ordered.
func (u *Unit) Publishers(c domain.ConferenceID) []domain.ParticipantID {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	held, ok := u.conferences[c]
	if !ok {
		return nil
	}
	return sortedParticipants(held.publishers)
}

// Subscribers is who has been admitted to receive, ordered.
func (u *Unit) Subscribers(c domain.ConferenceID) []domain.ParticipantID {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	held, ok := u.conferences[c]
	if !ok {
		return nil
	}
	return sortedParticipants(held.subscribers)
}

// Sources is every source this unit can forward in a conference, which is what
// its own publishers offer together with what reaches it over a link both units
// have made. That union is what makes a relayed conference observable in both
// fakes with no real unit anywhere.
func (u *Unit) Sources(c domain.ConferenceID) []domain.Source {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	return u.reachable(c)
}

// Links is every path this unit carries a conference over, ordered.
func (u *Unit) Links(c domain.ConferenceID) []mediaport.LinkID {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	held, ok := u.conferences[c]
	if !ok {
		return nil
	}
	out := make([]mediaport.LinkID, 0, len(held.links))
	for _, l := range held.links {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// Reception is what this unit will try to send one subscriber, which is the last
// set it accepted from SetReception.
func (u *Unit) Reception(c domain.ConferenceID, p domain.ParticipantID) []domain.Subscription {
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	held, ok := u.conferences[c]
	if !ok {
		return nil
	}
	out := make([]domain.Subscription, len(held.receptions[p]))
	copy(out, held.receptions[p])
	return out
}

// The port. Everything below this line is mediaport.Unit and nothing else.

var _ mediaport.Unit = (*Unit)(nil)

// OpenConference is the port's OpenConference.
func (u *Unit) OpenConference(c domain.ConferenceID, p mediaport.Profile) error {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()

	if err := u.reachableState(c); err != nil {
		return err
	}
	if p.Codecs() == "" {
		return fmt.Errorf("opening %s with no profile: %w", c, mediaport.ErrInvalid)
	}
	if u.servable != nil && !u.servable[p.Codecs()] {
		return fmt.Errorf("opening %s with profile %q this unit does not serve: %w", c, p.Codecs(), mediaport.ErrInvalid)
	}
	if held, ok := u.conferences[c]; ok {
		if held.profile != p {
			return fmt.Errorf("conference %s is held with profile %q: %w", c, held.profile.Codecs(), mediaport.ErrConflict)
		}
		return nil
	}
	if u.refusing[OpeningAConference()] {
		return fmt.Errorf("opening %s: %w", c, mediaport.ErrRefused)
	}
	u.conferences[c] = newConference(p)
	return nil
}

// CloseConference is the port's CloseConference. Closing one that is not there
// succeeds, including one lost to a restart, because the state the caller wanted
// is the state that exists.
func (u *Unit) CloseConference(c domain.ConferenceID) error {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()

	if !u.alive {
		return fmt.Errorf("closing %s: %w", c, mediaport.ErrUnavailable)
	}
	delete(u.lost, c)
	delete(u.conferences, c)
	for _, other := range u.fabric.units {
		if other == u {
			continue
		}
		if held, ok := other.conferences[c]; ok {
			delete(held.links, u.name)
		}
	}
	return nil
}

// AdmitPublisher is the port's AdmitPublisher.
func (u *Unit) AdmitPublisher(c domain.ConferenceID, p domain.ParticipantID, sources []domain.Source) (mediaport.Transport, error) {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()

	held, err := u.conferenceFor(c, "admitting a publisher to")
	if err != nil {
		return mediaport.Transport{}, err
	}
	if p.String() == "" {
		return mediaport.Transport{}, fmt.Errorf("admitting a publisher with no identifier to %s: %w", c, mediaport.ErrInvalid)
	}
	if held.publishers[p] {
		return mediaport.Transport{}, fmt.Errorf("participant %s already publishes in %s: %w", p, c, mediaport.ErrConflict)
	}
	for _, s := range sources {
		if s.Publisher() != p {
			return mediaport.Transport{}, fmt.Errorf("source %s is published by %s and not by %s: %w", s.ID(), s.Publisher(), p, mediaport.ErrInvalid)
		}
		if err := outsideProfile(held.profile, s); err != nil {
			return mediaport.Transport{}, err
		}
		if _, taken := held.sources[s.ID()]; taken {
			return mediaport.Transport{}, fmt.Errorf("source %s in %s: %w", s.ID(), c, mediaport.ErrConflict)
		}
	}
	if u.refusing[AdmittingAPublisher()] {
		return mediaport.Transport{}, fmt.Errorf("admitting publisher %s to %s: %w", p, c, mediaport.ErrRefused)
	}

	held.publishers[p] = true
	for _, s := range sources {
		held.sources[s.ID()] = s
	}
	return u.transportFor(c, p), nil
}

// AdmitSubscriber is the port's AdmitSubscriber.
func (u *Unit) AdmitSubscriber(c domain.ConferenceID, p domain.ParticipantID) (mediaport.Transport, error) {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()

	held, err := u.conferenceFor(c, "admitting a subscriber to")
	if err != nil {
		return mediaport.Transport{}, err
	}
	if p.String() == "" {
		return mediaport.Transport{}, fmt.Errorf("admitting a subscriber with no identifier to %s: %w", c, mediaport.ErrInvalid)
	}
	if held.subscribers[p] {
		return mediaport.Transport{}, fmt.Errorf("participant %s already receives in %s: %w", p, c, mediaport.ErrConflict)
	}
	if u.refusing[AdmittingASubscriber()] {
		return mediaport.Transport{}, fmt.Errorf("admitting subscriber %s to %s: %w", p, c, mediaport.ErrRefused)
	}

	held.subscribers[p] = true
	return u.transportFor(c, p), nil
}

// SetReception is the port's SetReception.
func (u *Unit) SetReception(c domain.ConferenceID, p domain.ParticipantID, entries []domain.Subscription) ([]domain.Subscription, error) {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()

	held, err := u.conferenceFor(c, "setting the reception of")
	if err != nil {
		return nil, err
	}
	if !held.subscribers[p] {
		return nil, fmt.Errorf("participant %s does not receive in %s: %w", p, c, mediaport.ErrUnknown)
	}

	reachable := map[domain.SourceID]domain.Source{}
	for _, s := range u.reachable(c) {
		reachable[s.ID()] = s
	}

	named := map[domain.SourceID]bool{}
	for _, e := range entries {
		switch {
		case e.Subscriber() != p:
			return nil, fmt.Errorf("entry for subscriber %s in a reception set for %s: %w", e.Subscriber(), p, mediaport.ErrInvalid)
		case named[e.Source()]:
			return nil, fmt.Errorf("source %s named twice in one reception set: %w", e.Source(), mediaport.ErrInvalid)
		}
		named[e.Source()] = true
		s, ok := reachable[e.Source()]
		if !ok {
			return nil, fmt.Errorf("source %s is not in conference %s: %w", e.Source(), c, mediaport.ErrInvalid)
		}
		if e.Layer() < 0 || e.Layer() >= s.Layers() {
			return nil, fmt.Errorf("layer %d of source %s, which offers %d: %w", e.Layer(), e.Source(), s.Layers(), mediaport.ErrInvalid)
		}
	}

	accepted := make([]domain.Subscription, len(entries))
	copy(accepted, entries)
	sort.Slice(accepted, func(i, j int) bool { return accepted[i].Source().String() < accepted[j].Source().String() })
	if u.acceptAtMost >= 0 && len(accepted) > u.acceptAtMost {
		accepted = accepted[:u.acceptAtMost]
	}
	held.receptions[p] = accepted

	out := make([]domain.Subscription, len(accepted))
	copy(out, accepted)
	return out, nil
}

// LinkConference is the port's LinkConference. Linking a conference to a unit it
// is already linked to answers the link that exists rather than making a second
// one, for the reason OpenConference is idempotent: a control plane that lost
// the answer may ask again.
func (u *Unit) LinkConference(c domain.ConferenceID, r mediaport.Reference) (mediaport.LinkID, error) {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()

	held, err := u.conferenceFor(c, "linking")
	if err != nil {
		return mediaport.LinkID{}, err
	}
	name, ok := referenceName(r)
	if !ok || name == u.name {
		return mediaport.LinkID{}, fmt.Errorf("linking %s to a reference this unit cannot use: %w", c, mediaport.ErrInvalid)
	}
	if _, known := u.fabric.units[name]; !known {
		return mediaport.LinkID{}, fmt.Errorf("linking %s to a reference this unit cannot use: %w", c, mediaport.ErrInvalid)
	}
	if existing, made := held.links[name]; made {
		return existing, nil
	}
	if u.refusing[CarryingALink()] {
		return mediaport.LinkID{}, fmt.Errorf("linking %s: %w", c, mediaport.ErrRefused)
	}

	u.links++
	id, err := mediaport.NewLinkID(fmt.Sprintf("%s-%s-%d", u.name, name, u.links))
	if err != nil {
		return mediaport.LinkID{}, err
	}
	held.links[name] = id
	return id, nil
}

// ReportCapacity is the port's ReportCapacity.
func (u *Unit) ReportCapacity() (float64, error) {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	if !u.alive {
		return 0, fmt.Errorf("reporting capacity: %w", mediaport.ErrUnavailable)
	}
	return u.load, nil
}

// ReportFaults is the port's ReportFaults.
func (u *Unit) ReportFaults() (mediaport.Faults, error) {
	u.pause()
	u.fabric.mu.Lock()
	defer u.fabric.mu.Unlock()
	if !u.alive {
		return nil, fmt.Errorf("opening the fault stream: %w", mediaport.ErrUnavailable)
	}
	s := &stream{fabric: u.fabric, ch: make(chan mediaport.Fault, StreamBuffer)}
	u.streams = append(u.streams, s)
	return s, nil
}

// The bookkeeping the operations above share.

// pause is the latency knob. It holds no lock while it waits, so a unit told to
// be slow is not a unit that stops answering another goroutine's question about
// what it holds.
func (u *Unit) pause() {
	u.fabric.mu.Lock()
	d := u.latency
	u.fabric.mu.Unlock()
	if d > 0 {
		<-u.clock.After(d)
	}
}

// reachableState is what every operation asks before it looks at its arguments:
// whether this unit is answering at all, and whether the conference named is one
// a restart took away.
func (u *Unit) reachableState(c domain.ConferenceID) error {
	if !u.alive {
		return fmt.Errorf("conference %s: %w", c, mediaport.ErrUnavailable)
	}
	if u.lost[c] {
		delete(u.lost, c)
		return fmt.Errorf("conference %s did not survive a restart of this unit: %w", c, mediaport.ErrLost)
	}
	return nil
}

func (u *Unit) conferenceFor(c domain.ConferenceID, doing string) (*conference, error) {
	if err := u.reachableState(c); err != nil {
		return nil, err
	}
	held, ok := u.conferences[c]
	if !ok {
		return nil, fmt.Errorf("%s %s: %w", doing, c, mediaport.ErrUnknown)
	}
	return held, nil
}

// reachable is every source a subscriber on this unit may be given: what this
// unit's own publishers offer, and what the publishers of a linked unit offer.
//
// A link counts only where both units have made one. That is the port's own
// rule read from the other side: LinkConference does not promise the far side is
// ready, so the control plane calls it on both and the conference spans once
// both have answered.
func (u *Unit) reachable(c domain.ConferenceID) []domain.Source {
	held, ok := u.conferences[c]
	if !ok {
		return nil
	}
	out := make([]domain.Source, 0, len(held.sources))
	for _, s := range held.sources {
		out = append(out, s)
	}
	for peer := range held.links {
		other, known := u.fabric.units[peer]
		if !known {
			continue
		}
		far, carried := other.conferences[c]
		if !carried {
			continue
		}
		if _, back := far.links[u.name]; !back {
			continue
		}
		for _, s := range far.sources {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID().String() < out[j].ID().String() })
	return out
}

// transportFor is the opaque answer of both admissions. It is derived rather
// than random because internal/random is the only place a source of randomness
// is made, and because a test that wants to know two admissions answered
// differently can read it. Nothing above the port may.
func (u *Unit) transportFor(c domain.ConferenceID, p domain.ParticipantID) mediaport.Transport {
	return mediaport.NewTransport([]byte(fmt.Sprintf("mediafake:%s/%s/%s", u.name, c, p)))
}

// outsideProfile is the arm of ErrInvalid a bookkeeper can decide: whether a
// source is inside the profile its conference was opened with. A video source
// offers at least one layer and no more than the profile carries, and an audio
// source offers exactly one, which is internal/domain's sentence about a Source.
func outsideProfile(p mediaport.Profile, s domain.Source) error {
	switch s.Kind() {
	case domain.Audio():
		if s.Layers() != 1 {
			return fmt.Errorf("audio source %s offers %d layers and audio has one: %w", s.ID(), s.Layers(), mediaport.ErrInvalid)
		}
	case domain.Video():
		if s.Layers() < 1 || s.Layers() > p.Layers() {
			return fmt.Errorf("video source %s offers %d layers and the profile carries %d: %w", s.ID(), s.Layers(), p.Layers(), mediaport.ErrInvalid)
		}
	default:
		return fmt.Errorf("source %s is neither audio nor video: %w", s.ID(), mediaport.ErrInvalid)
	}
	return nil
}

func referenceName(r mediaport.Reference) (string, bool) {
	const prefix = "mediafake:"
	b := string(r.Bytes())
	if len(b) <= len(prefix) || b[:len(prefix)] != prefix {
		return "", false
	}
	return b[len(prefix):], true
}

func sortedParticipants(set map[domain.ParticipantID]bool) []domain.ParticipantID {
	out := make([]domain.ParticipantID, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// A stream is one open fault stream.
type stream struct {
	fabric *Fabric
	ch     chan mediaport.Fault
	err    error
	closed bool
}

func (s *stream) Notices() <-chan mediaport.Fault { return s.ch }

func (s *stream) Err() error {
	s.fabric.mu.Lock()
	defer s.fabric.mu.Unlock()
	return s.err
}

// Stop ends the stream from the caller's side, leaving Err nil, because a
// stream the caller closed is not a stream that broke.
func (s *stream) Stop() {
	s.fabric.mu.Lock()
	defer s.fabric.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}

// deliver is called with the fabric lock held. It answers whether the stream is
// still open afterwards.
func (s *stream) deliver(f mediaport.Fault) bool {
	if s.closed {
		return false
	}
	select {
	case s.ch <- f:
		return true
	default:
		s.breakStream(mediaport.ErrUnavailable)
		return false
	}
}

// breakStream is called with the fabric lock held.
func (s *stream) breakStream(err error) {
	if s.closed {
		return
	}
	s.closed = true
	s.err = err
	close(s.ch)
}
