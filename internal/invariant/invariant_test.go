// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package invariant

import (
	"strconv"
	"strings"
	"testing"

	"github.com/iderex/hoersaal/internal/config"
	"github.com/iderex/hoersaal/internal/guard"
)

// The cases that red each enforced rule. Issue #95 asks for one per rule and
// asks that they be kept, so they live here rather than in a branch that was
// pushed once to show a red tick and then reverted. Each is the one-character
// mistake somebody actually makes: the identifier that was to hand, the URL the
// unit's own documentation prints, the log line written while chasing a bug.
var refusedCases = []struct {
	rule string
	path string
	src  string
}{
	{
		rule: RuleClockAndRandom,
		path: "internal/pool/pool.go",
		src: `package pool

import "time"

func Stale() bool { return time.Now().IsZero() }
`,
	},
	{
		rule: RuleForwardingUnitName,
		path: "internal/pool/pool.go",
		src: `package pool

const statsPath = "/colibri/stats"

func StatsPath() string { return statsPath }
`,
	},
	{
		rule: RuleLoggedIdentifier,
		path: "internal/pool/pool.go",
		src: `package pool

import "log/slog"

type joiner struct{ participantID string }

func admitted(j joiner) { slog.Info("admitted", "who", j.participantID) }
`,
	},
	{
		rule: RuleConfigurationKey,
		path: "cmd/hoersaal/main.go",
		src: `package main

import "fmt"

func advise() { fmt.Printf("%s is the floor\n", "pool.minimun") }
`,
	},
}

// The neighbours. Each is one change away from a case above and each is correct
// code, so a rule that refused one of these would be a rule somebody deletes
// rather than argues with.
var acceptedCases = []struct {
	why  string
	path string
	src  string
}{
	{
		why:  "duration arithmetic reads no clock",
		path: "internal/pool/pool.go",
		src: `package pool

import "time"

const timeout = 30 * time.Second

func Timeout() time.Duration { return timeout }
`,
	},
	{
		why:  "the unit's name in a comment is prose, not a dependency",
		path: "internal/pool/pool.go",
		src: `package pool

// The forwarding unit behind the port is a jitsi videobridge, and this package
// does not know that. The adapter does.
func Units() int { return 0 }
`,
	},
	{
		why:  "the adapter is the one directory that may name the unit",
		path: AdapterDir + "/mediaunit.go",
		src: `package mediaunit

const statsPath = "/colibri/stats"

func StatsPath() string { return statsPath }
`,
	},
	{
		why:  "a conference identifier is not a person",
		path: "internal/pool/pool.go",
		src: `package pool

import "log/slog"

type room struct{ conferenceID string }

func started(r room) { slog.Info("started", "conference", r.conferenceID) }
`,
	},
	{
		why:  "printing a path is not logging a person",
		path: "cmd/tool/main.go",
		src: `package main

import "fmt"

func main() { fmt.Printf("read %s\n", "docs") }
`,
	},
	{
		why:  "the key the typo above missed is one an operator may set",
		path: "cmd/hoersaal/main.go",
		src: `package main

import "fmt"

func advise() { fmt.Printf("%s is the floor\n", "pool.minimum") }
`,
	},
	{
		why:  "prose that mentions a key is not a key",
		path: "cmd/hoersaal/main.go",
		src: `package main

import "fmt"

func advise() { fmt.Println("raise pool.minimun if the first arrival is refused") }
`,
	},
	{
		why:  "the directory that holds the list may write a key that is not on it",
		path: ConfigDir + "/config.go",
		src: `package config

func prefixes() []string { return []string{"pool.", "listen."} }
`,
	},
	{
		// This one is not hypothetical. The first run of the key rule over this
		// tree refused it, in internal/textbytes' own suite.
		why:  "a workflow file is named like a key and is a file",
		path: "internal/textbytes/textbytes_test.go",
		src: `package textbytes

const workflow = "unit.yml"

func Workflow() string { return workflow }
`,
	},
	{
		// Refused by the same first run, in internal/pool.
		why:  "a fragment of Go being printed is not a key",
		path: "internal/pool/pool.go",
		src: `package pool

import "fmt"

func show() { fmt.Print("pool.Pool{key: ") }
`,
	},
}

func TestEveryRuleNamesTheIssueThatRequiredIt(t *testing.T) {
	if len(Rules) == 0 {
		t.Fatal("the table is empty, so this suite asserts nothing")
	}
	seen := map[string]bool{}
	for _, r := range Rules {
		if r.ID == "" {
			t.Error("a rule has no id, and the id is what a refusal is searched for by")
		}
		if seen[r.ID] {
			t.Errorf("%s appears twice, so one of the two is unreachable", r.ID)
		}
		seen[r.ID] = true
		if !strings.HasPrefix(r.Issue, "#") {
			t.Errorf("%s names no issue; a rule nobody can trace to an argument is a preference", r.ID)
		}
		if r.Subject == "" {
			t.Errorf("%s states no subject, so a reader cannot tell what it read", r.ID)
		}
	}
}

func TestAnEnforcedRuleRunsAndADeclaredRuleSaysWhatItWaitsFor(t *testing.T) {
	for _, r := range Rules {
		switch {
		case r.Enforced() && r.check == nil:
			t.Errorf("%s reports itself enforced and has no checker, so it refuses nothing while the table says it does", r.ID)
		case !r.Enforced() && r.check != nil:
			t.Errorf("%s carries a checker and a reason for not running, and the command runs neither", r.ID)
		case !r.Enforced() && !strings.Contains(r.Waiting, "#"):
			t.Errorf("%s waits on something and does not name the issue that will produce it", r.ID)
		}
	}
}

