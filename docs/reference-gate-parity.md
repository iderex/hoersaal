# Parity with the reference gate

This records, element by element, whether this repository adopts, adapts or drops
what the gate on `iderex/jellyfin-plugin-sso` does. It answers issue #89. The
reference is the standard this board is held to rather than a standard invented
here, and every departure from it is written down with its reason, in both
directions: something that gate has and this one does not, and something this one
needs that gate has no counterpart for.

Parity is not copying. That gate is for a plugin loaded into somebody else's
process. This is a network service that terminates untrusted connections, carries
personal communications, and starts machines on its own initiative.

## What an element is here

An element is one of three things, and each list below is derived from a command
rather than maintained by hand:

- a check-run name the reference ruleset requires on its protected branch,
- a workflow in the reference repository, whether or not it is required,
- the coverage bar the reference build enforces, which is a threshold inside a
  job rather than a check-run name of its own.

What that leaves out is stated at the end, because an enumeration whose edge is
not stated reads as complete when it is not.

## The three states of the gates, at the commit this landed on

    gh api repos/iderex/jellyfin-plugin-sso/rulesets --jq '.[] | [.id,.name,.enforcement] | @tsv'
    18802863	Protect main and 5.0	active

    gh api repos/iderex/jellyfin-plugin-sso/rulesets/18802863 --jq '{enforcement, bypass: .bypass_actors, required: [.rules[] | select(.type=="required_status_checks") | .parameters.required_status_checks[].context]}'
    {"bypass":[],"enforcement":"active","required":["build","ABI floor build","Package (JPRM) / Build package","Package (JPRM) / Generate SBOM","CodeQL","Analyze (csharp)","DCO sign-off","Deterministic PR-hygiene checks","Enforce greppable invariants","Reject Trojan Source Unicode","Audit workflows (zizmor)","prettier","dependency-review"]}

    gh api repos/iderex/hoersaal/rulesets/20486335 --jq '{enforcement, bypass: .bypass_actors, types: [.rules[].type]}'
    {"bypass":[],"enforcement":"active","types":["deletion","non_fast_forward","pull_request"]}

So this repository requires no status check at all today. Both rulesets are active
and neither has a bypass actor. What this repository does run, and what it names
those runs, is the other side of the comparison:

    gh api repos/iderex/hoersaal/commits/c70f12de1f68166ac0004d7b1b2889bd0b1601b4/check-runs --jq '[.check_runs[].name] | unique | join(", ")'
    Audit workflows (zizmor), Reject Trojan Source Unicode, Scorecard analysis, analysis, build, lint, sbom, unit

Eight named runs exist and none of them is required, which is the distance issue
#28 closes. Every element below that says "already runs here" means exactly that
and nothing about being required.

The coverage bar, quoted through the API so that reading it needs no clone of the
reference repository:

    gh api repos/iderex/jellyfin-plugin-sso/contents/scripts/check-coverage.py --jq '.content' | base64 -d | grep -n 'SECURITY_LINE_BAR ='
    68:SECURITY_LINE_BAR = 92.0

The workflow list the second and third sections are derived from:

    gh api repos/iderex/jellyfin-plugin-sso/contents/.github/workflows --jq '.[].name' | tr '\n' ' '
    build.yml codeql.yml dco.yml dependency-review.yml dotnet.yml e2e-login.yml fuzz.yml manifest-freshness.yml nightly-betas.yml opengrep.yml pr-hygiene.yml prettier.yml publish-beta.yml publish-failure-alert.yml publish-jf12-beta.yml publish-jf12-stable.yml publish.yml regenerate-manifest.yml scorecard.yml stryker-mutation.yml unicode-guard.yml wiki-lint.yml zizmor.yml

## The required checks

**build**, adapted, issues #17, #18, #20 and #92. One job there compiles with
warnings as errors, restores in locked mode, scans for vulnerable dependencies,
runs the suite, and enforces the coverage bar. Here those are separate checks, so
a failing compile and a failing suite are two different red lights rather than
one, and `build` and `unit` already run. The locked restore is #20 and the
coverage bar is #92, both still open.

