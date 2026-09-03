# The threat model

This says what this service is attacked by, what stops each thing, and where
that refusal lives. It answers issue #105.

It is written against a design that already exists on this board rather than
after the software is finished, and its job is to find what that design got
wrong. One entry below exists only because writing it found something no issue
on this board was holding, and it is recorded as a gap rather than smoothed
over.

## How to read an entry

Every entry says four things: what somebody does, what property stops them,
the code or the check that holds that property today, and what is left over.

Where nothing holds it today, the entry says so in those words and names the
open issue that will. An absence written as an absence is the point of the
exercise; an entry with no mechanism and no issue is a defect in this document
and there are none of those below.

Nothing here is measured. These are properties of code and of decisions, and
the evidence for each is the file it is in rather than a run. Where a claim
would need a measurement, it is written as a claim and the run that would
settle it is named.

## What exists today, so a mechanism can be told from a plan

Most of this service is decided and not yet built. The parts that are code are
the ones an unauthenticated stranger touches first, which is the right end to
have built, and the effect on this document is that the early entries name
files and the later ones name issues.

    git ls-tree -d --name-only origin/main internal/
    internal/admission
    internal/arch
    internal/boundary
    internal/clock
    internal/config
    internal/coverage
    internal/doclint
    internal/domain
    internal/fuzzing
    internal/guard
    internal/invariant
    internal/mediafake
    internal/mediaharness
    internal/mediaport
    internal/mediaunit
    internal/placement
    internal/pool
    internal/presence
    internal/prhygiene
    internal/random
    internal/roomcred
    internal/secret
    internal/selfcheck
    internal/srcheader
    internal/textbytes
    internal/wire

THIS LISTING AND THE SENTENCE UNDER IT WENT STALE TOGETHER, AND THE SENTENCE IS
THE HALF THAT MATTERED. It named `internal/mediafake`, `internal/mediaport`,
`internal/mediaunit` and `internal/placement` as holding a package comment and
no code. Three of the four now hold several hundred lines each, and one still
does not:

    for d in mediafake mediaport mediaunit placement; do
      printf '%s ' "$d"
      git ls-tree -r --name-only origin/main -- "internal/$d" | wc -l
    done
    mediafake 3
    mediaport 3
    mediaunit 1
    placement 3

`internal/mediaunit` is the one that is still a package comment and nothing
else, and it is the adapter, which is the only one of the four that could
refuse anything on a machine carrying somebody's media. That is stated here
because an entry naming it is naming a plan, and a reader who assumed otherwise
would be reading this document as an inventory of defences it does not have.

The drift was found by reading this document against the tree rather than by
anybody meeting it, and it is the reason the entries below carry their own
re-run rather than resting on this listing.

## A stranger sends bytes to the signalling endpoint

The first thing anyone reaches without holding anything. What they send is
arbitrary and there is no prior exchange in which they were refused.

What stops the ordinary version of it is that the thing reading those bytes
decides nothing and reaches nothing. `internal/wire` reads the envelope,
refuses everything that is not one, and hands the payload on untouched.
`Decode` takes bytes and returns a message or an error; it takes no
connection, holds nothing between calls, and reaches nothing outside itself.

The bounds are numbers rather than intentions, and each is argued in
[decisions/signalling-transport.md](decisions/signalling-transport.md) rather
than beside the code, so the two cannot come to hold different values. A
message over 64 KiB is refused before it is parsed, so a stranger cannot make
this process do work proportional to what they sent. A type name over 64 bytes
is refused. A member the envelope does not have, a member given twice, a member
of the wrong kind, an empty type and a missing type are all refused rather than
ignored.

That the refusals hold over input nobody wrote by hand is the fuzzing in
`internal/fuzzing`, whose targets take bytes and nothing else, so what the
fuzzer varies is what the network varies.

What is left. The transport underneath, the point at which a connection is
accepted at all, and any limit on how many connections one source may open are
not in this tree. `internal/wire` is the framing half of the signalling work
and the connection half is not built, so nothing here bounds the number of
strangers, only what each one may say. Issue #64 is where the deployment
refuses rather than degrades.

