# The media plane

This names the forwarding unit the service is built on and the shape it is used
in. It answers issue #5. The interface it is used through is in
[media-plane-port.md](media-plane-port.md), and this document does not repeat
it. The survey the choice is made from is in
[media-plane-port-candidates.md](media-plane-port-candidates.md), which was
written against the port and deliberately did not choose.

## What this is decided from, and what that bounds

Documents, at the addresses the candidates record names, read on 2026-08-06.
Nothing was deployed, no unit was started, no source tree was read and no figure
below was measured. So every statement about a candidate is a statement about
its published interface, which is weaker than a statement about the software,
and the rows where that difference could change the answer are named at the end
rather than left for a reader to find.

## The choice

An existing forwarding unit, Jitsi Videobridge, driven as a separate process
over its own control interface.

The means is that unit plus the interface it already publishes, and it fits
because it is the only candidate whose published interface answers all three of
the things the scaling model cannot do without: a conference held across more
than one unit, a normalised capacity number that is allowed to exceed one, and a
liveness answer the caller asks for rather than waits on. The two nearest
rejected candidates each fail the first of those, and the first is the one this
project exists to solve.

## The five questions, for the chosen unit

**Can it report the capacity signal.** Partly, and the rest is ours. It reports
`stress_level` on `/colibri/stats`, documented as zero for no load and one for
full capacity with values above one permitted, which is the same shape and the
same decision about not saturating as [capacity-signal.md](capacity-signal.md)
reached on its own. It is not the same number. The signal this project needs is
computed from what the unit has committed to send, and no candidate reports
that, which the candidates record states for all four. So the adapter computes
load from the receptions the control plane itself admitted, which it knows
without asking anything, and takes `stress_level` as the distress term the
capacity document already reserves for the failures the first two terms do not
model. The unit supplies the term that cannot be derived from our own
bookkeeping, and we supply the two that must lead.

**Can several units relay one conference.** Yes, and this is the answer that
decides the choice. The relay document describes pairs of bridges connected with
ICE and DTLS over either SCTP or websockets, and the bridge carries `relay_id`
in its statistics, which is exactly the reference LinkConference passes from one
unit to the other without reading. Of the four candidates surveyed this is the
only one whose published interface holds one conference across two units.

**Can it be started and stopped as a unit of a pool.** The interface has the
half that is hard to add. `graceful_shutdown` appears in the statistics the
candidates record lists, and `/about/health` answers 200 or a 5xx code, so a
unit can be told to stop taking work and can be asked whether it is still
serving, which is what draining and retiring need. What the published pages read
do not settle is what the process needs at startup before it will accept a
conference, and that is the first thing the adapter work on issue #43 has to
find out. Starting a process is not the risk here; starting one that is ready in
a time the provisioning window on issue #63 can afford is.

**What runtime it drags in.** A Java virtual machine, on every machine that
carries media. The unit is Kotlin:

    gh api repos/jitsi/jitsi-videobridge --jq '"\(.full_name) \(.license.spdx_id) \(.language)"'
    jitsi/jitsi-videobridge Apache-2.0 Kotlin

This is the largest cost of the decision and it is paid knowingly. It is a
runtime the tree does not carry today, it sets a floor under the memory and the
image size of every media machine, and it is a garbage-collected runtime in the
one place where a pause is expensive. Two things hold it to its smallest
surface. It runs in its own process, so it is not in the control plane's address
space and cannot decide the control plane's language, which is what issue #15
asks. And it is reached only through the port, so it is confined to the far side
of one interface and to the machines in the pool.

**Can it be replaced later.** Yes, at the cost of one adapter, and that is what
the port was written before any adapter for. The port names no type, constant or
field of any unit, which the port document states and shows the grep for. The
things that would make replacement hard are the ones where we have taken the
unit's own model rather than our own, and there is exactly one: the relay
reference passed between units is opaque to us and its shape is the unit's. That
is a value carried through and never read, so a different unit's reference
substitutes without changing anything above the port.

## The five questions, for the two nearest rejected

### LiveKit

Rejected because a conference cannot be held across two units. The distributed
deployment page describes a node selector choosing which node hosts a room and
names no operation that spreads one room across two nodes, so a room is capped
by the largest machine in the cluster. That is the ceiling this project was
started to remove, and no amount of adapter work in front of it moves it.

On the capacity signal it is the strongest of the four: nodes report stats and a
node is eligible for a new room while its utilisation is below `sysload_limit`,
which is a normalised load with a threshold over it, arrived at independently of
[capacity-signal.md](capacity-signal.md) and agreeing with it. On the pool
question it is the weakest fit in a way that is structural rather than
incidental, because its nodes are cluster members that discover each other and
share state, and a pool that starts and stops units is asking a cluster to
change its membership rather than asking a unit to exist. Its runtime is Go and
its licence is Apache-2.0, and of the four this is the runtime that would cost
the least to carry. It could be replaced later on the same terms as any other,
since the mismatch is not about replaceability.

