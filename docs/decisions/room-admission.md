# Room admission and the join handshake

This answers issue #35. It fixes the sequence by which somebody holding a room
credential becomes a participant on a forwarding unit, what each step is allowed
to fail with, what the client is told in every one of those cases, and what
happens to an admission the client never completes.

It decides nothing about the transport underneath, which is
[signalling-transport.md](signalling-transport.md), nothing about what a
credential is, which is
[accounts-and-room-credentials.md](accounts-and-room-credentials.md), nothing
about which unit is chosen, which is [placement-seam.md](placement-seam.md), and
nothing about what a role permits, which is issue #34. It composes those four and
adds the order they run in.

`internal/admission` is this document in code. Every number and every name below
is in one of the two places and not in both, so they cannot drift into two
answers: the codes a client receives are the values in that package and the
reasoning for each is here.

## The sequence

Six steps. The first four are the exchange, and the last two are what happens
after the client has what it needs.

1. **The client asks.** One `admission.request` message naming the conference,
   carrying the credential, saying whether this participant intends to publish
   and, if so, what they intend to send.
2. **The credential is read.** It is verified against this installation's key,
   against the room the connection is in, and against its own window.
3. **The powers are asked.** Whether the role the credential carries may publish
   at all. This happens before anything is placed and before any unit is told
   anything.
4. **The placer is asked.** It answers a unit or refuses, and a refusal is a
   normal answer rather than an error.
5. **The unit is told.** `AdmitPublisher` where the participant publishes and
   `AdmitSubscriber` where they do not, which is the only step that leaves
   anything behind. The unit answers with transport parameters that are opaque
   here.
6. **The client connects and the room learns.** The client takes the transport
   parameters to the unit, and the control plane is told the admission completed.
   Presence is [presence.md](presence.md) and the state machine around joining,
   leaving and rejoining is issue #36.

### Why the order is that order

The credential is read first because everything after it costs something, and a
stranger must not be able to spend a placement decision by sending a message.

The powers are asked second so that a request to publish which the role does not
carry is refused before a unit exists in the story at all. `docs/threat-model.md`
names this under a participant publishing when they hold no floor, and it is the
condition of this issue that is easiest to satisfy in a way that looks right and
is not: a check made after the unit was told is a check made after the damage.

The placer is third because it is the first step that has to be told what the
participant is, and by then the participant is known to be entitled to be one.

The unit is last because it is the only step with a side effect. Everything that
can be refused is refused before this repository has left anything on a machine.

## What the client is told, and nothing else

Nine refusals, each its own code in the `admission.refused` message. They are
codes rather than sentences because a client acts on which one it received, and
a sentence is a translation problem rather than a protocol.

| code | what happened | what the person can do |
| --- | --- | --- |
| `malformed-request` | the message is not an admission request, or names a room this connection is not in, or offers something the model refuses | nothing; a client that sends this has a defect |
| `credential-refused` | the credential did not verify | ask whoever sent the link for another one |
| `not-permitted` | the credential is good and its role may not do what was asked | join without publishing, or ask for a credential that carries the floor |
| `conference-not-open` | the room is not taking participants yet | wait |
| `no-capacity` | the deployment holds no units at all | tell the operator |
| `room-full` | no unit can take another participant | wait, or tell the operator |
| `conference-at-its-unit-ceiling` | the conference already occupies as many units as it may | tell the operator |
| `unit-refused` | the unit could take this participant and will not | try again |
| `unit-unavailable` | the unit did not answer, or this deployment cannot reach it | try again |

Two of these deserve their reason written down rather than being read off the
table.

**`credential-refused` is one code for four failures.** Unreadable, signed with
another key, naming another room and outside its window are all one answer.
Which of the four failed is a fact about the credential a forger is holding, and
answering it turns the refusal into an oracle that says how close an attempt
was. An honest client loses nothing by the merge, because all four are repaired
the same way.

**`unit-unavailable` also covers this deployment disagreeing with itself.** A
unit the placer named and the pool cannot reach, a conference the unit does not
hold, and a participant identifier already in use there are all answered with it.
From where the client stands they are the same event, their remedy is identical,
and naming the difference would describe this deployment's internals to a
stranger. What the difference is worth is inside the deployment, where the pool
decides what happens to the unit, and `docs/design/scaling-loop.md` is where that
is written.

