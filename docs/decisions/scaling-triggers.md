# What triggers a new unit, and what retires one

This says when the pool grows, when it shrinks, and what happens to somebody who
arrives while it is still growing. It answers issue #9. The number every
condition below is written on is the load defined in
[capacity-signal.md](capacity-signal.md), and the levels it fixes are used here
rather than restated with different values.

Nothing here is measured. Every figure is a first setting with the run that
confirms or moves it named beside it, and the two quantities the whole design
turns on, the rate at which load rises and the time it takes to have a unit, are
both unmeasured today.

## Scale out

The pool asks for a unit when no unit that could take the arriving work is below
0.75. Written as a condition over the pool rather than over one machine: the
smallest load among the units still eligible to admit has reached 0.75.

On a pool of one unit that is exactly the sentence in
[capacity-signal.md](capacity-signal.md), which is where 0.75 comes from and
where the inequality that placed it is argued. The generalisation is what a pool
of more than one needs, because a single unit at 0.78 beside four empty ones is
not a pool that is short of capacity, and a trigger that fired on it would buy a
machine to sit next to four idle ones.

It is a level and not a rate of change. A rate estimator is a second mechanism
with its own smoothing and its own arguments about how much history to keep, and
the rate is already accounted for once, in where the level sits: the gap between
0.75 and 1.00 exists precisely to be larger than the load that arrives while a
unit is being provisioned. Putting the rate in a second place would mean two
mechanisms that can disagree about the same room.

### The observation window

The condition has to hold across two consecutive reports of the load, and there
is no averaging.

It is deliberately near the shortest window that means anything, because the
window is bought with the only budget there is. Every interval spent confirming
is another interval of growth against a gap of 0.25, and the inequality in
[capacity-signal.md](capacity-signal.md) has to hold with the window inside it,
not beside it.

A short window is affordable here for a reason that would not hold for a
measured signal. The load is committed rather than observed: it moves when the
control plane admits or releases a reception, which is something the control
plane did, so the number the unit reports is a confirmation of a change the
control plane already knows about. It does not have the sampling noise that
makes a measured signal need smoothing. The second sample is there so that one
stale or lost report cannot start a machine, not to filter a signal that jitters.

The distress term is the exception, since it is observed and does have noise. It
is the term that can only raise the number, and a distress spike that clears
inside one reporting interval will not survive two samples, which is the second
thing the window is for.

## Scale in

Retirement is separate from admission and its conditions are not the mirror of
the ones above. It is deliberately harder to retire a unit than to buy one.

A unit becomes a candidate for retirement when everything it is holding would
fit on the rest of the pool with every remaining unit staying below 0.60 after
the move, and that has been continuously true for the whole of the scale-in
window. 0.60 is the level at which a unit stops being a preferred home for a new
conference, and retiring into a pool that is above it would mean retiring a
machine and immediately buying another.

The retirement itself never interrupts anybody. The candidate stops being
offered new work, keeps everything it already has until those conferences end on
their own, and is released when it is empty. That is the drain on issue #61. A
conference that outlives the drain keeps its unit; there is no time after which
a room is ended so that a machine can be released, because the machine is the
cheaper of the two and the room is the thing the service is for.

Nothing is moved between units to make a retirement possible. The reason is the
same one in [room-topology.md](room-topology.md): moving a live participant
costs them a reconnection and buys a smaller bill, and that is not a trade to
make on behalf of somebody who did not ask for it.

### The scale-in window

Long, and set by the shape of the demand rather than by the shape of the signal.
The first setting is one hour.

The reason is what this software is for. Lectures start on the hour, and a pool
that releases a machine twenty minutes into a quiet stretch has to buy it back
before the next one begins, paying the provisioning delay again with a full room
waiting on it. A window shorter than the period of the demand turns the pool
into a machine that buys high and sells low, every hour, forever.

One hour is a first setting because it is the period a teaching timetable has,
and it is the value that stands until a deployment's own arrival pattern
disagrees with it. The soak work on issue #70 is the run that would show the
disagreement, since it is the only one that watches the pool move over hours.
The floor below which the pool does not shrink at all, the minimum time a unit
lives before it may be retired, and the cooldown between decisions are on issue
#62 and are not fixed here.

## Which way this project prefers to be wrong

This project prefers to buy a machine it did not need over making a room wait for
one it did.

That sentence is the one the tuning issues are held to. Issue #62, which sets the
cooldown, the hysteresis, the minimum lifetime and the floor, issue #60, which
proves a scale-out happens before quality moves, issue #71, which measures the
join storm, and issue #72, which measures the cost per participant, all move
numbers that sit between these two failures, and where a change makes one of them
better and the other worse, this is the direction it is allowed to go.

The reason is not that the money does not matter. It is that the two costs are
not the same kind of thing. A machine nobody needed appears on a bill, where the
operator can see it, count it, and cap it with the floor and the ceiling that are
theirs to set. Quality lost at the top of the hour is paid by three hundred
people who did not choose it, cannot see why it happened, and mostly do not
report it. The first cost is measured on issue #72 and published on issue #113.
The second is only visible on the curve on issue #4, after the lecture is over.

There is a limit to this, and it is the honest cost statement on issue #14. A
pool that is wrong in this direction all day stops being a preference and
becomes the embarrassment that issue warns about, which is why the eagerness is
bounded by the floor and the cooldown on issue #62 rather than left open.

## While the pool is growing

The interval between asking for a unit and having one is tens of seconds on real
hardware. It is called T in the inequality in
[capacity-signal.md](capacity-signal.md), it is unmeasured, and issue #63 is the
work that produces it, including the part where an image is pulled onto a machine
that did not exist a moment ago, which issue #110 measures.

The existing units keep working normally throughout. Nothing changes about how
they admit, because the gap between 0.75 and 0.90 exists to be spent exactly
here: a unit that triggered the request at 0.75 goes on taking participants and
conferences while the new one is being made, and only stops at 0.90. In the case
this is designed for, nobody waits and nobody notices, because the pool started
growing while there was still room.

A participant arriving when every eligible unit has reached 0.90 is the case
where the window was not enough. They are not admitted into a room that has no
capacity for them, and they are not silently dropped either. The control plane
holds the join, tells the client that the room is being given more capacity, and
admits them as soon as a unit takes participants. What the person is shown at
that moment is the client's decision on issue #76 and is not settled here; what
this document fixes is that the server's answer is an explicit wait with a reason
rather than a failure the client has to guess at, and that the wait is visible in
the operator's view on issue #66.

The wait is bounded. If no unit has taken the participant within the provisioning
time issue #63 measures, plus a margin, the join is refused with the reason
rather than held longer, because an indefinite wait is a promise the pool has
already failed to keep and pretending otherwise costs the person the time they
would have spent joining something else. Where the pool cannot grow at all, that
refusal is the fail-closed behaviour on issue #64, and it says so rather than
timing out.

Degrading the room to fit one more person is not what happens here. The layer and
congestion decisions on issues #45 and #49 respond to what a path can carry, and
they are not a way to admit a participant the unit has no capacity for. A unit
above 0.90 refuses, and the refusal is what causes the pool to be short by a
machine rather than causing everybody already in the room to pay for the arrival.
