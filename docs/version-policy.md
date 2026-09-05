# The version policy, and what a number promises

This says what each part of a version number promises. It answers issue #114. It
is written for two readers who want different things from the same number: an
operator deciding whether an upgrade will interrupt a lecture, and somebody who
wrote a client and wants to know whether it still works.

Nothing has been released under this policy. There is no tag and no artefact:

    git ls-remote --tags origin
    (no output)

So the policy is written and reviewable and has not met a release, which is the
condition issue #114 stays open on and is stated here rather than left for a
reader to discover.

## The entry this depends on, and the answer it assumes

Issue #114 asks that this policy name the entry on issue #1 it depends on and
state which answer it assumes. The entry is the seventh, whether the protocol is a
stable contract and from which release, and it is answered rather than reserved.

The answer is the middle of the three the entry offers. The wire protocol is
unstable until the first operator release, which is milestone M10, and it is a
contract from that release onward. Until then the instability is carried in the
negotiated protocol version itself rather than only in a document, so somebody
writing a client meets it during the handshake and not in a page they may never
open.

This policy assumes that answer and does not hold under a different one. Promising
stability from the first tagged release would make every rule below apply to a
protocol that no client outside this project had yet used; promising nothing would
leave the MAJOR column empty.

## The number

Three parts, `MAJOR.MINOR.PATCH`. What each may change is set out per subject
below, because a promise stated in the abstract is one every reader resolves
differently.

The four subjects are the wire protocol, the compatibility window between the
control plane and a unit, the configuration, and the data on disk. They are the
four places an upgrade can hurt, and they hurt different people.

### The wire protocol

A MAJOR release may break it. A client written against the previous MAJOR may be
refused at the handshake, and refused is the word: the refusal is readable and
names the version, which is issue #32's mechanism rather than a promise made here.

A MINOR release is additive only. A new message, a new extension, a new member of
the participant-visible session state. Extensions are negotiated separately from
the version so a capability can arrive without a version bump, and unknown fields
are ignored on both sides from the first release, which is what makes additive
mean anything. A client written against an earlier MINOR of the same MAJOR keeps
working and does not see the addition.

A PATCH release changes nothing on the wire.

Before M10 this table describes what the numbers will mean and not what they do
mean, because the protocol is unstable and says so in the version it negotiates.

### The compatibility window between the control plane and a unit

Within a MAJOR, a control plane speaks to every unit of that MAJOR, whatever
MINOR it is. Across a MAJOR, it does not, and the upgrade drains the pool first.

The window is that shape rather than a count of releases because the drain is what
bounds it. [decisions/session-continuity.md](decisions/session-continuity.md)
fixes that a session never moves, so a unit that is being retired keeps what it
holds until its last participant leaves, and issue #61 is the retirement that
waits rather than interrupting. So the longest a control plane has to go on
speaking to a unit it did not start is the longest session on that unit, which is
a property of the lecture and not of the release train. A window written as a
number of releases would be a promise about something nobody measures.

### The configuration

A MAJOR release may remove a key. A MINOR may add one. A PATCH may do neither.

Removing a key is louder here than it is in most software, because issue #82 stops
startup on a key it does not know rather than ignoring it. A file that names a
removed key does not start, which is the right behaviour and is the reason the
deprecation notice below is not a formality. Adding a key is silent in the other
direction: a file written before the key existed omits it and starts, so an
addition never breaks a deployment.

What may be added at all is bounded by
[decisions/what-an-operator-may-set.md](decisions/what-an-operator-may-set.md)
and by the rule that document carries for admitting a new one. A version number
promises when a key may appear. It does not promise that one may.

### The data on disk

A MAJOR release may require a conversion after which the previous binary can no
longer read the store. A MINOR may migrate, and the migration leaves the previous
binary able to read what it still reads. A PATCH does not migrate.

That is the migration route in
[decisions/control-plane-state.md](decisions/control-plane-state.md) read as a
promise rather than as a mechanism: forward-only numbered steps applied at
startup, a refusal to start against a database newer than the binary, and no
removal of data the previous binary still reads. The refusal is what turns a
downgrade from a corruption into a message.

## Deprecation

Something being removed is announced in a release where it still works, and the
announcement names the release that removes it. An announcement that does not name
the removing release is not a notice; it is a warning nobody can plan against.

The minimum notice, per subject:

- On the wire and in the data on disk, one full MAJOR. The thing is announced in a
  release of MAJOR N where it still works, and it is removed no earlier than MAJOR
  N+1. Both are places where the party who pays for a surprise is not the party
  who upgraded: a third-party client and a store that has to be converted.
- In the configuration, one MINOR release in which the key still works, the
  release notes name the release that removes it, and startup says so. The floor
  is a release rather than a duration because an operator who upgrades one release
  at a time is the person this is for, and a duration would let them step over the
  warning.

Nothing is removed from a PATCH release, which follows from the table above rather
than being a separate rule.

## What this promises about a lecture

The question an operator actually asks is whether an upgrade interrupts a room,
and it has one answer across every part of the number: the upgrade does not, and
the drain does.

A control plane that restarts rebuilds what
[decisions/control-plane-state.md](decisions/control-plane-state.md) calls rebuilt
state when clients reconnect. A unit is not upgraded in place; it is drained and
retired, and a session never moves, so the room a participant is in ends when they
leave it rather than when a machine is replaced. That is why the MAJOR column
above can be as loud as it is without a lecture being the thing it costs.

## The condition this does not meet

The fourth condition of issue #114 is that the first release follows this policy
and its release notes name every change against its category. There is no release,
so nothing has followed it and no notes have been written against these
categories.

So the policy is not in force yet, and that is not a formality. A version policy
nothing has been released under is one whose first contact with reality is after
somebody has tagged something, and the categories are exactly the part that
turns out to be wrong then. Issue #115 tags the first release, and the condition
closes with the notes written against it.