**ABI floor build**, dropped. It builds the plugin against the oldest host
version whose interface it claims to support. Nothing here loads into somebody
else's process and there is no host interface to hold a floor against, so the
element has no analogue rather than a cheaper version.

**Package (JPRM) / Build package**, adapted, issue #110. Their artefact is a
plugin package for a plugin host. Ours is a container image an operator runs, so
the element survives with its subject replaced.

**Package (JPRM) / Generate SBOM**, adapted upward, issue #21. Theirs is produced
where the package is built. Here the document is produced by every build, from the
compiled artefact rather than from the manifest, and `sbom` already runs. What is
still owed is attaching it to a release, which is #115 and is why #21 is open.

**CodeQL** and **Analyze (csharp)**, adopted and already running, issue #90.
These are two required names for one element there: the workflow name and the
job name of the matrix language. Here it is one name, `code-scanning`, because
the languages are analysed in one job rather than in a matrix, so the name does
not move when a language is added. The language differs and the element does
not. What is left is requiring it.

**DCO sign-off**, adopted and already running, issues #23 and #28. The gate here
is the same first-party check rather than an external app, the certificate it
gates against is in the tree, and what is left is requiring it.

**Deterministic PR-hygiene checks**, adopted, issue #96.

**Enforce greppable invariants**, adopted, issue #95. Theirs runs opengrep over
the repository. What this project needs the element for is listed on #95 and is
mostly not the same set of rules, which is a difference in the rules rather than
in the element.

**Reject Trojan Source Unicode**, adopted and already running, issues #26 and #28.

**Audit workflows (zizmor)**, adopted and already running, issue #28.

**prettier**, adapted into two, issues #19 and #97. Theirs formats the languages a
plugin repository carries. Here formatting the code is `lint`, which already runs
`gofmt` over the whole tree, and formatting and linting the documentation is #97,
which nothing does yet. A single element there is two here because the code and
the prose are checked by different tools.

**dependency-review**, adopted and already running, issues #20 and #28.

## The elements that are not required there

**Scorecard supply-chain security**, adopted and already running, issue #28.
`Scorecard analysis` is in the list of runs above.

**Fuzz (SharpFuzz)**, adapted and already running, issue #94. Non-gating by
construction there, on a weekly schedule and manual dispatch with no
pull-request trigger, and non-gating here for the same reason. The surfaces that
take bytes from strangers are different ones here, and the protocol decoder is
the first of them. Two departures, both with their reason at the workflow:
Go's own fuzzing rather than a fuzzing library, because the toolchain the tree
already carries has it; and twice a week rather than weekly, because an Actions
cache is evicted after seven days unread and the corpus is what the schedule
exists to accumulate.

**Stryker mutation testing**, adapted, issue #93, reporting rather than gating,
which is the posture it already has there.

**E2E Login Harness**, adapted, issues #51 and #75. Theirs logs into real identity
providers on a schedule and on a pull request that touches the harness. The
counterpart here is the media integration harness on #51, named for what it is,
plus the browser-dependent part of the client on #75. This element is the one that
grows rather than shrinks in translation, because a conference has no equivalent
of a single login to assert.

**Repo Invariant Lint (Opengrep)** is the workflow behind `Enforce greppable
invariants` above and is not a second element.

**Wiki Lint**, adapted, issue #97. There is no wiki here, and the documentation in
the tree is what #97 covers.

**Manifest freshness**, dropped. It asserts that a published plugin manifest lists
the newest release of each generation. There is no manifest and no plugin channel
here, so nothing corresponds. The nearest thing on this board, the release
readiness checklist on #117, is a different element and is not counted as this one
adopted.

