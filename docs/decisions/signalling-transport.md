# The signalling transport and its framing

This says how a client talks to the control plane. It answers issue #31. The
framing half is built in `internal/wire/wire.go` and the outbound half in
`internal/wire/send.go`, and every number either of them refuses against is
fixed here rather than beside the code.

What this document does not do is name the messages. The envelope is settled
here; what may travel inside it belongs to the exchanges that use it, which are
issues #35 and #37. Which version of the envelope is being spoken, and how two
ends agree on one, is issue #32. Whether the result is a contract anybody may
rely on, and from which release, is a decision reserved on issue #1 and is not
taken here.

Nothing here is measured. The load this is designed for is three hundred
connections held for the length of a lecture, and the run that produces figures
for it is the join storm on issue #71.

## The transport

WebSocket, over the same HTTPS listener the client used to reach the service in
the first place.

The reason is what sits between a lecture theatre and this server. A university
or a company puts a reverse proxy, a TLS terminator and often an inspecting
gateway in that path, and the only long-lived connection all three of them
reliably carry is a WebSocket upgraded from an ordinary HTTPS request on port
443. Anything on a second port is a firewall change an operator has to ask
somebody else for, and a lecture that needs a change request before it can start
is a lecture that happens on a commercial service instead.

What was weighed against it. Long polling works through everything and turns one
connection into a request every few seconds, which at three hundred clients is a
load made of nothing but overhead. Server-sent events with a POST channel back
is two half-connections whose ordering has to be reasoned about separately, for
no gain over an upgrade that is just as widely carried. A raw TCP or QUIC
connection on its own port is refused by the proxy case above. WebTransport is
the shape this would be built on if it could be relied on, and it cannot yet,
which is a sentence with an expiry date rather than a principle.

### The connection a proxy closes

A proxy that drops a connection which has been quiet for sixty seconds is the
normal case rather than a misconfiguration, and a service that discovers this by
losing three hundred clients at once has designed a reconnect storm.

So the server sends a ping every 20 seconds and expects the answer within 20
seconds. A connection that has been quiet for that long is not quiet on the
wire, so no idle timer short of twenty seconds can reach it, and a client that
has gone away is noticed within 40 seconds rather than never. The read deadline
is 60 seconds, which is three ping intervals, so a single lost packet is not a
disconnection.

Reconnecting is the client's half and is issue #76. What matters here is that a
disconnection is expected and cheap rather than exceptional.

## The framing

One protocol message per WebSocket message, in text, as JSON.

WebSocket already frames, so a second length prefix inside it would be a length
that can disagree with the one the transport already has. The message boundary
is the frame boundary and there is nothing to reassemble.

Text rather than binary. What text costs is bytes on the wire and processor time
on both ends. What it buys is that an operator with a problem can read what is
happening with tools they already have, and that a third-party client can be
written in an afternoon in whatever language the person writing it knows. For
this service the cost is small and bounded on purpose: the control plane carries
decisions rather than media, its messages are small, and the one that is sent to
everybody is the presence summary, which issue #37 bounds in size and rate for
reasons of its own. The media is where the bytes are, and none of it comes
through here.

That trade is not permanent. If the presence path measured on issue #71 turns out
to be dominated by encoding, the envelope has a version and this is the kind of
change a version is for.

### The envelope

Two members, `type` and `data`, and nothing else. `type` names what the message
is. `data` carries the message itself and is handed to whoever owns that type
exactly as it arrived.

Three refusals are worth their reasons.

A member the envelope does not have is refused rather than ignored, so a client
that misspells one is told rather than having it silently dropped and being left
to work out why nothing happened.

A member given twice is refused rather than resolved. A JSON reader that takes
the last of two members and one that takes the first will read one message as two
different things, and the reader that decides is whichever one the attacker did
not think of. Refusing is the only answer that cannot be told apart by whoever is
probing.

Anything after the object is refused. A message carrying a second JSON value
after the first is two readers disagreeing about what arrived, which is the same
defect one step out.

## The numbers

**A message of at most 64 KiB, refused before it is parsed.** Every message this
protocol has is a decision, an identifier or a credential, and the largest of
them is not close to this. The limit is not a constraint on legitimate use; it is
there so that an unauthenticated stranger cannot decide how much work this
process does or how much memory it holds. It is refused on the size of the bytes
before any of them are read.

**A type name of at most 64 bytes.** Same reason, one level in. A type is a
short name from a fixed set, and nothing is gained by accepting a long one.

**64 messages queued for one client.** This is the number that decides how far
behind a client may fall before it is disconnected. It is a count of messages
rather than of bytes because what it bounds is lateness, and because the messages
are all small. Sixty-four is roughly a minute of the busiest thing a client
receives during a lecture, which is presence changes at the rate issue #37
coalesces them to, so a client that is briefly slow keeps its session and one
that has stopped reading loses it.

**10 seconds for one write to one client.** A write that has not completed in ten
seconds is not a slow network, it is a client that is not reading, and the
connection is what is holding this process's memory in the meantime.

**A ping every 20 seconds, an answer expected within 20, and a read deadline of
60.** The reasons are in the section above.

## Backpressure, and what it refuses

A client that stops reading must not be able to grow a buffer in this process
until the machine dies. That is the failure this whole section exists for, and it
is one machine's memory divided by three hundred clients rather than an abstract
concern.

The mechanism is a fixed queue per client and a disconnection when it is full.
There is one writer per connection, it takes messages in the order they were
queued, and putting a message on the queue never blocks: either it fits, or the
client is disconnected before the send returns. So the memory one client can
cause is bounded by the queue, and the caller that was trying to reach that
client is not held up by it.

A disconnected client is not a lost participant. It reconnects and is given what
it needs to rejoin, which is issue #36, and the presence summary it comes back to
is a query rather than a replay of what it missed, which is issue #37. That is
why disconnecting is an acceptable answer here at all.

Two things follow from the shape and are stated rather than left to be found.
Where the client was closed deliberately, what is already queued is written
first, because a message that was accepted has been promised. Where it was
disconnected for a fault, the backlog is dropped, because writing it would mean
waiting on the connection that is already the problem.

## What is built and what is not

Built and tested: the decoder, the encoder and the outbound queue with its
timeout, all of them behind interfaces that need no network.

Not built: anything that listens. There is no server in this tree, so the ping
interval, the answer window and the read deadline above are numbers this document
fixes and no code yet reads. They are implemented where the connection is
accepted, which is issue #35, and the fact that they are unimplemented today is
this paragraph rather than something a reader has to notice.

The decoder is deliberately a function from bytes to a message or an error, with
no connection in its signature. That is what lets the property suite on issue #41
and the fuzzing on issue #94 enter at the same door a stranger does, instead of
building a second one that behaves almost the same.
