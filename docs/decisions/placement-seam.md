# The placement seam, and what the placer may read

This is the contract of the component that answers which unit carries a thing.
It answers issue #11. The load it reads is defined in
[capacity-signal.md](capacity-signal.md), the shape it places into is in
[room-topology.md](room-topology.md), and what the caller does with a refusal is
in [scaling-triggers.md](scaling-triggers.md).

The seam exists because this is the component most likely to be replaced. The
first policy is a guess that will be wrong the moment real rooms disagree with
it, and the cost of being wrong should be one function rather than a rewrite. So
what the placer may read is written down before any placer exists, and it is
written as a small set rather than as an exclusion list, because a component that
can read everything is one nobody can test.

## The two questions

The placer answers one question in two forms.

Place a conference. The conference does not exist on any unit yet and nothing has
been admitted for it.

Place a participant. The conference is already running on one or more units, and
one more participant is arriving.

They are one seam and not two, because the answer is the same kind of thing and
the second is the harder version of the first. They are separate calls because
their inputs differ, and because the participant case is the only one that has a
conference to keep together.

## The inputs

Three records. Nothing else is passed and nothing else is reachable.

**The pool.** For each unit the pool holds: its identifier, its effective load,
the time the load it is derived from was reported by the unit, and whether it is
admitting, draining or gone. The states are the pool's, on issue #56, and the
placer reads them without interpreting them; a draining unit is not eligible and
that is the whole of what draining means here.

Effective load is the load the unit last reported plus the load of every
placement made against that unit since, and it is what the placer reads rather
than the reported number. This is possible only because the signal is committed
rather than measured, so a placement's cost is known at the moment it is decided
and does not have to wait for the unit to notice it. It is also what makes two
placements in the same second visible to each other, which the section on
simultaneous placement turns on.

**The conference.** Its identifier, and for a participant placement, the units
already carrying it with the number of participants each holds and the bitrate of
the sources of this conference each is publishing. That last figure is B(i) in
[room-topology.md](room-topology.md), and it is there so that a placer can see
what adding a unit to a conference would cost the units already carrying it.
Nothing about the people in the conference is passed, because the placer has no
use for them and a placer that could read them would be a second place where
personal data lives.

**The arriving participant.** Whether it will publish, and if so the sources it
offers with their layer arrangement. That is all that is known at that moment and
it is all that is passed.

The participant's network location is deliberately not in the set. It would
matter for a pool spread across sites, that pool is not what any decision on this
board has described yet, and a field that exists before there is a policy that
reads it is a field the first placer will use for something else. Adding it later
is a change to this record and to this document, which is what writing the
contract down is for.

## The answers

Two, and there is no third.

**Place it on this unit**, naming one unit from the pool it was given.

**Refuse**, with one of three reasons. The pool holds no eligible unit. The
conference has reached the unit ceiling in
[room-topology.md](room-topology.md) and no unit already carrying it can take
more. The pool holds no units at all.

A refusal is an answer and not an error. It is the normal way a pool that is full
says so, it is the event the scale-out condition in
[scaling-triggers.md](scaling-triggers.md) is written against, and a seam that
could not express it would force the caller to read a failure as a policy
statement.

## What the caller does with each

With a placement, the caller calls the port. It opens the conference on the unit
if it is not there, admits the participant, links the unit into the conference's
mesh if it is new to that conference, and only then tells the client anything.
The order matters: the mesh is complete before a participant on a new unit can be
heard by anybody, and the port's LinkConference is called on both units and the
conference treated as spanning only once both have answered.

If the unit answers Refused, the caller asks the placer again with that unit
excluded from the pool it passes. This is bounded, and the bound is a small
number of attempts rather than a retry until success, because a pool where three
units in a row refuse a placement the view said they would take is a pool whose
view is wrong, and the honest answer at that point is the refusal.

If the unit answers Unavailable, the caller does not know whether the operation
happened, so it treats the unit as suspect, reports it to the pool, and asks
again with that unit excluded. The pool decides liveness by asking, which is the
port's own statement, and one Unavailable is not a death.