**Nightly betas**, **publish**, **publish-beta**, **publish-jf12-beta**,
**publish-jf12-stable**, **publish-failure-alert** and **regenerate-manifest**,
outside the gate. None of them runs on a pull request; they run on a tag, on a
schedule or on manual dispatch, and they publish rather than judge. Where their
work has a counterpart here it is the release milestone, #110 to #117, rather than
a gate element.

    for w in publish publish-beta publish-jf12-beta publish-jf12-stable publish-failure-alert regenerate-manifest; do printf '%s: ' "$w.yml"; gh api "repos/iderex/jellyfin-plugin-sso/contents/.github/workflows/$w.yml" --jq '.content' | base64 -d | sed -n '/^on:/,/^[a-z]/p' | grep -oE 'push|pull_request|schedule|workflow_dispatch|tags' | sort -u | tr '\n' ' '; echo; done
    publish.yml: push tags workflow_dispatch
    publish-beta.yml: workflow_dispatch
    publish-jf12-beta.yml: workflow_dispatch
    publish-jf12-stable.yml: push tags workflow_dispatch
    publish-failure-alert.yml: schedule workflow_dispatch
    regenerate-manifest.yml: workflow_dispatch

**The security-surface coverage bar**, adopted, issue #92. The reference pins the
bar on the modules that decide security outcomes rather than on the whole
codebase, at the figure quoted above. The surfaces that decide admission,
authorisation and placement are the counterpart here, and placement is a surface
that gate has nothing like, because a wrong placement is a room that degrades
rather than a permission wrongly granted.

## What this repository adds

Each of these is a deviation upward and owes its own argument rather than
borrowing the reference's authority.

**Verified signatures on the protected branch**, issue #99. The reference ruleset
does not require them:

    gh api repos/iderex/jellyfin-plugin-sso/rulesets/18802863 --jq '[.rules[].type]'
    ["deletion","non_fast_forward","required_status_checks","pull_request"]

There is no `required_signatures` in that list and none in this repository's
either, so this is a deviation upward from both states rather than a gap being
closed. The argument and its cost are on #99 and are not restated here.

**A headless unit job that asserts the absences**, issue #18, already running. The
job refuses a display, a media device and elevation rather than assuming a runner
lacks them, and it fails on zero tests. The reference has no reason to care
whether a runner has a camera; a conferencing project has every reason.

**A count leg on each check that can pass over nothing**, already running in
`unit`, `lint` and `sbom`. A green run over an empty set is the failure these legs
exist against, and the reference gate does not carry the shape.

**A refusal of a direct clock read, a random source outside one place and a sleep
anywhere**, issue #27, already running inside the suite. Almost everything this
service does is a duration, so a suite that waits is a suite that gets rerun until
it is green.

**Architecture rules as tests**, issue #98. The boundary between the control plane
and the media plane is a property of the import graph, and the reference has no
boundary of that kind to hold.

**The scaling suite**, issue #65, and the load evidence on the whole of M5. A
plugin cannot be asked what it does at three hundred participants or when the pool
cannot grow, and this project is mostly about the answer.

**The bench and the media integration harness**, issues #2 and #51, which are what
every number about quality has to come from.

## What this document does not enumerate

Steps inside a job are below the granularity of the lists above, and two of them
are worth naming so their absence is not read as a verdict. The reference build
runs a Jellyfin compatibility metadata check and a VEX document check, at
`scripts/check-jellyfin-compat.sh` and `scripts/check-vex.py`. The first is the
same host-interface concern as the ABI floor and drops with it. The second has no
issue on this board, and this document is the first place that is written down;
whether a vulnerability-exploitability statement belongs beside the bill of
materials on #21 is not decided here.

Three workflow headers in this repository name issue numbers that belong to the
reference repository rather than to this board:

    for w in dco unicode-guard zizmor; do printf '%s: ' "$w.yml"; grep -oE '#[0-9]{2,4}' ".github/workflows/$w.yml" | sort -u | tr '\n' ' '; echo; done
    dco.yml: #746
    unicode-guard.yml: #954
    zizmor.yml: #263

    gh issue list --repo iderex/hoersaal --state all --limit 1 --json number --jq '.[0].number'
    117

Those three elements were adopted with their prose, and the prose points at
issues nobody here can open. No issue holds the repair, and this document is not
the place it is made, because a header is not under `docs/`.
