// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// The suite is in the package rather than beside it, matching internal/placement
// and for the same reason. Nothing below reaches for an unexported name, so what
// the cases exercise is only what a caller of the port can reach, and the one
// exception is deliberate: the fake's knobs are exported because a test is who
// sets them.
package mediafake

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/domain"
	"github.com/iderex/hoersaal/internal/mediaport"
)

// start is the instant every clock in this file reads before a test moves it. It
// is a literal because internal/guard refuses a clock read outside
// internal/clock.
var start = time.Date(2026, time.August, 9, 9, 0, 0, 0, time.UTC)

func profile(t *testing.T, layers int) mediaport.Profile {
	t.Helper()
	p, err := mediaport.NewProfile("speech-and-picture", layers)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	return p
}

func conf(t *testing.T, v string) domain.ConferenceID {
	t.Helper()
	c, err := domain.NewConferenceID(v)
	if err != nil {
		t.Fatalf("conference %q: %v", v, err)
	}
	return c
}

func part(t *testing.T, v string) domain.ParticipantID {
	t.Helper()
	p, err := domain.NewParticipantID(v)
	if err != nil {
		t.Fatalf("participant %q: %v", v, err)
	}
	return p
}

func source(t *testing.T, id string, publisher domain.ParticipantID, kind domain.Kind, layers int) domain.Source {
	t.Helper()
	sid, err := domain.NewSourceID(id)
	if err != nil {
		t.Fatalf("source id %q: %v", id, err)
	}
	s, err := domain.NewSource(sid, publisher, kind, layers)
	if err != nil {
		t.Fatalf("source %q: %v", id, err)
	}
	return s
}

func wants(t *testing.T, subscriber domain.ParticipantID, s domain.Source, layer int) domain.Subscription {
	t.Helper()
	sub, err := domain.NewSubscription(subscriber, s, layer)
	if err != nil {
		t.Fatalf("subscription of %s to %s: %v", subscriber, s.ID(), err)
	}
	return sub
}

// unit is one fake in its own fabric, which is what a test that never links
// wants.
func unit(t *testing.T) *Unit {
	t.Helper()
	u, err := NewFabric().Add("one", clock.NewTest(start))
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	return u
}

// opened is a unit already holding a conference under the three-layer profile,
// which is the starting point of most of what follows.
func opened(t *testing.T) (*Unit, domain.ConferenceID) {
	t.Helper()
	u := unit(t)
	c := conf(t, "lecture")
	if err := u.OpenConference(c, profile(t, 3)); err != nil {
		t.Fatalf("opening %s: %v", c, err)
	}
	return u, c
}

func TestTheFabricRefusesAUnitNoTestCouldHaveMeant(t *testing.T) {
	f := NewFabric()
	if _, err := f.Add("", clock.NewTest(start)); !errors.Is(err, mediaport.ErrEmpty) {
		t.Fatalf("a unit with no name = %v", err)
	}
	if _, err := f.Add("one", nil); !errors.Is(err, mediaport.ErrEmpty) {
		t.Fatalf("a unit with no clock = %v", err)
	}
	if _, err := f.Add("one", clock.NewTest(start)); err != nil {
		t.Fatalf("the first unit: %v", err)
	}
	if _, err := f.Add("one", clock.NewTest(start)); !errors.Is(err, mediaport.ErrConflict) {
		t.Fatalf("a second unit under one name = %v", err)
	}
}

// Opening the same conference with the same profile is the case a control plane
// that lost its answer relies on, and opening it with a different one is the
// case that says the caller and the unit disagree about what is there.
func TestOpeningAConference(t *testing.T) {
	u := unit(t)
	c := conf(t, "lecture")
	three, one := profile(t, 3), profile(t, 1)

	if err := u.OpenConference(c, three); err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := u.OpenConference(c, three); err != nil {
		t.Fatalf("opening the same conference with the same profile: %v", err)
	}
	if err := u.OpenConference(c, one); !errors.Is(err, mediaport.ErrConflict) {
		t.Fatalf("opening a held identifier with another profile = %v", err)
	}
	if got := u.Conferences(); len(got) != 1 || got[0] != c {
		t.Fatalf("the unit holds %v", got)
	}
	held, ok := u.Profile(c)
	if !ok || held != three {
		t.Fatalf("the conference is held with %v (%v)", held, ok)
	}
}

