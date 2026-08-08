// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package presence answers what a client is told about who else is in the room.
//
// It answers issue #37, and the reasoning behind every number and every shape in
// it is in docs/decisions/presence.md rather than here, so the two cannot drift
// into two different answers.
//
// The whole package exists against one product. A room of three hundred where
// every change is sent to everybody costs three hundred messages for one join
// and ninety thousand for a lecture filling up, and the size of each of those
// messages grows with the room as well. Two separate things have to stop growing
// and this package stops them separately.
//
// The size stops growing because the summary is a count, a bounded set of the
// participants the room is attending to, and nothing else. It is the same bytes
// for a room of three and a room of three thousand, which TestTheSummaryIsThe
// SameSizeWhateverTheRoomHolds asserts on the encoded message rather than on the
// structure.
//
// The number stops growing because changes are coalesced: a burst of joins
// inside one window produces one summary and not one per join. The window is a
// minimum interval between summaries rather than a timer, so what bounds the
// traffic is elapsed time and never the number of changes.
//
// The summary carries no entry for its recipient, and that absence is the reason
// the fan-out is one encoded message and three hundred writes rather than three
// hundred of each. What a participant is told about itself does not change when
// somebody else joins, so it belongs to the admission exchange on issue #35 and
// is not re-sent here.
//
// The full list is not a broadcast at all. It is Page, which a client asks for
// when it wants to display names, and it is bounded twice: by MaxPage and by the
// largest message the transport carries, because a bound in entries alone is not
// a bound in bytes when the identifiers are somebody else's.
//
// What this package deliberately does not hold. It has no conference, no
// connection and no store: a Roll is handed to it and every answer is a function
// of that Roll, which is what lets a room of three hundred be a test rather than
// a deployment. It reads no clock of its own; Emitter takes one. And it decides
// nothing about who is worth attending to, because the order that arrives in is
// the caller's, and the sets that fix it are issues #46 and #47.
//
// What is not answered here and would look as though it were. A participant on
// one control plane instance learning about a join handled by another is the
// fan-out path issue #39 has to settle before it can be built, and nothing in
// this package assumes either answer: a Roll is a value, so an instance that
// assembles one from somewhere else produces the same summary as one that
// assembles it from its own state.
package presence

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/domain"
	"github.com/iderex/hoersaal/internal/wire"
)

// The message types, as they appear in the envelope wire.Encode writes. They are
// constants here because this package owns them: internal/wire reads the
// envelope and hands the payload to whoever owns the type, and this is the owner
// the comment there names.
const (
	// TypeSummary is the message every participant receives when presence
	// changes.
	TypeSummary = "presence.summary"

	// TypePage is one page of the full list, sent in answer to a request.
	TypePage = "presence.page"
)

// The numbers. Each is argued in docs/decisions/presence.md; what is written
// here is what the number is, not why it is that value.
const (
	// MaxAttending is the largest number of participants a summary names. A room
	// with more of them says how many there are and carries the first
	// MaxAttending of the order it was given.
	MaxAttending = 8

	// Window is the shortest interval between two summaries to one room. It is a
	// minimum gap rather than a period: a window in which nothing changed
	// produces no summary at all.
	Window = 500 * time.Millisecond

	// MaxPage is the largest number of entries a client may ask for in one page.
	// It is not the only bound on a page; see Roll.Page.
	MaxPage = 100
)

// The refusals. They are values so that a caller can tell the kind of mistake
// apart without reading this file or matching on a string.
var (
	// ErrEmpty is an identifier or a role with nothing in it.
	ErrEmpty = errors.New("presence: empty identifier")

	// ErrDuplicate is a participant offered to one roll twice.
	ErrDuplicate = errors.New("presence: participant already in this roll")

	// ErrUnknown is a participant named as attending who is not in the roll.
	ErrUnknown = errors.New("presence: not in this roll")

	// ErrPageSize is a page asked for with a size outside 1..MaxPage.
	ErrPageSize = errors.New("presence: page size outside the permitted range")

	// ErrWindow is an emitter asked for with no clock or a window that is not
	// positive.
	ErrWindow = errors.New("presence: an emitter needs a clock and a positive window")
)