What one message may cause is bounded now, and that is a change since this
entry was written rather than an addition to it. `internal/admission` reads a
credential before anything that costs something happens, so a stranger holding
none reaches neither the placer nor a forwarding unit, and the exchange is
argued in [decisions/room-admission.md](decisions/room-admission.md). How many
strangers may open a connection at all is untouched by that, which is the
sentence above and is still the whole of the gap.

## A stranger presents a credential they wrote themselves

Somebody who was never sent a link mints their own, or edits one they were sent
so that it names them a presenter, or names a different room, or lasts longer.

Refused by the signature, which covers every byte of the credential including
the layout version and the role, in `internal/roomcred`. Every number that
package refuses against is fixed in
[decisions/accounts-and-room-credentials.md](decisions/accounts-and-room-credentials.md)
with the reason for each, so neither can move without the other.

Three of those refusals are worth naming individually because they are the ones
an attacker actually tries.

Replay into another room is refused at the place that reads the credential
rather than by a caller remembering to compare afterwards. Verification takes
the conference being entered as an argument, so a caller that forgot would not
compile.

Replay after the occasion is refused by the window, judged against a clock the
verifier is handed rather than one it reads for itself, which is what makes the
refusal testable rather than a thing that only fails at midnight.

Work proportional to what was sent is refused before any of it is decoded: a
token over 1024 bytes is rejected on its length, and the largest credential the
layout can hold is well under that.

What is left, and it is the honest part of this entry. The credential is a
bearer credential, so somebody who forwards their link has given away what it
grants, and no property of the bytes can refuse that. What bounds it is the
window, the single conference and the single role. There is no revocation,
because withdrawing a signed bearer credential needs somewhere to write down
that it was withdrawn and nothing here writes anything down; the case that
leaves open is a person who has not joined yet and whose credential has not
expired, and the operator's answer today is to end the room. There is no key
rotation either, so an installation changing its key invalidates every
credential minted under the old one at that moment. Both absences are argued
where the numbers are, and issue #86 is where the key's own handling is owed.

## A participant reaches media they are not subscribed to

Somebody who was admitted legitimately asks the forwarding unit for a stream
nobody offered them, or keeps receiving one after their subscription ended.

Nothing holds this today, and what moved is where the gap sits.
`internal/mediaport` is the interface the control plane uses to talk to a
forwarding unit, and the eight operations, their arguments and their errors are
transcribed from [decisions/media-plane-port.md](decisions/media-plane-port.md)
into it; `internal/mediafake` satisfies that interface as a bookkeeper carrying
no media. What is still empty is `internal/mediaunit`, the adapter, which is
the only one of the three that could refuse anything on a real unit.

THIS PARAGRAPH SAID ALL THREE WERE EMPTY AND TWO OF THEM ARE NOT:

    git show origin/main:internal/mediaport/mediaport.go | wc -l
    383
    git show origin/main:internal/mediafake/mediafake.go | wc -l
    765
    git ls-tree -r --name-only origin/main -- internal/mediaunit
    internal/mediaunit/doc.go

It was found by reading this document against the tree while the entries below
were being re-checked, rather than by anybody meeting the gap. The correction
does not move this entry: an interface and a fake refuse nothing on a machine
that is carrying somebody's media.

Written as an absence. What subscriptions are and what each participant
receives is issue #44. The unit that enforces it is issue #43, and the fake
that lets the enforcement be tested with no media at all is issue #42. The
property this entry needs from those three is that the unit refuses on its own
authority rather than trusting a subscription identifier a client sent, and
that is stated here so the issues can be held to it.

One thing is decided rather than open, and it is the part most easily got
wrong. The media plane credential a participant receives must carry no power
the room credential it came from did not, because the unit and not the control
plane is what enforces it at that point. That is a condition of issue #35 with
a test named in it, and `internal/domain` already separates the two identifiers
the check depends on.

## A participant publishes when they hold no floor

Somebody admitted as an attendee sends media anyway, or asks the unit to accept
a publication they were not granted.