// The arm of Invalid a bookkeeper cannot decide for itself, which is why Serves
// exists and why this asserts against a set the test closed rather than against
// anything the fake worked out.
func TestAProfileTheUnitDoesNotServeIsRefusedAsInvalid(t *testing.T) {
	u := unit(t)
	u.Serves("speech-only")
	if err := u.OpenConference(conf(t, "lecture"), profile(t, 3)); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("a profile outside the served set = %v", err)
	}
	u.Serves("speech-and-picture")
	if err := u.OpenConference(conf(t, "lecture"), profile(t, 3)); err != nil {
		t.Fatalf("a profile inside the served set: %v", err)
	}
}

func TestClosingAConferenceThatIsNotThereSucceeds(t *testing.T) {
	u, c := opened(t)
	if err := u.CloseConference(conf(t, "never-opened")); err != nil {
		t.Fatalf("closing a conference that is not there: %v", err)
	}
	if err := u.CloseConference(c); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if got := u.Conferences(); len(got) != 0 {
		t.Fatalf("the unit still holds %v", got)
	}
	if err := u.CloseConference(c); err != nil {
		t.Fatalf("closing twice: %v", err)
	}
}

func TestAdmittingAPublisher(t *testing.T) {
	u, c := opened(t)
	p := part(t, "speaker")
	camera := source(t, "camera", p, domain.Video(), 3)
	microphone := source(t, "microphone", p, domain.Audio(), 1)

	if _, err := u.AdmitPublisher(conf(t, "elsewhere"), p, nil); !errors.Is(err, mediaport.ErrUnknown) {
		t.Fatalf("admitting into a conference this unit does not hold = %v", err)
	}

	tr, err := u.AdmitPublisher(c, p, []domain.Source{camera, microphone})
	if err != nil {
		t.Fatalf("admitting a publisher: %v", err)
	}
	if len(tr.Bytes()) == 0 {
		t.Fatalf("the admission answered with no transport parameters")
	}
	if _, err := u.AdmitPublisher(c, p, nil); !errors.Is(err, mediaport.ErrConflict) {
		t.Fatalf("admitting the same publisher twice = %v", err)
	}
	if got := u.Publishers(c); len(got) != 1 || got[0] != p {
		t.Fatalf("the conference publishes for %v", got)
	}
	if got := u.Sources(c); len(got) != 2 {
		t.Fatalf("the conference carries %d sources", len(got))
	}
}

// Every arm of Invalid and Conflict that AdmitPublisher can decide from its own
// records. The near miss is the video source with one layer too many: a profile
// carrying three layers is a source offering at most three, and the mistake
// somebody makes is offering exactly one more.
func TestAdmittingAPublisherRefusesASourceOutsideTheProfile(t *testing.T) {
	p, other := part(t, "speaker"), part(t, "somebody-else")
	for _, c := range []struct {
		name   string
		source func(t *testing.T) domain.Source
		want   error
	}{
		{"one layer past the profile", func(t *testing.T) domain.Source {
			return source(t, "camera", p, domain.Video(), 4)
		}, mediaport.ErrInvalid},
		{"audio in layers", func(t *testing.T) domain.Source {
			return source(t, "microphone", p, domain.Audio(), 2)
		}, mediaport.ErrInvalid},
		{"published by somebody else", func(t *testing.T) domain.Source {
			return source(t, "camera", other, domain.Video(), 2)
		}, mediaport.ErrInvalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			u, id := opened(t)
			if _, err := u.AdmitPublisher(id, p, []domain.Source{c.source(t)}); !errors.Is(err, c.want) {
				t.Fatalf("%s = %v, want %v", c.name, err, c.want)
			}
			if got := u.Publishers(id); len(got) != 0 {
				t.Fatalf("a refused admission left %v publishing", got)
			}
			if got := u.Sources(id); len(got) != 0 {
				t.Fatalf("a refused admission left %d sources behind", len(got))
			}
		})
	}

	u, id := opened(t)
	first := source(t, "camera", p, domain.Video(), 2)
	if _, err := u.AdmitPublisher(id, p, []domain.Source{first}); err != nil {
		t.Fatalf("the first publisher: %v", err)
	}
	clash := source(t, "camera", other, domain.Video(), 2)
	if _, err := u.AdmitPublisher(id, other, []domain.Source{clash}); !errors.Is(err, mediaport.ErrConflict) {
		t.Fatalf("a second source under one identifier = %v", err)
	}
}

