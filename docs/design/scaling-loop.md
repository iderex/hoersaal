# The scaling loop

This is one description of how the pool grows, shrinks and places work, put
together out of the decisions that were made separately. It answers issue #53
and lands before the code below it.

It decides nothing. Every number in it belongs to a decision document and is
pointed at rather than repeated, because a value restated in two places drifts in
one of them and the reader cannot tell which. The four documents it is assembled
from are [capacity-signal.md](../decisions/capacity-signal.md) for the signal and
its levels, [scaling-triggers.md](../decisions/scaling-triggers.md) for when the
pool grows and shrinks, [room-topology.md](../decisions/room-topology.md) for the
arrangement of a conference across units, and
[placement-seam.md](../decisions/placement-seam.md) for the contract of the
component that chooses one. The operations between the control plane and a unit
are in [media-plane-port.md](../decisions/media-plane-port.md) and are used here
with the names that document gives them.

Nothing here is measured. Nothing in the four documents it rests on is measured
either, and the two quantities the whole loop turns on, the rate at which load
rises and the time it takes to have a unit, are both unmeasured today.

## The components

Six, and each one is named the same way everywhere below.

The control plane is what handles a join. It holds what a room is, it calls the
placer, it calls the port, and it is the only component that talks to a unit.

The placer answers which unit carries a thing. It is a function rather than a
service, it holds nothing between calls, and its whole contract is
[placement-seam.md](../decisions/placement-seam.md). Issue #39 decides whether
the control plane runs as one instance or several, and the seam is written so
that the answer does not change the placer.

The pool is the authoritative record of which units exist, what state each is in,
and the load each last reported with the time that answer arrived. Its
registration, its states and what makes its view authoritative are issue #56.

The provisioner makes a unit exist and makes one stop existing. Its interface and
its first driver are issue #63.

A unit is one forwarding unit, reached only through the port. It holds
conferences, participants, receptions and links, and it holds no state the
control plane cannot rebuild, which is what
[media-plane-port.md](../decisions/media-plane-port.md) says a unit is.

The operator view is what a person watching the deployment sees while any of this
happens. It is issue #66.

## The messages

Every message in the loop, with its direction and what it carries.

The pool asks a unit for its load, with ReportCapacity, and the unit answers one
number and nothing else. The answer says nothing about when it was computed, so
the pool records the time the answer arrived and decides for itself when an
answer is too old to use.

A unit tells the pool what it has lost, over the ReportFaults stream. A notice
names a whole unit, a conference, a participant or a link. A notice means the
thing is gone and is not coming back on its own. Silence means nothing at all,
which is why the pool decides liveness by asking and this stream shortens the
delay rather than being the mechanism.

The control plane asks the placer to place a conference, or to place a
participant, passing three records and nothing else: the pool, the conference,
and the arriving participant.

The placer answers with one unit, or with a refusal carrying one of three
reasons. No eligible unit, the conference has reached its unit ceiling, or the
pool holds no units at all. A refusal is an answer and not an error.

The control plane calls the port on the unit it was given. OpenConference if the
conference is not on that unit, LinkConference on both sides if the unit is new
to a conference that is already somewhere else, AdmitPublisher or AdmitSubscriber
for the participant, and SetReception for what that participant receives. Only
then does it tell the client anything, so the mesh is complete before a
participant on a new unit can be heard.

The control plane, or the pool on its behalf, asks the provisioner for a unit,
and asks it to release one. What that call looks like is issue #63.

The pool tells the operator view what is happening while it changes, which is
issue #66, and the wait a joining participant is held in is one of the things
that has to be visible there.

## The states of a unit

Five, and who may move a unit between them is as much of the answer as the names.

Two vocabularies for these states already exist and they do not agree, which is
worth stating here rather than resolving quietly.
[placement-seam.md](../decisions/placement-seam.md) names three, admitting,
draining and gone, because those are the three the placer reads. Issue #56, which
is where the pool is built, names five: requested, starting, serving, draining
and gone. The disagreement is over one name, since #56's serving and the seam's
admitting are the same state.

This note follows #56 for the shape, which is five states and the same
transitions, and follows the seam for the name of the one they differ on. The
seam is a landed decision and #56 is an open issue, so where the two collide the
landed one wins until #56 lands and says otherwise. Whichever name survives, one
of the two documents has to change, and that is #56's to settle.

The reason there are five rather than the four the wording of issue #53 uses is
the one #56 already gives: the interval in which a machine exists and does not
yet answer the port belongs to the provisioner and not to the pool, and
collapsing it into the requested state leaves the pool unable to tell a request
that has produced nothing from one that has produced a machine which is still
starting. Those two want different responses when the wait runs long.

Requested. The provisioner has been asked for a unit and nothing exists yet. The
pool creates this state when it decides to grow. It is a real state rather than a
gap, because the interval it covers is the one the whole scale-out inequality is
about, and a pool that cannot see its own outstanding request will ask twice.

Starting. The provisioner reports that a machine exists and it does not yet
answer the port. Only the provisioner moves a unit into this state.

