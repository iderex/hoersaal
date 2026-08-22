// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package presence_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/domain"
	"github.com/iderex/hoersaal/internal/presence"
	"github.com/iderex/hoersaal/internal/wire"
)

// start is what every test clock here reads before it is advanced. It is a fixed
// instant rather than the machine's, which is the rule internal/guard refuses
// the other side of.
var start = time.Date(2026, time.August, 8, 9, 0, 0, 0, time.UTC)

// entry builds one participant with identifiers of a fixed width, so that two
// rooms of different sizes differ in how many entries they hold and not in how
// long each one is. The size property below is only readable if that is true.
func entry(t *testing.T, n int) presence.Entry {
	t.Helper()
	return entryNamed(t, fmt.Sprintf("p%05d", n), fmt.Sprintf("i%05d", n))
}

func entryNamed(t *testing.T, participant, identity string) presence.Entry {
	t.Helper()
	pid, err := domain.NewParticipantID(participant)
	if err != nil {
		t.Fatalf("participant identifier %q: %v", participant, err)
	}
	iid, err := domain.NewIdentityID(identity)
	if err != nil {
		t.Fatalf("identity identifier %q: %v", identity, err)
	}
	role, err := domain.NewRole("attendee")
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	e, err := presence.NewEntry(pid, iid, role)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	return e
}

// roomOf builds a roll of n participants with the first MaxAttending of them
// attending, which is the shape a lecture has: a handful of people the room is
// looking at and everybody else listening.
func roomOf(t *testing.T, n int) presence.Roll {
	t.Helper()
	entries := make([]presence.Entry, 0, n)
	for i := range n {
		entries = append(entries, entry(t, i))
	}
	attending := make([]domain.ParticipantID, 0, presence.MaxAttending)
	for i := 0; i < presence.MaxAttending && i < n; i++ {
		attending = append(attending, entries[i].ID())
	}
	roll, err := presence.NewRoll(entries, attending)
	if err != nil {
		t.Fatalf("roll of %d: %v", n, err)
	}
	return roll
}

func encodedSummary(t *testing.T, roll presence.Roll, revision uint64) []byte {
	t.Helper()
	m, err := roll.Summary(revision).Message()
	if err != nil {
		t.Fatalf("summary message: %v", err)
	}
	b, err := wire.Encode(m)
	if err != nil {
		t.Fatalf("encoding the summary: %v", err)
	}
	return b
}

func TestAnEntryRefusesAnEmptyValue(t *testing.T) {
	pid, err := domain.NewParticipantID("p")
	if err != nil {
		t.Fatalf("participant: %v", err)
	}
	iid, err := domain.NewIdentityID("i")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	role, err := domain.NewRole("attendee")
	if err != nil {
		t.Fatalf("role: %v", err)
	}

	for _, c := range []struct {
		name     string
		id       domain.ParticipantID
		identity domain.IdentityID
		role     domain.Role
	}{
		{"no participant", domain.ParticipantID{}, iid, role},
		{"no identity", pid, domain.IdentityID{}, role},
		{"no role", pid, iid, domain.Role{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := presence.NewEntry(c.id, c.identity, c.role); !errors.Is(err, presence.ErrEmpty) {
				t.Fatalf("wanted ErrEmpty, got %v", err)
			}
		})
	}
}

func TestARollRefusesTheSameParticipantTwice(t *testing.T) {
	e := entry(t, 1)
	if _, err := presence.NewRoll([]presence.Entry{e, e}, nil); !errors.Is(err, presence.ErrDuplicate) {
		t.Fatalf("wanted ErrDuplicate, got %v", err)
	}
}

// A roll that quietly dropped a name it could not resolve would produce a
// summary describing a room somebody had already left, and the client would have
// no way to tell that from a room they were still in.
func TestARollRefusesAnAttendingParticipantItDoesNotHold(t *testing.T) {
	held := entry(t, 1)
	absent := entry(t, 2)
	_, err := presence.NewRoll([]presence.Entry{held}, []domain.ParticipantID{absent.ID()})
	if !errors.Is(err, presence.ErrUnknown) {
		t.Fatalf("wanted ErrUnknown, got %v", err)
	}
}