func TestAdmittingASubscriber(t *testing.T) {
	u, c := opened(t)
	p := part(t, "seat-1")

	if _, err := u.AdmitSubscriber(conf(t, "elsewhere"), p); !errors.Is(err, mediaport.ErrUnknown) {
		t.Fatalf("admitting into a conference this unit does not hold = %v", err)
	}
	if _, err := u.AdmitSubscriber(c, p); err != nil {
		t.Fatalf("admitting a subscriber: %v", err)
	}
	if _, err := u.AdmitSubscriber(c, p); !errors.Is(err, mediaport.ErrConflict) {
		t.Fatalf("admitting the same subscriber twice = %v", err)
	}
	if got := u.Subscribers(c); len(got) != 1 || got[0] != p {
		t.Fatalf("the conference receives for %v", got)
	}
	if got := u.Reception(c, p); len(got) != 0 {
		t.Fatalf("a subscriber who has asked for nothing receives %v", got)
	}
}

// The four operations the port gives ErrRefused for, and no others: the type
// Refuse takes is closed at those four, so a test cannot ask this fake for a
// refusal the port does not have.
func TestTheUnitRefusesWhatTheTestToldItTo(t *testing.T) {
	u := unit(t)
	c := conf(t, "lecture")
	u.Refuse(OpeningAConference())
	if err := u.OpenConference(c, profile(t, 3)); !errors.Is(err, mediaport.ErrRefused) {
		t.Fatalf("a refused open = %v", err)
	}
	u.Allow(OpeningAConference())
	if err := u.OpenConference(c, profile(t, 3)); err != nil {
		t.Fatalf("an allowed open: %v", err)
	}

	p := part(t, "speaker")
	u.Refuse(AdmittingAPublisher(), AdmittingASubscriber(), CarryingALink())
	if _, err := u.AdmitPublisher(c, p, nil); !errors.Is(err, mediaport.ErrRefused) {
		t.Fatalf("a refused publisher = %v", err)
	}
	if _, err := u.AdmitSubscriber(c, p); !errors.Is(err, mediaport.ErrRefused) {
		t.Fatalf("a refused subscriber = %v", err)
	}
}

// SetReception replaces rather than adds, which is the port's whole reason for
// having one operation instead of two.
func TestSettingAReceptionReplacesTheWholeSet(t *testing.T) {
	u, c := opened(t)
	speaker, seat := part(t, "speaker"), part(t, "seat-1")
	camera := source(t, "camera", speaker, domain.Video(), 3)
	microphone := source(t, "microphone", speaker, domain.Audio(), 1)
	if _, err := u.AdmitPublisher(c, speaker, []domain.Source{camera, microphone}); err != nil {
		t.Fatalf("the publisher: %v", err)
	}
	if _, err := u.AdmitSubscriber(c, seat); err != nil {
		t.Fatalf("the subscriber: %v", err)
	}

	both := []domain.Subscription{wants(t, seat, camera, 2), wants(t, seat, microphone, 0)}
	accepted, err := u.SetReception(c, seat, both)
	if err != nil {
		t.Fatalf("setting a reception of two: %v", err)
	}
	if len(accepted) != 2 {
		t.Fatalf("the unit accepted %d of two", len(accepted))
	}

	accepted, err = u.SetReception(c, seat, []domain.Subscription{wants(t, seat, microphone, 0)})
	if err != nil {
		t.Fatalf("setting a reception of one: %v", err)
	}
	if len(accepted) != 1 || accepted[0].Source() != microphone.ID() {
		t.Fatalf("the unit accepted %v", accepted)
	}
	if got := u.Reception(c, seat); len(got) != 1 || got[0].Source() != microphone.ID() {
		t.Fatalf("the unit believes it is sending %v; a source absent from the set is not received", got)
	}

	accepted, err = u.SetReception(c, seat, nil)
	if err != nil {
		t.Fatalf("setting an empty reception: %v", err)
	}
	if len(accepted) != 0 || len(u.Reception(c, seat)) != 0 {
		t.Fatalf("an empty set left %v behind", u.Reception(c, seat))
	}
}

