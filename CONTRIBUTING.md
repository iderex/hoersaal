# Contributing

This is the guide the sign-off gate points at. It is written for somebody who
has just cloned the repository and wants their first change to pass on the first
attempt.

Planning happens on the issue tracker first. Every change starts as an issue and
lands as a pull request that closes it, and the issue is where the argument for
the change lives. The pull request template asks what changed, what failure that
prevents, and the commands that were run with their output. Fill all three in.

## Run this before you push

These are the commands the tree is held to, in the order it is cheapest to run
them, and they are the same commands the automated run executes. Passing them
here is what makes passing there predictable.

    go build ./...
    go vet ./...
    go test -v -count=1 ./...
    gofmt -l .

`go test` is run with `-count=1` so that nothing is answered out of the test
cache, and with `-v` so the run says which tests executed rather than only that
it was green. A suite that executed no tests is a failure and not a pass.

`gofmt -l .` names the files whose formatting differs from what the formatter
would write, and it exits zero whether or not it named any, so read its output
and not its exit code. Any name at all is the failure. `gofmt -w .` writes the
correction.

Static analysis needs the analyser installed, which is one command, and it is
pinned so that what runs is decided here rather than by whatever is newest on
the day:

    go install honnef.co/go/tools/cmd/staticcheck@2025.1.1
    staticcheck -checks=all ./...

The check set is `all`, which is wider than the tool's own default. Suppression
is per finding and at the site, with the check id and a reason, which is
`//lint:ignore`. A tracked configuration file is refused, because it turns a
check off for the whole tree from a place nobody reading the code will see.

The dependency graph is locked, and the flag that refuses a resolve outside the
lock is `-mod=readonly`. It is the toolchain's own default, and it is worth
naming anyway, because a `GOFLAGS` set once in a shell profile is how the
default stops being the default without anybody deciding that:

    go build -mod=readonly ./...
    go test -mod=readonly -run '^$' ./...
    go mod verify

The second line compiles the test binaries without running them, which reaches
the requirements only the suite has. A change that adds a dependency updates
`go.mod` and `go.sum` in the same commit, with `go mod tidy`, and leaves the
tree clean afterwards. A build that rewrote the lock is a build that decided
something, and the lock is meant to be the decision rather than the record of
one.

Two things this section deliberately does not do. It does not list the checks
that run, because the run prints what it examined and a list here would drift
against it. And it does not promise that a green local run is a green remote
one: the commands are the same, the machine is not.

One thing that will surprise you on Windows. Git can be configured to convert
line endings on checkout, and where it is, every Go file in the working tree
carries a carriage return that the formatter will not write, so `gofmt -l .`
names all of them at once and nothing is wrong with the tree. Issue #26 is where
the repository stops leaving that to a personal setting.

## The sign-off

Every commit carries a `Signed-off-by` trailer naming its author, which is how
the author asserts the Developer Certificate of Origin in [DCO](DCO). The gate
compares the trailer against the commit author, so the name and the address have
to match what git is configured with, character for character.

    git commit -s

If you have already committed without it, add it across the range rather than by
hand:

    git rebase --signoff <base>

What the trailer is not, because the two instruments it is mistaken for are
heavier and ask for something this project does not ask for. It is not a
copyright assignment: you keep the copyright in what you wrote, and nothing here
transfers it to anybody. It is not a contributor licence agreement: there is no
separate document to sign, no record kept apart from the commit itself, and no
grant beyond the one the licence already makes.

What it is instead is a statement about the contribution, made by the person
making it, that they have the right to submit it under this repository's
licence. That licence is AGPL-3.0, in [LICENSE](LICENSE), and clause (a) of the
[DCO](DCO) is the sentence the trailer asserts about it. Nothing more is
collected and nothing more is meant.

That difference is the reason this repository gates on the DCO rather than on
something stronger. A heavier instrument changes who is willing to contribute,
and it buys nothing this project needs.

## Branches, commits and the size of a change

A branch is named for the area it touches and then the topic, lowercase, with a
slash between them, like `docs/contribution-guide` or `ci/build-check`.

A commit message says what changed and what failure that change prevents. Where
the change is a correction, it also says what was wrong and how it was found,
because the second question is the one that stops the same defect arriving
again. The last line before the sign-off is the issue reference, `Closes #N`
where the change completes the issue and `Refs #N` where it does not.