The one place its rejection would be wrong is if room cascade exists outside the
pages read. That is a question about the software rather than about its
documentation, and the paragraph at the end says what would reopen it.

### mediasoup

Rejected because it is a library rather than a unit, and because the one thing
it does offer for holding a conference across machines is the row the candidates
record marks as unsettled.

On relaying, `router.pipeToRouter` pipes a producer into another router, which
is per publisher rather than per conference, and whether the same call reaches a
router in another process on another machine is not answered by the page read.
That is the single row where the difference between absent from the page and
absent from the software matters most, and a decision that rests on it is a
decision resting on the thing least known. On the capacity signal it has no
single normalised number, only per object statistics and subprocess resource
usage, so the whole signal would be ours to build, including the distress term
the chosen unit supplies. On the pool question the unit would be a process we
write and start, which is the most control any candidate offers and the reason
this one is not dismissed lightly. Its runtime is the cost: the library is C++
driven from Node, so choosing it drags a Node runtime into the media machines
and, because it is embedded rather than driven over an interface, into the
process that embeds it. Its licence is ISC. Replacement later would be the same
one adapter as any other candidate, though more of what the port promises would
be implemented in that adapter rather than by the unit, so more of the work
would be thrown away.

### The third shape, a unit written here

Not rejected on its merits, which are real: it is the only option that
guarantees the capacity signal is exactly the one
[capacity-signal.md](capacity-signal.md) specifies, rather than a term computed
above the unit and a term taken from it. It is rejected on size. Writing a
forwarding unit is the largest single piece of work anybody could put on this
board, it is work that does not distinguish this project from the ones it sits
next to, and the thing that does distinguish this project is everything above
the port. Choosing it would mean spending the whole of the effort on the part
that is not the point.

## What this decision gives up

A Java virtual machine on every media machine, with the memory floor, the image
size and the collector that come with it. The first run of issue #110, which
builds the image, is where that stops being a sentence and becomes a number.

Fate sharing is not given up, and that is the one place the rejected embedded
shape was better. A separate process is a process that can die on its own, and
everything the port says about Unavailable and Lost exists because of it. That
cost is paid deliberately, because a media plane that shares a fate with the
control plane takes every conference in the deployment down with one unit, which
is the opposite of the property this project is for.

An interface we do not own. Colibri2 is the unit's own control interface and it
changes at the unit's pace, not ours. Every such change lands in the adapter and
in nothing else, which is what the port buys, but it is work that arrives on
somebody else's schedule and it never stops arriving.

The freedom to assume a mapping exists. One row of the survey is not answered
for the chosen unit: SetReception, which is how a subscriber's set of received
sources and layers is changed. The colibri2 document read shows conference and
endpoint creation and a dominant speaker query, and no message that changes what
one endpoint receives. Whether the bridge takes such an instruction over another
interface is not answered by the pages read. This is the largest open risk in
the decision and it is named here rather than in a footnote, because a
subscriber whose reception cannot be changed is a room where speaker selection,
layer policy and congestion response all have nowhere to land, and those are
issues #45, #46 and #49.

## The licence of the chosen unit

Apache-2.0, from the command above. This repository is under the GNU Affero
General Public License version 3, which the tree carries and the readme points
at.

It does not constrain the licence question on this board, for a reason that is
about the shape rather than about the licences. The unit is a separate process
reached over its own network interface, so this repository does not link it, does
not include it and does not distribute it. What is shipped alongside it is a
packaging question that arrives with issue #111, and it is the point at which the
question changes from whether we may use this unit to what an operator is handed.

Two things are stated as claims rather than as verified facts, because verifying
them is legal work and this document is not it. That Apache-2.0 permits its
material to be combined into a work under version 3 of the GNU licences, in that
direction and not the other, is the widely stated position and is what makes the
packaging question tractable at all. And that driving a separate process over a
network interface is not the kind of combination that propagates a licence is the
ordinary reading, not a settled one. Neither claim is load-bearing today, since
nothing here is shipped yet. Both become load-bearing at issue #111, and issue
#102 is where the notices for what ships are produced.

## What would reverse this

Three things, and each is a run rather than an argument.

If the adapter work on issue #43 finds no route by which a subscriber's
reception can be changed, the chosen unit cannot serve a room where anybody
selects anything, and the decision returns to this document with that finding.

If a room cascade turns out to exist in a rejected candidate, the survey it was
rejected from was wrong about the software rather than about the page, and the
comparison is worth making again. The candidates record already marks every such
row, so the list of things to check is written down.

If the virtual machine's startup time makes a unit unable to be serving inside
the provisioning window, the pool cannot grow fast enough for the trigger on
issue #9 to mean anything, and the cost of that runtime stops being a memory
floor and becomes the thing that breaks the loop. Issue #63 produces that number.