func TestARollRefusesTheSameAttendingParticipantTwice(t *testing.T) {
	e := entry(t, 1)
	_, err := presence.NewRoll([]presence.Entry{e}, []domain.ParticipantID{e.ID(), e.ID()})
	if !errors.Is(err, presence.ErrDuplicate) {
		t.Fatalf("wanted ErrDuplicate, got %v", err)
	}
}

// The first condition on issue #37: the presence message does not grow with the
// number of participants. It is asserted on the encoded bytes rather than on the
// structure, because a structure that is the same shape can still encode to more
// bytes, and the bytes are what the join storm pays for.
//
// The rooms below differ by three orders of magnitude and the identifiers in
// them are all the same width, so the only thing that may move is the decimal
// width of the two counts: 8 to 8888 is three more digits in each of
// participants and identities, which is six bytes. Anything above that is the
// message growing with the room.
func TestTheSummaryIsTheSameSizeWhateverTheRoomHolds(t *testing.T) {
	const roomsDiffer = 6

	sizes := []int{8, 88, 888, 8888}
	lengths := make([]int, 0, len(sizes))
	for _, n := range sizes {
		b := encodedSummary(t, roomOf(t, n), 1)
		lengths = append(lengths, len(b))
		t.Logf("a room of %d encodes to %d bytes", n, len(b))
	}

	smallest, largest := lengths[0], lengths[0]
	for _, l := range lengths {
		if l < smallest {
			smallest = l
		}
		if l > largest {
			largest = l
		}
	}
	if largest-smallest > roomsDiffer {
		t.Fatalf("the summary grew by %d bytes across rooms of %v, which is more than the %d the counts can account for", largest-smallest, sizes, roomsDiffer)
	}
}

func TestTheSummaryNamesAtMostTheCapAndSaysHowManyThereWere(t *testing.T) {
	const attendingCount = presence.MaxAttending + 5

	entries := make([]presence.Entry, 0, attendingCount)
	attending := make([]domain.ParticipantID, 0, attendingCount)
	for i := range attendingCount {
		e := entry(t, i)
		entries = append(entries, e)
		attending = append(attending, e.ID())
	}
	roll, err := presence.NewRoll(entries, attending)
	if err != nil {
		t.Fatalf("roll: %v", err)
	}

	s := roll.Summary(1)
	if got := len(s.Attending()); got != presence.MaxAttending {
		t.Fatalf("the summary named %d participants, wanted the cap of %d", got, presence.MaxAttending)
	}
	if got := s.AttendingTotal(); got != attendingCount {
		t.Fatalf("the summary reported %d attending, wanted %d", got, attendingCount)
	}
}

// The order attending arrives in is the caller's priority, and the cap keeps the
// front of it rather than an arbitrary slice, because the caller is the only
// thing here that knows who matters most.
func TestTheCapKeepsTheFrontOfTheOrderItWasGiven(t *testing.T) {
	const total = presence.MaxAttending + 3

	entries := make([]presence.Entry, 0, total)
	for i := range total {
		entries = append(entries, entry(t, i))
	}
	reversed := make([]domain.ParticipantID, 0, total)
	for i := total - 1; i >= 0; i-- {
		reversed = append(reversed, entries[i].ID())
	}
	roll, err := presence.NewRoll(entries, reversed)
	if err != nil {
		t.Fatalf("roll: %v", err)
	}

	named := roll.Summary(1).Attending()
	for i, e := range named {
		want := entries[total-1-i].ID()
		if e.ID() != want {
			t.Fatalf("position %d named %s, wanted %s", i, e.ID(), want)
		}
	}
}

