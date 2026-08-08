# Placing a new conference, and what the other policies would have cost

This is the policy that answers where a conference nobody has admitted anybody to
yet is put. It answers issue #57.

The contract it sits inside is
[placement-seam.md](placement-seam.md), which fixes what the placer may read,
what it may answer, and that the same three records always produce the same
answer. That document also specifies the policy below under "The naive placer",
so nothing here is a new decision about the shape. What is decided here is that
this is the policy the first release ships with, and what the alternatives in
issue #57 would have cost, which the seam does not say.

The loads it compares are the signal in [capacity-signal.md](capacity-signal.md).
The arrangement a conference takes once it outgrows one unit is
[room-topology.md](room-topology.md).

## The policy

A unit is eligible when the pool says it is admitting and its effective load is
below the point at which a unit takes nothing new.

Among the eligible units, the conference goes to the one with the lowest
effective load. Ties are broken by the smallest unit identifier, so the answer is
total rather than merely deterministic and does not become an accident of the
order the pool view happened to return its rows in.

If no unit is eligible, the answer is a refusal, and a pool with no rows at all
refuses with a different reason from a pool whose rows are all full or draining.

It is in `internal/placement`, as `Naive` behind `ConferencePlacer`.

## Why the emptiest unit

Because of what a wrong answer costs at this moment rather than because it is the
better shape in general.

A new conference has no size. Nothing about it says whether three people or three
hundred will arrive, and the placer is asked before the first of them is
admitted. A policy that fills one unit before starting another is making a
prediction about that number, and the room it gets wrong is the one it just put
on a unit that had almost nothing left. That room reaches a second unit within
minutes, and from then on every remote subscriber in it pays the extra link
[room-topology.md](room-topology.md) prices, for the whole of its life.

Choosing the emptiest unit makes no prediction. It gives every new conference the
largest amount of room the pool currently has, which is the arrangement under
which the most conferences stay on one unit, and staying on one unit is what the
topology document says is worth having.

The soft decision point in [capacity-signal.md](capacity-signal.md), where a unit
stops being a preferred home for a new conference while staying eligible, is a
consequence of this order rather than a second rule beside it. Ordering by lowest
effective load already prefers every unit below that point to every unit above
it. Writing it a second time as a comparison would be a second place to get it
wrong, and the two could then disagree.

## What filling one unit first would have bought, and what it costs

It buys two real things. Conferences stay whole, because a unit that is filled
deliberately is a unit whose remaining space is known rather than spread in
fragments across the pool. And it leaves whole units empty, which is what lets a
deployment scale in at all: a pool of six units each at a third of their capacity
cannot retire any of them without moving somebody, and
[scaling-triggers.md](scaling-triggers.md) is where that cost is argued.

What it costs is the prediction above. Filling a unit to its eligibility ceiling
before starting another means that at any moment there is exactly one unit with
space on it, and every conference created in that window lands on the same unit,
whatever their sizes turn out to be. The first of them to grow finds the unit
already carrying the others.

It also costs the thing this project is measured on. A unit filled deliberately
runs at the top of its range, which is where the quality curve on issue #4 is
steepest, so the whole pool's quality becomes a function of how accurate the
capacity signal is. Under the spread policy a unit reaches that part of the curve
only when the pool is genuinely running out, which is also the moment
[scaling-triggers.md](scaling-triggers.md) has already asked for another machine.

This is the alternative most likely to replace the policy above, and what would
settle it is the bench on issue #2 rather than more argument: the figure that
decides it is how often a conference outgrows the unit it was placed on, and
nobody has measured it.

## What preferring the nearest unit would need

It is not rejected on its merits, because it cannot be written today.

The placer is not given the participant's network location, and
[placement-seam.md](placement-seam.md) says why that field is absent rather than
forgotten: no decision on this board describes a pool spread across sites, and a
field that exists before there is a policy reading it is a field the first placer
uses for something else. Adding it is a change to that document and to the record
the pool fills in.

It is also meaningless in the deployment the bundle on issue #111 describes,
which is one machine, and close to meaningless in the deployment behind it, which
is two machines in one rack. So the cost of not having it is nothing until a
deployment spans locations, and the cost of having it early is a field in the
seam that every later policy has to keep answering for.

## The refusal

A refusal is an answer and not an error, which is
[placement-seam.md](placement-seam.md)'s own sentence, and it is why the placer
returns no error at all.

Two of the three reasons that document names can arise here. A pool with no rows
at all is one nothing has registered into, and the answer to it is not to grow
the pool but to find out why it is empty. A pool whose rows are all draining,
gone, or at or above the ceiling is a pool that is full, and that is the event
the scale-out condition in [scaling-triggers.md](scaling-triggers.md) is written
against.

The third reason, the conference reaching the unit ceiling in
[room-topology.md](room-topology.md), cannot arise for a conference that is on no
unit. It arrives with participant placement on issue #58 rather than being
declared here and never returned.

What a refusal must not be is a placement onto a unit that is already refusing.
A room that is running is worth more than a room that has not started, and a new
conference put onto a saturated unit damages the first to serve the second.

## What this policy does not do

It reads the load and nothing else, so it does not know that the conference
arriving is a lecture rather than a meeting, and it could not act on that if it
did.

It does not weigh what placing a conference on a unit costs the units already in
some other conference's mesh, which is the term
[room-topology.md](room-topology.md) makes explicit. For a conference that is on
no unit that term is zero, so the omission is exact here and is not exact for
issue #58.

It holds nothing between calls. A later policy that wants to avoid a unit it was
recently refused by reads that from an input, which means adding a field to the
pool record and writing down what it means here, and that cost is deliberate: it
is what keeps every future placer as testable as this one.

## What would change this document

The measurement on issue #2, which is how often a conference outgrows the unit it
was placed on, since that is the figure separating this policy from filling one
unit first.

A pool that spans sites, which is what would make the nearest-unit policy
answerable rather than unwritable.

A change to the eligibility point in [capacity-signal.md](capacity-signal.md),
which is derived there from a rate and an interval neither of which has been
measured.
