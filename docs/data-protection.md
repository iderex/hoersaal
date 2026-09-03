# Data protection: the conversation stays on the host

This is the statement an operator hands to the people in their rooms and to
their own regulator, and it answers issue #103. It says what data this software
holds, where, for how long and what removes it, and it lists every connection
the software can make out of the machine it runs on. It is written for somebody
who has to answer for a deployment rather than for somebody deciding whether to
like the project, so it names categories precisely rather than reassuringly.

Two things it is not. It is not legal advice, and the basis on which an operator
processes anything is theirs to determine; where their obligations begin is
[what-this-is-not-for.md](what-this-is-not-for.md), and nothing here moves that
line. And it is not the threat model: who attacks this service and what refuses
them is [threat-model.md](threat-model.md), and this document points at it
rather than repeating it, because a second copy of a boundary is where a
condition gets added by accident.

Most of this service is decided and not yet built, and a data protection
statement that hid that would be describing a deployment that does not exist.
So each section below says whether the sentence describes code in this tree or a
decision the code will be built against, and an absence is written as an
absence. Where a sentence rests on a command, the command is beside it, run at
the commit this document landed in.

## The claim, and the condition a reader would expect beside it

Personal data does not leave the host. Media flows between the participants and
the forwarding units the operator runs, and nowhere else. The control plane
talks to no service the operator did not name in its configuration. Nothing
calls home, on any schedule, for any reason: I decided on 2026-08-08 that there
is no telemetry in this software, not off by default and not opt-in, and
[decisions/what-an-operator-may-set.md](decisions/what-an-operator-may-set.md)
carries that as a setting that does not exist rather than one that is off. The
condition a reader would expect to find here is federation, and there is none.
[decisions/federation.md](decisions/federation.md) is quoted rather than
paraphrased, because it asks to be:

> Nothing. No identifier, no media, no metadata, no presence, no room listing
> and no account.

That is the whole of what crosses between two installations of this software,
there is no setting, build tag or deployment shape in which one talks to
another, and the protocol leaves no room for it. So the claim above carries no
condition, and the day it acquires one this document changes first, under a
protocol version rather than a setting. The one qualification that does belong
in this paragraph is about whose machines the units are. A forwarding unit is a
machine the operator's chosen provisioning driver started, and I decided on
2026-08-09 that a driver interface exists and no driver for a rented machine
ships in this software. An operator who installs such a driver has chosen to run
audio and video across a machine a third party administers, and that is their
statement to make to their participants, not this document's to make for them.

The other qualification is the operator. A self-hosted service places the
conversation in the hands of whoever runs it: they hold the key that signs every
room credential, the machines the media crosses, and whatever the control plane
persists, and nothing in this software refuses them. That is a property of
self-hosting rather than a gap, and [threat-model.md](threat-model.md) says
under "The operator" what that trust covers.

## What data exists, where it is held, for how long, and what removes it

The rule that sorts every item is fixed in
[decisions/control-plane-state.md](decisions/control-plane-state.md): state that
exists because a room exists is durable, and state that exists because a session
exists is rebuilt and never written down. Everything below falls on one side of
that line.

**The configuration.** Nine keys, listed in
[decisions/what-an-operator-may-set.md](decisions/what-an-operator-may-set.md):
where the service listens and the certificate it presents, where the store file
is, three numbers about money, and which provisioning driver runs with the
machines or the endpoint it may use. It is the operator's own file, held where
they put it, for as long as they keep it, and it names no participant. Code:
`internal/config` reads exactly these keys and refuses any other by name.

**Rooms, their access rules and scheduled lectures.** Durable, in a single
SQLite file at the path the operator names, because a link circulated for a
lecture has to point at something after a restart. A room exists until the
operator deletes it, and deleting it is what removes its record. Decision only:
nothing in this tree opens a store yet, which the startup self-check in
`internal/selfcheck` reports as not verified rather than as passed.

**The moderation trail.** Durable, because a record of who removed whom that
disappears on restart is not a record. What is in it, and so whether it names a
person, is issue #88 and is not decided. Until it is, this document cannot say
what that trail holds, and says so.

