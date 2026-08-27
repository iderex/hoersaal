// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package admission_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/admission"
	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/domain"
	"github.com/iderex/hoersaal/internal/mediafake"
	"github.com/iderex/hoersaal/internal/mediaport"
	"github.com/iderex/hoersaal/internal/placement"
	"github.com/iderex/hoersaal/internal/roomcred"
	"github.com/iderex/hoersaal/internal/secret"
	"github.com/iderex/hoersaal/internal/wire"
)

// The bench. Every test builds one of these and changes the one thing it is
// about, so that a failure names the difference rather than the setup.

const (
	theRoom     = "lecture-hall-1"
	theUnitName = "unit-a"
	speaker     = "speaker"
	attendee    = "attendee"
)

var start = time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)

// counter is a Names that answers in order. A test that could not say what the
// next identifier will be could not assert against it.
type counter struct {
	n    int
	fail error
}

func (c *counter) New() (string, error) {
	if c.fail != nil {
		return "", c.fail
	}
	c.n++
	return fmt.Sprintf("id-%d", c.n), nil
}

// roles is a Powers that carries the two names this bench uses. What a role
// bundles is issue #34; this is the answer the desk is handed.
type roles struct{ publishers map[string]bool }

func (r roles) MayPublish(role domain.Role) bool { return r.publishers[role.String()] }

// units resolves a placement identifier to a fake. An identifier absent from it
// is a unit the deployment cannot reach.
type units map[string]mediaport.Unit

func (u units) Unit(id placement.UnitID) (mediaport.Unit, bool) {
	unit, ok := u[id.String()]
	return unit, ok
}

type bench struct {
	t      *testing.T
	desk   *admission.Desk
	room   admission.Room
	unit   *mediafake.Unit
	clock  *clock.Test
	issuer *roomcred.Issuer
	names  *counter
}

type option func(*settings)

type settings struct {
	load      float64
	emptyPool bool
	ceiling   int
	holdUnit  bool
	powers    admission.Powers
	names     *counter
	window    time.Duration
	open      bool
}

// The options, each one thing about the deployment the desk is asked about. The
// placer is always the real one from internal/placement, so a refusal a test
// drives is the refusal that policy actually produces rather than one a double
// asserted into existence.
func withLoad(l float64) option         { return func(s *settings) { s.load = l } }
func withCeiling(n int) option          { return func(s *settings) { s.ceiling = n } }
func withNoUnits() option               { return func(s *settings) { s.emptyPool = true } }
func withAnUnreachableUnit() option     { return func(s *settings) { s.holdUnit = false } }
func withNames(n *counter) option       { return func(s *settings) { s.names = n } }
func withWindow(d time.Duration) option { return func(s *settings) { s.window = d } }
func closed() option                    { return func(s *settings) { s.open = false } }

func newBench(t *testing.T, opts ...option) *bench {
	t.Helper()

	c := clock.NewTest(start)
	unit, err := mediafake.NewFabric().Add(theUnitName, c)
	if err != nil {
		t.Fatalf("the fake unit: %v", err)
	}
	conference := conferenceID(t, theRoom)
	profile, err := mediaport.NewProfile("opus,vp8", 3)
	if err != nil {
		t.Fatalf("the profile: %v", err)
	}
	if err := unit.OpenConference(conference, profile); err != nil {
		t.Fatalf("opening the conference on the fake: %v", err)
	}

	unitID, err := placement.NewUnitID(theUnitName)
	if err != nil {
		t.Fatalf("the unit identifier: %v", err)
	}

	s := settings{
		load:     0.1,
		ceiling:  4,
		holdUnit: true,
		powers:   roles{publishers: map[string]bool{speaker: true}},
		names:    &counter{},
		open:     true,
	}
	for _, o := range opts {
		o(&s)
	}

	held := units{}
	if s.holdUnit {
		held[theUnitName] = unit
	}

	key := secret.Bytes(strings.Repeat("k", roomcred.MinKeyBytes))
	issuer, err := roomcred.NewIssuer(key)
	if err != nil {
		t.Fatalf("the issuer: %v", err)
	}
	verifier, err := roomcred.NewVerifier(key, c)
	if err != nil {
		t.Fatalf("the verifier: %v", err)
	}

	desk, err := admission.NewDesk(verifier, s.powers, placement.Naive{}, held, s.names, c, s.window)
	if err != nil {
		t.Fatalf("the desk: %v", err)
	}

	carrying, err := placement.NewCarrying(unitID, 1, 0)
	if err != nil {
		t.Fatalf("the carrying record: %v", err)
	}
	record, err := placement.NewConference(conference, s.ceiling, carrying)
	if err != nil {
		t.Fatalf("the placement record: %v", err)
	}
	pool, err := poolOf(unitID, s.load, s.emptyPool, c)
	if err != nil {
		t.Fatalf("the pool: %v", err)
	}

	return &bench{
		t:      t,
		desk:   desk,
		room:   admission.Room{ID: conference, Open: s.open, Pool: pool, Placement: record},
		unit:   unit,
		clock:  c,
		issuer: issuer,
		names:  s.names,
	}
}

