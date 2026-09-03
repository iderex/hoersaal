# Mutation testing: what the suite would notice

Coverage says a line ran. This asks whether a test would have noticed if the
line were wrong, and it answers issue #93. A mutation tool changes one operator
at a time in a package, runs that package's tests against each change, and
counts the changes nothing failed on. A change that lives through the suite is
a survivor, and a survivor is a statement about a specific test that does not
test what it appears to.

## Reporting rather than gating, and why that is a deviation

Every other check on this board refuses a merge when it is red. This one runs
on a schedule against the default branch and refuses nothing, and
[reference-gate-parity.md](reference-gate-parity.md) carries that as a
deliberate deviation rather than a gap. Two reasons, and the second is the one
that would make a gate wrong rather than merely slow.

It is slow. Every mutant is a rebuild and a rerun of one package's tests, and
the count of mutants grows with the code. The first recorded run took under a
minute per surface, which is already too long to pay on every pull request and
will not get shorter.

It is not a verdict on a change. The tool gives each mutant a timeout derived
from the package's own test duration, and a mutant that reaches it is counted
as neither killed nor survived. That figure moves with the machine and not with
the code, so the same commit scores differently between two runs, and a gate on
it would red a change for something that was already in the tree. What a
survivor produces instead is a test, or a reason written below, and the test
is what reds an ordinary run afterwards.

## What runs, and over what

`.github/workflows/mutation.yml`, weekly and on dispatch. The tool is
`gremlins`, a Go module installed at a pinned version, which fits the means
check because it adds nothing to the tree: no dependency in `go.mod`, no
configuration file, and the surfaces it runs over are not written into the
workflow. They are read from the coverage bar:

    go run ./cmd/coverage -surfaces
    github.com/iderex/hoersaal/internal/roomcred
    github.com/iderex/hoersaal/internal/wire
    github.com/iderex/hoersaal/internal/placement
    github.com/iderex/hoersaal/internal/pool
    github.com/iderex/hoersaal/internal/admission

A package added to `internal/coverage` is mutated on the next run without
anybody remembering to add it, and the workflow refuses a run that named no
surface or produced no mutant, so a run over nothing is red rather than clean.

## How the score is recorded, so a trend is visible

Every run writes one row per surface into a tab-separated file: the run
number, the commit, the surface, and the counts of mutants in total, killed,
survived, not covered and timed out, with the efficacy the tool computes, which
is killed over killed plus survived. The file and the tool's own record per
surface are kept as an artefact named `mutation-score` on every run, for ninety
days.

The step after the measurement reads the artefacts of the runs before it and
prints them with this run's rows as one table in the run's summary, sorted by
run number, so a reader of any run sees where the score came from and not a
single figure. An expired artefact is counted as skipped rather than silently
absent, and the summary says how many were read and how many were not.

## The first recorded run

Run on the machine this document was written on rather than on the runner,
because the workflow lands with this document and had not run yet. The
figures are therefore this tree at the commit before the triage below, on
Windows, with the tool at `v0.5.0` and a timeout coefficient of sixty, which
was the first value at which no mutant timed out. Each line is what the tool
printed for that surface:

    gremlins unleash --timeout-coefficient 60
    internal/roomcred    Killed: 32, Lived: 8, Not covered: 1   Test efficacy: 80.00%
    internal/wire        Killed: 25, Lived: 1, Not covered: 2   Test efficacy: 96.15%
    internal/placement   Killed: 22, Lived: 2, Not covered: 10  Test efficacy: 91.67%
    internal/pool        Killed: 43, Lived: 5, Not covered: 7   Test efficacy: 89.58%
    internal/admission   Killed: 28, Lived: 2, Not covered: 9   Test efficacy: 93.33%

Eighteen survivors. Not-covered mutants are lines no test reaches at all, which
is the coverage bar's subject on issue #92 and not this document's; they are
printed here so the two are not confused.

Two bounds on those figures. The timeouts at smaller coefficients were compile
time on a laptop and not the tests, so the runner's numbers will differ in the
timed-out column and nowhere else that matters. And a coefficient this large
means a mutant that produces an endless loop costs sixty times the package's
test duration before it is counted, which is the price of a zero in that
column.

## The triage of the eighteen

Each survivor is either a new test or a reason it is harmless. A reason is not
a shrug: it names why the changed operator cannot produce a different answer on
any input the code can meet.

### Answered by a test

- `internal/admission/admission.go:581`, the sort tiebreak in `Sweep` negated.
  Two grants made at one clock reading came back in either order and nothing
  noticed. `TestAdmissionsGrantedInOneInstantAreSweptInIdentifierOrder` grants
  three in one instant and asserts the identifier order.