// The second condition on issue #37, first half: a burst of joins produces a
// bounded number of messages per client. Three hundred people arriving inside
// one window is one summary, and the naive shape it replaces is three hundred.
func TestABurstOfJoinsInsideOneWindowCostsOneSummary(t *testing.T) {
	const joins = 300

	c := clock.NewTest(start)
	e, err := presence.NewEmitter(c, presence.Window)
	if err != nil {
		t.Fatalf("emitter: %v", err)
	}

	summaries := 0
	for range joins {
		e.Note()
		if _, due := e.Take(); due {
			summaries++
		}
	}
	if summaries != 1 {
		t.Fatalf("%d joins inside one window produced %d summaries, wanted 1", joins, summaries)
	}
	t.Logf("%d joins inside one window: %d summaries per client, against %d without coalescing", joins, summaries, joins)
}

// The second half of the same condition. What bounds the traffic is elapsed time
// and never the number of changes, so a storm spread over a stated stretch costs
// at most one summary per window in it, whatever arrives.
func TestChangesSpreadOverTimeAreBoundedByTheElapsedTime(t *testing.T) {
	const (
		joins   = 300
		between = 300 * time.Millisecond
	)
	elapsed := time.Duration(joins) * between
	bound := int(elapsed/presence.Window) + 1

	c := clock.NewTest(start)
	e, err := presence.NewEmitter(c, presence.Window)
	if err != nil {
		t.Fatalf("emitter: %v", err)
	}

	summaries := 0
	for range joins {
		e.Note()
		if _, due := e.Take(); due {
			summaries++
		}
		c.Advance(between)
	}
	if summaries > bound {
		t.Fatalf("%d joins over %s produced %d summaries, which is over the bound of %d", joins, elapsed, summaries, bound)
	}
	if summaries >= joins {
		t.Fatalf("%d joins produced %d summaries, which is no better than sending one per join", joins, summaries)
	}
	t.Logf("%d joins over %s: %d summaries per client, bound %d, without coalescing %d", joins, elapsed, summaries, bound, joins)
}

// A window in which nothing happened is not a message. This is the difference
// between a minimum gap and a period, and it is what stops an idle room of three
// hundred costing anything at all.
func TestAWindowInWhichNothingChangedProducesNoSummary(t *testing.T) {
	c := clock.NewTest(start)
	e, err := presence.NewEmitter(c, presence.Window)
	if err != nil {
		t.Fatalf("emitter: %v", err)
	}

	e.Note()
	if _, due := e.Take(); !due {
		t.Fatal("the first change was not due")
	}
	for range 100 {
		c.Advance(presence.Window)
		if r, due := e.Take(); due {
			t.Fatalf("an idle room produced summary %d", r)
		}
	}
}

func TestARevisionRisesByOnePerSummary(t *testing.T) {
	c := clock.NewTest(start)
	e, err := presence.NewEmitter(c, presence.Window)
	if err != nil {
		t.Fatalf("emitter: %v", err)
	}

	for want := uint64(1); want <= 5; want++ {
		e.Note()
		got, due := e.Take()
		if !due {
			t.Fatalf("summary %d was not due", want)
		}
		if got != want {
			t.Fatalf("revision %d, wanted %d", got, want)
		}
		c.Advance(presence.Window)
	}
}

// Wait is what a caller schedules against, so it has to answer for the change it
// is holding rather than for the clock alone.
func TestWaitReportsWhatIsLeftOfTheWindow(t *testing.T) {
	c := clock.NewTest(start)
	e, err := presence.NewEmitter(c, presence.Window)
	if err != nil {
		t.Fatalf("emitter: %v", err)
	}

	if _, waiting := e.Wait(); waiting {
		t.Fatal("an emitter with nothing pending reported a wait")
	}
	e.Note()
	if _, due := e.Take(); !due {
		t.Fatal("the first change was not due")
	}

	c.Advance(presence.Window / 5)
	e.Note()
	left, waiting := e.Wait()
	if !waiting {
		t.Fatal("a pending change reported no wait")
	}
	if want := presence.Window - presence.Window/5; left != want {
		t.Fatalf("wait reported %s, wanted %s", left, want)
	}
}