Admitting. The unit has registered with the pool and answers the port. Only the
unit's own registration moves it here, and this is the only state in which the
placer may choose it.

Draining. The unit takes no new work and keeps what it has until those
conferences end on their own. The pool moves a unit here, either because the
scale-in condition held or because an operator asked. Nothing else may, and in
particular the placer never does: the placer reads the state and does not
interpret it, and a draining unit is simply not eligible.

Gone. The unit is not there. The pool moves a unit here when it stops answering
or when a drain finished and the provisioner released it. A unit in this state
never returns; a machine that comes back registers as a new unit, because the
port's Lost answer is what a restarted unit gives and a control plane that
treated it as the same unit would keep believing in conferences that are not
there.

## What is authoritative and what is a view

The pool is authoritative for which units exist and what state each is in. Every
other component reads a view of it, and no other component may write it.

The load in the pool is the last number the unit reported plus the load of every
placement made against that unit since it was reported. That sum is the effective
load and it is what the placer reads. This is possible only because the signal is
committed rather than measured, so a placement's cost is known when it is decided
rather than when the unit notices it, and it is what makes two placements in the
same moment visible to each other.

The unit is authoritative for what it is actually holding, and the difference
between the two is not an error to be eliminated. The port's Refused exists
exactly so that the control plane's view being slightly behind is not a failure.

The control plane is authoritative for what a room is, including which units
carry a conference. That record is not the pool's, and the two answer different
questions: the pool says which machines exist, the control plane says which of
them this conference is on. Two components disagreeing about which units exist is
the failure that produces a room believed to be on a machine that no longer runs,
which is why there is one writer and not two.

The placer is authoritative for nothing. It holds no state across calls, every
tie is broken by the unit identifier in ascending order, and the same three
records always produce the same answer.

## The loop, from a signal to a conference using a new unit

Every unit that is admitting or draining reports its load, and the pool records
each answer with the time it arrived.

The pool asks for a unit when the smallest load among the units still eligible to
admit has reached the level
[scaling-triggers.md](../decisions/scaling-triggers.md) sets for it, and has been
there across two consecutive reports with no averaging. That level, and the
inequality that placed it, are in
[capacity-signal.md](../decisions/capacity-signal.md) under issue #8. The
condition is over the pool and not over one machine, so one busy unit beside four
empty ones does not buy a fifth.

The unit is Requested. The existing units go on admitting exactly as before,
because the gap between the level that triggers the request and the level at
which a unit refuses is there to be spent here. In the case this is designed for,
nobody waits and nobody notices.

The provisioner makes the machine, the machine registers, and the unit becomes
Admitting. From the pool's point of view nothing is different about it except
that its load is low, so it wins the next placement by the ordinary rule rather
than by a rule about new units.

A conference is placed when it is created, and a participant is placed when they
join a conference that already exists. Both go through the same seam. With an
answer, the control plane calls the port in the order the placement document
fixes, and the conference spans a second unit only once LinkConference has been
answered on both sides.

A conference reaches a second unit at the moment a participant cannot be placed
on the unit already carrying it, and for no other reason. That is
[room-topology.md](../decisions/room-topology.md)'s rule, and the condition is
the placer's refusal rather than a setting.

The pool shrinks under a condition that is deliberately not the mirror of the one
that grew it, over a window long enough that a quiet stretch between lectures
does not sell a machine that has to be bought back. Both are in
[scaling-triggers.md](../decisions/scaling-triggers.md) under issue #9. A unit is
Draining while it empties, and it is Gone when the provisioner has released it.

## The timing

Four intervals matter, and only two of them are fixed anywhere.

The reporting interval is how often a unit's load reaches the pool. No decision
document fixes it, and this note does not fix one either. It is owed by the work
that reports the signal from a unit, which is issue #55, and it is bounded from
one side by the observation window below and from the other by the gap between
the level at which a unit stops taking new work and its calibrated capacity,
which [capacity-signal.md](../decisions/capacity-signal.md) says exists to cover
one interval of growth. Writing a number here would be inventing the value that
two open issues are supposed to produce.

The observation window is two consecutive reports with no averaging, fixed by
[scaling-triggers.md](../decisions/scaling-triggers.md).

The scale-in window is long and is fixed as a first setting by that same
document, with the reason it is a property of the demand rather than of the
signal.

The provisioning time is the interval between asking for a unit and that unit
taking participants. It is called T there, it is unmeasured, and issue #63 is the
work that produces it. The bounded wait a joining participant is held in is
written against it.

The cooldown between decisions, the hysteresis, the minimum time a unit lives
before it may be retired, and the floor below which the pool does not shrink are
issue #62 and are fixed nowhere yet. This note does not fix them either, and the
loop above is written so that adding them narrows when a transition may happen
without changing what the transitions are.

## What each component does when another stops answering