// The cases issue #95 asks be kept. Each one reds exactly its own rule, which is
// the leg that separates a checker that refuses the right thing from one that
// refuses everything.
func TestEachEnforcedRuleRefusesItsOwnCase(t *testing.T) {
	for _, c := range refusedCases {
		findings, err := CheckFile(c.path, []byte(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.rule, err)
		}
		if len(findings) == 0 {
			t.Errorf("%s: the case was accepted, so the rule does not bite", c.rule)
			continue
		}
		for _, f := range findings {
			if f.Rule != c.rule {
				t.Errorf("%s: the case also reds %s, so neither refusal proves the other", c.rule, f.Rule)
			}
			if f.Line == 0 {
				t.Errorf("%s: the refusal carries no line", c.rule)
			}
			if !strings.Contains(f.Detail, "issue #") {
				t.Errorf("%s: the refusal does not say where the rule came from: %s", c.rule, f.Detail)
			}
		}
	}
}

// Every enforced rule has a case above. A rule added to the table without one
// would otherwise ship unproved, which is the thing this suite exists against.
func TestEveryEnforcedRuleHasACaseThatRedsIt(t *testing.T) {
	proved := map[string]bool{}
	for _, c := range refusedCases {
		proved[c.rule] = true
	}
	for _, r := range Rules {
		if r.Enforced() && !proved[r.ID] {
			t.Errorf("%s runs and no case reds it, so nothing shows it refuses what it names", r.ID)
		}
	}
}

func TestTheNeighboursAreAccepted(t *testing.T) {
	for _, c := range acceptedCases {
		findings, err := CheckFile(c.path, []byte(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.why, err)
		}
		for _, f := range findings {
			t.Errorf("%s: refused %s", c.why, f)
		}
	}
}

// This package holds the vocabulary as data and is exempt from the rule that
// reads it. The exemption is stated in the package comment and it is asserted
// here, because an exemption nobody can see is how a rule stops covering the
// tree without anybody deciding that.
func TestThisPackageIsExemptFromTheForwardingUnitRule(t *testing.T) {
	src := `package invariant

const leaked = "colibri"
`
	findings, err := CheckFile(SelfDir+"/leak.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("this package is meant to be exempt and was refused: %v", findings)
	}

	// The same bytes anywhere else are a finding, which is what makes the line
	// above an exemption rather than the rule being off.
	findings, err = CheckFile("internal/pool/leak.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != RuleForwardingUnitName {
		t.Fatalf("the same bytes outside this package should be one forwarding unit finding, got %v", findings)
	}
}

// internal/config holds the key list as data and is exempt from the rule that
// reads it. The exemption is stated in this package's comment and it is
// asserted here, because an exemption nobody can see is how a rule stops
// covering the tree without anybody deciding that.
func TestTheConfigurationPackageIsExemptFromTheKeyRule(t *testing.T) {
	src := `package config

const invented = "listen.tls"
`
	findings, err := CheckFile(ConfigDir+"/config.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("the directory holding the list is meant to be exempt and was refused: %v", findings)
	}

	// The same bytes anywhere else are a finding, which is what makes the line
	// above an exemption rather than the rule being off.
	findings, err = CheckFile("internal/pool/pool.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != RuleConfigurationKey {
		t.Fatalf("the same bytes outside that directory should be one configuration key finding, got %v", findings)
	}
}

// The key rule reads the list out of internal/config rather than keeping a
// second copy of it, so a key added there is accepted here on the same commit.
// Two lists would drift and the drift would be silent, which is the reason the
// clock rule below is read out of internal/guard as well.
func TestTheKeyRuleReadsTheListRatherThanACopy(t *testing.T) {
	keys := config.Keys()
	if len(keys) == 0 {
		t.Fatal("the list is empty, so this rule reads nothing")
	}
	for _, key := range keys {
		src := "package pool\n\nconst k = " + strconv.Quote(key) + "\n"
		findings, err := CheckFile("internal/pool/pool.go", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range findings {
			if f.Rule == RuleConfigurationKey {
				t.Errorf("%s is a key the loader accepts and the rule refused it: %s", key, f.Detail)
			}
		}
	}
}

// The subject line for the clock rule names two files as its exceptions. That is
// internal/guard's answer rather than this package's, so it is read out of that
// package instead of being repeated and trusted.
func TestTheClockExceptionsAreTheOnesGuardHolds(t *testing.T) {
	reads := `package p

import "time"

var t0 = time.Now()
`
	for _, place := range []string{clockPlace, "internal/pool/pool.go"} {
		found, err := guard.CheckFile(place, []byte(reads))
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case place == clockPlace && len(found) != 0:
			t.Errorf("%s is named as the file allowed to read the clock and guard refused it: %v", place, found)
		case place != clockPlace && len(found) == 0:
			t.Errorf("%s reads the machine clock and guard accepted it", place)
		}
	}

	makes := `package p

import "math/rand"

var n = rand.Int()
`
	found, err := guard.CheckFile(randomPlace, []byte(makes))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("%s is named as the file allowed to make randomness and guard refused it: %v", randomPlace, found)
	}
}

// The tree this ships in passes its own rules. A checker that reds the
// repository it lands in is a checker nobody can land, and this is the leg that
// says so before a run does.
func TestTheTreeItselfIsClean(t *testing.T) {
	findings, files, err := CheckTree("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no Go file was read, so this test asserts nothing")
	}
	for _, f := range findings {
		t.Errorf("the tree is refused: %s", f)
	}
}