func poolOf(id placement.UnitID, load float64, empty bool, c *clock.Test) (placement.Pool, error) {
	if empty {
		return placement.NewPool()
	}
	unit, err := placement.NewUnit(id, load, c.Now(), placement.Admitting())
	if err != nil {
		return placement.Pool{}, err
	}
	return placement.NewPool(unit)
}

func conferenceID(t *testing.T, v string) domain.ConferenceID {
	t.Helper()
	id, err := domain.NewConferenceID(v)
	if err != nil {
		t.Fatalf("the conference identifier: %v", err)
	}
	return id
}

// credential mints one for this bench's room, with the role named.
func (b *bench) credential(role string) string {
	b.t.Helper()
	token, err := b.issuer.Issue(roomcred.Claims{
		Conference: theRoom,
		Subject:    "identity-7",
		Role:       role,
		NotBefore:  start.Add(-time.Minute),
		Expires:    start.Add(time.Hour),
	})
	if err != nil {
		b.t.Fatalf("issuing a credential: %v", err)
	}
	return token
}

// request builds the message a client sends.
func (b *bench) request(credential string, publishing bool, offers ...map[string]any) wire.Message {
	b.t.Helper()
	body := map[string]any{
		"conference": theRoom,
		"credential": credential,
		"publishing": publishing,
	}
	if len(offers) > 0 {
		body["offers"] = offers
	}
	payload, err := json.Marshal(body)
	if err != nil {
		b.t.Fatalf("encoding the request: %v", err)
	}
	return wire.Message{Type: admission.TypeRequest, Payload: payload}
}

func video(layers int) map[string]any { return map[string]any{"kind": "video", "layers": layers} }

// admit runs one attempt and fails the test where the desk itself faulted.
func (b *bench) admit(m wire.Message) admission.Outcome {
	b.t.Helper()
	out, err := b.desk.Admit(b.room, m)
	if err != nil {
		b.t.Fatalf("the desk faulted where it should have answered: %v", err)
	}
	return out
}