// A unit that cannot serve the whole set answers with a smaller one rather than
// with an error, which is the one place the port deliberately has no Refused.
func TestAUnitToldToAcceptLessAnswersWithLess(t *testing.T) {
	u, c := opened(t)
	speaker, seat := part(t, "speaker"), part(t, "seat-1")
	camera := source(t, "camera", speaker, domain.Video(), 3)
	microphone := source(t, "microphone", speaker, domain.Audio(), 1)
	if _, err := u.AdmitPublisher(c, speaker, []domain.Source{camera, microphone}); err != nil {
		t.Fatalf("the publisher: %v", err)
	}
	if _, err := u.AdmitSubscriber(c, seat); err != nil {
		t.Fatalf("the subscriber: %v", err)
	}

	u.AcceptAtMost(1)
	accepted, err := u.SetReception(c, seat, []domain.Subscription{
		wants(t, seat, camera, 2), wants(t, seat, microphone, 0),
	})
	if err != nil {
		t.Fatalf("setting a reception of two against a unit accepting one: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("the unit accepted %d entries and was told to take one", len(accepted))
	}
	if got := u.Reception(c, seat); len(got) != 1 || got[0].Source() != accepted[0].Source() {
		t.Fatalf("the unit believes it is sending %v and answered %v", got, accepted)
	}
}

func TestSettingAReceptionRefusesWhatTheConferenceCannotCarry(t *testing.T) {
	u, c := opened(t)
	speaker, seat, stranger := part(t, "speaker"), part(t, "seat-1"), part(t, "seat-2")
	camera := source(t, "camera", speaker, domain.Video(), 3)
	if _, err := u.AdmitPublisher(c, speaker, []domain.Source{camera}); err != nil {
		t.Fatalf("the publisher: %v", err)
	}
	if _, err := u.AdmitSubscriber(c, seat); err != nil {
		t.Fatalf("the subscriber: %v", err)
	}

	if _, err := u.SetReception(conf(t, "elsewhere"), seat, nil); !errors.Is(err, mediaport.ErrUnknown) {
		t.Fatalf("a conference this unit does not hold = %v", err)
	}
	if _, err := u.SetReception(c, stranger, nil); !errors.Is(err, mediaport.ErrUnknown) {
		t.Fatalf("a participant that was never admitted to receive = %v", err)
	}

	// A source of another conference on this unit, which is the case the port
	// answers Invalid for and the case issue #44 asks a test for.
	other := conf(t, "seminar")
	if err := u.OpenConference(other, profile(t, 3)); err != nil {
		t.Fatalf("the second conference: %v", err)
	}
	elsewhere := part(t, "other-speaker")
	theirs := source(t, "their-camera", elsewhere, domain.Video(), 2)
	if _, err := u.AdmitPublisher(other, elsewhere, []domain.Source{theirs}); err != nil {
		t.Fatalf("the second publisher: %v", err)
	}
	if _, err := u.SetReception(c, seat, []domain.Subscription{wants(t, seat, theirs, 0)}); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("subscribing across conferences = %v", err)
	}

	if _, err := u.SetReception(c, seat, []domain.Subscription{
		wants(t, seat, camera, 0), wants(t, seat, camera, 1),
	}); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("one source named twice in one set = %v", err)
	}
	if _, err := u.SetReception(c, seat, []domain.Subscription{wants(t, stranger, camera, 0)}); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("an entry for another subscriber = %v", err)
	}
	if got := u.Reception(c, seat); len(got) != 0 {
		t.Fatalf("a refused set left %v behind", got)
	}
}

