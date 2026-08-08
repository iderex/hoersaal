# Federation, and what stays true without it

This decides whether two installations of this software may hold one room
between them. It answers issue #13. The sentence the legal milestone has to
write depends on the answer, so the answer is written here first and quoted
there rather than paraphrased.

## The decision

There is no federation. Two installations of this software do not hold a room
between them, do not exchange participants, and do not assert anybody's identity
to each other. The protocol does not leave room for it either.

## Why not the option that leaves room for it

The middle option, designing for federation and not building it, is the one that
looks free. It is not, and the cost lands in the three places this project can
least afford.

It puts a host into the identifiers on the wire. An identifier that names a host
has to be carried by every client, understood by every version, and kept working
by the stability promise on issue #7 of the question issue once that promise
starts. A field that exists is a field somebody uses, and the first use is
usually not the one it was reserved for.

It makes the sovereignty sentence conditional years before anything is built. A
statement that personal data stays on the host, with a footnote about a feature
that does not exist yet, is a statement an operator has to read twice and a
statement a data protection officer has to ask about. The unconditional version
is the one that is worth having, and it can only be said while it is true.

And it invites the thing it was supposed to prevent. Room left in a protocol is
room a fork fills, while the documentation the fork inherited goes on saying the
feature is not possible. That is worse than either honest answer.

The third option, federation as a feature that an operator turns on, is refused
for what it does to the documentation rather than for what it costs to build.
Once it exists, every sentence about where the conversation stays acquires a
condition, every operator has to understand that condition, and the burden
outlives whatever interest the feature was built for.

## What crosses the boundary

Nothing. No identifier, no media, no metadata, no presence, no room listing and
no account.

That is the whole of this section and it stays that way. If any of it ever stops
being true, this document changes first, and the change is a protocol version
under issue #32 rather than a setting, because a boundary that can be crossed by
turning something on is not a boundary.

## What an operator does to turn it on

Nothing, because there is nothing to turn on. There is no setting, no build tag
and no deployment shape in which this software talks to another installation of
itself. An operator is shown nothing at that moment because no such moment
exists.

## What is not federation, and is easy to confuse with it

A pool of forwarding units is not federation. Every unit in a pool belongs to one
deployment run by one operator, the conference is one conference, and the link
between two units carries the media of that one conference, which
[room-topology.md](room-topology.md) describes. Nobody's identity crosses
anything, because there is only ever one control plane deciding who is in the
room. Whether those units may be machines the operator does not own is a
different question, it is open, and it is entry 3 of issue #1. This document does
not answer it and does not depend on it.

A person joining a room on somebody else's installation is not federation. They
open a link, they are admitted by that installation as a participant of that
installation, and they are one of its participants for as long as they are in the
room. Their own installation, if they run one, is not involved and is not told.
That is how somebody attends a lecture at another university today, and it costs
this project nothing to keep working.

A room whose participants come from more than one organisation is not federation
either. It is one host, one room, and an admission decision on issue #35.

## Where the boundary lives

In the code, on issue #104, and not in this sentence. The place where an identity
asserted by another installation would enter is the place that refuses, and it
refuses because there is no route by which such an assertion is accepted rather
than because a flag is off. A test that proves the refusal bites is what issue
#104 owes, and it is the difference between a boundary and a promise.

The outward half of that is now `internal/boundary`. It is the one directory in
which a connection out of this process may be made, it holds none, and its check
refuses one written anywhere else, over every `.go` file in this repository
including the tests. Each finding sends the reader back to this document rather
than restating it. The refusal is proved by fixtures the checker is asked about
directly rather than by the tree happening to be clean, because a tree with no
connections in it would pass a rule that had stopped refusing.

`PROSE, NOT ENFORCEMENT` for the inward half, and it is the residual rather than
the whole. The sentence above is about the place an assertion from another
installation would enter, and what refuses one is that no route accepts one. An
absent route is an absence, and no reading of the syntax tree finds an absence.
What would judge it is a session a test can drive to the end without anything
crossing, which is the second condition of #104 and waits on #35 and #36. Two
further gaps are named in `internal/boundary`'s own package comment and are not
repeated here: a library that dials on a caller's behalf, and a process started
to do it.

## What this costs

It closes off the deployment where several small institutions each run their own
installation and hold joint lectures without anybody's participants leaving their
own host. That is a real use, it is the strongest argument for the option this
document rejects, and the answer it gets here is that those lectures happen on
one of the hosts, with the others' people joining it as its participants.

It also means this project will be compared unfavourably with software that
lists federation as a feature. That comparison is fair, and the reply is the
sentence the privacy statement gets to write without a condition attached.

## What the legal milestone does with this

Issue #103 writes the data protection statement and quotes this document rather
than paraphrasing it, because a paraphrase of a boundary is where a condition
gets added by accident. Issue #105 writes the threat model, and the absence of
any inbound route from another installation is a statement about the attack
surface as much as about the data.