- `internal/pool/pool.go:573` and `pool.go:591`, `load < 0` widened to
  `load <= 0` in `Report` and in `Commit`. A refusal of zero left the suite
  green, and under it an idle unit could never report.
  `TestZeroIsALoadAndNotARefusal` reports and commits exactly zero.
- `internal/pool/pool.go:673`, the draining half of what `Sweep` judges
  negated. A sweep that skipped every draining unit passed, so a unit that
  stopped answering while it drained would have stood forever.
  `TestSweepRetiresADrainingUnitWhoseSignalWentStale` drains one, lets its
  signal go stale and asserts it is retired for that cause.
- `internal/roomcred/roomcred.go:175` and `roomcred.go:320`, the field maximum
  widened to refuse exactly `MaxFieldBytes` at minting and at reading. The
  suite refused one byte over and held nothing at the line.
  `TestAFieldOfExactlyTheMaximumIsMintedAndRead` round-trips a field of
  exactly the maximum in each of the three positions. The same test kills the
  three arithmetic mutants at `roomcred.go:281`, because a capacity of
  `64 - len` goes negative at that length and `make` refuses it.
- `internal/wire/wire.go:220`, the message maximum widened to refuse exactly
  `MaxMessageBytes` on the way out. `Decode` had its case at the line and
  `Encode` had one byte over. `TestEncodeAcceptsAMessageOfExactlyTheMaximum`
  encodes a message that lands on the line, decodes it back, and refuses one
  byte more.

### Answered by a reason

- `internal/admission/admission.go:581`, the same tiebreak with `<` widened to
  `<=`. The two comparators disagree only on equal identifiers, and
  `TestEveryParticipantGetsItsOwnIdentifier` is the assertion that two
  admissions never share one, so no input the desk can hold reaches the
  difference.
- `internal/placement/placement.go:322`, `<` widened to `<=` on the effective
  load inside `prefer`. The line is guarded by `a.effectiveLoad !=
  b.effectiveLoad`, so the two loads are never equal where the comparison
  runs and the two operators are the same function there.
- `internal/placement/placement.go:324`, `<` widened to `<=` on the unit
  identifier in the same function. A pool view holds one row per identifier,
  so two rows never carry equal identifiers, and on distinct identifiers the
  two operators agree. The negation of this line is killed, so the direction
  is held.
- `internal/pool/pool.go:513`, `age < 0` widened to `age <= 0` before the sign
  is flipped. At zero the flip produces zero, so the branch being taken or not
  changes no value.
- `internal/pool/pool.go:704`, `<` widened to `<=` in the sort that orders
  `Units`. The rows come out of a map keyed by identifier, so no two rows are
  equal under the comparison and the sort is the same sort.
  `TestUnitsIsOrderedByIdentifierWhateverOrderTheyArrivedIn` holds the
  direction.
- `internal/roomcred/roomcred.go:305`, `< 1` widened to `<= 1` on the payload
  length in `decode`. A one-byte payload is refused either way as malformed;
  what moves is which sentence follows `ErrMalformed`, and every caller and
  every test judges the error and not the sentence.
- `internal/roomcred/roomcred.go:315` and `roomcred.go:323`, the two
  length checks inside the field loop of `decode` widened by one. A well-formed
  payload always has the two window instants after the last field, so neither
  check meets its boundary on a credential this build minted, and on a
  malformed one both operators refuse with `ErrMalformed` and differ only in
  the sentence.

## The run after the triage

The same command, on the same machine, after the five tests above landed:

    gremlins unleash --timeout-coefficient 60
    internal/roomcred    Killed: 37, Lived: 3, Not covered: 1   Test efficacy: 92.50%
    internal/wire        Killed: 26, Lived: 0, Not covered: 2   Test efficacy: 100.00%
    internal/placement   Killed: 22, Lived: 2, Not covered: 10  Test efficacy: 91.67%
    internal/pool        Killed: 46, Lived: 2, Not covered: 7   Test efficacy: 95.83%
    internal/admission   Killed: 29, Lived: 1, Not covered: 9   Test efficacy: 96.67%

Eight survivors, which are the eight above with a reason. The `wire` run
reported two timeouts on this machine in that second pass and none in the
first, on an unchanged package, which is the bound stated above showing itself
rather than a change in the code.

## What a later run owes

A new survivor is triaged here or answered by a test, in the change that
answers it, so this document stays the record of every survivor that has stood
and why. A survivor listed above under a reason that stops being true, because
the guard it rests on was removed, is a defect in that change and not in this
document, and the reason names the guard so that a reader can check.