// refusedAs reads the reason out of the message the client would receive, which
// is the thing the client actually acts on, rather than out of the outcome.
func refusedAs(t *testing.T, out admission.Outcome) string {
	t.Helper()
	m := out.Message()
	if m.Type != admission.TypeRefused {
		t.Fatalf("the message is %q and a refusal is %q", m.Type, admission.TypeRefused)
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(m.Payload, &body); err != nil {
		t.Fatalf("the refusal payload: %v", err)
	}
	return body.Reason
}

// The sequence, end to end.

func TestAnAttendeeIsAdmittedAsASubscriberAndNothingElse(t *testing.T) {
	b := newBench(t)

	out := b.admit(b.request(b.credential(attendee), false))

	grant, ok := out.Granted()
	if !ok {
		t.Fatalf("refused with %q and this credential is good", refusedAs(t, out))
	}
	if out.Message().Type != admission.TypeGranted {
		t.Errorf("the message is %q", out.Message().Type)
	}
	if grant.Publishing {
		t.Error("the grant says publishing and the request did not ask to")
	}
	if got := len(b.unit.Subscribers(b.room.ID)); got != 1 {
		t.Errorf("the unit holds %d subscribers, want 1", got)
	}
	if got := b.unit.Publishers(b.room.ID); len(got) != 0 {
		t.Errorf("the unit holds publishers %v, and nobody asked to publish", got)
	}
	if len(grant.Transport.Bytes()) == 0 {
		t.Error("the grant carries no transport parameters, so the client has nothing to connect with")
	}
	if grant.Identity.String() != "identity-7" {
		t.Errorf("the identity is %q, want the subject the credential carried", grant.Identity.String())
	}
	if grant.Participant.String() != "id-1" {
		t.Errorf("the participant is %q and the first identifier this bench mints is id-1", grant.Participant.String())
	}
}

func TestASpeakerIsAdmittedAsAPublisherWithTheSourcesTheyOffered(t *testing.T) {
	b := newBench(t)

	out := b.admit(b.request(b.credential(speaker), true, video(3)))

	grant, ok := out.Granted()
	if !ok {
		t.Fatalf("refused with %q", refusedAs(t, out))
	}
	if !grant.Publishing {
		t.Error("the grant does not say publishing")
	}
	if got := len(grant.Sources); got != 1 {
		t.Fatalf("the grant carries %d sources, want the one that was offered", got)
	}
	if got := grant.Sources[0].Publisher(); got != grant.Participant {
		t.Errorf("the source names %q as its publisher and the participant is %q", got.String(), grant.Participant.String())
	}
	if got := len(b.unit.Publishers(b.room.ID)); got != 1 {
		t.Errorf("the unit holds %d publishers, want 1", got)
	}
	if got := len(b.unit.Sources(b.room.ID)); got != 1 {
		t.Errorf("the unit holds %d sources, want the one that was offered", got)
	}
}

func TestEveryParticipantGetsItsOwnIdentifier(t *testing.T) {
	b := newBench(t)

	first, ok := b.admit(b.request(b.credential(attendee), false)).Granted()
	if !ok {
		t.Fatal("the first admission was refused")
	}
	second, ok := b.admit(b.request(b.credential(attendee), false)).Granted()
	if !ok {
		t.Fatal("the second admission was refused")
	}
	if first.Participant == second.Participant {
		t.Errorf("both admissions minted %q, and one person on two devices is two participants", first.Participant.String())
	}
}

// The condition about power. The media plane credential carries no power the
// room credential did not.

func TestAListenerWhoAsksToPublishIsRefusedAndNoUnitIsToldAnything(t *testing.T) {
	b := newBench(t)

	out := b.admit(b.request(b.credential(attendee), true, video(3)))

	if _, granted := out.Granted(); granted {
		t.Fatal("a credential whose role may not publish was admitted as a publisher")
	}
	if got := refusedAs(t, out); got != admission.NotPermitted().String() {
		t.Errorf("the client is told %q, want %q", got, admission.NotPermitted())
	}
	if got := b.unit.Publishers(b.room.ID); len(got) != 0 {
		t.Errorf("the unit was made ready to accept a publication from %v", got)
	}
	if got := b.unit.Subscribers(b.room.ID); len(got) != 0 {
		t.Errorf("the unit was told about %v, and the refusal is meant to come before any unit is told anything", got)
	}
	if got := b.desk.Outstanding(); got != 0 {
		t.Errorf("%d admissions are outstanding after a refusal", got)
	}
}

func TestAListenerAdmittedAsASubscriberCannotBeMadeAPublisherByTheirOwnRequest(t *testing.T) {
	b := newBench(t)

	// The same credential, the same room, and the only difference is what the
	// client asked for. The one that asks for less is granted and the one that
	// asks for more is refused, which is the whole of what this exchange
	// promises about the two.
	if _, ok := b.admit(b.request(b.credential(attendee), false)).Granted(); !ok {
		t.Fatal("the listening request was refused")
	}
	if _, ok := b.admit(b.request(b.credential(attendee), true, video(1))).Granted(); ok {
		t.Fatal("the publishing request was granted on the same credential")
	}
	if got := b.unit.Publishers(b.room.ID); len(got) != 0 {
		t.Errorf("the unit holds publishers %v", got)
	}
}

// The refusals, one per reason, read as the client reads them.

func TestEveryRefusalReachesTheClientAsItsOwnCode(t *testing.T) {
	for _, c := range []struct {
		name  string
		want  admission.Reason
		drive func(*testing.T) admission.Outcome
	}{
		{
			name: "a message that is not a request",
			want: admission.MalformedRequest(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t)
				return b.admit(wire.Message{Type: "something.else", Payload: json.RawMessage(`{}`)})
			},
		},
		{
			name: "a credential this installation did not sign",
			want: admission.CredentialRefused(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t)
				return b.admit(b.request("not-a-credential", false))
			},
		},
		{
			name: "a role that may not publish asking to",
			want: admission.NotPermitted(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t)
				return b.admit(b.request(b.credential(attendee), true, video(1)))
			},
		},
		{
			name: "a room that is not taking participants",
			want: admission.ConferenceNotOpen(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t, closed())
				return b.admit(b.request(b.credential(attendee), false))
			},
		},
		{
			name: "a deployment with no units at all",
			want: admission.NoCapacity(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t, withNoUnits())
				return b.admit(b.request(b.credential(attendee), false))
			},
		},
		{
			name: "a pool holding nothing eligible",
			want: admission.RoomFull(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t, withLoad(0.95))
				return b.admit(b.request(b.credential(attendee), false))
			},
		},
		{
			name: "a conference already on as many units as it may occupy",
			want: admission.ConferenceAtItsUnitCeiling(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t, withLoad(0.95), withCeiling(1))
				return b.admit(b.request(b.credential(attendee), false))
			},
		},
		{
			name: "a unit that will not admit another subscriber",
			want: admission.UnitRefused(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t)
				b.unit.Refuse(mediafake.AdmittingASubscriber())
				return b.admit(b.request(b.credential(attendee), false))
			},
		},
		{
			name: "a unit that does not answer",
			want: admission.UnitUnavailable(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t)
				b.unit.Die()
				return b.admit(b.request(b.credential(attendee), false))
			},
		},
		{
			name: "a unit the placer named and the deployment cannot reach",
			want: admission.UnitUnavailable(),
			drive: func(t *testing.T) admission.Outcome {
				b := newBench(t, withAnUnreachableUnit())
				return b.admit(b.request(b.credential(attendee), false))
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := c.drive(t)
			if _, granted := out.Granted(); granted {
				t.Fatalf("granted, and this case is meant to be refused with %q", c.want)
			}
			if got, ok := out.Refused(); !ok || got != c.want {
				t.Errorf("the outcome carries %q, want %q", got, c.want)
			}
			if got := refusedAs(t, out); got != c.want.String() {
				t.Errorf("the client is told %q, want %q", got, c.want)
			}
		})
	}
}