// A layer outside the arrangement is refused against the source this unit holds
// rather than against the copy the caller made the entry from. The two can
// differ, and the near miss is the layer exactly one past the top.
func TestSettingAReceptionRefusesALayerTheSourceDoesNotOffer(t *testing.T) {
	u, c := opened(t)
	speaker, seat := part(t, "speaker"), part(t, "seat-1")
	held := source(t, "camera", speaker, domain.Video(), 2)
	if _, err := u.AdmitPublisher(c, speaker, []domain.Source{held}); err != nil {
		t.Fatalf("the publisher: %v", err)
	}
	if _, err := u.AdmitSubscriber(c, seat); err != nil {
		t.Fatalf("the subscriber: %v", err)
	}

	// The caller's copy offers three layers; the unit holds one offering two.
	richer := source(t, "camera", speaker, domain.Video(), 3)
	if _, err := u.SetReception(c, seat, []domain.Subscription{wants(t, seat, richer, 2)}); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("a layer the held source does not offer = %v", err)
	}
	if _, err := u.SetReception(c, seat, []domain.Subscription{wants(t, seat, richer, 1)}); err != nil {
		t.Fatalf("the top layer the held source does offer: %v", err)
	}
}

// The third done-when of issue #42: a conference relayed between two fakes is
// observable in both, so a cascade test needs no real unit.
func TestAConferenceRelayedBetweenTwoFakesIsObservableInBoth(t *testing.T) {
	f := NewFabric()
	a, err := f.Add("a", clock.NewTest(start))
	if err != nil {
		t.Fatalf("unit a: %v", err)
	}
	b, err := f.Add("b", clock.NewTest(start))
	if err != nil {
		t.Fatalf("unit b: %v", err)
	}

	c := conf(t, "lecture")
	p := profile(t, 3)
	if err := a.OpenConference(c, p); err != nil {
		t.Fatalf("opening on a: %v", err)
	}
	if err := b.OpenConference(c, p); err != nil {
		t.Fatalf("opening on b: %v", err)
	}

	speaker, seat := part(t, "speaker"), part(t, "seat-1")
	camera := source(t, "camera", speaker, domain.Video(), 3)
	if _, err := a.AdmitPublisher(c, speaker, []domain.Source{camera}); err != nil {
		t.Fatalf("the publisher on a: %v", err)
	}
	if _, err := b.AdmitSubscriber(c, seat); err != nil {
		t.Fatalf("the subscriber on b: %v", err)
	}

	// One side alone is not a span. The port does not promise the far side is
	// ready, so the control plane calls this on both.
	if _, err := b.LinkConference(c, a.Reference()); err != nil {
		t.Fatalf("linking b to a: %v", err)
	}
	if _, err := b.SetReception(c, seat, []domain.Subscription{wants(t, seat, camera, 1)}); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("a source across a half-made link = %v", err)
	}

	link, err := a.LinkConference(c, b.Reference())
	if err != nil {
		t.Fatalf("linking a to b: %v", err)
	}
	if got := a.Links(c); len(got) != 1 || got[0] != link {
		t.Fatalf("a carries %v", got)
	}
	if got := b.Links(c); len(got) != 1 {
		t.Fatalf("b carries %v", got)
	}

	if got := b.Sources(c); len(got) != 1 || got[0].ID() != camera.ID() {
		t.Fatalf("b sees %v of a's conference", got)
	}
	accepted, err := b.SetReception(c, seat, []domain.Subscription{wants(t, seat, camera, 1)})
	if err != nil {
		t.Fatalf("receiving a source published on the other unit: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("b accepted %v", accepted)
	}

	// Linking twice answers the link that exists rather than making a second.
	again, err := a.LinkConference(c, b.Reference())
	if err != nil {
		t.Fatalf("linking a to b again: %v", err)
	}
	if again != link {
		t.Fatalf("the second link answered %v and the first %v", again, link)
	}

	// A link ends when the conference ends on either side, which is why closing
	// is the only way one goes.
	if err := a.CloseConference(c); err != nil {
		t.Fatalf("closing on a: %v", err)
	}
	if got := b.Links(c); len(got) != 0 {
		t.Fatalf("b still carries %v after the conference ended on a", got)
	}
	if got := b.Sources(c); len(got) != 0 {
		t.Fatalf("b still sees %v after the conference ended on a", got)
	}
}