// An Entry is one participant as presence describes it: which endpoint, whose it
// is, and the name of what they may do. It carries nothing else, and in
// particular nothing a person wrote, because a summary goes to everybody in the
// room and a room title or a display name is issue #85's problem the moment it
// reaches a log.
type Entry struct {
	id       domain.ParticipantID
	identity domain.IdentityID
	role     domain.Role
}

// NewEntry refuses an empty value in any of the three, so an Entry that exists
// is one every field of which was given.
func NewEntry(id domain.ParticipantID, identity domain.IdentityID, role domain.Role) (Entry, error) {
	switch {
	case id.String() == "":
		return Entry{}, fmt.Errorf("entry participant: %w", ErrEmpty)
	case identity.String() == "":
		return Entry{}, fmt.Errorf("entry identity: %w", ErrEmpty)
	case role.String() == "":
		return Entry{}, fmt.Errorf("entry role: %w", ErrEmpty)
	}
	return Entry{id: id, identity: identity, role: role}, nil
}

// ID is which endpoint this is.
func (e Entry) ID() domain.ParticipantID { return e.id }

// Identity is who the endpoint belongs to. Two entries may return the same one,
// which is the same person on two devices.
func (e Entry) Identity() domain.IdentityID { return e.identity }

// Role is the name of what they may do, resolved on issue #34.
func (e Entry) Role() domain.Role { return e.role }

// entryJSON is the wire shape of an Entry. The identifiers are opaque values in
// internal/domain, so the mapping to strings is written here rather than left to
// the encoder, which would otherwise emit an empty object for each of them.
type entryJSON struct {
	Participant string `json:"participant"`
	Identity    string `json:"identity"`
	Role        string `json:"role"`
}

func (e Entry) wireForm() entryJSON {
	return entryJSON{
		Participant: e.id.String(),
		Identity:    e.identity.String(),
		Role:        e.role.String(),
	}
}

// A Roll is everybody in one room, together with the participants the room is
// attending to. It is a value: two calls on one Roll answer the same thing, and
// a Roll assembled from another control plane instance's state answers the same
// as one assembled here, which is what keeps issue #39's answer out of this
// package.
//
// The order of the entries is fixed here rather than taken from the caller,
// because paging over an order the caller chooses per call is paging that can
// miss somebody. The order of attending is the caller's, because it is a
// priority and this package has no way to judge one.
type Roll struct {
	entries        []Entry
	attending      []Entry
	attendingTotal int
	identities     int
}

// NewRoll refuses a roll it could not answer about honestly: a participant given
// twice, and a participant named as attending who is not in the room. Both are
// refused rather than dropped, because a summary that quietly omits somebody is
// indistinguishable from one describing a room they left.
//
// attending is in the caller's priority order and may be longer than
// MaxAttending. The whole length is kept as the count the summary reports; the
// entries beyond the cap are not.
func NewRoll(entries []Entry, attending []domain.ParticipantID) (Roll, error) {
	held := make(map[domain.ParticipantID]Entry, len(entries))
	identities := make(map[domain.IdentityID]struct{}, len(entries))
	ordered := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.id.String() == "" {
			return Roll{}, fmt.Errorf("roll entry: %w", ErrEmpty)
		}
		if _, already := held[e.id]; already {
			return Roll{}, fmt.Errorf("participant %s: %w", e.id, ErrDuplicate)
		}
		held[e.id] = e
		identities[e.identity] = struct{}{}
		ordered = append(ordered, e)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].id.String() < ordered[j].id.String() })

	named := make(map[domain.ParticipantID]struct{}, len(attending))
	front := make([]Entry, 0, len(attending))
	for _, id := range attending {
		e, in := held[id]
		if !in {
			return Roll{}, fmt.Errorf("attending participant %s: %w", id, ErrUnknown)
		}
		if _, already := named[id]; already {
			return Roll{}, fmt.Errorf("attending participant %s: %w", id, ErrDuplicate)
		}
		named[id] = struct{}{}
		if len(front) < MaxAttending {
			front = append(front, e)
		}
	}

	return Roll{
		entries:        ordered,
		attending:      front,
		attendingTotal: len(attending),
		identities:     len(identities),
	}, nil
}

// ParticipantCount is how many endpoints are in the room. This is the number the
// capacity model counts, and internal/domain is where the distinction it rests
// on is argued.
func (r Roll) ParticipantCount() int { return len(r.entries) }

