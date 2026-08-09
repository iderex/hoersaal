# hoersaal

A self-hosted conferencing service that reliably runs meetings of hundreds out of the box, without manual bridge clustering or tuning. It scales itself; the operator never hand-tunes a bridge.

Planning happens on the issue tracker first. Every decision that shapes
the architecture is written down there with its reasons before the code
that depends on it exists.

See [NOTICE.md](NOTICE.md) for the intended-use notice, and
[docs/what-this-is-not-for.md](docs/what-this-is-not-for.md) for what this
project is for, what it will not be built for, and where an operator's own
obligations begin.

See [LICENSE](LICENSE) for the terms, the GNU Affero General Public License version 3.

## What the checks on every change do not cover

The suite that runs on every change runs without a display, a camera, a
microphone or a forwarding unit, and that is a birth requirement rather than a
convenience. It also means a green board is not a statement about anything that
needs real media on a real network.

Those properties belong to the media integration harness, `cmd/mediaharness`,
which is a separate command that nothing in the automated run invokes. It needs
hardware and a network the project controls. Run without them it prints what was
missing, what each missing thing would take, and which properties are therefore
not shown, and it exits non-zero rather than skipping, because a skipped harness
and a passing one look the same from outside.

What is covered only there, today: the adapter against a real unit, the
distribution of simulcast layers across subscribers with different capacity, how
much of a first syllable survives a speaker switch, the order in which a
constrained subscriber's streams are actually reduced, whether the capacity
signal moves before quality does, one conference carried across two units,
whatever a browser decides about the client, join timing on real paths, what
happens to people already in the room past a ceiling, and the client's own
budget lines on stated hardware.

That list is printed by the command out of `internal/mediaharness` rather than
kept here as a second copy, each entry naming the issue that owes it. No run of
it has been recorded yet, so nothing above is evidence about any of those
properties.