Nothing holds this today either. `internal/domain` carries a role, and a role
there is a name given no meaning on purpose; what powers a role name grants is
issue #34, and the moderation the server enforces rather than requests is issue
#38.

Written as an absence, with the shape the answer has to have. Refusing a
publication is the unit's decision at the moment it accepts one, not a check
the client is asked to make, and not a check the control plane makes once at
admission and never again. Issue #38 is written in those terms.

The credential half exists now and the rest does not, so the two are separated
here rather than left as one absence. `internal/admission` decides which of the
two admissions the unit is asked for from the role the credential carries,
before the unit is asked at all, so a role that may not publish reaches
AdmitSubscriber and never AdmitPublisher:

    go test -run TestAListenerWhoAsksToPublishIsRefusedAndNoUnitIsToldAnything -v ./internal/admission/
    --- PASS: TestAListenerWhoAsksToPublishIsRefusedAndNoUnitIsToldAnything

That test asserts the unit was told about nobody at all, which is what
separates a check made before the damage from one made after it. What it does
not do is make the unit refuse. The exchange only declines to ask for more than
was granted, and a unit that accepted a publication it was never told to accept
would be caught by nothing here. That is the adapter on issue #43 and the
enforcement on issue #38, and it is why this entry stays an absence.

## The client is modified

Every constraint expressed in the interface is a suggestion to somebody who
rebuilt the client, and a conferencing service that forgets this ships a mute
button that does not mute.

Nothing in this tree enforces anything against a client yet, because there is
no client and no server exchange for one to disregard. The design position is
that the server decides and the client renders, and it appears as a condition
on the issues rather than as a sentence here: moderation the server enforces
rather than requests is issue #38, the hand raise and the question queue held
by the server is issue #78, and the permission model that both read is issue
#34.

Written as an absence. What this document adds to those issues is the test
shape that proves the property rather than describing it, which is a client
that has had the constraint removed and is shown to be refused anyway. An issue
satisfied by a client that behaves has not been satisfied.

## A machine registers itself as a unit and is not ours

A unit joining the pool is telling the control plane where to send other
people's media. A pool that trusts whatever registers is a media interception
service, and this is the entry where that is stated plainly rather than left to
be inferred.