With a refusal, a new conference waits and the pool is asked to grow. A joining
participant waits, in the bounded and visible way
[scaling-triggers.md](scaling-triggers.md) specifies, and is refused with the
reason if the wait runs out. A refusal for the conference ceiling is not a reason
to grow the pool, because another unit would not be allowed to carry this
conference anyway, and growing on it would buy a machine for a room that cannot
use it.

## Where placement runs

In whatever is handling the join, as a function of the three records above.

It is not a service, not a process and not a place. There is no placement server
to be a single point of failure for every join in the deployment, and there is
nothing to deploy, scale or restart for placement to work. Whether the control
plane runs as one instance or several is issue #39 and is not settled here; the
seam is written so that the answer does not change it. One instance calls a
function. Several instances each call the same function over the pool view they
share, and the pool view being shared is what issue #56 is about.

## Two placements at the same moment

The placer being a pure function of its inputs makes this more likely rather than
less. Two instances reading the same pool view make the same choice, so they do
not scatter across the pool the way two randomised placers would. They collide,
on purpose, on the unit that is genuinely the best answer.

Two things absorb it. The first is that the pool view holds effective load,
which includes commitments made and not yet reported, so an instance that has
placed has already moved the number the other instance reads. Whatever
serialisation the pool view uses for its own writes is therefore the
serialisation for placement, and there is no second mechanism to keep in step
with it. The second is that the unit itself refuses. The port's Refused means the
unit could do this and will not, because of what it is already holding, and it
exists exactly so that the control plane's view being slightly behind is not a
failure. The caller then asks again with that unit excluded, which is the path
already described above.

So it is absorbed rather than prevented, and that is the choice. Preventing it
means a lock held across a call to a machine that might not answer, which turns
every slow unit into a stall on every join in the deployment.

## Whether the placer is deterministic

It is. The same three records produce the same answer, and the placer holds
nothing across calls.

There is no state to describe and no clock to control, which is what makes the
tests on issue #65 possible: the whole scaling loop can be proved by handing the
placer a pool and reading its answer, with no media, no hardware and no waiting.
Every tie is broken by the unit identifier in ascending order, so the answer is
total rather than merely deterministic, and a tie does not become an accident of
the order the pool view happened to return.

A later placer that wants history, such as one that avoids a unit it has recently
been refused by, does not get to keep it inside the placer. It reads it from an
input, which means adding a field to the pool record and writing down what it
means here. That is a deliberate cost, and it is the cost of keeping every future
placer as testable as this one.

## The naive placer

This is the one the scaling tests use. It is specified here so that it is a
stated policy the tests are written against rather than something invented
alongside them.

A unit is eligible when the pool says it is admitting and its effective load is
below 0.90.

To place a conference: choose the eligible unit with the lowest effective load,
ties broken by the smallest unit identifier. If none is eligible, refuse.

To place a participant: consider first the units already carrying the conference
that are eligible, and choose the one with the lowest effective load, ties broken
the same way. If none of them is eligible, consider the other eligible units, and
choose in the same order, subject to the conference's unit ceiling. If the
ceiling is reached, refuse with the ceiling reason. If nothing is eligible,
refuse with the pool reason.

What is naive about it, said plainly so that a later placer is not measured
against it as though it were the target. It reads only the load, so it does not
know that the participant arriving will cost more than the last one. It does not
account for what placing a publisher on a new unit costs every other unit in that
conference's mesh, which is the term
[room-topology.md](room-topology.md) makes explicit and which a real placer has
to weigh. It prefers the emptiest unit, which spreads conferences across the pool
and is the opposite of what the mesh cost wants. And it never refuses for a good
reason, only because nothing is eligible.

It is still the right thing for the tests, because the loop it proves is the one
where a room fills, a unit is asked for, it arrives, and a participant lands on
it. That loop is the same whichever policy sits in the seam, which is what having
a seam is for.
