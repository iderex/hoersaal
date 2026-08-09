# What an adversary can do to the capacity signal

This says which parts of the number the scaling loop reads can be moved by
somebody the control plane never admitted, by what route, and what bounds the
cost when they are. It answers issue #150. The signal is defined in
[capacity-signal.md](capacity-signal.md) and the levels that act on it in
[scaling-triggers.md](scaling-triggers.md), and this document uses both without
redefining either.

[../threat-model.md](../threat-model.md) raised this and did not answer it. That
document names the third term as something it found and no issue held, and names
this issue as where the answer goes. What is here is the answer to the first
question, the second and the fourth of this issue's conditions, and a statement
of what the third one needs that this tree cannot yet give it.

Nothing here is measured. There is no unit, no pool and no bench, so every
sentence below about how a term moves is a claim about a design and not a reading
of a run. Where a claim would need a measurement to settle it, the run that would
produce it is named instead of a number.

## Term by term, and whether admission stands in the way

The signal is three ratios combined by taking the largest. The question this
issue asks is not which of them is largest in a healthy room. It is which of them
somebody can move while holding nothing, because a term that can be moved from
outside the admission path is a term that turns a flood into a purchase.

### Committed egress

It cannot be moved without an admission, and the route is the only route.

The numerator is the sum, over every reception the unit has accepted, of the
target bitrate of the layer it accepted. A reception exists because the control
plane told the unit to accept it, so the term moves when an admission succeeds
and at no other moment. Reaching it means getting past the join handshake on
issue #35 and past the authorisation decision on issue #34, and whatever those
two refuse, they refuse this term as well.

What a party who is past admission can do to it is a different question and it is
not an attack on the signal. A participant who subscribes to every source in a
large room raises this term exactly as much as the room costs, which is the term
reporting the truth. What bounds the bill there is subscription management on
issue #44 and the powers on issue #34, and what bounds it in money is the
operator's ceiling below.

### Committed packet rate

The same answer, for the same reason, and it is worth writing separately because
the two terms are not moved by the same room.

The numerator is the same sum taken over packet rates. A room of many small
audio streams reaches this term first and a room of few large video streams
reaches the egress term first, so which of the two binds is a property of the
room rather than of the attacker. Neither of them moves without an accepted
reception.

### Observed distress

It can be moved by somebody holding no admission, and this is the term the issue
was opened for.

It is the fraction of the last window in which the unit could not hand a packet
to the operating system when it wanted to. That is a property of the machine and
of its interface, and neither the machine nor the interface knows which packets
belong to a conference. Two routes reach it, and both are open to somebody who
has never been admitted to anything:

Filling the machine's outbound path. The term rises when a send would block, and
a send blocks when the queue in front of the interface is full, whoever filled
it. Anything sharing that interface reaches this term, including traffic that
this software neither accepted nor answered.

Spending the machine's processor and its interrupt budget. A machine occupied by
arriving packets it will discard is a machine that hands packets over more
slowly, and the term counts the stall and not the cause. This is the route that
needs no bandwidth to match the unit's own, and it is the one to measure first
when there is something to measure. The run that would settle how much pressure
is needed is the calibration on issue #54, which is the same run that fixes the
denominator this term is a ratio to.

Neither route is refused by anything in this tree, because there is nothing in
this tree that forwards a packet. Reachability, which decides what a unit answers
and from where, is issue #52.

## What this decides, in the words the issue asks for

The distress term is not bounded so that it cannot start a machine on its own. It
may, and this document states why it may rather than bounding it.

The reason is what the term is for. It is the term that exists for the failures
nobody modelled, it can only ever raise the number, and a unit that is failing
for an unmodelled reason has to be able to say so with the one number it reports.
Bounding it below the level at which the pool acts would leave the signal
reporting health while the machine is failing, which is the exact defect
[capacity-signal.md](capacity-signal.md) argues the term into existence against.
Removing it costs more than the attack does.