func TestTheReasonsAreDistinctAndReasonsHoldsThemAll(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range admission.Reasons {
		if r.String() == "" {
			t.Error("a reason with no code on the wire")
		}
		if seen[r.String()] {
			t.Errorf("two reasons share the code %q, so a client cannot tell them apart", r)
		}
		seen[r.String()] = true
	}

	// Every reason this package can produce is in the list, and the list holds no
	// reason it cannot. The set below is the one the case table above drives, so
	// a tenth reason added to the code with no case fails here rather than
	// reaching a client as a code nothing describes.
	produced := []admission.Reason{
		admission.MalformedRequest(),
		admission.CredentialRefused(),
		admission.NotPermitted(),
		admission.ConferenceNotOpen(),
		admission.NoCapacity(),
		admission.RoomFull(),
		admission.ConferenceAtItsUnitCeiling(),
		admission.UnitRefused(),
		admission.UnitUnavailable(),
	}
	if len(produced) != len(admission.Reasons) {
		t.Fatalf("Reasons holds %d and %d are produced", len(admission.Reasons), len(produced))
	}
	for _, r := range produced {
		if !seen[r.String()] {
			t.Errorf("%q is produced and is not in Reasons", r)
		}
	}
}

func TestACredentialForAnotherRoomIsRefusedWithoutSayingWhichPartWasWrong(t *testing.T) {
	b := newBench(t)

	other, err := b.issuer.Issue(roomcred.Claims{
		Conference: "another-room",
		Subject:    "identity-7",
		Role:       speaker,
		NotBefore:  start.Add(-time.Minute),
		Expires:    start.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	// The message names this room and the credential names another, which is the
	// edit a forger makes. It is one refusal with the expired case and the
	// forged-signature case, which is what stops the refusal being an oracle.
	expired, err := b.issuer.Issue(roomcred.Claims{
		Conference: theRoom,
		Subject:    "identity-7",
		Role:       speaker,
		NotBefore:  start.Add(-2 * time.Hour),
		Expires:    start.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	for _, token := range []string{other, expired, "not-a-credential", ""} {
		out := b.admit(b.request(token, false))
		if _, granted := out.Granted(); granted {
			t.Fatalf("a credential this room should not accept was admitted")
		}
		got := refusedAs(t, out)
		// An empty credential never reaches the verifier, so it is malformed
		// rather than refused; every other shape is one code.
		if token == "" {
			if got != admission.MalformedRequest().String() {
				t.Errorf("an empty credential is told %q", got)
			}
			continue
		}
		if got != admission.CredentialRefused().String() {
			t.Errorf("the client is told %q, want %q", got, admission.CredentialRefused())
		}
	}
}

func TestARequestThisExchangeCannotReadIsRefusedRatherThanPartlyRead(t *testing.T) {
	b := newBench(t)
	good := b.credential(attendee)

	for _, c := range []struct {
		name    string
		payload string
	}{
		{"a member this exchange does not have", `{"conference":"lecture-hall-1","credential":"C","publishing":false,"role":"speaker"}`},
		{"sources offered by somebody who is not publishing", `{"conference":"lecture-hall-1","credential":"C","publishing":false,"offers":[{"kind":"video","layers":3}]}`},
		{"publishing with nothing offered", `{"conference":"lecture-hall-1","credential":"C","publishing":true}`},
		{"another room named in the message", `{"conference":"another-room","credential":"C","publishing":false}`},
		{"no conference at all", `{"credential":"C","publishing":false}`},
		{"a second object after the first", `{"conference":"lecture-hall-1","credential":"C"}{"conference":"lecture-hall-1","credential":"C"}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := wire.Message{Type: admission.TypeRequest, Payload: json.RawMessage(strings.ReplaceAll(c.payload, `"C"`, `"`+good+`"`))}
			out := b.admit(m)
			if _, granted := out.Granted(); granted {
				t.Fatal("granted")
			}
			if got := refusedAs(t, out); got != admission.MalformedRequest().String() {
				t.Errorf("the client is told %q, want %q", got, admission.MalformedRequest())
			}
		})
	}
}

func TestAnOfferTheModelRefusesReachesNoUnit(t *testing.T) {
	b := newBench(t)

	// A kind that is neither audio nor video, and a layer arrangement a source
	// may not have. Both are read out of the client's own message, so both are
	// malformed rather than a fault of this deployment.
	for _, offer := range []map[string]any{
		{"kind": "slides", "layers": 1},
		{"kind": "video", "layers": 0},
	} {
		out := b.admit(b.request(b.credential(speaker), true, offer))
		if _, granted := out.Granted(); granted {
			t.Fatalf("an offer of %v was admitted", offer)
		}
		if got := refusedAs(t, out); got != admission.MalformedRequest().String() {
			t.Errorf("the client is told %q for %v", got, offer)
		}
	}
	if got := b.unit.Publishers(b.room.ID); len(got) != 0 {
		t.Errorf("the unit holds publishers %v after two refused offers", got)
	}
}

// The condition about an admission nobody completes.

func TestAClientDroppedBetweenTheStepsIsSweptAndTheDeskStopsBelievingInIt(t *testing.T) {
	b := newBench(t, withWindow(time.Minute))

	grant, ok := b.admit(b.request(b.credential(attendee), false)).Granted()
	if !ok {
		t.Fatal("the admission was refused")
	}
	if got := b.desk.Outstanding(); got != 1 {
		t.Fatalf("%d admissions outstanding after one grant, want 1", got)
	}

	// The client is dropped here: it never connects to the unit, so Arrived is
	// never called.
	b.clock.Advance(time.Minute - time.Nanosecond)
	if got := b.desk.Sweep(); len(got) != 0 {
		t.Errorf("swept %d admissions before the deadline", len(got))
	}

	b.clock.Advance(time.Nanosecond)
	swept := b.desk.Sweep()
	if len(swept) != 1 {
		t.Fatalf("swept %d admissions at the deadline, want 1", len(swept))
	}
	if swept[0].Participant != grant.Participant {
		t.Errorf("swept %q and the grant was %q", swept[0].Participant.String(), grant.Participant.String())
	}
	if swept[0].Unit.String() != theUnitName {
		t.Errorf("the swept admission names unit %q, and whoever releases it needs the right one", swept[0].Unit.String())
	}
	if got := b.desk.Outstanding(); got != 0 {
		t.Errorf("%d admissions outstanding after the sweep", got)
	}
	if got := b.desk.Sweep(); len(got) != 0 {
		t.Errorf("a second sweep answered %d, and each abandoned admission is reported once", len(got))
	}
}

func TestTheUnitStillHoldsASweptAdmissionAndCloseConferenceIsWhatReleasesIt(t *testing.T) {
	b := newBench(t, withWindow(time.Minute))

	if _, ok := b.admit(b.request(b.credential(attendee), false)).Granted(); !ok {
		t.Fatal("the admission was refused")
	}
	b.clock.Advance(time.Minute)
	if len(b.desk.Sweep()) != 1 {
		t.Fatal("nothing was swept")
	}

	// The negative half, asserted rather than described. The port has eight
	// operations and none of them releases one participant, so the unit's side
	// of a swept admission is still there.
	if got := len(b.unit.Subscribers(b.room.ID)); got != 1 {
		t.Errorf("the unit holds %d subscribers after the sweep, and nothing here can release one", got)
	}

	// What does release it is the conference ending, which is the bound the
	// package comment states.
	if err := b.unit.CloseConference(b.room.ID); err != nil {
		t.Fatalf("closing the conference: %v", err)
	}
	if got := len(b.unit.Subscribers(b.room.ID)); got != 0 {
		t.Errorf("the unit holds %d subscribers after the conference closed", got)
	}
}

func TestAClientThatArrivesIsNotSwept(t *testing.T) {
	b := newBench(t, withWindow(time.Minute))

	grant, ok := b.admit(b.request(b.credential(attendee), false)).Granted()
	if !ok {
		t.Fatal("the admission was refused")
	}
	if !b.desk.Arrived(grant.Participant) {
		t.Error("the desk was not holding the admission it had just granted")
	}
	if b.desk.Arrived(grant.Participant) {
		t.Error("a second arrival for one admission was accepted as a first")
	}

	b.clock.Advance(time.Hour)
	if got := b.desk.Sweep(); len(got) != 0 {
		t.Errorf("swept %d admissions and the client had arrived", len(got))
	}
}

func TestARefusalLeavesNothingOutstanding(t *testing.T) {
	b := newBench(t, withLoad(0.95))

	if _, granted := b.admit(b.request(b.credential(attendee), false)).Granted(); granted {
		t.Fatal("granted")
	}
	if got := b.desk.Outstanding(); got != 0 {
		t.Errorf("%d admissions outstanding after a refusal", got)
	}
	b.clock.Advance(time.Hour)
	if got := b.desk.Sweep(); len(got) != 0 {
		t.Errorf("a refusal produced %d abandoned admissions", len(got))
	}
}

func TestAUnitThatFailedAfterBeingAskedLeavesNothingOutstanding(t *testing.T) {
	b := newBench(t)
	b.unit.Refuse(mediafake.AdmittingASubscriber())

	if _, granted := b.admit(b.request(b.credential(attendee), false)).Granted(); granted {
		t.Fatal("granted")
	}
	if got := b.desk.Outstanding(); got != 0 {
		t.Errorf("%d admissions outstanding, and the unit refused so there is nothing to reclaim", got)
	}
}

// The faults, which are the only thing that comes back as an error.

func TestADeskMissingASeamIsRefusedWhenItIsBuilt(t *testing.T) {
	c := clock.NewTest(start)
	key := secret.Bytes(strings.Repeat("k", roomcred.MinKeyBytes))
	v, err := roomcred.NewVerifier(key, c)
	if err != nil {
		t.Fatalf("the verifier: %v", err)
	}
	p := roles{}
	pl := placement.Naive{}
	u := units{}
	n := &counter{}

	for _, c2 := range []struct {
		name string
		make func() (*admission.Desk, error)
	}{
		{"no verifier", func() (*admission.Desk, error) { return admission.NewDesk(nil, p, pl, u, n, c, 0) }},
		{"no powers", func() (*admission.Desk, error) { return admission.NewDesk(v, nil, pl, u, n, c, 0) }},
		{"no placer", func() (*admission.Desk, error) { return admission.NewDesk(v, p, nil, u, n, c, 0) }},
		{"no units", func() (*admission.Desk, error) { return admission.NewDesk(v, p, pl, nil, n, c, 0) }},
		{"no names", func() (*admission.Desk, error) { return admission.NewDesk(v, p, pl, u, nil, c, 0) }},
		{"no clock", func() (*admission.Desk, error) { return admission.NewDesk(v, p, pl, u, n, nil, 0) }},
		{"a negative window", func() (*admission.Desk, error) { return admission.NewDesk(v, p, pl, u, n, c, -time.Second) }},
	} {
		t.Run(c2.name, func(t *testing.T) {
			if _, err := c2.make(); !errors.Is(err, admission.ErrNoSeam) {
				t.Errorf("the error is %v, want one wrapping ErrNoSeam", err)
			}
		})
	}
}

func TestAnIdentifierSourceThatFailsIsAFaultRatherThanARefusal(t *testing.T) {
	b := newBench(t, withNames(&counter{fail: errors.New("the source is empty")}))

	_, err := b.desk.Admit(b.room, b.request(b.credential(attendee), false))
	if !errors.Is(err, admission.ErrNames) {
		t.Fatalf("the error is %v, want one wrapping ErrNames", err)
	}
	if got := len(b.unit.Subscribers(b.room.ID)); got != 0 {
		t.Errorf("the unit was told about %d participants and no identifier could be minted", got)
	}
}

func TestARoomRecordThatDoesNotMatchTheRoomIsAFault(t *testing.T) {
	b := newBench(t)

	elsewhere, err := placement.NewConference(conferenceID(t, "another-room"), 4)
	if err != nil {
		t.Fatalf("the placement record: %v", err)
	}
	room := b.room
	room.Placement = elsewhere

	if _, err := b.desk.Admit(room, b.request(b.credential(attendee), false)); !errors.Is(err, admission.ErrRoom) {
		t.Errorf("the error is %v, want one wrapping ErrRoom", err)
	}
}

// The join storm, at the size the lecture actually has, so that the exchange is
// exercised at its own scale rather than at one.

func TestThreeHundredJoinsLeaveThreeHundredOutstandingAdmissions(t *testing.T) {
	b := newBench(t, withWindow(time.Minute))

	const room = 300
	for i := 0; i < room; i++ {
		if _, ok := b.admit(b.request(b.credential(attendee), false)).Granted(); !ok {
			t.Fatalf("join %d was refused", i)
		}
	}
	if got := b.desk.Outstanding(); got != room {
		t.Fatalf("%d admissions outstanding, want %d", got, room)
	}
	if got := len(b.unit.Subscribers(b.room.ID)); got != room {
		t.Errorf("the unit holds %d subscribers, want %d", got, room)
	}

	b.clock.Advance(time.Minute)
	if got := len(b.desk.Sweep()); got != room {
		t.Errorf("swept %d, want %d", got, room)
	}
}
