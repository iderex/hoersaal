// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// The suite is in the package rather than beside it, for the same reason
// internal/placement gives: internal/arch reads _test.go files too, and nothing
// below reaches for an unexported name, so what the cases exercise is still only
// what a caller can reach.
package mediaport

import (
	"errors"
	"testing"

	"github.com/iderex/hoersaal/internal/domain"
)

func conferenceID(t *testing.T, v string) domain.ConferenceID {
	t.Helper()
	c, err := domain.NewConferenceID(v)
	if err != nil {
		t.Fatalf("conference %q: %v", v, err)
	}
	return c
}

func participantID(t *testing.T, v string) domain.ParticipantID {
	t.Helper()
	p, err := domain.NewParticipantID(v)
	if err != nil {
		t.Fatalf("participant %q: %v", v, err)
	}
	return p
}

func TestAProfileRefusesWhatNoConferenceCouldBeOpenedWith(t *testing.T) {
	for _, c := range []struct {
		name   string
		codecs string
		layers int
		want   error
	}{
		{"no codec set", "", 3, ErrEmpty},
		{"no layers", "speech-and-picture", 0, ErrNoLayer},
		{"a negative arrangement", "speech-and-picture", -1, ErrNoLayer},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewProfile(c.codecs, c.layers); !errors.Is(err, c.want) {
				t.Fatalf("NewProfile(%q, %d) = %v, want %v", c.codecs, c.layers, err, c.want)
			}
		})
	}

	p, err := NewProfile("speech-and-picture", 3)
	if err != nil {
		t.Fatalf("a profile of three layers: %v", err)
	}
	if p.Codecs() != "speech-and-picture" || p.Layers() != 3 {
		t.Fatalf("profile reads back as %q and %d", p.Codecs(), p.Layers())
	}
}

// The opacity the port promises is not a property of the reader, so what a test
// can assert about it is that the value cannot be edited behind the caller's
// back. Both directions are covered because a copy on the way in and no copy on
// the way out leaves the same hole.
func TestTheOpaqueValuesCopyInBothDirections(t *testing.T) {
	given := []byte{1, 2, 3}

	tr := NewTransport(given)
	given[0] = 9
	if tr.Bytes()[0] != 1 {
		t.Fatalf("editing what was handed to NewTransport changed the transport")
	}
	out := tr.Bytes()
	out[1] = 9
	if tr.Bytes()[1] != 2 {
		t.Fatalf("editing what Bytes answered changed the transport")
	}

	given = []byte{4, 5, 6}
	ref := NewReference(given)
	given[0] = 9
	if ref.Bytes()[0] != 4 {
		t.Fatalf("editing what was handed to NewReference changed the reference")
	}
	out = ref.Bytes()
	out[1] = 9
	if ref.Bytes()[1] != 5 {
		t.Fatalf("editing what Bytes answered changed the reference")
	}
}

func TestALinkIdentifierRefusesAnEmptyOne(t *testing.T) {
	if _, err := NewLinkID(""); !errors.Is(err, ErrEmpty) {
		t.Fatalf("NewLinkID(\"\") = %v, want %v", err, ErrEmpty)
	}
	l, err := NewLinkID("a-b-1")
	if err != nil {
		t.Fatalf("NewLinkID: %v", err)
	}
	if l.String() != "a-b-1" {
		t.Fatalf("link reads back as %q", l.String())
	}
}

// A notice that names nothing is the one a control plane cannot act on, so it is
// refused where it is made rather than where it is read.
func TestAFaultNamesWhatWasLost(t *testing.T) {
	c := conferenceID(t, "lecture")
	p := participantID(t, "seat-1")
	l, err := NewLinkID("a-b-1")
	if err != nil {
		t.Fatalf("link: %v", err)
	}

	if _, err := ConferenceFault(domain.ConferenceID{}); !errors.Is(err, ErrNotAFault) {
		t.Fatalf("a conference fault naming no conference = %v", err)
	}
	if _, err := ParticipantFault(c, domain.ParticipantID{}); !errors.Is(err, ErrNotAFault) {
		t.Fatalf("a participant fault naming no participant = %v", err)
	}
	if _, err := ParticipantFault(domain.ConferenceID{}, p); !errors.Is(err, ErrNotAFault) {
		t.Fatalf("a participant fault naming no conference = %v", err)
	}
	if _, err := LinkFault(c, LinkID{}); !errors.Is(err, ErrNotAFault) {
		t.Fatalf("a link fault naming no link = %v", err)
	}

	if got := UnitFault(); got.Kind() != UnitGone() || got.Conference().String() != "" {
		t.Fatalf("a unit fault names %q of conference %q", got.Kind(), got.Conference())
	}

	cf, err := ConferenceFault(c)
	if err != nil {
		t.Fatalf("conference fault: %v", err)
	}
	if cf.Kind() != ConferenceGone() || cf.Conference() != c {
		t.Fatalf("conference fault reads back as %v", cf)
	}

	pf, err := ParticipantFault(c, p)
	if err != nil {
		t.Fatalf("participant fault: %v", err)
	}
	if pf.Kind() != ParticipantGone() || pf.Participant() != p || pf.Conference() != c {
		t.Fatalf("participant fault reads back as %v", pf)
	}

	lf, err := LinkFault(c, l)
	if err != nil {
		t.Fatalf("link fault: %v", err)
	}
	if lf.Kind() != LinkGone() || lf.Link() != l || lf.Conference() != c {
		t.Fatalf("link fault reads back as %v", lf)
	}
}

// Six errors, and each one distinct from the other five. The pair worth the
// assertion is Unavailable against Lost: they are the two a caller is most
// likely to collapse, and the port says collapsing them either tears down live
// conferences or leaves dead ones standing.
func TestTheSixErrorsAreSix(t *testing.T) {
	all := []error{ErrInvalid, ErrUnknown, ErrConflict, ErrRefused, ErrUnavailable, ErrLost}
	for i, a := range all {
		for j, b := range all {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Fatalf("error %d and error %d are the same error", i, j)
			}
		}
	}
}