**The provisioning record.** Durable: every request the control plane made for a
machine and every handle it was given back, kept so that a restart does not buy
a second set of machines. It holds machine handles and moments and no participant
data. Decision only; the provisioner is issue #63.

**Operator accounts.** The people who run rooms hold an account with a password
and a session. What is held is the password in Argon2id form with a per-password
salt, never the password, and a session identifier in a cookie marked HttpOnly,
Secure and SameSite=Lax that ends after 30 minutes idle or 12 hours whatever the
tab is doing. Failed sign-in attempts are held as a count in memory, per account
and per source address, and the count is gone after fifteen minutes without a
failure or when the process restarts. All of that is fixed in
[decisions/accounts-and-room-credentials.md](decisions/accounts-and-room-credentials.md)
and none of it is built: the account route and the library that hashes a
password are not in this tree.

**The room credential.** Most people in a room were sent a link, and the link
carries a signed credential naming the conference, a role, a window of at most
twelve hours, and an optional subject. The subject is an identifier this
installation made up for the occasion and is never a name, an address or a
directory identifier, because a link lives in browser histories, proxy logs and
whatever message it was sent in, and none of those places was assessed for
holding personal data. The credential is held by whoever holds the link, outside
this software, and it stops working when its window closes. It cannot be
withdrawn earlier, which the decision states rather than solves; an operator
whose link escaped ends the room. Code: `internal/roomcred` mints and verifies
this, and refuses a key shorter than 32 bytes, a token over 1024 bytes and a
lifetime over twelve hours.

**Who is connected, which unit they are on, what they receive, and who is
present.** Rebuilt. Each exists only while a connection exists, is never written
to the store, and is gone when the participant disconnects or the process
restarts. What a participant is told about the others is a summary of at most
eight identifiers and two counts, and the full list is a paged query that names
identifiers rather than people; the identifiers are the control plane's own and
a participant is not a person, since one person on a phone and a laptop is two
participants. Code: `internal/admission`, `internal/presence` and
`internal/domain` hold this as functions from a message to an answer. There is
no listener in this tree, so today no connection is ever accepted and none of
this state ever comes into existence on a running deployment.

**The media itself.** Audio, video and shared screens flow between participants
and the forwarding unit or units the operator runs. The control plane never
receives media; it tells a unit whom to admit and what each participant
receives, and the unit forwards. Nothing in this software stores, transcribes or
analyses what is said, and [what-this-is-not-for.md](what-this-is-not-for.md)
says which of those will never be built. Recording is not among them: I decided
on 2026-08-09 that recording may exist, off by default, with an indicator the
server enforces against every client including ones this project did not
write. It is not built, no issue on this board builds it, and nothing in this
tree records anything. When it is built, this section changes first, and a
recorder counts as a participant for every other statement in this document.

**The forwarding unit's own data.** The unit is Jitsi Videobridge, chosen in
[decisions/media-plane.md](decisions/media-plane.md) and run as a separate
process on the operator's machines. Its own logs, statistics and configuration
are that project's and are governed by its documentation, not by this one. What
this document can say is what crosses to it: an admission naming a participant
identifier, a role and what to forward, and never a name.

**Logs.** What may appear in a log at any level is issue #85 and is not decided.
Three things are fixed ahead of it: a failed sign-in is recorded as a count and
not as a line naming the person, the signing key is held in a type in
`internal/secret` that formats as a placeholder under every verb so it cannot
reach a log by being printed, and `internal/invariant` already refuses a log
call that takes a participant name, an address or a credential. Today the
service writes no line about any participant, because it accepts none. Until
issue #85 lands, an operator asked what their logs hold has to read them, and
this document does not claim otherwise.

**Metrics.** Issue #83, not built. What it fixes is that no metric label carries
a participant identifier or a room name that identifies a person, because a
metric is retained for a year and read by whoever runs the dashboard.

**Connection metadata.** A source address is used for one thing that is
decided, the rate limit on failed sign-ins above, and it is held in memory and
lost on restart. There is no access log, because there is no listener; what the
listener records when it arrives is part of issue #85.

