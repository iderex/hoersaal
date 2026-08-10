# What an operator may set, and where everything else comes from

This fixes the set of things an operator may configure. It answers issue #12. The
quantities it says are derived are derived in
[capacity-signal.md](capacity-signal.md) and
[scaling-triggers.md](scaling-triggers.md), the store it names is chosen in
[control-plane-state.md](control-plane-state.md), and the shape it refuses is a
support answer that begins by asking somebody to edit a file.

Nothing here is measured. Every number this document declines to make a setting
is a number some other issue produces, and each one is named where it is declined
rather than left as a promise that it exists somewhere.

## The position

Everything the scaling model needs is derived at runtime from what the machine
reports and what the room is doing. An operator says how much they have and where
their machines are. They do not say how the software behaves inside that.

The failure this exists against is ordinary. A knob is added because one
deployment needed it, its default is set to whatever suited that deployment, and
two years later the answer to every support question is a configuration file
nobody can reason about. The knob is not the defect. The defect is that the
derivation which was supposed to be right was wrong once and nobody wrote that
down, so the knob is now load-bearing and cannot be removed.

## The list

Four groups, and each is a different kind of statement. If a fifth group appears,
it appears through the rule below and not by being appended here.

**Where the service listens.** The address and the port of the HTTPS listener,
and the certificate it presents.
[signalling-transport.md](signalling-transport.md) puts the signalling connection
on that same listener, so this is one statement and not two. It is not tuning: a
machine cannot guess which interface an operator means to expose, and there is no
value it could derive that would be right.

**Where the store is.** The path of the file
[control-plane-state.md](control-plane-state.md) chooses. A place on a disk is
the operator's to name, and that document already asks the store to be a file the
operator can copy, which only means something if they know where it is.

**How much they have.** Three numbers, and every one of them is the operator
stating a quantity they are paying for rather than tuning how the software
behaves.

- The egress ceiling of a unit. This is the honest exception
  [capacity-signal.md](capacity-signal.md) already names and gives the reason for:
  a machine can report the speed of its interface and cannot report what its owner
  is paying for beyond it, so a unit on a ten gigabit interface behind a one
  gigabit uplink would calibrate against capacity that does not exist and find out
  during a lecture. The egress denominator is the smaller of what the machine
  reports and this number. It is also the figure the cost work on issue #14 turns
  into a bill.
- The largest number of units the pool may grow to. This is the hard ceiling on
  cost that issue #12 identifies as the one honest exception in its own body.
  Refusing to let an operator say how much money they have is not a virtue, and
  [scaling-triggers.md](scaling-triggers.md) leans on it directly: that document
  prefers to buy a machine nobody needed over making a room wait, and says the
  preference is bounded rather than open.
- The smallest number of units the pool keeps. A deployment that scales to zero
  cannot admit the person who arrives first. The same passage in
  [scaling-triggers.md](scaling-triggers.md) names the floor and the ceiling as
  the operator's, which is what makes the eagerness above affordable: the cost of
  being wrong in the direction that document prefers is visible on a bill and
  capped by a number its payer set.

**Where the machines come from.** Which provisioning driver to use, and the
machines or the endpoint that driver may use. Entry 3 on issue #1 is answered: a
driver interface exists and no provider driver ships in the box, so naming a
driver is naming one somebody chose and installed. The property that survives
whichever driver is named is that this software contacts no endpoint absent from
this configuration, and issue #103 is where that is asserted as a property rather
than as a list of hostnames.

That is the list. It fits on one screen, which is what issue #12 asks of it, and
the reason it fits is that everything below is derived.

## The same list, as the keys an operator writes

The prose above is the argument and this block is the same list in the form the
loader is held to. It is here rather than in the code because the list is this
document's, and a list that lived only in a table in Go would be a list the code
could extend by itself:

    listen.address
    listen.certificate
    listen.port
    pool.maximum
    pool.minimum
    provisioner.driver
    provisioner.machines
    store.path
    unit.egress-ceiling

Nine keys under the four groups above. `listen.` is the first group,
`provisioner.` is the fourth, `store.` is the second, and the third is split
across `unit.` and `pool.` because one of its three numbers is about a unit and
two are about the pool.

`internal/config` accepts exactly these and refuses anything else by name. Its
suite reads this block rather than holding a second copy of it, so a key added
to that package and not to this document reds the run, and adding one here is
the change the rule at the end of this document is about.

One thing the block does not say, and it is the sentence issue #82 asks for
rather than an omission here. `listen.certificate` has no default that works.
There is nothing on a machine nobody has prepared for it to point at, and the
two familiar ways out are both positions rather than code. The loader holds it
as absent, an empty configuration file starts, and nothing in this tree pretends
that deployment can serve HTTPS. That question is open on issue #82.

## Where each scaling quantity comes from instead

Every quantity in this section is one somebody will eventually ask to configure.
Each says where its value comes from and which run produces it, so that the answer
to the request is a place to look rather than a refusal.

