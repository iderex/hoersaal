# Where things go, and the two boundaries that decide it

This answers issue #16. The layout here is not the default one for a Go
repository, because two boundaries have to survive contact with the code and
neither survives being a paragraph. The directories are arranged so that
crossing a boundary is a line somebody has to write and a reader can see.

The rules below are the source the architecture conformance work on #98 turns
into tests. Nothing here is enforced by a check yet, and a rule with no check is
an explanation of a rule rather than a rule, so what follows says which side of
that line each sentence is on.

## The top level

`cmd/` holds the artefacts that can be started. One directory per artefact, each
holding only the wiring: reading configuration, constructing what the process
needs, and handing control to a package under `internal/`. Nothing decides
anything here. `cmd/hoersaal` is the service.

Not everything startable is the service. `cmd/prhygiene` is a check the
automated run starts, and it is here for the reason the paragraph above gives
rather than as an exception to it: reading an event payload and walking a commit
range is wiring, the rules it applies are in `internal/prhygiene` where the
suite exercises them, and a check whose decision lived beside its input could
only be proved by opening a pull request that tripped it.

`internal/` holds everything the service is made of. It is the whole of the code
apart from the wiring above it, and the Go toolchain refuses an import of it
from outside this module, so the packages here are not a published surface and
do not have to be treated as one.

`docs/` holds the written record. `docs/decisions/` holds the decisions that
shape the architecture, one file per decision, each carrying the reasoning that
produced it. `docs/design/` holds the notes that describe how several of those
decisions run as one thing. Documents at the root of `docs/` are the ones that
belong to neither, this one among them.

`.github/` holds the automated run and the templates. Nothing the service does
at runtime is here.

That is the whole top level. A fifth directory is a change to this document
before it is a change to the tree.

## The first boundary: the control plane may not depend on the media plane

The control plane decides who is in which conference, what they are allowed to
do and which unit carries them. The media plane receives media from publishers
and sends it to subscribers. Everything the control plane needs from the media
plane goes through the port, which is specified in
[decisions/media-plane-port.md](decisions/media-plane-port.md) and named there
before any implementation existed, so that the interface is not shaped by the
first thing that implemented it.

Three separate places, which is what makes the dependency visible rather than
structural:

`internal/mediaport` is the port. It is the interface the control plane calls
and the vocabulary that document fixes, transcribed into this language. It sits
above the boundary and not inside the media plane, because the control plane
depends on it and the whole point is that the control plane goes on compiling
when the media plane is gone.

`internal/mediafake` is the fake that satisfies the port without any media at
all, which is issue #42. It exists so the control plane can be tested end to end
on a machine with no devices, and it is what the suite reaches for. It names no
forwarding unit.

`internal/mediaunit` is the adapter, which is issue #43. It is the only place in
this repository allowed to name a type, a constant or a field belonging to the
chosen forwarding unit, and the only place allowed to import that unit's
libraries. It is reached by `cmd/hoersaal` and by nothing else.

So the rule has two halves, and both are about imports:

- No package under `internal/` other than `internal/mediaunit` imports
  `internal/mediaunit` or anything belonging to the chosen unit.
- Removing `internal/mediaunit` leaves every other package compiling and every
  test passing.

The second half is the one worth running, because it fails for a reason nobody
can argue with. The command is in the pull request that landed this document and
it is one line.

The first half is now a check. `internal/arch` refuses an import of
`internal/mediaunit` from anywhere but `cmd/hoersaal` and the adapter itself,
over every `.go` file in the tree including the tests, and the sentence it prints
sends the reader back to this section rather than restating the rule.

`PROSE, NOT ENFORCEMENT` for the second half, and that is the residual rather
than the whole. Nothing runs the removal. The import rule is what makes the
removal safe and it is not the same statement: a package reaching the adapter
without importing it, through a build-tagged file the parse skips or through a
third package it is allowed to import, is outside what the check reads and would
still take the control plane with it. The command that settles it is one line and
lives in the pull requests that argued this, and until something runs it on every
change, the second half is a property somebody checks by hand.

## The second boundary: the placer may not depend on the transport

`internal/placement` answers which unit carries a conference and which unit
carries an arriving participant. It is specified in
[decisions/placement-seam.md](decisions/placement-seam.md), which fixes what it
may read: three records and nothing else.

It is the component most likely to be replaced, because the first policy is a
guess that real rooms will disagree with, and the cost of being wrong should be
one function rather than a rewrite. That is only true while it stays a function
of its inputs. So it holds no connection, opens no socket, reads no clock of its
own and asks nothing of the pool that was not handed to it. A placer that can
reach the network is one whose answer depends on when it was asked, and nobody
can test that.

Concretely: `internal/placement` imports nothing that speaks a protocol, and
takes its clock as an argument like everything else in this tree does. It has no
`internal/mediaunit` import either, which follows from the first boundary rather
than being a second rule.

This is now a check, in `internal/arch`. The half about this repository is an
allow-list, because [decisions/placement-seam.md](decisions/placement-seam.md)
argues for a small set rather than an exclusion list: `internal/placement` may
import `internal/domain` and nothing else from this module, which is what refuses
`internal/clock`. The half about the standard library is an exclusion list, for
the asymmetry that package's comment gives, and it names the network, the file
system, the operating system and a store.

`PROSE, NOT ENFORCEMENT` for what a placer does with what it imports. A placer
holding an address it was handed is reading one of its three records and a placer
dialling that address is the thing this section refuses, and the two are
different packages, so the check can tell them apart. What it cannot tell apart
is a placer that was handed something live through an interface, because the
import is then somebody else's, and no reading of the imports finds it.

## The rest of internal

Everything else under `internal/` is a package named for the thing it holds,
flat, one level down. `internal/clock` is the only place that reads the
machine's clock and `internal/random` the only place that makes randomness, both
for the reasons issue #27 gives; `internal/guard` refuses a use of either
elsewhere; `internal/textbytes` refuses a carriage return in tracked text;
`internal/arch` refuses the imports the two boundaries above forbid and a
top-level directory this document does not name; `internal/prhygiene` refuses a
pull request whose commits, or whose body, do not say which issue they came
from; `internal/fuzzing` holds the fuzz targets for the surfaces a stranger
reaches, and holds them together rather than beside each package so that the
corpus is one thing to carry and every target enters through the exported
surface a stranger enters through; `internal/secret` holds the type a value
worth stealing is kept in, whose formatting produces a placeholder under every
verb, which is issue #86.

Flat rather than grouped, because a grouping is a claim about which packages
belong together and every such claim so far has been wrong within a milestone.
The two boundaries above are the exception: they are grouped because the
grouping is the rule.

## What a placeholder is

A directory in this document that has no code yet holds one file, `doc.go`,
carrying the package clause and a comment saying what belongs there and which
issue fills it. That is so a reader of the tree sees the same shape as a reader
of this document, rather than having to imagine the difference.

A placeholder is not a reservation. If an issue closes and the directory it
named is still empty, the directory goes and this document changes with it.
