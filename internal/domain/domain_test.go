package domain

import (
	"errors"
	"testing"
)

// The helpers below build the valid pieces, so each test below is about the one
// thing it names. Every one of them fails the test rather than returning an
// error, because a helper that produced an invalid value would make the test
// about the helper.

func ident(t *testing.T, v string) IdentityID {
	t.Helper()
	id, err := NewIdentityID(v)
	if err != nil {
		t.Fatalf("NewIdentityID(%q): %v", v, err)
	}
	return id
}

func part(t *testing.T, v string) ParticipantID {
	t.Helper()
	id, err := NewParticipantID(v)
	if err != nil {
		t.Fatalf("NewParticipantID(%q): %v", v, err)
	}
	return id
}

func conf(t *testing.T, v string) ConferenceID {
	t.Helper()
	id, err := NewConferenceID(v)
	if err != nil {
		t.Fatalf("NewConferenceID(%q): %v", v, err)
	}
	return id
}

func srcID(t *testing.T, v string) SourceID {
	t.Helper()
	id, err := NewSourceID(v)
	if err != nil {
		t.Fatalf("NewSourceID(%q): %v", v, err)
	}
	return id
}

func role(t *testing.T, name string) Role {
	t.Helper()
	r, err := NewRole(name)
	if err != nil {
		t.Fatalf("NewRole(%q): %v", name, err)
	}
	return r
}

func participant(t *testing.T, id, identity, conference, name string) Participant {
	t.Helper()
	p, err := NewParticipant(part(t, id), ident(t, identity), conf(t, conference), role(t, name))
	if err != nil {
		t.Fatalf("NewParticipant(%q): %v", id, err)
	}
	return p
}

func source(t *testing.T, id, publisher string, kind Kind, layers int) Source {
	t.Helper()
	s, err := NewSource(srcID(t, id), part(t, publisher), kind, layers)
	if err != nil {
		t.Fatalf("NewSource(%q): %v", id, err)
	}
	return s
}

// room is a conference with a publisher, a subscriber and one video source of
// three layers already in it.
func room(t *testing.T) (*Conference, Participant, Participant, Source) {
	t.Helper()
	c, err := NewConference(conf(t, "lecture"))
	if err != nil {
		t.Fatalf("NewConference: %v", err)
	}
	speaker := participant(t, "speaker", "prof", "lecture", "presenter")
	listener := participant(t, "listener", "student", "lecture", "attendee")
	camera := source(t, "camera", "speaker", Video(), 3)
	for _, p := range []Participant{speaker, listener} {
		if err := c.Admit(p); err != nil {
			t.Fatalf("Admit(%s): %v", p.ID(), err)
		}
	}
	if err := c.Publish(camera); err != nil {
		t.Fatalf("Publish(camera): %v", err)
	}
	return c, speaker, listener, camera
}

func TestAnEmptyIdentifierIsRefusedWhereverOneCanEnter(t *testing.T) {
	calls := map[string]func() error{
		"identity": func() error { _, err := NewIdentityID(""); return err },
		"participant": func() error {
			_, err := NewParticipantID("")
			return err
		},
		"conference": func() error {
			_, err := NewConferenceID("")
			return err
		},
		"source": func() error { _, err := NewSourceID(""); return err },
		"role":   func() error { _, err := NewRole(""); return err },
		"the conference itself": func() error {
			_, err := NewConference(ConferenceID{})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrEmpty) {
				t.Fatalf("got %v, want ErrEmpty", err)
			}
		})
	}
}

func TestAParticipantIsMissingNothing(t *testing.T) {
	full := participant(t, "listener", "student", "lecture", "attendee")
	cases := map[string]Participant{
		"no identifier": {identity: full.identity, conference: full.conference, role: full.role},
		"no identity":   {id: full.id, conference: full.conference, role: full.role},
		"no conference": {id: full.id, identity: full.identity, role: full.role},
		"no role":       {id: full.id, identity: full.identity, conference: full.conference},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewParticipant(p.id, p.identity, p.conference, p.role)
			if !errors.Is(err, ErrEmpty) {
				t.Fatalf("got %v, want ErrEmpty", err)
			}
		})
	}
}