**How many participants fit on a unit.** Nowhere, as a number. It is not a
quantity this software holds: the load in [capacity-signal.md](capacity-signal.md)
is a ratio to what a unit was calibrated for, and the calibration is the unit
measuring itself, on issue #54. A room of three hundred listeners and a room of
thirty publishers are not the same load, which is why the count is the candidate
that document rejects first.

**The denominators the load is a ratio to.** The packet rate denominator and the
distress denominator come from the calibration a unit runs against itself, on
issue #54. The egress denominator is derived from the machine and the one number
in the list above.

**The levels at which the pool acts**, which are 0.60, 0.75, 0.90 and 1.00.
Derived, and [capacity-signal.md](capacity-signal.md) shows the derivation: 1.00
is the definition of calibrated capacity rather than a choice, 0.75 follows from
an inequality over R, the fastest rate at which load rises, and T, the time from
asking for a unit to that unit taking participants, and 0.90 follows from one
reporting interval of growth at R. R is measured by the join storm work on issue
#71 and T by the provisioning work on issue #63. Until those exist the two are
first settings, which that document says of itself. What moves them is the
inequality once the measurements exist, and not an argument and not a key.

**The window, the cooldown, the hysteresis and the minimum a unit lives.** Issue
#62, set from measurement, and the fourth condition of that issue is that an
operator can see the current values without being able to set them. Seeing them
is what makes a request to change one into a bug report against the number.

**Which unit a conference or a participant goes on.** The placer, in
[placement-seam.md](placement-seam.md). It is a function of three records and
there is nothing in it for an operator to reach.

**How much of a unit's egress the mesh of one conference may spend**, which is f
in [room-topology.md](room-topology.md). That document already says f is not an
operator setting and names this issue as the reason. It is derived from what the
deployment wants a unit to be for, and the run that produces it is the cascade
work on issue #59.

## What is not a setting and is not going to become one

Two entries, both answered by the maintainer on issue #1 rather than decided here.

**Telemetry.** There is none. Not off by default, not opt-in, and not a key whose
default is the promise. This software has no reporting path, so there is nothing
for a setting to switch, and the assertion on issue #103 that nothing outside this
configuration is contacted is what holds it. A key here would reduce a property to
a claim about one default.

**Federation.** [federation.md](federation.md) says nothing crosses the boundary
and that the protocol leaves no room for it, so there is nothing to turn on. A
setting for it would be the shape issue #104 refuses: a switch that implies a
capability the software does not have.

One thing that is answered and is not yet a settings surface, said here so that
whoever builds it does not read this list as forbidding it. Entry 2 on issue #1 is
answered, and recording exists, is off, and carries an indicator the server
enforces against every client including ones this project did not write. Nothing
in this tree records anything today, so the keys that answer belongs to are not
guessed at here; they arrive through the rule below. What the answer already
fixes, and what no key may therefore do, is suppress the indicator: an indicator a
setting can turn off is not an indicator, it is a request.

## The rule for adding a knob later

A key is added only with a written reason naming the deployment and the situation
in which the derived value was wrong. That reason is also a bug report against the
derivation, and the change that adds the key names the issue carrying it.

The second half is the half that is usually skipped and is the reason this rule
exists. A key added without an issue against the derivation is a key nobody can
ever remove, because the day somebody proposes removing it there is no record of
what it was standing in for. The issue is what makes the key provisional.

The change that adds one also cites this document, so a reader meeting a key in a
file can find the sentence that admits it.

Adding a key is additive and breaks no existing configuration, which is a
consequence of issue #82 stopping startup on an unknown key rather than ignoring
it: a file written before the key existed omits it and starts, and a file naming
a key this software does not have is refused instead of being half applied. That
is also why the absence of a telemetry entry above costs nothing later, and it is
the reason the list can be short now without being expensive to extend.

## What refuses a violation of this today

Nothing, and the gap has an address. `PROSE, NOT ENFORCEMENT`, issue #82.

No part of this tree reads configuration, so there is no key for a check to
compare against a list. The rule is declared and is deliberately not run, which
the invariants table prints rather than leaving to be inferred:

    go run ./cmd/invariant | grep -A 2 'rules not run'
    rules not run: 2
      no-display-dependency-in-the-client-decision-layer (#75): the client platform is undecided on #74, so the imports that carry a display have no names yet and a list written now would be a list against a guess
      no-configuration-key-outside-the-fixed-list (#82): #82 fixes the list of keys and nothing in this tree reads configuration yet, so there is neither a list to compare against nor a key to compare

So the list above is held by a reader until issue #82 lands the configuration that
refuses a key outside it. That issue is where this document stops being prose, and
the fourth condition of issue #12 is exactly that landing.

The other unenforced half is the rule for adding a knob, and it is unenforceable
rather than merely unenforced. Whether a reason names a real deployment and a real
failure of a derivation is a judgement about meaning, and no reading of this tree
makes it. The review is where a bad one is caught.