// IdentityCount is how many people those endpoints belong to. It is at most
// ParticipantCount and is smaller as soon as somebody joins twice.
func (r Roll) IdentityCount() int { return r.identities }

// A Summary is what every participant receives when presence changes. It is the
// same value for all of them, which is what makes a fan-out one encoding and N
// writes rather than N of each.
type Summary struct {
	revision       uint64
	participants   int
	identities     int
	attending      []Entry
	attendingTotal int
}

// Summary describes the room at revision. The revision comes from the Emitter
// rather than from here, because what counts as a change is the caller's
// question and this is a function of a Roll.
func (r Roll) Summary(revision uint64) Summary {
	return Summary{
		revision:       revision,
		participants:   len(r.entries),
		identities:     r.identities,
		attending:      r.attending,
		attendingTotal: r.attendingTotal,
	}
}

// Revision is which summary this is. A client that receives revision n after
// revision n-2 knows it missed one and can ask for a page rather than guessing.
func (s Summary) Revision() uint64 { return s.revision }

// Attending is the participants the room is attending to, at most MaxAttending
// of them.
func (s Summary) Attending() []Entry {
	out := make([]Entry, len(s.attending))
	copy(out, s.attending)
	return out
}

// AttendingTotal is how many there were before the cap, so a client can say that
// more exist rather than showing the cap as the whole truth.
func (s Summary) AttendingTotal() int { return s.attendingTotal }

type summaryJSON struct {
	Revision       uint64      `json:"revision"`
	Participants   int         `json:"participants"`
	Identities     int         `json:"identities"`
	Attending      []entryJSON `json:"attending"`
	AttendingTotal int         `json:"attendingTotal"`
}

// Message is the summary as one envelope, ready to write. It is built once for a
// room and written to every participant in it.
func (s Summary) Message() (wire.Message, error) {
	shape := summaryJSON{
		Revision:       s.revision,
		Participants:   s.participants,
		Identities:     s.identities,
		Attending:      make([]entryJSON, 0, len(s.attending)),
		AttendingTotal: s.attendingTotal,
	}
	for _, e := range s.attending {
		shape.Attending = append(shape.Attending, e.wireForm())
	}
	payload, err := json.Marshal(shape)
	if err != nil {
		return wire.Message{}, fmt.Errorf("presence: encoding a summary: %w", err)
	}
	return wire.Message{Type: TypeSummary, Payload: payload}, nil
}

// A Page is one answer to a request for the full list.
type Page struct {
	entries []Entry
	next    string
	total   int
}

// Entries is the participants on this page, in the roll's order.
func (p Page) Entries() []Entry {
	out := make([]Entry, len(p.entries))
	copy(out, p.entries)
	return out
}

// Next is the cursor to ask the following page with. It is empty when this page
// is the last one, so a client stops on a value rather than on a count it
// derived itself.
func (p Page) Next() string { return p.next }

// Total is how many participants the whole list holds, so a client can show
// progress without walking it first.
func (p Page) Total() int { return p.total }

type pageJSON struct {
	Entries []entryJSON `json:"entries"`
	Next    string      `json:"next"`
	Total   int         `json:"total"`
}

// Message is the page as one envelope.
func (p Page) Message() (wire.Message, error) {
	payload, err := json.Marshal(p.wireForm())
	if err != nil {
		return wire.Message{}, fmt.Errorf("presence: encoding a page: %w", err)
	}
	return wire.Message{Type: TypePage, Payload: payload}, nil
}

func (p Page) wireForm() pageJSON {
	shape := pageJSON{Entries: make([]entryJSON, 0, len(p.entries)), Next: p.next, Total: p.total}
	for _, e := range p.entries {
		shape.Entries = append(shape.Entries, e.wireForm())
	}
	return shape
}