The three placement refusals stay three codes rather than becoming one. A person
who cannot join because the room is full waits; a person who cannot join because
the deployment has no units at all is looking at an operator's problem; and a
conference at its unit ceiling is neither, and is not a reason to grow the pool.
Collapsing them would be cheaper to write and would send two of the three to the
wrong place.

## The media plane credential carries no power the room credential did not

This is the condition of the issue that the threat model calls out, and the
mechanism is one sentence: which of the two admissions the unit is asked for is
decided from the role, before the unit is asked.

A credential whose role may not publish reaches `AdmitSubscriber` and never
`AdmitPublisher`. The transport parameters that come back are therefore
parameters for a participant the unit will not accept a publication from, and the
unit rather than the control plane is what enforces that at the moment media
arrives. This document does not claim more than that: what the transport
parameters contain is opaque above the port by
[media-plane-port.md](media-plane-port.md)'s own promise, so what stops a
listener publishing is what the unit was told, and not anything about the bytes
the client received.

What a role bundles is issue #34 and is not decided here. The exchange asks one
question, `MayPublish`, of a seam, and takes the answer. A table of roles written
in the admission package would have decided issue #34 by accident, which is the
same mistake `internal/domain` avoided by making a role a name and nothing more.

## An admission nobody completes

Every step can fail, and a failure after step 5 leaves an admission on a unit
that nobody will ever use: the client was dropped, the answer never reached it,
or the person closed the tab.

The route has two halves and only one of them is in this repository.

**The control plane's half, which is built.** A granted admission is held with a
deadline. The client arriving takes it out of that set. A sweep after the deadline
reports it and forgets it, so the control plane stops believing in an admission
that never completed, and a client that comes back afterwards is a new admission
rather than a claim on the old one. The window is a duration rather than a
number of attempts, because what is being waited for is a client connecting to a
unit over a path this process cannot watch. `internal/admission` fixes a default
and takes the value as an argument, and no decision document fixes a number:
issue #71 measures the join storm and is where one would come from.

**The unit's half, which is not built and cannot be from here.**
[media-plane-port.md](media-plane-port.md) has eight operations and none of them
releases one participant. The only operation that releases anything about a
participant is `CloseConference`, which releases every participant in the
conference. So an abandoned admission costs the unit until the conference ends,
and nothing this repository can call shortens that.

That is stated rather than described as a reclaim, and the difference matters
because the first reading of the sweep is that it undoes the admission. It does
not. What the sweep undoes is this process's belief in it. The sweep answers with
the unit each abandoned admission is on, so the day the port grows an operation
for it the call site is already written and named, and until then this paragraph
is the whole of what a reader is entitled to conclude.

Two things bound how bad that is, and neither removes it. An admission that never
connected has no reception, so it contributes nothing to the term the capacity
signal counts, which is `capacity-signal-under-attack.md`'s own sentence about
when that term moves. And a participant identifier is minted per admission, so an
orphan never blocks the same person from joining again.

## What this exchange does not decide

The connection underneath. Where a connection is accepted, the ping interval, the
answer window and the read deadline are
[signalling-transport.md](signalling-transport.md)'s numbers, and nothing in this
tree listens yet. This exchange is a function from a message and a room record to
an answer, which is what lets a room of three hundred be a test.

How many strangers may open a connection at all. Nothing here bounds that, and
`docs/threat-model.md` says so in the same words. What this bounds is what one
message can cause.

Where a participant identifier comes from. It is minted through a seam rather
than in the exchange, because a participant identifier is not a secret, it is
sent to everybody in the room by `internal/presence`, and what it must not be is
predictable across a deployment. Where those bytes come from depends on whether
the control plane runs as one instance or several, which is issue #39 and is
unanswered. Nothing in this repository satisfies that seam today.

Leaving and rejoining. Issue #36 owns the state machine an admitted participant
lives in afterwards, and this exchange ends at the moment the admission is
complete.

## The issues this belongs to

Issue #35 is this document and `internal/admission`.

Issue #34 owns `MayPublish`, which is a seam here.

Issue #36 owns what happens after step 6, and issue #39 owns whether more than
one control plane instance may run the exchange at once.

Issue #71 is where the window in the abandoned-admission section stops being a
number nobody measured.

Issue #104's second condition is a session a test can drive to the end without
anything crossing the federation boundary, and it names issues #35 and #36 as
what it waits for. This is the first half of that.