func TestASourceIsAudioOrVideoAndNothingElse(t *testing.T) {
	_, err := NewSource(srcID(t, "camera"), part(t, "speaker"), Kind{}, 3)
	if !errors.Is(err, ErrNotAKind) {
		t.Fatalf("got %v, want ErrNotAKind", err)
	}
	for _, k := range []Kind{Audio(), Video()} {
		if _, err := NewSource(srcID(t, "s"), part(t, "speaker"), k, 1); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
}

func TestASourceNobodyCanReceiveIsRefused(t *testing.T) {
	for _, layers := range []int{0, -1} {
		_, err := NewSource(srcID(t, "camera"), part(t, "speaker"), Video(), layers)
		if !errors.Is(err, ErrNoLayer) {
			t.Fatalf("%d layers: got %v, want ErrNoLayer", layers, err)
		}
	}
}

func TestAPublisherDoesNotSubscribeToItsOwnSource(t *testing.T) {
	camera := source(t, "camera", "speaker", Video(), 3)
	_, err := NewSubscription(part(t, "speaker"), camera, 0)
	if !errors.Is(err, ErrOwnSource) {
		t.Fatalf("got %v, want ErrOwnSource", err)
	}
}

func TestALayerTheSourceDoesNotOfferIsRefused(t *testing.T) {
	camera := source(t, "camera", "speaker", Video(), 3)
	for _, layer := range []int{-1, 3, 4} {
		_, err := NewSubscription(part(t, "listener"), camera, layer)
		if !errors.Is(err, ErrNoLayer) {
			t.Fatalf("layer %d of three: got %v, want ErrNoLayer", layer, err)
		}
	}
	if _, err := NewSubscription(part(t, "listener"), camera, 2); err != nil {
		t.Fatalf("the highest layer the source offers: %v", err)
	}
}

func TestAParticipantMadeForAnotherConferenceIsRefused(t *testing.T) {
	c, _, _, _ := room(t)
	elsewhere := participant(t, "visitor", "guest", "seminar", "attendee")
	if err := c.Admit(elsewhere); !errors.Is(err, ErrElsewhere) {
		t.Fatalf("got %v, want ErrElsewhere", err)
	}
	if _, held := c.Participant(elsewhere.ID()); held {
		t.Fatal("the refused participant is in the conference")
	}
}

func TestASecondThingUnderOneIdentifierIsRefused(t *testing.T) {
	c, _, listener, camera := room(t)
	again, err := NewParticipant(listener.ID(), ident(t, "somebody else"), conf(t, "lecture"), role(t, "attendee"))
	if err != nil {
		t.Fatalf("NewParticipant: %v", err)
	}
	if err := c.Admit(again); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("participant: got %v, want ErrDuplicate", err)
	}
	if held, _ := c.Participant(listener.ID()); held.Identity() != listener.Identity() {
		t.Fatal("the refused admission replaced the participant that was there")
	}
	if err := c.Publish(camera); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("source: got %v, want ErrDuplicate", err)
	}
}

func TestAConferenceHoldsNothingFromSomebodyNotInIt(t *testing.T) {
	c, _, listener, camera := room(t)
	stranger := source(t, "microphone", "stranger", Audio(), 1)
	if err := c.Publish(stranger); !errors.Is(err, ErrUnknown) {
		t.Fatalf("source: got %v, want ErrUnknown", err)
	}
	s, err := NewSubscription(part(t, "stranger"), camera, 0)
	if err != nil {
		t.Fatalf("NewSubscription: %v", err)
	}
	if err := c.Subscribe(s); !errors.Is(err, ErrUnknown) {
		t.Fatalf("subscriber: got %v, want ErrUnknown", err)
	}
	absent := source(t, "screen", "speaker", Video(), 1)
	s, err = NewSubscription(listener.ID(), absent, 0)
	if err != nil {
		t.Fatalf("NewSubscription: %v", err)
	}
	if err := c.Subscribe(s); !errors.Is(err, ErrUnknown) {
		t.Fatalf("source: got %v, want ErrUnknown", err)
	}
}

func TestTheLayerIsCheckedAgainstTheSourceTheConferenceHolds(t *testing.T) {
	c, _, listener, _ := room(t)
	// Made against a copy that offers more layers than the one the conference
	// holds, which is the case a check at construction alone would miss.
	generous := source(t, "camera", "speaker", Video(), 6)
	s, err := NewSubscription(listener.ID(), generous, 5)
	if err != nil {
		t.Fatalf("NewSubscription: %v", err)
	}
	if err := c.Subscribe(s); !errors.Is(err, ErrNoLayer) {
		t.Fatalf("got %v, want ErrNoLayer", err)
	}
}