## Every outbound connection this software can make

None, today. That is the whole list, and it is asserted by the suite rather
than written here and hoped:

    go test -count=1 -v -run 'TestTreeIsClean|TestThePlaceItselfHoldsNoConnectionToday|TestTheServiceStartsNoProcessThatCouldReachOut' ./internal/boundary
    === RUN   TestTreeIsClean
    --- PASS: TestTreeIsClean (0.04s)
    === RUN   TestThePlaceItselfHoldsNoConnectionToday
        boundary_test.go:361: files in the place read as though they were elsewhere: 2, connections found: 0
    --- PASS: TestThePlaceItselfHoldsNoConnectionToday (0.00s)
    === RUN   TestTheServiceStartsNoProcessThatCouldReachOut
        boundary_test.go:389: packages linked into the service: cmd/hoersaal internal/config internal/selfcheck
    --- PASS: TestTheServiceStartsNoProcessThatCouldReachOut (0.00s)
    PASS

Three assertions, each covering what the one before it cannot. The first reads
every Go file in this repository, tests included, and refuses a connection made
anywhere but `internal/boundary`, which is the one directory allowed to make one
and which sends every finding back to the federation decision. The second reads
that directory as though it were not allowed to, and finds that it makes none
either. The third walks the import closure of the service binary and refuses a
process being started anywhere in it, because a process is how something other
than this binary would reach out on its behalf. What that test cannot see is
also bounded: this module has no dependencies at all, so there is no library to
dial on a caller's behalf, and the command that shows it reads the whole of the
module file:

    grep -c '^require' go.mod
    0

So the condition issue #103 asks for, that no endpoint other than the listed
ones is contacted during a full session, is held today in a stronger and
narrower form than a session test: no code path in this tree can contact any
endpoint, session or not, because none exists. A session in this tree is a
function from a message and a room record to an answer, and the suite drives
three hundred of them in `internal/admission` without a socket. The session
form of the assertion, a connection accepted and a session driven to its end
with nothing crossing, is the second condition of issue #104 and arrives with
the listener on issues #35 and #36.

What the list will hold, each entry named by the issue that adds it, so that a
reader can tell a planned connection from an undisclosed one:

- **The forwarding unit named in the configuration.** Issue #43 builds the
  adapter, and the connection is made in `internal/boundary` to the control
  interface of a unit the operator listed. It is triggered by an admission, a
  change to what a participant receives, and the capacity signal being read.
  It carries participant identifiers and never names.
- **The machines or the endpoint the provisioning driver may use.** Issue #63
  builds the driver interface and the first driver, over machines the operator
  listed. It is triggered by the pool growing or shrinking, and it carries
  nothing about any participant.
- **Nothing else.** No update check, no crash report, no telemetry, and no
  identity provider: the seam for one is described in
  [decisions/accounts-and-room-credentials.md](decisions/accounts-and-room-credentials.md),
  nothing is built against it, and no issue on this board holds it.

An entry that appears in the tree and not in this list is a defect against this
document, and the three tests above are where it is first refused.

## What a person can ask for, and what an operator can answer

An operator asked what is held about a participant can answer from the list
above, and the honest answer for somebody who joined by link is short: nothing
durable. The credential in their link names no person, their connection state
was never written down, and the media was forwarded and not kept. What may
remain is whatever the moderation trail holds once issue #88 decides it, and
whatever a log holds until issue #85 decides that, and this document will say
which when they land. Somebody who holds an operator account is held as a
password hash and a session, both of which the operator can delete.

What removes the durable items is the operator: deleting a room, deleting an
account, and deleting or editing the store file, which is a single file they
can copy and can destroy. Nothing in this software keeps a second copy anywhere.

## What changes this document

It is written against the tree at the commit it landed in, and each of these
moves a sentence in it: the listener arriving on issues #35 and #36, the
adapter's connection on issue #43, the provisioner's on issue #63, the log
policy on issue #85, the metrics on issue #83, the moderation trail on issue
#88, the account route being built, and recording being built. A change that
makes any sentence above false is a change to this document first.