func TestLinkingRefusesAReferenceThisUnitCannotUse(t *testing.T) {
	f := NewFabric()
	a, err := f.Add("a", clock.NewTest(start))
	if err != nil {
		t.Fatalf("unit a: %v", err)
	}
	elsewhere, err := NewFabric().Add("b", clock.NewTest(start))
	if err != nil {
		t.Fatalf("a unit of another fabric: %v", err)
	}
	c := conf(t, "lecture")

	if _, err := a.LinkConference(c, elsewhere.Reference()); !errors.Is(err, mediaport.ErrUnknown) {
		t.Fatalf("linking a conference this unit does not hold = %v", err)
	}
	if err := a.OpenConference(c, profile(t, 3)); err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := a.LinkConference(c, elsewhere.Reference()); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("a reference from another fabric = %v", err)
	}
	if _, err := a.LinkConference(c, a.Reference()); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("a unit linked to itself = %v", err)
	}
	if _, err := a.LinkConference(c, mediaport.NewReference([]byte("something-else"))); !errors.Is(err, mediaport.ErrInvalid) {
		t.Fatalf("a reference of another shape = %v", err)
	}

	b, err := f.Add("b", clock.NewTest(start))
	if err != nil {
		t.Fatalf("unit b: %v", err)
	}
	a.Refuse(CarryingALink())
	if _, err := a.LinkConference(c, b.Reference()); !errors.Is(err, mediaport.ErrRefused) {
		t.Fatalf("a refused link = %v", err)
	}
}

// The capacity signal is what the test set and never what the fake worked out,
// which is the second done-when of issue #42 and the reason no number in this
// package is derived from what a conference holds.
func TestTheCapacitySignalIsTheOneTheTestSet(t *testing.T) {
	u, c := opened(t)
	load, err := u.ReportCapacity()
	if err != nil {
		t.Fatalf("reporting capacity: %v", err)
	}
	if load != 0 {
		t.Fatalf("a unit nobody has set reports %v", load)
	}

	// Filling the conference moves nothing, because the derivation belongs to
	// docs/decisions/capacity-signal.md over denominators issue #54 has not
	// produced.
	speaker := part(t, "speaker")
	if _, err := u.AdmitPublisher(c, speaker, []domain.Source{source(t, "camera", speaker, domain.Video(), 3)}); err != nil {
		t.Fatalf("the publisher: %v", err)
	}
	if load, err = u.ReportCapacity(); err != nil || load != 0 {
		t.Fatalf("admitting a publisher moved the signal to %v (%v)", load, err)
	}

	// Above one is reported rather than clipped.
	u.SetLoad(1.4)
	if load, err = u.ReportCapacity(); err != nil || load != 1.4 {
		t.Fatalf("a unit over its calibration reports %v (%v)", load, err)
	}
}

// Dying, and coming back having lost what it held. The pair worth separating is
// Unavailable against Lost: not knowing, and knowing the bad answer.
func TestAUnitThatDiesAnswersUnavailableAndOneThatRestartsAnswersLost(t *testing.T) {
	u, c := opened(t)
	p := profile(t, 3)

	u.Die()
	if err := u.OpenConference(c, p); !errors.Is(err, mediaport.ErrUnavailable) {
		t.Fatalf("a dead unit answered %v", err)
	}
	if _, err := u.ReportCapacity(); !errors.Is(err, mediaport.ErrUnavailable) {
		t.Fatalf("a dead unit reported capacity as %v", err)
	}
	if _, err := u.ReportFaults(); !errors.Is(err, mediaport.ErrUnavailable) {
		t.Fatalf("a dead unit opened a fault stream: %v", err)
	}
	if err := u.CloseConference(c); !errors.Is(err, mediaport.ErrUnavailable) {
		t.Fatalf("a dead unit closed a conference: %v", err)
	}

	u.Restart()
	if _, err := u.AdmitSubscriber(c, part(t, "seat-1")); !errors.Is(err, mediaport.ErrLost) {
		t.Fatalf("a restarted unit answered %v for state the caller believed in", err)
	}
	// Told once. The call after that meets a unit that never held it.
	if _, err := u.AdmitSubscriber(c, part(t, "seat-1")); !errors.Is(err, mediaport.ErrUnknown) {
		t.Fatalf("the second call after a restart answered %v", err)
	}
	if err := u.OpenConference(c, p); err != nil {
		t.Fatalf("reopening after a restart: %v", err)
	}
}

