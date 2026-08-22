// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package doclint

import (
	"strings"
	"testing"
)

// tree is the fixture stand-in for a repository, so a rule is proved without a
// checkout on disk and without a rule's proof depending on what happens to be in
// this tree on the day it runs.
func tree(paths ...string) Exists {
	held := map[string]bool{}
	for _, p := range paths {
		held[p] = true
	}
	return func(p string) bool { return held[p] }
}

// TestTreeIsClean is the check itself. It reads the repository it lives in.
func TestTreeIsClean(t *testing.T) {
	findings, _, _, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

// TestTheWalkReadTheDocuments is the leg against a green run over nothing. A
// walk that read no document refuses nothing and looks exactly like a walk that
// read every document and found them clean.
func TestTheWalkReadTheDocuments(t *testing.T) {
	_, documents, refs, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	seen := map[string]bool{}
	for _, d := range documents {
		seen[d] = true
	}
	for _, want := range []string{
		"README.md",
		"CONTRIBUTING.md",
		"docs/decisions/room-topology.md",
		".github/PULL_REQUEST_TEMPLATE.md",
	} {
		if !seen[want] {
			t.Errorf("the walk did not read %s, so it is not covering the documents it reports on", want)
		}
	}
	if len(refs) == 0 {
		t.Error("the walk found no reference of any kind, so nothing was resolved and the run proves nothing")
	}
	resolved := 0
	for _, r := range refs {
		if r.Resolved {
			resolved++
		}
	}
	if resolved == 0 {
		t.Error("no reference in the tree resolved, so the resolver answered nothing and every link passed by not being read")
	}
	// The count of what was not followed is reported and not asserted. It is
	// zero in this tree today, because every address these documents carry sits
	// inside a code block, and a test demanding a non-zero one would be
	// demanding a property of the prose rather than of the check. That the
	// counting works at all is proved on a fixture in
	// TestAnExternalReferenceIsRecordedAsNotFollowed.
	t.Logf("%d documents, %d references, %d of them not followed", len(documents), len(refs), CountExternal(refs))
}

// TestEachRuleRefusesWhatItNames is the proof that the rules bite, one case per
// rule, each of them a mistake somebody makes.
func TestEachRuleRefusesWhatItNames(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		src    string
		exists Exists
		rule   string
	}{
		{
			name: "a moved file behind a link",
			path: "docs/decisions/room-topology.md",
			src:  "# Topology\n\nIt uses [capacity-signal.md](capacity-signal.md) without redefining it.\n",
			// The neighbour was renamed and this document was not touched.
			exists: tree("docs/decisions/capacity.md"),
			rule:   RuleLinkResolves,
		},
		{
			name:   "a link out of a document at the root",
			path:   "README.md",
			src:    "# hoersaal\n\nSee [NOTICE.md](NOTICE.md) for the notice.\n",
			exists: tree("docs/what-this-is-not-for.md"),
			rule:   RuleLinkResolves,
		},
		{
			name:   "a package named in prose that no longer exists",
			path:   "docs/repository-layout.md",
			src:    "# Layout\n\nThe placer is `internal/placement` and takes its clock.\n",
			exists: tree("internal/domain"),
			rule:   RulePathResolves,
		},
		{
			name:   "a file named in prose that no longer exists",
			path:   "CONTRIBUTING.md",
			src:    "# Contributing\n\nThe rules are in `internal/prhygiene/prhygiene.go`.\n",
			exists: tree("internal/prhygiene"),
			rule:   RulePathResolves,
		},
		{
			name:   "trailing whitespace",
			path:   "docs/x.md",
			src:    "# X\n\nA line with a space at the end. \n",
			exists: tree(),
			rule:   RuleTrailingWhitespace,
		},
		{
			name:   "a hard tab in prose",
			path:   "docs/x.md",
			src:    "# X\n\nA line\twith a tab in it.\n",
			exists: tree(),
			rule:   RuleHardTab,
		},
		{
			name:   "no newline at the end",
			path:   "docs/x.md",
			src:    "# X\n\nThe last line stops here.",
			exists: tree(),
			rule:   RuleFinalNewline,
		},
		{
			name:   "a blank line at the end",
			path:   "docs/x.md",
			src:    "# X\n\nThe last line stops here.\n\n",
			exists: tree(),
			rule:   RuleFinalNewline,
		},
		{
			name:   "two blank lines in a row",
			path:   "docs/x.md",
			src:    "# X\n\n\nA paragraph after two of them.\n",
			exists: tree(),
			rule:   RuleBlankRun,
		},
		{
			name:   "a fence that is never closed",
			path:   "docs/x.md",
			src:    "# X\n\n```\nsomething\n",
			exists: tree(),
			rule:   RuleFenceClosed,
		},
		{
			name:   "a heading level skipped",
			path:   "docs/x.md",
			src:    "# X\n\n### Three, under one\n\nText.\n",
			exists: tree(),
			rule:   RuleHeadingLevel,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings, _ := CheckFile(c.path, []byte(c.src), c.exists)
			if len(findings) == 0 {
				t.Fatalf("accepted a document the %s rule exists to refuse", c.rule)
			}
			hit := false
			joined := ""
			for _, f := range findings {
				joined += f.String() + "\n"
				if f.Rule == c.rule {
					hit = true
				}
			}
			if !hit {
				t.Errorf("refused it under some other rule; wanted %s and got:\n%s", c.rule, joined)
			}
		})
	}
}