// Page is the entries after the cursor, at most size of them. An empty cursor
// starts at the beginning, and the cursor a client sends back is the
// participant identifier this package handed it rather than an offset, so a page
// boundary is not moved by somebody joining between two requests.
//
// It is bounded twice and the second bound is the one that matters. size is
// refused outside 1..MaxPage, which bounds the entries. Then the page is
// shortened until the message it encodes to fits wire.MaxMessageBytes, because
// the identifiers in it are the control plane's and a bound in entries is not a
// bound in bytes. A page shortened that way still carries a cursor, so the walk
// finishes; only a single entry too large to encode at all is refused, and that
// is a message the transport could not carry under any paging.
func (r Roll) Page(after string, size int) (Page, error) {
	if size < 1 || size > MaxPage {
		return Page{}, fmt.Errorf("page size %d, permitted 1 to %d: %w", size, MaxPage, ErrPageSize)
	}

	start := 0
	if after != "" {
		start = sort.Search(len(r.entries), func(i int) bool { return r.entries[i].id.String() > after })
	}
	end := start + size
	if end > len(r.entries) {
		end = len(r.entries)
	}

	page := Page{entries: r.entries[start:end], total: len(r.entries)}
	page.cursorAt(start, len(r.entries))
	for len(page.entries) > 1 && !page.fits() {
		page.entries = page.entries[:len(page.entries)-1]
		page.cursorAt(start, len(r.entries))
	}
	if len(page.entries) == 1 && !page.fits() {
		return Page{}, fmt.Errorf("one entry encodes over the %d byte maximum: %w", wire.MaxMessageBytes, wire.ErrTooLarge)
	}
	return page, nil
}

// cursorAt writes the cursor for the page as it currently stands. It is called
// again after every shortening, because the cursor is the last entry on the page
// and shortening moves it. start is where the page began in the roll and all is
// how long the roll is, which together say whether this page reached the end.
func (p *Page) cursorAt(start, all int) {
	if len(p.entries) == 0 || start+len(p.entries) >= all {
		p.next = ""
		return
	}
	p.next = p.entries[len(p.entries)-1].id.String()
}

// fits reports whether this page encodes to a message the transport carries.
func (p Page) fits() bool {
	m, err := p.Message()
	if err != nil {
		return false
	}
	b, err := wire.Encode(m)
	return err == nil && len(b) <= wire.MaxMessageBytes
}

// An Emitter decides when a summary may be sent. It is the coalescing half of
// this package and it holds no room, so one exists per room and a test drives it
// with a clock it controls.
//
// The rule is a minimum gap. A change marks the room pending; a pending room
// yields a summary once Window has passed since the last one was taken, and
// yields nothing at all when nothing changed. So a burst of any size inside one
// window costs one summary, and what bounds the count is elapsed time rather
// than the number of changes, which is the product issue #37 exists to break.
type Emitter struct {
	clock    clock.Clock
	window   time.Duration
	last     time.Time
	sent     bool
	pending  bool
	revision uint64
}

// NewEmitter refuses no clock and a window that is not positive. A window of
// zero is not coalescing turned off, it is the shape where the bound this type
// exists for does not hold, so it is refused rather than accepted as a setting.
func NewEmitter(c clock.Clock, window time.Duration) (*Emitter, error) {
	if c == nil || window <= 0 {
		return nil, fmt.Errorf("window %s: %w", window, ErrWindow)
	}
	return &Emitter{clock: c, window: window}, nil
}

// Note records that presence changed. It is cheap and it is called once per
// change, however many arrive: the point of this type is that a caller does not
// have to decide which changes are worth sending.
func (e *Emitter) Note() { e.pending = true }

// Pending reports whether a change is waiting to be sent.
func (e *Emitter) Pending() bool { return e.pending }

// Take yields the revision of the next summary, and whether one is due at all.
// It is due when something changed and either nothing has been sent yet or the
// window has passed. Taking clears the pending change and starts the window
// again, so the caller builds one Summary at that revision and writes it to
// everybody.
func (e *Emitter) Take() (uint64, bool) {
	if !e.pending {
		return e.revision, false
	}
	now := e.clock.Now()
	if e.sent && now.Sub(e.last) < e.window {
		return e.revision, false
	}
	e.pending = false
	e.sent = true
	e.last = now
	e.revision++
	return e.revision, true
}

// Wait is how long until a pending change may be taken, and whether there is one
// to wait for. A caller schedules against this rather than polling, and a test
// asks it instead of watching for something not to happen.
func (e *Emitter) Wait() (time.Duration, bool) {
	if !e.pending {
		return 0, false
	}
	if !e.sent {
		return 0, true
	}
	left := e.window - e.clock.Now().Sub(e.last)
	if left < 0 {
		left = 0
	}
	return left, true
}
