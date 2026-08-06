# The room topology, and where a second unit enters

This says how one conference is arranged when it does not fit on one forwarding
unit. It answers issue #7. It uses the vocabulary of
[media-plane-port.md](media-plane-port.md) without redefining it, and the number
it turns on is the load defined in [capacity-signal.md](capacity-signal.md).

Nothing here is measured. There is no bench yet, issue #2 builds it, and the two
places below where a value would be needed name the run that produces it instead
of carrying a number.

## The rule

A conference lives on one unit for as long as it fits. When it does not, it is
carried by several units, and every unit carrying part of it holds a link to
every other. There is no root, no relay-only unit and no tier.

Which of the two states a conference is in is not configured and is not a
property of the conference. It is decided one placement at a time by the placer
on issue #11, from the load of the units in the pool. A conference occupies a
second unit at the moment a participant cannot be placed on the unit already
carrying it, and for no other reason. The condition is therefore the placer's
refusal, which [capacity-signal.md](capacity-signal.md) fixes at a load of 0.90,
and the thing that evaluates it is the placer.

Nothing moves a conference that is already placed. A room that grew across two
units stays across two units until it ends, because moving a live participant
between units costs them a reconnection and buys a smaller inter-unit bill,
which is the wrong trade for the party who did not ask for it.

## Why a mesh

The four shapes differ in what they cost per subscriber and in what they take
down when a machine dies, and for the room this project is built for those two
answers point the same way.

The cost of a link is paid per source, not per subscriber. A publisher's media
crosses a link between two units once, however many subscribers on the far side
receive it. So the inter-unit bill of a room of three hundred is set by how many
people are speaking, and speaker selection on issue #46 is what bounds that
number. A mesh whose cost does not grow with the subscribers is affordable in
exactly the room where the subscriber count is the problem.

A mesh adds one hop and never two. A subscriber on the publisher's unit gets no
extra delay at all. A subscriber anywhere else gets one link, and it is one link
whatever the size of the mesh, because every unit carrying the conference has a
direct path to every other. A tree cannot promise this: a subscriber on one leaf
receiving a publisher on another leaf pays two hops, and the shape gets worse as
it grows.

A mesh has no unit whose death is worse than any other's. A tree has a root, and
a root is a machine whose failure takes a conference that was otherwise healthy.
A tier split has the same problem in a different place, and it also assumes the
room stays a lecture. It stops being the efficient shape the moment the audience
starts talking, which is what the question queue on issue #78 exists to make
happen, so it is the wrong shape to build a room's arrangement on.

What a mesh costs is that its links grow as the square of the units. That is
what the ceiling below is for, and it is the reason this document does not treat
a large mesh as the normal case. The normal case is one unit. The mesh is what
happens when the room is larger than the machine, and it is bounded.

## The cost, as a formula

Let U be the number of units carrying one conference. Let B be the total bitrate
of the sources of that conference that any remote subscriber is receiving, which
after speaker selection is bounded by the number of forwarded speakers and not
by the number of participants. Let B(i) be the part of B published on unit i, so
that B is the sum of B(i) over the units.

Links in the mesh:

    U * (U - 1) / 2

Links held by one unit:

    U - 1

Inter-unit egress committed by unit i:

    (U - 1) * B(i)

Inter-unit egress committed across the whole conference:

    (U - 1) * B

None of those three carries a participant count, and that is the property the
shape was chosen for. Participants enter only through B, and only through the
ones that are being forwarded.

Added one-way delay for a subscriber, where L is the one-way latency of one
inter-unit link:

    0    if the subscriber is on the unit its publisher is on
    L    otherwise

There is no term in U. That is the difference between this shape and a tree, in
one line.

## The ceiling on U

Inter-unit egress is egress. It is committed the moment a link carries a source,
it counts against the same denominator as everything else the unit sends, and
[capacity-signal.md](capacity-signal.md) is where that denominator is defined. So
a mesh that grows without a bound spends a unit's capacity on its neighbours
rather than on its subscribers, and the room pays for its own arrangement.

The bound, with E the egress denominator of the unit and f the fraction of a
unit's egress the deployment is willing to spend on links:

    (U - 1) * B(i)  <=  f * E

f is not a number this document sets, and it is not an operator setting either,
because issue #12 fixes what an operator may set and this is not on that list.
It is derived from what the deployment wants a unit to be for, and the run that
produces the figures behind it is the cascade work on issue #59 measured on the
bench from issue #2. Until that run exists there is no honest value here, and
writing one down would be a number nobody could defend.

What happens at the ceiling is that the pool grows rather than the mesh. A
conference that has reached its unit ceiling and still cannot place a
participant is a refusal, and refusing is a real answer the placer is required
to be able to give. The alternative, adding a unit and quietly letting the links
eat the capacity the room was placed for, is the failure mode where everybody in
the room pays and nobody can see why.

## What happens when a unit dies

A unit carrying part of a conference has one role, and that is the point of the
shape. There is no root and no relay, so this section is short by construction
rather than by omission.

The conference is on one unit and that unit dies. The conference is gone. Every
participant is disconnected and rejoins, and the control plane places them
again, which will be onto a different unit because the pool has removed the dead
one. Nothing about the room survives on the media plane, and nothing needs to,
because the control plane holds what the room is and the media plane holds no
state the control plane cannot rebuild, which is what
[media-plane-port.md](media-plane-port.md) says a unit is.

The conference is on several units and one of them dies. The participants on the
dead unit are gone and rejoin. Everybody on the surviving units keeps their
session, keeps receiving each other, and loses only the sources that were
published on the dead unit. The links to the dead unit end on their own, because
a link ends when the conference ends on either side, and the port has no
operation that removes a link under a live conference. The surviving mesh is a
smaller complete mesh and needs no repair, since every remaining unit already
held a direct link to every other one. Rejoining participants are placed by the
placer like any others, which may put them back on the surviving units or on a
new one, and a new one is linked to the rest as it joins.

A unit dies while a link to it is being established. The port's LinkConference
promises nothing about the far side, and the control plane treats a conference
as spanning only once both units have answered, so a half-established link is a
conference that never spanned. Nothing has to be undone on the surviving side
beyond what the death itself already does.

How the death is noticed is not this document's business. The pool decides
liveness by asking, the fault stream shortens the delay rather than being the
mechanism, and both are in the port.

## The issues that implement this

The scaling milestone is where this shape becomes code. Issue #53 writes the
scaling design note this document is an input to. Issue #57 places a new
conference and is where the one-unit case starts. Issue #58 places a participant
joining a conference that is already running and is where the second unit is
first reached for. Issue #59 cascades one conference across units and is where
the mesh and its ceiling are built and measured. Issue #61 drains a unit and
retires it without dropping a session, which is the planned version of the death
described above.
