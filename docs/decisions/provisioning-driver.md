# The provisioning driver: what the loop may ask a machine-maker

Where a new machine comes from differs completely between operators, and the
scaling loop must not know. This decides the seam between the two: the
operations, the failures, and the timing a driver is held to. It answers the
first condition of issue #63 and is the document `internal/provision` is the
code of.

Nothing here is measured. The one number this seam produces, the provisioning
time, is named below as unmeasured with the run that would measure it.

## The decision

Three operations, no more, and every driver answers all three.

**Ask** asks for one machine of a stated size and answers at once with a
handle, or with a refusal. The size is the egress the unit on the machine
should be able to commit, in bits per second, with zero meaning any machine at
all. It is the one quantity an operator already states about a unit, in
`unit.egress-ceiling` on
[what-an-operator-may-set.md](what-an-operator-may-set.md), and the capacity
model on issue #54 derives everything else, so a driver is not asked for a
number nobody holds.

**Exists** reports every machine the driver holds, in a stable order, with
whether each one is there yet. This is the operation
[control-plane-state.md](control-plane-state.md) asked this issue for: a
control plane coming back reconciles the requests it recorded against what the
driver actually made, and the driver's answer wins.

**Remove** gives a machine back. A handle the driver does not hold is refused
rather than accepted quietly, because a loop that removed the wrong machine
twice and heard nothing would be a loop that cannot count what it pays for.

The handle carries the machine's name as the operator or the driver wrote it,
and the identifier the pool knows the unit on it by. The two are the same
string for a listed machine, and that coupling is deliberate: the unit on a
listed machine registers itself with the pool under that name, and the pool's
registration refuses a name it is not expecting because that unit is not in
Starting.

## The failures, in three classes

A driver answers in one of three ways when it cannot do what it was asked, and
the loop treats each differently, which is why they are three rather than one.

**No capacity at any price.** `ErrNoCapacity`. Every listed machine is in use,
or there is no driver. This is terminal: the loop reports it and does not ask
again until something changes, which is what issue #64 builds and what the
fixed-pool paragraph below turns on. A loop that retried this would be a loop
asking a fixed pool to grow every few minutes for the life of the deployment.

**The caller's mistake.** `ErrUnknown` from Remove, and the refusals a
driver's constructor makes. Nothing to retry: the code that made the call is
wrong.

**Transient.** Anything else. A provider that is down, a quota that is hit, a
credential that expired. The loop may ask again, and how soon is the cooldown
on issue #62 and not this document's.

The first and the third are held apart by a test in `internal/provisionfake`,
because a loop that treated them alike would either retry a pool that can
never grow or give up on a provider that is merely down.

## The timing

**Ask does not wait.** A machine that takes minutes to appear is the ordinary
case for a driver that makes one, and a call that blocked for it would put a
sleep at the centre of a loop this repository refuses to let sleep. So Ask
records and answers, and the machine's arrival is read from Exists, on
whatever clock the driver was handed. That is what lets a slow driver be
tested against a controlled clock in no time at all.

**The provisioning time is unmeasured.** [../design/scaling-loop.md](../design/scaling-loop.md)
calls it T, the interval between asking for a unit and that unit taking
participants, and says this issue is the work that produces it. Under the seam
above T has two halves: from Ask to the machine existing, which is the
driver's, and from the machine existing to the unit registering and answering
the port, which is the unit's. For the listed driver the first half is zero by
construction and the second is how long the operator's unit takes to start on
a machine that was already there, which is a number from a real machine and
not from a checkout. The run that produces it is the media integration
harness on issue #51 against a listed unit, and until it has run every
sentence written against T is written against an unmeasured value.

**A driver never sleeps and never reads the machine's clock.** It takes a
clock where it needs one, which `internal/guard` refuses to let it avoid.

## The first driver: machines the operator listed

`provision.Listed` hands out the machines in `provisioner.machines`, in the
order the operator wrote them, and takes them back. It starts nothing. The
operator runs the forwarding unit on each listed machine and the unit registers
itself with the installation's key, so this driver reaches no network, needs no
credential, and is testable by anyone, which is why it is the first one.

It does not read the size. A listed machine is what it is, and a machine too
small for the room it was given is found by the capacity signal on
[capacity-signal.md](capacity-signal.md), which is the instrument that exists
for exactly that. A driver that refused a listed machine as too small would be
guessing at a number the operator did not state.

It refuses, when built, an empty list, an empty name and a name listed twice,
so that every machine it can hand out is one that can be reached and is handed
out once. It is chosen by `provisioner.driver: listed`, which
`internal/config` accepts and refuses without a machine to list.

## The fixed pool, which is the absence of a driver

`provision.None` is `provisioner.driver: none`, the default, and the
deployment issue #111 ships first: one machine, one unit that registered on
its own, and asking for another always fails with `ErrNoCapacity`. It is a
legitimate configuration and not a degraded one, and what the loop owes it is
to report that answer once and stop asking, which is the fourth condition of
issue #63 read from the loop's side and is built where the loop is, on issue
#64.

## The test doubles

`internal/provisionfake` holds two drivers that behave badly in one named way
each, so a scenario built on one is a scenario about that one thing. `Failing`
refuses every Ask with a transient error and counts the asks. `Slow` hands out
a machine at once and reports it as existing only after a delay on the clock
it was handed.

The third double issue #63 names, a unit that never becomes healthy, is not a
driver, and saying why is the point of writing it here. Whether the unit on a
machine answers the port is the pool's question, read through registration and
health on [../design/scaling-loop.md](../design/scaling-loop.md), and a driver
that faked that answer would be a second source of a fact the pool is
authoritative for. So that scenario is `Slow` with no delay, whose machine is
there at once, and a pool at which nothing registers from it. The division is
written in the fake package's own comment as well.

## What this does not decide

The loop that calls this. When to ask is issue #60 and the cooldown around it
is issue #62; what to do when the answer is no capacity is issue #64; what to
tell the operator while it happens is issue #66. The third and fourth
conditions of issue #63, which ask that the loop be tested against each double
and against the fixed pool, are met on the day those exist and not before, and
issue #63 says so rather than closing on the seam alone.

Whether a machine may be one the operator does not own. Entry 3 of issue #1
answered it: a driver interface exists and no driver for a rented machine
ships, so a driver that makes one is an operator's to write and install
against this seam, and what that means for where the media goes is
[../data-protection.md](../data-protection.md)'s sentence.

The durable record of requests and the reconciliation at startup, which
[control-plane-state.md](control-plane-state.md) fixes and issue #56's
successor work wires. This seam gives that record the operation it needs and
does not keep the record.

## The issues this belongs to

Issue #63 is answered here for its first condition and in `internal/provision`
and `internal/provisionfake` for the driver half of the other three. Issues
#60, #62, #64 and #66 hold the loop that calls this seam. Issue #30, through
[control-plane-state.md](control-plane-state.md), is what asked Exists to
exist. Issue #111 is the deployment the fixed pool is for.