THIS ENTRY SAID NOTHING HELD IT AND THAT THERE WAS NO POOL. There is one, and
it authenticates. `internal/pool` is the authoritative record of which units
exist, issue #56 landed it, and a unit joining presents a proof that it holds
the key of the installation rather than merely claiming an identifier:

    git show origin/main:internal/pool/pool.go | grep -n "^func Prove"
    318:func Prove(key secret.Bytes, unit placement.UnitID, issuedAt time.Time) ([]byte, error) {

    gh api repos/iderex/hoersaal/issues/56 --jq "[.number,.state,.state_reason]|@tsv"
    56	closed	completed

So this entry is no longer an absence. It was found by reading this document
against the tree, and the sentence it replaces had been false since that issue
landed.

What the mechanism does not reach gets the same sentence, because a reader who
takes the repair for the whole answer is making the mistake this entry exists
against. A proof of the key says the machine was given the key. It says nothing
about that machine still being the one the operator installed it on, and
nothing in this tree notices a key that leaked. Where the key comes from and
how it rotates is issue #86 and is not answered.

Two things beside it, because an attacker does not have to forge a registration
to be in the path. Reachability is checked as part of becoming eligible rather
than as a later formality, which is issue #52. And whether the machines in the
pool may be ones the operator does not own is entry 3 of issue #1, which I
reserved and answered on 2026-08-09: a driver interface exists and no driver
for a rented machine ships in this software, so a unit on somebody else's
machine is one the operator installed a driver for. THIS PARAGRAPH WENT ON
CALLING THAT ENTRY UNSETTLED AFTER IT WAS ANSWERED, and the entry above was
written to hold under either answer, which it does: a rented machine presents
the installation's key like any other, and what the proof does not reach is
the same for both. What a rented unit means for where the media goes is
[data-protection.md](data-protection.md)'s sentence rather than this one's.

## Driving the capacity signal to spend the operator's money

This is the class most conferencing services do not have. Anything that can
move the number the scaling loop reads can make the deployment start machines,
which turns a denial of service into a bill.

The signal is specified in
[decisions/capacity-signal.md](decisions/capacity-signal.md) and the levels
that act on it in
[decisions/scaling-triggers.md](decisions/scaling-triggers.md). It is one
number, computed from three terms and combined by taking the largest.

Two of the three terms cannot be moved without an admission. Committed egress
and committed packet rate both rise at the moment the unit accepts a reception,
which is something the control plane did, so moving them means getting past
admission first. That is a real bound and it is the reason this entry is not
worse than it is.

What bounds the bill once somebody is past admission is not the signal. It is
the ceiling the operator sets, which is issue #64, the floor, the cooldown, the
hysteresis and the minimum lifetime on issue #62, and the honest cost statement
on issue #14. A hard ceiling is named on issue #12 as one of the few things an
operator is allowed to set, for exactly this reason. The design's stated
preference is to buy a machine it did not need rather than make a room wait,
which is the right preference for quality and is also the direction an attacker
pushes, so the bound has to be a number the operator set rather than a
disposition.

### The third term, which this document found and no issue holds

The third term is observed distress, the fraction of the last window in which
the unit could not hand a packet to the operating system when it wanted to. It
is observed rather than committed, it can only raise the number, and nothing in
either decision says what stops it being raised by somebody holding no
admission at all.

That matters because of how it is read. A new unit is asked for when the
smallest load among eligible units reaches 0.75 across two consecutive reports,
and the second report is there so that one stale report cannot start a machine.
Two consecutive reports is a short window on purpose. So pressure applied to a
machine from outside the admission path, sustained across that window, reaches
the same trigger that a room filling up reaches, and the loop cannot tell the
two apart because the port carries one number and not which term produced it.

No issue held this before this document was written. Issue #54 derives the
denominators and issue #55 reports the signal and proves it leads quality;
neither asks what an adversary can do to it. Issue #150 was opened for it, so
this entry is an absence with an issue rather than an absence on its own.

The residual after that issue lands is stated now rather than later: the
operator's ceiling bounds the bill and nothing bounds the quality lost while
the machine is under pressure, because that half is not a scaling failure at
all.

## The operator

A self-hosted service places a great deal in the hands of one person, and the
documentation is where that is said rather than somewhere it can be discovered.

The operator holds the signing key that mints every room credential, so they
can mint one for any room in any role. They hold the machines the media is
forwarded by, so they are in the media path by construction. They hold whatever
the control plane persists, which is issue #30. They set the configuration,
which is issue #82, and they read whatever is logged, which is issue #85.

Nothing in this software refuses the operator, and no version of it could. What
is owed instead is that the boundary is drawn where a person can see it, so
this entry exists for that and nothing more. The parts that reduce what the
operator can do accidentally rather than deliberately are real and are named:
a secret is held in a type that formats as a placeholder under every verb, in
`internal/secret`, so the ordinary route by which a key reaches a log is
refused by the type rather than by a rule somebody has read; and an audit trail
of moderation and nothing more is issue #88.

What is not addressed and is not claimed to be: an operator who wants the
conversation is not stopped by anything here, and no arrangement of this
software would stop them.

## Another installation of this software

There is none of this. Two installations do not hold a room between them, do
not exchange participants and do not assert anybody's identity to each other,
and the protocol does not leave room for it. The decision and its cost are in
[decisions/federation.md](decisions/federation.md).

The outward half is code. `internal/boundary` is the one directory in which a
connection out of this process may be made, it holds none, and its check
refuses one written anywhere else over every Go file in this repository
including the tests. The refusal is proved against fixtures rather than against
a tree that happens to be clean, because a tree with no connections in it would
pass a rule that had stopped refusing.

The inward half is prose. What refuses an assertion from another installation
is that no route accepts one, and an absent route is an absence that no reading
of the syntax tree finds. What would judge it is a session a test can drive to
the end without anything crossing, which is the second condition of issue #104
and waits on issues #35 and #36.

Three further gaps are named in the package comment on `internal/boundary` and
are repeated here only as a list, since a reader of this document is entitled to
know the edge: a library that dials on a caller's behalf, a process started to
do it, and anything reached through a build-tagged file the parse skips.

## Recording

The service cannot record a meeting. There is no recording capability in this
tree and no issue on this board to build one.

THIS ENTRY CALLED THAT AN OPEN QUESTION, AND IT IS A DECISION. Whether the
service may record at all is entry 2 of issue #1, which I reserved because a
lecture with three hundred people in it is a personal communication for every
one of them and what the operator would then be holding is a legal position
rather than a feature, and which I answered on 2026-08-09: recording may exist,
it is off by default, and it carries an indicator the server enforces against
every client including ones this project did not write. The indicator is the
server's and not the client's because a client that can hide it turns an
indicator into a request.

What this document records is the attack surface as it stands, which is that
there is nothing to enable, nothing to suppress and no indicator to defeat,
because nothing is built and no issue on this board builds it. This entry is
not an absence with no issue behind it, which the rule at the top forbids: there
is no mechanism because there is no capability, and a threat against a
capability that does not exist has nothing to hold it. The day an issue builds
recording, this entry gains the threat the decision names, a participant or an
operator recording without the in-room indicator, and the property that stops
it is that the indicator is a member of the server-authoritative session state
under issue #32 rather than a thing a client draws. For the capacity and
placement model a recorder counts as a participant, so the entry above about
driving the capacity signal already holds for one.

## Secrets reaching a log or a bug report

The signing key, and later whatever an external identity provider needs, are
worth stealing, and the line that leaks one is written by somebody printing a
struct that happens to contain it rather than by somebody printing the key.

Held by `internal/secret`, which is a type whose every formatting verb produces
a placeholder, and by the two structs in `internal/roomcred` that hold a key
and carry their own formatting so that reflection cannot reach around the type.

What it does not do is written in its own package comment and is not softened
here. It does not keep bytes out of a core dump, out of swap or out of another
process, it does not erase them, and it does not stop code that has the secret
from writing it somewhere on purpose.

What is left is issue #86, which owes where an operator's key comes from,
rotation with a live session to have an effect on, and a diagnostic bundle.
What may appear in a log at all is issue #85, and metrics carrying no personal
data in a label is issue #83.

## What this document does not cover

The client, because there is none. Issues #74 to #81 are the milestone, and the
entry above about a modified client is about the server's obligation rather
than about the client's own exposure.

Supply chain. What ships is described by the bill of materials produced on
every build, the notices are issue #102, and neither is a threat entry here.
This document is about what an attacker does to a running deployment.

The transport's own cryptography. What carries the signalling is
[decisions/signalling-transport.md](decisions/signalling-transport.md) and what
carries media is the forwarding unit's, and neither is re-argued here.

Anything that needs a number. Whether a limit is the right limit is a question
for the bench on issue #2 and the load evidence on issues #67 to #73, and no
entry above claims a figure.

## What the security policy and the privacy statement do with this

They refer to this document rather than repeating parts of it, because a
paraphrase of a boundary is where a condition gets added by accident.

THIS PARAGRAPH SAID NEITHER EXISTED AND ONE OF THEM DOES. `SECURITY.md` is on
the default branch and points here rather than paraphrasing:

    git grep -n "threat-model" origin/main -- SECURITY.md
    origin/main:SECURITY.md:151:`docs/threat-model.md` is the long form: who attacks this service, what refuses

THE PARAGRAPH THAT STOOD HERE SAID THE DATA PROTECTION STATEMENT DID NOT EXIST
AND COULD NOT BE FINISHED WHILE ENTRY 2 OF ISSUE #1 WAS OPEN. Both halves had
stopped being true: I answered entry 2 on 2026-08-09, and the statement is
[data-protection.md](data-protection.md), which points here for who attacks
this service and what refuses them rather than carrying a second copy:

    git grep -c 'threat-model.md' -- docs/data-protection.md
    docs/data-protection.md:2

So this condition of issue #105 is met for both documents.