func TestAnEmitterRefusesAWindowThatIsNotPositive(t *testing.T) {
	c := clock.NewTest(start)
	for _, d := range []time.Duration{0, -presence.Window} {
		if _, err := presence.NewEmitter(c, d); !errors.Is(err, presence.ErrWindow) {
			t.Fatalf("a window of %s: wanted ErrWindow, got %v", d, err)
		}
	}
	if _, err := presence.NewEmitter(nil, presence.Window); !errors.Is(err, presence.ErrWindow) {
		t.Fatalf("no clock: wanted ErrWindow, got %v", err)
	}
}

// The third condition on issue #37: the full list is a paged query with a stated
// maximum page, and the maximum is refused rather than quietly adjusted.
func TestAPageIsRefusedOutsideTheStatedMaximum(t *testing.T) {
	roll := roomOf(t, 300)
	for _, size := range []int{0, -1, presence.MaxPage + 1} {
		if _, err := roll.Page("", size); !errors.Is(err, presence.ErrPageSize) {
			t.Fatalf("a page size of %d: wanted ErrPageSize, got %v", size, err)
		}
	}
}

func TestWalkingThePagesReachesEveryParticipantExactlyOnce(t *testing.T) {
	const room = 300

	roll := roomOf(t, room)
	seen := map[string]int{}
	cursor := ""
	pages := 0
	for {
		p, err := roll.Page(cursor, presence.MaxPage)
		if err != nil {
			t.Fatalf("page after %q: %v", cursor, err)
		}
		pages++
		if p.Total() != room {
			t.Fatalf("page %d reported a total of %d, wanted %d", pages, p.Total(), room)
		}
		for _, e := range p.Entries() {
			seen[e.ID().String()]++
		}
		if p.Next() == "" {
			break
		}
		cursor = p.Next()
		if pages > room {
			t.Fatal("the walk did not finish")
		}
	}

	if len(seen) != room {
		t.Fatalf("the walk reached %d participants, wanted %d", len(seen), room)
	}
	for id, times := range seen {
		if times != 1 {
			t.Fatalf("participant %s appeared %d times", id, times)
		}
	}
	t.Logf("a room of %d walked in %d pages of at most %d", room, pages, presence.MaxPage)
}

// The cursor is an identifier and not an offset, so somebody joining between two
// requests moves nobody past the boundary. With an offset the participant who
// was at the boundary is skipped, which is the defect this shape exists against.
func TestTheCursorIsAnIdentifierRatherThanAnOffset(t *testing.T) {
	const room = 20

	before := roomOf(t, room)
	first, err := before.Page("", 5)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}

	// Somebody arrives who sorts ahead of everybody already there, which is what
	// would shift every offset by one.
	entries := make([]presence.Entry, 0, room+1)
	entries = append(entries, entryNamed(t, "a00000", "a00000"))
	for i := range room {
		entries = append(entries, entry(t, i))
	}
	after, err := presence.NewRoll(entries, nil)
	if err != nil {
		t.Fatalf("roll: %v", err)
	}

	second, err := after.Page(first.Next(), 5)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Entries()) == 0 {
		t.Fatal("the second page was empty")
	}
	want := fmt.Sprintf("p%05d", 5)
	if got := second.Entries()[0].ID().String(); got != want {
		t.Fatalf("the second page began at %s, wanted %s", got, want)
	}
}