// The knobs are the test's settings rather than the unit's state, so a restart
// leaves them where they were.
func TestARestartKeepsWhatTheTestSetAndDropsWhatTheUnitHeld(t *testing.T) {
	u, _ := opened(t)
	u.SetLoad(0.7)
	u.Restart()
	if got := u.Conferences(); len(got) != 0 {
		t.Fatalf("a restarted unit still holds %v", got)
	}
	load, err := u.ReportCapacity()
	if err != nil {
		t.Fatalf("reporting capacity after a restart: %v", err)
	}
	if load != 0.7 {
		t.Fatalf("the restart moved the signal the test set to %v", load)
	}

}

func TestTheFaultStreamCarriesWhatTheTestFailed(t *testing.T) {
	u, c := opened(t)
	faults, err := u.ReportFaults()
	if err != nil {
		t.Fatalf("opening the fault stream: %v", err)
	}

	notice, err := mediaport.ParticipantFault(c, part(t, "seat-1"))
	if err != nil {
		t.Fatalf("the notice: %v", err)
	}
	u.Fail(notice)

	got, open := <-faults.Notices()
	if !open {
		t.Fatalf("the stream closed instead of delivering")
	}
	if got.Kind() != mediaport.ParticipantGone() || got.Conference() != c {
		t.Fatalf("the stream delivered %v", got)
	}
	if err := faults.Err(); err != nil {
		t.Fatalf("an open stream reports %v", err)
	}

	// A stream the caller stopped is not a stream that broke.
	faults.Stop()
	faults.Stop()
	if _, open := <-faults.Notices(); open {
		t.Fatalf("a stopped stream is still open")
	}
	if err := faults.Err(); err != nil {
		t.Fatalf("a stopped stream reports %v, and it did not break", err)
	}
}

// A unit that has died sends no notice about itself. That is the port's sentence
// and it is the reason the pool decides liveness by asking rather than waiting,
// so a fake that helpfully announced its own death would let a test pass that
// the real thing would hang.
func TestADeadUnitBreaksTheStreamAndAnnouncesNothing(t *testing.T) {
	u, _ := opened(t)
	faults, err := u.ReportFaults()
	if err != nil {
		t.Fatalf("opening the fault stream: %v", err)
	}

	u.Die()
	notice, open := <-faults.Notices()
	if open {
		t.Fatalf("a dead unit sent %v", notice)
	}
	if err := faults.Err(); !errors.Is(err, mediaport.ErrUnavailable) {
		t.Fatalf("a broken stream reports %v", err)
	}
}

// The latency knob, and the property that makes it usable at all: nothing
// sleeps. The call is waiting on the clock, and it finishes when the test moves
// the clock rather than when a runner gets round to it.
func TestAUnitToldToBeSlowWaitsOnTheClockAndNotOnTheRunner(t *testing.T) {
	c := clock.NewTest(start)
	u, err := NewFabric().Add("one", c)
	if err != nil {
		t.Fatalf("unit: %v", err)
	}

	if _, err := u.ReportCapacity(); err != nil {
		t.Fatalf("reporting capacity: %v", err)
	}
	if c.Waiting() != 0 {
		t.Fatalf("a unit with no latency asked the clock for %d waits", c.Waiting())
	}

	u.SetLatency(2 * time.Minute)
	answered := make(chan error, 1)
	go func() {
		_, err := u.ReportCapacity()
		answered <- err
	}()
	for c.Waiting() == 0 {
		runtime.Gosched()
	}
	select {
	case err := <-answered:
		t.Fatalf("a slow unit answered before the clock moved: %v", err)
	default:
	}
	c.Advance(2 * time.Minute)
	if err := <-answered; err != nil {
		t.Fatalf("the slow answer: %v", err)
	}
}

// The fourth done-when of issue #42 asks that the whole control plane suite run
// against the fake with no display, no media device and no network. There is no
// control plane suite yet, so what is assertable here is this package's half of
// it: the fake is the port, and everything it needs is in this process.
func TestTheFakeIsThePort(t *testing.T) {
	var u mediaport.Unit
	f, err := NewFabric().Add("one", clock.NewTest(start))
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	u = f
	if _, err := u.ReportCapacity(); err != nil {
		t.Fatalf("the port through the interface: %v", err)
	}
}
