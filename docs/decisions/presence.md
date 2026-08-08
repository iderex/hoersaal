# Presence at three hundred people

This is what a client is told about who else is in the room, and what that costs.
It answers issue #37. The code is `internal/presence` and every number below is a
constant in it, so the two are read together rather than kept in step by hand.

The shape this is written against is a lecture. A few people publish, a great
many listen, and everybody arrives in the ninety seconds before the hour. A
conferencing server that sends every participant change to every participant
costs three hundred messages for one join and ninety thousand for a room filling
up, and each of those messages carries a list that also grows with the room. Two
different things grow, so two different things have to be stopped, and they are
stopped separately below.

## What a client is actually told

The summary carries a revision, the participant count, the identity count, the
participants the room is attending to, and how many of those there were before
the cap. That is the whole message.

The two counts are separate because they answer different questions. The
participant count is endpoints and is what the capacity model counts. The
identity count is people, and it is smaller as soon as somebody joins from a
phone and a laptop at once. `internal/domain` is where that distinction is
argued; this document only carries it forward rather than restating the reason.

The revision rises by one per summary. A client that receives revision `n` after
revision `n-2` knows it missed one and asks for a page, which is the only place
this design needs a client to notice anything.

There is no entry for the recipient in it, and that absence is doing work. What a
participant is told about itself does not change when somebody else joins, so it
belongs to the admission exchange on issue #35 and is not re-sent. What it buys
is that the summary is one value for the whole room: it is built once, encoded
once, and written to everybody. A summary carrying a per-recipient field would
be three hundred encodings instead of one, and the fan-out is the part of a join
storm that is already the most expensive.

## The size, which does not grow with the room

The attending set is capped at `MaxAttending`, which is 8, and the summary says
how many there were before the cap so a client can show that more exist rather
than reading the cap as the whole truth. The order the set arrives in is the
caller's priority and the cap keeps the front of it, because nothing in the
presence code can judge who matters most.

Eight is a presence number and not a media number. It is meant to be the handful
of people a person can attend to at once: a presenter, a moderator, and the few
holding the floor or waiting to. The sets that decide who is forwarded are issues
#46 and #47, and neither has landed. When they do, the two caps are compared and
the smaller one wins, because a client naming somebody it never receives is worse
than a client naming fewer people than the unit sends.

What that buys is measurable rather than argued, and the assertion is on the
encoded bytes rather than on the structure:

    go test -count=1 -v -run TestTheSummaryIsTheSameSizeWhateverTheRoomHolds ./internal/presence
    a room of 8 encodes to 618 bytes
    a room of 88 encodes to 620 bytes
    a room of 888 encodes to 622 bytes
    a room of 8888 encodes to 624 bytes

Six bytes across three orders of magnitude, and the six are the decimal digits
the two counts gained. The identifiers in those rooms are all the same width on
purpose, so what the test measures is the room growing and not the names getting
longer.

## The window, and what it does and does not bound

Changes are coalesced. `Window` is 500 milliseconds and it is a minimum gap
between two summaries rather than a period: a window in which nothing changed
produces no message at all, so an idle room of three hundred costs nothing.

Half a second is the interval a client's view of the room is allowed to lag. It
is short enough that a room reads as live to somebody watching it and long enough
that the joins arriving together at the top of the hour collapse into one
message. It is a judgement and not a measurement, and it is written here so that
the measurement on issue #71 has something to disagree with. If the join storm
figures say a different number, this is the line that changes.

What the window bounds is the thing worth being precise about. It bounds messages
by elapsed time and never by the number of changes, which is the product this
issue exists to break:

    go test -count=1 -v -run 'TestABurstOfJoinsInsideOneWindow|TestChangesSpreadOverTime' ./internal/presence
    300 joins inside one window: 1 summaries per client, against 300 without coalescing
    300 joins over 1m30s: 150 summaries per client, bound 181, without coalescing 300

The first line is the whole mechanism. Three hundred people arriving inside one
window is one summary to each client, not three hundred.

The second line is the bound that is left, and it is not a triumph. A storm
spread evenly over ninety seconds still costs one summary per window it crosses,
which is 150 here against 300 for the naive shape. Coalescing halves it and does
not remove it, because the count genuinely moves throughout. Whether that is
cheap enough, and whether a deployment should anticipate a lecture that has not
started, is measured on issue #71 rather than asserted here.

## The full list, which is a query and not a broadcast

Nobody is sent three hundred names. A client that wants to display them asks, and
the answer is paged.

`MaxPage` is 100 and a request outside 1 to 100 is refused rather than quietly
adjusted, because a client that asked for a thousand should be told rather than
handed a hundred and left to work out which.

A page is bounded twice and the second bound is the one that matters. A bound in
entries is not a bound in bytes: the identifiers are the control plane's and
nothing in presence decides how long they are. So a page shortens itself until
the message it encodes to fits the largest message the transport carries, which
is `internal/wire`'s `MaxMessageBytes`, and it still hands back a cursor, so the
walk finishes rather than stopping short:

    go test -count=1 -v -run TestAPageNeverEncodesOverTheMessageMaximum ./internal/presence
    identifiers of 900 bytes: 9 pages, largest 64315 bytes against a maximum of 65536

Three hundred participants under identifiers of nine hundred bytes come back in
nine pages rather than three, every one of them inside the maximum, and all three
hundred are reached exactly once. The only case refused is a single entry too
large to encode at all, which is a message no paging could carry.

The cursor is a participant identifier and not an offset. Somebody joining
between two requests moves every offset by one and the participant standing at
the page boundary is skipped, silently, in the query a client uses to find out
who is in the room. With an identifier the boundary is a name and nothing moves
it.

## The figures for a room of three hundred

    go test -count=1 -v -run TestThreeHundredParticipants ./internal/presence
    one join into a room of 300: 1 encoding, 300 writes, 186600 bytes, summary 622 bytes
    a burst of 300 joins inside one window: 1 summaries, 300 writes, against 90000 writes one message per join

Those are counted rather than sampled: the test builds the messages this package
produces and counts them. What they are not is a measurement of a running
service, because there is no control plane yet to run one against. The cost of a
write, the cost of the fan-out itself, and what any of it does to a real machine
are on issue #71 and the load milestone, and nothing here should be read as
answering them.

## What this does not settle

A participant on one control plane instance learning about a join handled by
another. Issue #39 decides whether more than one instance is a supported shape at
all, and a fan-out path written before that answer would be inventing it. What
this design does instead is stay out of the way: a roll is a value, so an
instance assembling one from somewhere else produces the same summary as one
assembling it from its own state, and neither answer to #39 changes the message
or the window.

Hand raising. The issue describes the attending set as whoever is publishing or
has their hand up, and the second half has nowhere to come from yet. Issue #78
holds the queue. When it lands, a raised hand is another member of the same set
rather than a second message, which is the reason the set is a priority order
handed in from outside rather than derived from what is being published.

What a client does with any of it. The summary says the room has three hundred
people and names eight of them; whether an attendee sees a number, a list or
nothing at all is the attendee view on issue #77.