There is one bound already in the design, it is real, and it is smaller than it
looks. The scale-out condition in [scaling-triggers.md](scaling-triggers.md) is
over the pool rather than over one machine: the pool asks for a unit when the
smallest load among the units still eligible has reached 0.75. So distress on one
unit of several does not ask for anything while another eligible unit is below
that level, and reaching the trigger means reaching every eligible unit at once.
On a pool of one it is not a bound at all, because the smallest load among
eligible units is that unit's load. The smallest number of units the pool keeps
is one of the three numbers the operator sets in
[what-an-operator-may-set.md](what-an-operator-may-set.md), so how much this
bound is worth in a given deployment is the operator's to decide and is not this
document's to claim.

## What the operator sees, and what they set instead

What bounds the bill is not in the signal. It is the largest number of units the
pool may grow to, which is the operator's under
[what-an-operator-may-set.md](what-an-operator-may-set.md) and is named there as
the hard ceiling on cost. Sustained pressure on every eligible unit walks the
pool up to that ceiling and stops there, so the worst case is a bill the operator
capped rather than an unbounded one. The cooldown, the hysteresis and the minimum
time a unit lives are issue #62 and bound how fast that walk can be. Where the
pool cannot grow at all, the refusal is the fail-closed behaviour on issue #64.

What the operator does not see is which of the three terms did it. The port
carries one number, [media-plane-port.md](media-plane-port.md) fixes the result
of ReportCapacity as the load and nothing else, and
[capacity-signal.md](capacity-signal.md) gives the reason: the placer reads dozens
of these on every join and cannot be doing arithmetic over vectors. So a room
filling up and a machine under pressure arrive at the pool as the same number,
and the operator's view on issue #66 has nothing to tell them apart with.

That is the whole of what an operator is left with today: a ceiling that bounds
the money and no answer to the question of why the money was spent.

## What would bound it, and why that is not written here

The repair is attribution rather than a threshold. If the pool could tell a
scale-out driven by committed load from one driven by distress, it could treat
the two differently, and the operator's view could say which happened. That is
the third condition of issue #150 and it is not met by this document.

It is not met here because it is a change to two landed decisions and to three
things that do not exist. ReportCapacity would have to carry more than one
number, which contradicts [media-plane-port.md](media-plane-port.md) at the
operation and [capacity-signal.md](capacity-signal.md) at the sentence that keeps
the answer off the port. The unit that would compute the second number is issue
#55, the pool that would read it is issue #56, and the placer that ranks on it is
the seam in [placement-seam.md](placement-seam.md). Amending two documents ahead
of all three would leave the board with a decision nothing follows, which is the
shape a reader cannot tell from a decision that was implemented.

So the amendment is named and not made. Whoever takes the third condition takes
both documents with it, and the argument they need is here rather than waiting to
be rediscovered: the cost of the second number is one more ratio on an operation
the placer already calls, and the thing it buys is the difference between an
operator who can see a flood and one who can only see a bill.

## What refuses any of this today

Nothing. `PROSE, NOT ENFORCEMENT`, issue #150.

There is no unit, no pool and no port implementation, so there is no route for a
check to read and nothing for it to refuse. The invariants table prints the rules
this tree declares and which of them run, and no rule here is among them:

    go run ./cmd/invariant

This document is held by a reader until the third condition of issue #150 lands,
and that condition is the first thing in it that a test could be written against.

## The residual, stated as a residual

The ceiling bounds the money and bounds nothing else. A machine under sustained
pressure serves the room it is holding worse while it is under pressure, whether
or not the pool buys anything, and no number an operator sets changes that. That
half is not a scaling failure and it is not repaired by attribution either;
[../threat-model.md](../threat-model.md) says the same thing about it and this
document does not improve on it.

There is no measured figure anywhere in this document. Nothing has been run
against a unit, because there is no unit, and no sentence above should be read as
though something had been.
