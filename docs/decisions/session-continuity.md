# What happens to a live session when the pool changes

This says whether a participant who is already in a room can be moved to another
forwarding unit, and what they see when the pool grows, shrinks or loses a
machine underneath them. It answers issue #10.

Nothing here is measured. The one interval this document would otherwise put a
number on is named below with the work that produces it rather than with a value.

## The decision

A session never moves. A participant stays on the unit that admitted them until
they leave or until that unit is gone, and no change to the pool relocates a live
session in either direction.

That is the first of the three options issue #10 set out. It is also the position
three landed documents already act on, none of which is the record for it.
[room-topology.md](room-topology.md) fixes it for a conference that has grown
across units. [scaling-triggers.md](scaling-triggers.md) fixes it for retirement.
[../design/scaling-loop.md](../design/scaling-loop.md) carries it into the loop
and states in its own first paragraph that it decides nothing. So the choice was
made three times in passing and recorded nowhere, and this document is that
record. It adds no new position and it is not a ratification of a value: what it
adds is the part none of the three carries, which is what a participant sees and
what the control plane may therefore hold.

## Why not the other two

Moving a session only when its unit is retiring buys the ability to release a
machine that is holding one long room. It costs an interruption to a person who
did not ask for one and cannot see the reason for it. The corpus already prices
that trade in the other direction, in
[scaling-triggers.md](scaling-triggers.md), and the price has not changed: the
machine is cheaper than the room.

Moving a session with the media continuing means carrying transport state,
encryption state and sequence numbering from one process to another without the
endpoint noticing. It is the only option with no visible cost and it is the most
expensive thing on this milestone to build, and it would have to be built before
anything above it could be tested, because it changes what a unit is rather than
what the pool does with one.

Neither is refused for ever. What would reopen this is a measurement rather than
an argument, and the shape of it is in the residual below.

## What is visible, per kind of pool change

The pool grows. Nothing is visible to anybody already in a room. A new unit takes
new arrivals and existing units go on admitting exactly as they did.

A unit becomes a candidate for retirement and drains. Nothing is visible. The
unit stops being offered new work and keeps what it holds.

A drained unit is released. Nothing is visible, because a unit is released only
once it is empty.

A unit dies. The participants on that unit are disconnected and rejoin, and the
participants on every other unit carrying that conference keep their session and
lose only the sources that were published on the dead one. That case is set out
per role in [room-topology.md](room-topology.md), and it is not a change the
software chose to make.

So of the four, exactly one is visible, and it is the one no decision on this
milestone can remove.

## The interruption budget

Issue #10 asks for the maximum visible interruption in milliseconds where one is
visible at all. Under this decision no planned change to the pool makes one
visible, so there is no budget to write for one, and that is a consequence of the
choice rather than an omission from it.

The unplanned case has an interruption and this document does not carry a number
for it. It is bounded by three intervals and not one of the three has a value
anywhere in the tree today. How long the pool takes to decide a unit is gone,
which is liveness by asking and belongs to issue #56. How long a client waits
before it tries to rejoin, which is the reconnect window on issue #36. How long a
rejoin takes to be placed, which is issue #58 and is the figure that issue asks
to be measured against a join storm rather than assumed.

Writing a millisecond figure here would be inventing the value those three issues
exist to produce. When they have produced it, it is a line on the quality curve
rather than a paragraph, which is what issue #10 says of it and what issue #4
collects.

## What a participant sees, and what their client is told

On a planned change, nothing. No event reaches the client, there is no notice to
display, and nothing about the session is renegotiated. A client that shows the
person something here would be showing them an operator's decision they have no
action to take about.

On a unit death, the transport to that unit ends, and the client has to be able
to tell two things apart that look identical from where it sits. The room has
ended, and the room is still running and this path to it is gone. They are
different things to display and different things to do, so the protocol carries
the distinction rather than leaving a client to infer it from a closed socket.
Where that is encoded is issues #31 and #32; what this document fixes is that the
distinction exists and which way a client defaults when it cannot get one.

The default is to rejoin. A client that guesses the room has ended removes
somebody from a lecture that is still running, and there is no recovery from that
except the person noticing. A client that guesses the room is still there
attempts a rejoin against a room that has ended, and admission refuses it with a
reason it can display. The two mistakes are not the same size.

The rejoin itself is an ordinary arrival. Issue #36 defers to this document for
what happens when the unit a participant is returning to is gone, and the answer
is that the placer places them like anybody else, with no rule about returning
participants, which is what [room-topology.md](room-topology.md) already says of
the participants of a dead unit. Inside the reconnect window and with the unit
still there, they return to that unit, which is #36's own rule and is not changed
here.

## What this means for control plane state

Because a session never moves, the binding between a participant and a unit lives
exactly as long as that participant's connection and never longer. It is rebuilt
on reconnect and it is not durable. A control plane that had written it down and
came back holding it would name a pairing that stopped being true at the moment it
restarted, and would be wrong in the direction that is hardest to notice, since a
stale binding is a well-formed answer.

The same follows for what the participant is receiving. A unit holds no state the
control plane cannot rebuild, which is what
[media-plane-port.md](media-plane-port.md) says a unit is, so the receptions on a
unit are a projection of the control plane's own record of the room and not a
second copy of it.

What is durable is what describes a room that outlives any session in it, and
what an operator set. Neither is touched by a pool change, which is why this
decision has nothing to say about them beyond that they are on the other side of
the line.

The item-by-item division is issue #30 and is deliberately not written here. What
this section gives that issue is the rule that decides each item rather than the
list itself: state that exists because a session exists is rebuilt, and state that
exists because a room exists is durable. The pool's own place on that line is the
one question the rule does not answer, because the pool is not a session and is
not a room, and issue #30 raises it for that reason.

## The residual

An all-day room pins its unit. Under this decision the unit carrying it cannot be
released while it runs, and issue #10 named that cost when it named the option.
Nothing on the scaling milestone removes it: the floor, the minimum lifetime and
the cooldown on issue #62 all bound how often the pool changes and none of them
ends a room to release a machine.

What this costs is one machine per long room, for as long as the room runs, and
it is only worth reopening the decision if that number turns out to be large. The
run that would say is the soak on issue #70, which is the only one that watches
the pool move over hours. This is a residual stated rather than a risk accepted
quietly, and the measurement that would change the answer is named so that a
later argument has something to argue with.

## The issues that read this

Issue #30 divides control plane state into durable and rebuilt and says in its own
body that it cannot write that division before this document exists.

Issue #36 is the join, leave and rejoin state machine, and it defers to this
document for the case where the unit a participant is returning to has gone.

Issue #61 drains a unit and retires it. This document is the reason that drain
waits rather than moves, and the reason it has no deadline.