The subject carries the reference as well, bracketed, at the end:

    Refuse a pull request that names no issue [#96]

Both, because they are read in different places. `Closes #N` in the body is what
the tracker acts on. The bracketed form in the subject is what survives into
`git log --oneline`, `git blame` and a bisect, none of which show a body, and
those are where somebody meets a line of this repository years from now and
wants to know what it was for. Two references to one issue is the cost;
`internal/prhygiene` refuses a subject without one.

Bracketed rather than bare so that a count, a port or a version in a subject is
not read as a reference. Where a commit belongs to two issues, write both:
`[#96][#89]`.

One topic per commit and per pull request. A commit carrying two unrelated
changes has a message describing one of them, and a reviewer who wants half of
it has nowhere to say so.

A change that will not fit under review without losing quality is not a change
that needs an exception, it is an issue whose scope was planned wrong, and the
first response is to divide the issue rather than the finished diff.

## Arriving from outside this repository

The issue references above are this board's convention, and nobody arriving from
outside it can know an issue number before the issue exists. So the pull request
hygiene check does not apply its refusing rules to a pull request from an author
who is not on this repository. It says in the run that it did not apply them,
because a check that was skipped and a check that found nothing are different
statements and a green tick shows the same for both.

What that leaves you to do is nothing. Open the pull request, describe the
change and what it fixes, and the reference is added by whoever picks it up:
they file the issue where none exists yet, and they amend the subjects when they
land it. If you would rather do it yourself, open the issue first and use its
number; that is faster to merge and it is not asked of you.

The rules that are not skipped for anybody are the sign-off, which is a legal
assertion rather than a convention, and everything the other checks judge about
the code.

## Where the decisions live

`docs/decisions/` holds the decisions that shape the architecture, one file per
decision, each with the reasoning that produced it. `docs/design/` holds the
notes that describe how several of those decisions run as one thing.

A change that contradicts a decision document is a change that has to be argued
rather than merged. Argue it on the issue, against the document, and land the
document's replacement before or with the code that depends on it. Silently
building against a different answer leaves two correct-looking documents and one
tree that follows neither.

## Everything runs without a display and without elevation

Every test in this repository runs on a machine with no display server, no
camera, no microphone and no privilege beyond an ordinary user account. That is
a birth requirement rather than a target, and the automated run asserts the
absences rather than assuming them, so a test that quietly depends on one of
them fails there even where it passed on the machine it was written on.

What this rules out, concretely. A test that opens a capture device. A test that
needs a window or a toolkit that wants one. A test that installs a certificate
into a machine store, registers a service, edits a firewall, or asks for
administrative consent in any other way.

A change that genuinely needs hardware does not get an exception here. It goes
to the media integration harness, which is a separate runner with the devices
attached and its own way of reporting, and which issue #51 builds and names for
what it is. Until that harness exists the honest move is to say in the pull
request body that the case is not covered, rather than to add a test that passes
only where somebody has plugged something in.

The absence this rule does not cover is worth knowing about. A stock runner
carries a render node, an unprivileged job cannot remove it, and so a test that
reached hardware video encoding through it would pass on the runner and fail on
a machine without one. That case is not caught by anything today.

The neighbouring rule is about time rather than hardware. Nothing outside
`internal/clock` reads the machine's clock, nothing outside `internal/random`
makes a source of randomness, and nothing anywhere sleeps. Production code takes
a clock and a test hands it one it controls, so a test covering a two minute
window advances that clock by two minutes and finishes with the rest of the run.
`internal/guard` refuses the three by reading the syntax tree, and the reasoning
is in its package comment. Reaching the network from a test is refused by
nothing today, so it is a rule a reviewer holds rather than a rule the tree
holds.

## Fixtures whose bytes have to be exact

A protocol fixture and a media fixture are questions asked of a decoder in
bytes, so a byte that changes between the author and the test changes the
question without changing the answer anybody reads.

`.gitattributes` is where that is settled. Tracked text is stored and checked
out with line feeds on every platform, and the paths that hold raw bytes are
declared binary there by suffix, so nothing normalises them in either
direction. Adding a suffix to that list is the whole change; nothing holds a
second copy of it.

Two conventions follow from that, and they are the ones to reach for in this
order.

Where the exact bytes are small, write them in the source as a string with
escapes, `\r` and `\x00` and the rest, rather than as a raw literal. A raw
literal is text, text is normalised, and the carriage return the fixture exists
to prove is the one thing normalisation removes. The tests in
`internal/textbytes` are written that way and say so.

Where the bytes are a payload rather than a line, put them in a file under a
suffix that `.gitattributes` declares binary, and add the suffix if it is not
there yet. A payload stored under an undeclared suffix is normalised on the way
in and there is nothing left to compare against.

`internal/textbytes` refuses a carriage return in any file the attributes call
text, including the workflow files, and it refuses an attributes declaration it
cannot read rather than judging a narrower set than it reports.

## The means check

Before an artefact is built, whether the means fits is checked and the answer is
written down. The means is the language, the format, the tool, the runtime,
whatever the thing will be made of. Every time, and never carried over from
habit, because a means that was right for the last artefact is an assumption
about this one.

What the check asks:

- Can the means carry a refusal? A property nothing can refuse is an
  explanation of a rule rather than a rule.
- Does it add a language, a runtime or a dependency the tree does not already
  carry, and is that cost being paid knowingly rather than by accident?
- Would the artefact be testable by the suite that already exists, or does it
  need a parallel apparatus nobody will maintain?
- Is something outside this repository forcing a different means, and is that
  force real, named, and held to its smallest surface?

Record the answer in the pull request body, in the section the template reserves
for it: name the means and say why it fits. The check is not a preference for
one language, and it answers in both directions. There are surfaces where a
means this tree does not otherwise carry is genuinely the right one, and there
the forced means, held to its minimum, is the answer.

What can be checked is that the question was asked, and only because the answer
is written down. Whether the answer was right is a judgement, and the review is
where a wrong one is caught.