func TestThePublisherIsRecheckedAgainstTheSourceTheConferenceHolds(t *testing.T) {
	c, _, listener, _ := room(t)
	// The conference holds a source of its own under this identifier, published
	// by the participant who is about to subscribe to it. The subscription was
	// made against a copy naming somebody else as the publisher, so the check at
	// construction passes and only the conference can catch it.
	own := source(t, "slides", listener.ID().String(), Video(), 1)
	if err := c.Publish(own); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	elsewhere := source(t, "slides", "speaker", Video(), 1)
	s, err := NewSubscription(listener.ID(), elsewhere, 0)
	if err != nil {
		t.Fatalf("NewSubscription: %v", err)
	}
	if err := c.Subscribe(s); !errors.Is(err, ErrOwnSource) {
		t.Fatalf("got %v, want ErrOwnSource", err)
	}
}

func TestOnePersonOnTwoDevicesIsTwoParticipantsAndOneIdentity(t *testing.T) {
	c, _, _, _ := room(t)
	phone := participant(t, "listener-phone", "student", "lecture", "attendee")
	if err := c.Admit(phone); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if got, want := c.ParticipantCount(), 3; got != want {
		t.Fatalf("participants: got %d, want %d", got, want)
	}
	if got, want := c.IdentityCount(), 2; got != want {
		t.Fatalf("identities: got %d, want %d", got, want)
	}
}

func TestBeingInTheConferenceSubscribesNobodyToAnything(t *testing.T) {
	c, _, listener, camera := room(t)
	if got := c.Reception(listener.ID()); len(got) != 0 {
		t.Fatalf("a participant who subscribed to nothing receives %v", got)
	}
	s, err := NewSubscription(listener.ID(), camera, 1)
	if err != nil {
		t.Fatalf("NewSubscription: %v", err)
	}
	if err := c.Subscribe(s); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	got := c.Reception(listener.ID())
	if len(got) != 1 || got[0].Source() != camera.ID() || got[0].Layer() != 1 {
		t.Fatalf("after subscribing, reception is %v", got)
	}
}

func TestASecondSubscriptionForOnePairReplacesTheFirst(t *testing.T) {
	c, _, listener, camera := room(t)
	for _, layer := range []int{2, 0} {
		s, err := NewSubscription(listener.ID(), camera, layer)
		if err != nil {
			t.Fatalf("NewSubscription: %v", err)
		}
		if err := c.Subscribe(s); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}
	got := c.Reception(listener.ID())
	if len(got) != 1 {
		t.Fatalf("one subscriber and one source produced %d entries: %v", len(got), got)
	}
	if got[0].Layer() != 0 {
		t.Fatalf("the layer is %d, so the second subscription did not replace the first", got[0].Layer())
	}
}

func TestReceptionIsOrderedTheSameWayEveryTime(t *testing.T) {
	c, speaker, listener, camera := room(t)
	microphone := source(t, "audio", "speaker", Audio(), 1)
	if err := c.Publish(microphone); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	for _, s := range []Source{camera, microphone} {
		sub, err := NewSubscription(listener.ID(), s, 0)
		if err != nil {
			t.Fatalf("NewSubscription: %v", err)
		}
		if err := c.Subscribe(sub); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}
	// The map the subscriptions live in has no order, so the assertion is that
	// repeated reads agree and that they agree with the source identifiers.
	first := c.Reception(listener.ID())
	for i := 0; i < 8; i++ {
		again := c.Reception(listener.ID())
		if len(again) != len(first) {
			t.Fatalf("read %d returned %d entries, the first returned %d", i, len(again), len(first))
		}
		for j := range again {
			if again[j] != first[j] {
				t.Fatalf("read %d entry %d is %v, the first was %v", i, j, again[j], first[j])
			}
		}
	}
	if first[0].Source() != microphone.ID() || first[1].Source() != camera.ID() {
		t.Fatalf("order is %v, want audio before camera", first)
	}
	if got := c.Reception(speaker.ID()); len(got) != 0 {
		t.Fatalf("the publisher receives %v", got)
	}
}