// TestTheRulesAcceptWhatTheyShould is the other half. A checker that refuses
// everything is not a checker, and the cases here are the ones that made the
// path rule the shape it is: the prose in this repository that looks exactly
// like a path and is not one.
func TestTheRulesAcceptWhatTheyShould(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		src    string
		exists Exists
	}{
		{
			name:   "a branch name that is not a path",
			path:   "CONTRIBUTING.md",
			src:    "# Contributing\n\nA branch is named like `docs/contribution-guide` or `ci/build-check`.\n",
			exists: tree(),
		},
		{
			name:   "a repository in another account",
			path:   "docs/reference-gate-parity.md",
			src:    "# Parity\n\nThe reference is `iderex/jellyfin-plugin-sso` and its gate.\n",
			exists: tree(),
		},
		{
			name:   "a script in another repository",
			path:   "docs/reference-gate-parity.md",
			src:    "# Parity\n\nThe reference build runs `scripts/check-vex.py` as a step.\n",
			exists: tree(),
		},
		{
			name:   "an address this check does not follow",
			path:   "docs/x.md",
			src:    "# X\n\nSee [the licence](https://www.gnu.org/licenses/agpl-3.0.html) for the terms.\n",
			exists: tree(),
		},
		{
			name:   "a link with an anchor on it",
			path:   "docs/design/scaling-loop.md",
			src:    "# Loop\n\nSee [the signal](../decisions/capacity-signal.md#the-rule) for the number.\n",
			exists: tree("docs/decisions/capacity-signal.md"),
		},
		{
			name:   "a path inside command output",
			path:   "docs/x.md",
			src:    "# X\n\nWhat it printed:\n\n    ls internal/gone/away.go\n    internal/gone/away.go\n",
			exists: tree(),
		},
		{
			name:   "a tab inside command output",
			path:   "docs/x.md",
			src:    "# X\n\nWhat it printed:\n\n    18802863\tProtect main\tactive\n",
			exists: tree(),
		},
		{
			name:   "a document with no level one heading",
			path:   ".github/PULL_REQUEST_TEMPLATE.md",
			src:    "## What changed\n\nOne topic.\n\n## What was run\n\nThe commands.\n",
			exists: tree(),
		},
		{
			name:   "a package and a file that both exist",
			path:   "docs/repository-layout.md",
			src:    "# Layout\n\nThe placer is `internal/placement` and the wire is `internal/wire/wire.go`.\n",
			exists: tree("internal/placement", "internal/wire/wire.go"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings, _ := CheckFile(c.path, []byte(c.src), c.exists)
			if len(findings) != 0 {
				t.Errorf("refused prose the rule is not about:\n%v", findings)
			}
		})
	}
}

// TestAnExternalReferenceIsRecordedAsNotFollowed keeps the two statements apart.
// A link that was not checked and a link that was checked and resolved are
// different things, and a run that reported them the same would let "nothing
// outside is covered" read as "everything outside is fine".
func TestAnExternalReferenceIsRecordedAsNotFollowed(t *testing.T) {
	src := "# X\n\nSee [there](https://example.invalid/gone) and [here](NOTICE.md).\n"
	findings, refs := CheckFile("README.md", []byte(src), tree("NOTICE.md"))
	if len(findings) != 0 {
		t.Fatalf("refused something: %v", findings)
	}
	if len(refs) != 2 {
		t.Fatalf("found %d references in a document holding two", len(refs))
	}
	if CountExternal(refs) != 1 {
		t.Errorf("counted %d external references in a document holding one", CountExternal(refs))
	}
	for _, r := range refs {
		if r.External && r.Resolved {
			t.Errorf("%s is reported as both not followed and resolved", r.Target)
		}
	}
}

// TestTheBoundOnThePathRuleIsWhereItSaysItIs states the edge as a table, because
// the edge is the whole design of that rule and a reader deciding whether to
// widen it needs to see what widening would start refusing.
func TestTheBoundOnThePathRuleIsWhereItSaysItIs(t *testing.T) {
	inside := []string{
		"internal/wire/wire.go",
		"cmd/hoersaal/main.go",
		"docs/decisions/room-topology.md",
		".github/workflows/unit.yml",
		"internal/placement",
		"cmd/prhygiene",
	}
	outside := []string{
		"docs/contribution-guide",
		"ci/build-check",
		"iderex/jellyfin-plugin-sso",
		"scripts/check-vex.py",
		"go.mod",
		"internal",
	}
	for _, s := range inside {
		if !IsPathReference(s) {
			t.Errorf("%q is a reference into this repository and the rule does not read it as one", s)
		}
	}
	for _, s := range outside {
		if IsPathReference(s) {
			t.Errorf("%q is not a path in this repository and the rule reads it as one", s)
		}
	}
}

// TestTheTopLevelBoundIsTheLayoutsOwn stops the list in this package from
// drifting away from the one internal/arch enforces. If a fifth top-level
// directory is ever added there, a path reference into it would be silently
// unjudged here, which is the quiet half of a drift.
func TestTheTopLevelBoundIsTheLayoutsOwn(t *testing.T) {
	_, documents, _, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	seen := map[string]bool{}
	for _, d := range documents {
		if first, _, ok := strings.Cut(d, "/"); ok {
			seen[first] = true
		}
	}
	for dir := range seen {
		if !contains(PathRefTopLevel, dir) {
			t.Errorf("the tree holds documents under %s/ and doclint.PathRefTopLevel does not name it, so a reference into it is not judged", dir)
		}
	}
}