The unit does not answer the port. The caller gets Unavailable, which means it
does not know whether the operation happened. It treats the unit as suspect,
reports it to the pool, and asks the placer again with that unit excluded. One
Unavailable is not a death. The pool decides liveness by asking, and the fault
stream only shortens the delay.

The unit answers and reports that state the caller believed in is gone. That is
Lost, and it is the answer a restarted unit gives. The control plane rebuilds
what it needs on a unit the pool still says is admitting, or treats the
conference as having lost that unit if it is not. Treating Lost and Unavailable
alike is what either tears down live conferences on a slow network or leaves dead
ones standing, and the port keeps them apart so that this loop can.

The unit dies. What that costs a conference is the failure section of
[room-topology.md](../decisions/room-topology.md), per role the unit could have
had, and the short version is that a mesh has no unit whose death is worse than
another's. The participants on the dead unit rejoin and are placed again. The
surviving units keep their sessions and lose only the sources that were published
on the dead one. The links to the dead unit end on their own.

The placer refuses. That is not a failure and the loop is written on it. A new
conference waits and the pool is asked to grow. A joining participant waits in
the bounded and visible way
[scaling-triggers.md](../decisions/scaling-triggers.md) specifies, and is refused
with the reason if the wait runs out. A refusal for the conference ceiling does
not grow the pool, because another unit would not be allowed to carry that
conference anyway.

The placer cannot be asked. It is a function in the process handling the join, so
there is no case where it is unreachable while the control plane is running.
There is also nothing to restart for placement to work, and that is the reason it
was made a seam rather than a service.

The provisioner does not answer, or answers that it cannot make a unit. The pool
cannot grow, and the loop's answer is to refuse rather than to admit into
capacity that does not exist. That refusal is issue #64, it says why rather than
timing out, and it is visible in the operator view on issue #66. A pool that
cannot grow keeps serving what it already holds; nothing about an existing
conference changes because a new machine could not be made.

The pool's own view is stale or unavailable. The placer is handed the view it has
and answers over it, and the unit's Refused absorbs the difference. A control
plane that cannot read the pool at all cannot place, and a join that cannot be
placed is a join that waits and then is refused with the reason, which is the
same path as a full pool.

## What the loop does not do

It does not react to a single sample. Every condition is over a window, and the
distress term that could spike is the one the second sample exists to filter.

It does not move a conference or a participant that is already placed. Not to
balance the pool, not to make a retirement possible, and not to reduce the
inter-unit bill. Moving a live participant costs them a reconnection and buys a
smaller cost for somebody else, and that is not a trade to make on behalf of the
person who pays it.

It does not provision for a lecture that has not started. Nothing in this loop
reads a timetable, and no part of it acts on demand that has not arrived. That is
a deliberate absence rather than an oversight: it would be a second trigger with
its own inputs and its own failure mode, and it would fire on a room nobody
turned up to.

It does not degrade a room to admit one more person. A unit at the level where it
takes nothing new refuses, and the layer and congestion decisions respond to what
a path can carry rather than being a way to fit an arrival in.

It does not end a room so that a machine can be released. The machine is the
cheaper of the two.

## The issues on this milestone, and the section each implements

The mapping is written from this side. Whether each issue names its section is
the fourth condition on issue #53 and is not something this document can assert
about another issue.

Issue #54 derives what one unit holds and produces the denominators the signal is
a ratio to. It implements what "The components" says the unit is and what
[capacity-signal.md](../decisions/capacity-signal.md) leaves to a calibration.

Issue #55 reports the signal from a unit and proves it leads quality. It
implements the first message in "The messages" and owes the reporting interval in
"The timing".

Issue #56 builds the pool. It implements "The states of a unit" and "What is
authoritative and what is a view", which are the two sections this note would be
wrong without.

Issue #57 places a new conference onto a unit, and issue #58 places a participant
joining a conference that is already running. Together they implement the
placement half of "The loop", against the seam rather than against this note.

Issue #59 cascades one conference across units. It implements the sentence in
"The loop" about a conference reaching a second unit, and the mesh and its
ceiling in [room-topology.md](../decisions/room-topology.md).

Issue #60 triggers a scale-out once and proves it happens before quality moves.
It implements the trigger in "The loop" and is the run that tests the claim the
signal makes.

Issue #61 drains a unit and retires it without dropping a session. It implements
the Draining and Gone states and the scale-in half of "The loop".

Issue #62 sets the cooldown, the hysteresis, the minimum lifetime and the floor.
It implements the last paragraph of "The timing".

Issue #63 is the provisioning driver. It implements the Requested and Starting
states and produces the provisioning time in "The timing".

Issue #64 fails closed when the pool cannot grow. It implements the provisioner
paragraph in "What each component does when another stops answering".

Issue #65 proves the whole loop with no media and no hardware. It is the run that
exercises every section above, and it is possible because the placer is a
deterministic function with no clock, which
[placement-seam.md](../decisions/placement-seam.md) fixed for this reason.

Issue #66 says what is happening while the pool changes. It implements the last
message in "The messages" and the visibility the waiting participant and the
fail-closed refusal both depend on.
