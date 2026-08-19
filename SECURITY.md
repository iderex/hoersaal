# Security policy

## What this repository is, so that a report lands in the right place

hoersaal is a self-hosted conferencing service meant to carry rooms of a few
hundred people without an operator hand-tuning a bridge. Most of it is decided
and written down before it is built, and the effect on this policy is that the
surface has two halves needing different reports.

One half is code. `internal/wire` reads what a client sends and writes what goes
back, `internal/roomcred` mints and verifies the credential that admits somebody
to one room, `internal/presence` decides what a participant is told about who
else is in the room, and `internal/secret` keeps the signing key out of a log. A
defect in any of those is an ordinary vulnerability and I want it as one.

`internal/pool` belongs in that half too, and registration is the part of it
that is written. `Register` recomputes a keyed digest over the unit identifier
and the moment the unit says it made the proof, compares with `hmac.Equal`,
refuses a moment more than five minutes either side of now, refuses a unit that
is not in Starting, and refuses one that has not been recorded reachable. A
machine registering itself into the pool and being sent other people's media is
a report against running code, not against a plan.

The other half is design. There is no adapter to a real forwarding unit and no
listener: `internal/mediaunit` holds a package comment and nothing else,
`cmd/hoersaal` reads its configuration, prints a self-check and stops, and
nothing in this tree binds a socket. So a finding about media reaching somebody
who never subscribed to it is today a finding against a decision document and
not against running code. Send it anyway. I would rather have it now than after
code is written against it.

## Reporting

Use the private advisory form:

    https://github.com/iderex/hoersaal/security/advisories/new

That route is open, read rather than assumed:

    gh api repos/iderex/hoersaal/private-vulnerability-reporting
    {"enabled":true}

Please do not open a public issue for anything you think is exploitable. This
project plans in the open on its tracker, so a report filed there is the
disclosure whether or not it was meant as one.

One caveat about that door, which a reporter should hear from me rather than
discover. It is enabled and nobody has ever sent a report through it, so nothing
has confirmed by experience that a report arriving there reaches me. If you hear
nothing at all, that is the failure worth suspecting, and the fallback is a
public issue saying you have a report and nothing else.

I promise no time to acknowledge and no time to fix. A stated deadline that is
missed leaves a reporter unable to tell a slow answer from a report that never
arrived, which is the uncertainty a policy exists to remove, so no number is
better than a number I cannot hold.

## Which versions this applies to

The default branch, because there is nothing else:

    gh release list --repo iderex/hoersaal --limit 5
    (no output)
    git ls-remote --tags origin
    (no output)

No tagged release, no artefact, and no route by which an operator would be told
a fix exists. What a version number will promise is in `docs/version-policy.md`
and nothing has been released under it, so taking the default branch is the only
way a fix reaches a deployment today.

## What I would most like to hear about

A token `internal/roomcred` accepts that this installation did not sign, or that
it accepts for a room, a role or a moment the credential does not name. Verify
checks the signature before reading anything inside the credential and is handed
the conference to compare against, so a route around either is the best report.

A registration `internal/pool` admits without the operator's key, or one it
admits on a proof made for a different unit or a different moment. That a
captured proof works again inside the five-minute window is known and written
into the package; a route that needs no key at all is not.

A message that makes `wire.Decode` do work out of proportion to its size, or one
that decodes under the 64 KiB limit and re-encodes over it. The second happened
once already and is why `Encode` does not escape HTML.

A client that stops reading and grows this process's memory anyway, past the
fixed queue and write timeout in `internal/wire/send.go`. Or anything
`internal/presence` tells a participant about the room that they should not be
told, through the summary or through a page.

The signing key reaching a log, a diagnostic or an encoded structure by a path
`internal/secret` does not cover. It answers for every formatting verb and for
text and JSON, and the hole it names itself is assignment to a plain byte slice.

A route that moves committed egress or committed packet rate without an
admission. `docs/decisions/capacity-signal-under-attack.md` claims those two
terms cannot be moved without one, and a counterexample turns a flood into
somebody's cloud bill.

## What is not a vulnerability here

A room credential being usable by whoever holds the bytes. It is a bearer
credential on purpose, because the alternative is asking three hundred people to
register before a lecture. What bounds it is written into the signed bytes: one
conference, one role, one window of at most twelve hours. Somebody forwarding
their link has given away what it grants, and no property of the bytes refuses
that. Neither is the absence of revocation or key rotation a finding; both are
argued in `docs/decisions/accounts-and-room-credentials.md` and owed by issue
#86, and today an operator whose link escaped ends the room.

Findings against `internal/mediafake`, a bookkeeper for tests that forwards no
media and opens nothing, or against this repository's own tooling:
`cmd/coverage`, `cmd/doclint`, `cmd/invariant`, `cmd/mediaharness` and the
checks under `internal/` that read the tree. Those read this repository's own
files and the machine they were started on, and no traffic a participant sends.

`cmd/prhygiene` is not in that list, and I had it there wrongly.
`.github/workflows/pr-hygiene.yml` runs it on `pull_request` events, so what it
reads is a pull request anybody may open: the body, the author association, and
the two commit object names, which arrive as environment variables and become
an argument to `git log` through `os/exec`. `commitRange` holds both ends to
forty or sixty-four lowercase hexadecimal characters before they get there, and
`exec.Command` starts no shell. A way past that, or anything else in that
command a crafted pull request reaches, is a report I want.

Dependency scanner output. `go.mod` declares no requirements at all, so a report
naming a vulnerable transitive package names something this module lacks.

The operator being able to reach the conversation. They hold the key that signs
every room credential, the machines the media crosses, and whatever the control
plane will persist. Nothing in this software refuses them and no arrangement of
it could. That boundary is written down rather than defended.

Absences that are refusals rather than gaps: no federation between
installations, no transcription or analysis of what is said, no interception
interface handing a third party a copy of a room.
`docs/what-this-is-not-for.md` says which will never be built, and why.
Recording is not among them. That document does not name it, nothing in this
tree records anything, and whether this service may record at all is entry 2 of
issue #1, where it is reserved.

The observed-distress term of the capacity signal being movable by somebody
holding no admission. That is decided in writing and deliberately not bounded:
the term exists for the failures nobody modelled, and capping it would have a
unit reporting health while it fails. The operator's ceiling bounds the money.

## Where the reasoning already lives

`docs/threat-model.md` is the long form: who attacks this service, what refuses
each thing, where that refusal lives, with absences written as absences and the
issue that owes each one. This policy points at it rather than paraphrasing,
because a second copy of a boundary is where a condition gets added by accident.
Read it knowing two things. It calls `internal/mediafake`, `internal/mediaport`
and `internal/placement` empty of code, and each now holds several hundred
lines. And under "A machine registers itself as a unit and is not ours" it says
there is no pool, which the first section above corrects. Where the document and
the tree disagree, the tree is what runs.