// A bound in entries is not a bound in bytes: the identifiers are the control
// plane's and nothing here decides how long they are. A page shortens itself
// until the message it encodes to is one the transport carries, and it still
// hands back a cursor, so the walk finishes rather than stopping short.
func TestAPageNeverEncodesOverTheMessageMaximum(t *testing.T) {
	const (
		room = 300
		wide = 900
	)

	entries := make([]presence.Entry, 0, room)
	for i := range room {
		id := fmt.Sprintf("p%05d%s", i, strings.Repeat("x", wide))
		entries = append(entries, entryNamed(t, id, id))
	}
	roll, err := presence.NewRoll(entries, nil)
	if err != nil {
		t.Fatalf("roll: %v", err)
	}

	seen := 0
	cursor := ""
	pages := 0
	largest := 0
	for {
		p, err := roll.Page(cursor, presence.MaxPage)
		if err != nil {
			t.Fatalf("page after %q: %v", cursor, err)
		}
		m, err := p.Message()
		if err != nil {
			t.Fatalf("page message: %v", err)
		}
		b, err := wire.Encode(m)
		if err != nil {
			t.Fatalf("encoding a page: %v", err)
		}
		if len(b) > wire.MaxMessageBytes {
			t.Fatalf("a page encoded to %d bytes, over the maximum of %d", len(b), wire.MaxMessageBytes)
		}
		if len(b) > largest {
			largest = len(b)
		}
		pages++
		seen += len(p.Entries())
		if p.Next() == "" {
			break
		}
		cursor = p.Next()
		if pages > room {
			t.Fatal("the walk did not finish")
		}
	}

	if seen != room {
		t.Fatalf("the walk reached %d participants, wanted %d", seen, room)
	}
	if pages <= room/presence.MaxPage {
		t.Fatalf("%d pages carried identifiers of %d bytes without shortening, so the byte bound was never reached and this test proves nothing", pages, wide)
	}
	t.Logf("identifiers of %d bytes: %d pages, largest %d bytes against a maximum of %d", wide, pages, largest, wire.MaxMessageBytes)
}

// The fourth condition on issue #37. The figures are the point of this test, so
// they are printed as well as asserted, and the run they come from is the one
// that appears in the record.
func TestThreeHundredParticipants(t *testing.T) {
	const (
		room  = 300
		joins = 300
	)

	roll := roomOf(t, room)
	b := encodedSummary(t, roll, 1)

	// One join. The summary is built once for the room and written to everybody,
	// because it carries no entry for its recipient.
	oneJoinEncodings := 1
	oneJoinWrites := room
	oneJoinBytes := len(b) * room

	c := clock.NewTest(start)
	e, err := presence.NewEmitter(c, presence.Window)
	if err != nil {
		t.Fatalf("emitter: %v", err)
	}
	summaries := 0
	for range joins {
		e.Note()
		if _, due := e.Take(); due {
			summaries++
		}
	}
	burstWrites := summaries * room
	naiveWrites := joins * room

	t.Logf("one join into a room of %d: %d encoding, %d writes, %d bytes, summary %d bytes", room, oneJoinEncodings, oneJoinWrites, oneJoinBytes, len(b))
	t.Logf("a burst of %d joins inside one window: %d summaries, %d writes, against %d writes one message per join", joins, summaries, burstWrites, naiveWrites)

	if burstWrites >= naiveWrites {
		t.Fatalf("the burst cost %d writes against %d for one message per join", burstWrites, naiveWrites)
	}
	if oneJoinWrites != room {
		t.Fatalf("one join cost %d writes, wanted %d", oneJoinWrites, room)
	}
}

func TestTheMessagesCarryTheTypesThisPackageOwns(t *testing.T) {
	roll := roomOf(t, 10)

	s, err := roll.Summary(1).Message()
	if err != nil {
		t.Fatalf("summary message: %v", err)
	}
	if s.Type != presence.TypeSummary {
		t.Fatalf("the summary carried type %q, wanted %q", s.Type, presence.TypeSummary)
	}

	p, err := roll.Page("", 5)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	m, err := p.Message()
	if err != nil {
		t.Fatalf("page message: %v", err)
	}
	if m.Type != presence.TypePage {
		t.Fatalf("the page carried type %q, wanted %q", m.Type, presence.TypePage)
	}

	// Both go out through the envelope the transport actually carries, so a
	// payload this package builds that the envelope refuses is caught here
	// rather than at a connection.
	for _, msg := range []wire.Message{s, m} {
		b, err := wire.Encode(msg)
		if err != nil {
			t.Fatalf("encoding %s: %v", msg.Type, err)
		}
		back, err := wire.Decode(b)
		if err != nil {
			t.Fatalf("decoding %s: %v", msg.Type, err)
		}
		if back.Type != msg.Type {
			t.Fatalf("%s decoded as %q", msg.Type, back.Type)
		}
	}
}
