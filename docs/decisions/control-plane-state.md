# What the control plane keeps, and what it rebuilds

This divides the control plane's state into what survives a restart and what is
rebuilt when clients reconnect, names the store, and says how a schema change is
made. It answers the first three conditions of issue #30. The fourth, a restart
with a live pool shown to reattach rather than reprovision, is a run against a
pool that does not exist yet and is left to issue #56.

Nothing here is measured. The one place a rate would decide the answer is named
below with the reason no number is needed for it.

## The rule the division comes from

State that exists because a session exists is rebuilt. State that exists because
a room exists is durable.

That rule is a consequence of
[session-continuity.md](session-continuity.md) rather than a new choice. Because
a session never moves, the binding between a participant and a unit lives exactly
as long as that participant's connection. A control plane that wrote it down and
came back holding it would answer with a pairing that stopped being true at the
moment it restarted, and it would be wrong in the direction that is hardest to
notice, since a stale binding is a well-formed answer to the question it is
asked.

## The division

Durable, because it exists whether or not anybody is connected.

What an operator set. Nothing in the system can recompute it, and losing it means
a deployment that comes back configured differently from the one that went down.
What an operator may set at all is issue #12 and is not decided here.

A room's identity and its access rules. A room that exists between two lectures
is the thing a link points at, and a restart that forgot it would turn every
circulated link into a dead one.

A scheduled lecture and its time. It describes a room before any session exists
and it is the case the durable half is for.

The moderation trail is durable and is issue #88's to define. This document does
not say what is in it, only that it is not rebuilt: a record of who removed whom
that disappears on restart is not a record.

Rebuilt, because it exists only while somebody is connected.

Who is currently connected. The connection is the fact, and a restarted process
holds none, so a list of the connected is a claim about sockets that no longer
exist.

Which unit a participant is on. This is the rule above in its first and clearest
case.

What a participant is receiving. It is a projection of the room's own record onto
a unit rather than a second copy of anything, which is what
[media-plane-port.md](media-plane-port.md) means when it says a unit holds no
state the control plane cannot rebuild.

The participant list and presence. Derived from the connections, so derived from
something that is gone.

Each unit's load and its state. The pool decides liveness by asking, and an
answer that arrived before a restart is an answer about a machine that has since
had time to change.

## The pool is the one that is neither

The pool is not a session and it is not a room, and issue #30 raises it for that
reason. A restarted control plane that has forgotten the units will ask the
provisioner for a second set of them, which costs the operator money in the
direction nobody notices, because nothing is broken and no alarm fires.

Two routes, and this document takes both rather than one.

The control plane keeps a durable record of every provisioning request it made
and every machine handle it was given back. That is what stops a double
provision in the interval between coming back and finding out what exists, and
that interval is exactly when a room is filling and the scale-out condition is
most likely to hold.

The control plane reconciles that record against the provisioner as part of
starting, and the provisioner's answer wins. That is what stops the record from
drifting into a list of machines that were released, were never made, or were
made and then died.

Neither alone is enough, which is why it is both. A durable record on its own
drifts and there is nothing to correct it against. A query on its own leaves the
window, and the window is the risky one.

What this asks of two open issues, so that neither has to derive it again. Issue
#63 owes a provisioner interface that can be asked what it has made, not only
asked to make one. Issue #56 owes the reconciliation and the run that shows a
restart reattaching, which is the fourth condition of issue #30 and is why that
condition is not met here.

The rest of what the pool holds, each unit's state and its last reported load, is
rebuilt. A unit that comes back registers as a new unit, which
[../design/scaling-loop.md](../design/scaling-loop.md) already fixes, and the
same sentence answers the control plane's restart: what the pool knows about a
unit is what the unit has told it since.

## The store

SQLite, in a file the operator can copy.

The means check, against the three things issue #30 says the choice is judged on.

An operator who wanted one service and not three. An embedded store is the only
kind that adds none. This project is aimed at somebody running a lecture service
on machines they own, and a second daemon to install, back up, patch and hold a
password for is a second thing they can get wrong before their first lecture.

A unit suite that can use it with no external process. An embedded store is
opened by the test itself, on a temporary path or in memory, so the suite keeps
the property the headless job asserts: no container, no daemon, no port.

The cost at the write rate a join storm produces. This is the question the
division above answers rather than the store. A join creates a connection, a
placement and a set of receptions, and every one of those is rebuilt state, so a
join storm produces no durable write at all. What the store carries is rooms
being created and edited and an operator changing a setting, which is paced by
people rather than by arrivals. A store chosen for a write rate this workload
does not have would be a second service bought for nothing.

What it costs, stated rather than left to be found. One writer at a time, which
is a real constraint and is affordable only because of the paragraph above; if a
later change makes a join write durably, this choice has to be re-argued rather
than tuned. An operator who already runs a database cannot point this at it, and
that refusal is deliberate: a second store is a knob, and issue #12 is where a
knob has to justify itself. And a driver is a dependency this tree does not have
yet, so it is the first entry in the lock issue #20 built the refusal around.

Which driver is not decided here. What is required of it is that it needs no C
toolchain to build, so that the container image on issue #110 stays one command
and cross-compilation does not become a third thing to get right. The means check
is run again, on the module, in the change that adds it.

## How a schema changes

Written before the first schema exists, because a migration route decided after
the first upgrade is decided by whatever that upgrade needed.

The database carries its own schema version. Migrations are forward-only,
numbered, and applied at startup by the binary that needs them, so an operator
upgrades by replacing the binary and starting it.

A binary refuses to start against a database with a version newer than it knows.
The alternative is a new binary reading a shape it does not understand and
writing something the old one cannot read back, and refusing is the failure an
operator can act on.

Each migration has a test that runs it against a database built by the previous
migration rather than against one built from the current schema. A migration
tested only against the shape it produces is a migration nobody has run.

No migration removes data the previous version still reads. That is what makes
replacing the binary reversible for one version, which is the only rollback an
operator has when a lecture is starting.

## What this does not settle

The fourth condition of issue #30, a restart with a live pool shown to reattach
rather than reprovision. It needs a pool and a provisioner, which are issues #56
and #63.

What an operator may set. This document says the settings are durable and issue
#12 says which settings exist. Two entries reserved on issue #1, the reach of the
pool and telemetry, decide part of that list, and nothing here assumes an answer
to either.

The shape of the moderation trail, which is issue #88.
