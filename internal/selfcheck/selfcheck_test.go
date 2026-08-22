// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package selfcheck

import (
	"strings"
	"testing"

	"github.com/iderex/hoersaal/internal/config"
)

// with is the settings a deployment has after an empty configuration file, with
// whatever this case is about changed. It goes through the loader rather than
// being composed, so no case here can assert against a configuration the loader
// would have refused.
func with(t *testing.T, body string) config.Settings {
	t.Helper()
	s, err := config.Load(strings.NewReader(body))
	if err != nil {
		t.Fatalf("the case's own configuration was refused: %v", err)
	}
	return s
}

// The third condition of issue #84, which is the one this package exists for. A
// step that could not be run reports as not verified, and the words are the
// ones the issue asks for.
func TestAStepThatCouldNotBeRunSaysSoRatherThanPassing(t *testing.T) {
	report := Run(with(t, "{}"))

	notVerified := 0
	for _, s := range report.Steps {
		if s.Outcome != NotVerified {
			continue
		}
		notVerified++
		if s.Outcome.String() != "NOT VERIFIED" {
			t.Errorf("%s reports as %q rather than in the words the issue asks for", s.Name, s.Outcome)
		}
		if s.Outcome == Passed {
			t.Errorf("%s could not be run and reports as passed", s.Name)
		}
	}
	if notVerified == 0 {
		t.Fatal("no step reported as not verified, so this suite asserts nothing about the case the package exists for")
	}
}

// The other half of the same condition, and the one a reader is most likely to
// take on trust. Nothing in this build opens a store, contacts a unit, binds a
// socket or signs a credential, so each of those steps must be unverified on
// every configuration rather than on the empty one.
func TestTheStepsThisBuildCannotRunAreNeverPassed(t *testing.T) {
	cases := []string{
		"{}",
		`{"store.path": "/var/lib/hoersaal/state.db"}`,
		`{"listen.address": "10.0.0.4", "listen.port": 9443, "listen.certificate": "/etc/hoersaal/full.pem"}`,
		`{"provisioner.driver": "none"}`,
	}
	absent := map[string]bool{
		"the listener": true, "the store": true, "the units": true, "the credential key": true,
	}

	for _, body := range cases {
		for _, s := range Run(with(t, body)).Steps {
			if absent[s.Name] && s.Outcome != NotVerified {
				t.Errorf("%s: %s reported %s, and nothing in this build can run it", body, s.Name, s.Outcome)
			}
		}
	}
}

// A step that could not be run names what is missing, and a step that failed
// names what to change. Those send an operator to two different places, so a
// detail that is empty or that does not name a place is a step nobody can act
// on, which is the second condition of the issue.
func TestEveryStepNamesSomethingToActOn(t *testing.T) {
	for _, body := range []string{"{}", `{"listen.certificate": "/etc/hoersaal/full.pem"}`} {
		report := Run(with(t, body))
		if len(report.Steps) == 0 {
			t.Fatal("the report holds no step")
		}
		for _, s := range report.Steps {
			if strings.TrimSpace(s.Name) == "" {
				t.Error("a step has no name")
			}
			if strings.TrimSpace(s.Detail) == "" {
				t.Errorf("%s says nothing, so there is nothing for an operator to do about it", s.Name)
			}
			if s.Outcome == NotVerified && !strings.Contains(s.Detail, "issue #") {
				t.Errorf("%s could not be run and does not say which issue produces what it needs: %s", s.Name, s.Detail)
			}
		}
	}
}

// The configuration is the one step that runs, and it runs because issue #82
// landed. Without a certificate it is a failure naming the key, because a
// deployment that cannot serve HTTPS is a thing to change rather than a thing
// this software has not reached.
func TestTheConfigurationStepRunsAndTheCertificateDecidesIt(t *testing.T) {
	failed := step(t, Run(with(t, "{}")), "the configuration")
	if failed.Outcome != Failed {
		t.Errorf("with no certificate the configuration step reported %s", failed.Outcome)
	}
	if !strings.Contains(failed.Detail, config.KeyListenCertificate) {
		t.Errorf("the failure does not name the key to set: %s", failed.Detail)
	}

	passed := step(t, Run(with(t, `{"listen.certificate": "/etc/hoersaal/full.pem"}`)), "the configuration")
	if passed.Outcome != Passed {
		t.Errorf("with a certificate the configuration step reported %s: %s", passed.Outcome, passed.Detail)
	}
}

// The first condition of issue #84 asks that liveness and readiness differ in
// at least one stated failure. This is that case: everything this build cannot
// verify leaves it unready, and none of it is repaired by a restart, so it
// stays alive.
func TestLivenessAndReadinessDifferOnAStepThatCouldNotBeRun(t *testing.T) {
	report := Run(with(t, "{}"))
	if report.Ready() {
		t.Error("a report with steps that could not be run says this instance is ready for participants")
	}
	if !report.Live() {
		t.Error("a report with nothing a restart would repair says this instance should be restarted")
	}
}

// Not verified counts against readiness, and that is the conservative
// direction rather than an oversight. The case is a report whose every step
// either passed or could not be run: no failure anywhere, and still not ready.
func TestNotVerifiedIsNotReady(t *testing.T) {
	clean := Report{Steps: []Step{
		{Name: "one", Outcome: Passed, Detail: "ran"},
		{Name: "two", Outcome: NotVerified, Detail: "did not run, issue #30"},
	}}
	if clean.Ready() {
		t.Error("a report with no failure and one unrun step says this instance is ready")
	}

	all := Report{Steps: []Step{{Name: "one", Outcome: Passed, Detail: "ran"}}}
	if !all.Ready() {
		t.Error("a report whose every step passed says this instance is not ready")
	}
}

// The zero value of an Outcome is the unknown one. A step somebody adds and
// forgets to answer has to report as unverified rather than as satisfied, and
// this is the assertion that keeps that true if the constants are ever
// reordered.
func TestTheZeroOutcomeIsTheUnknownOne(t *testing.T) {
	var zero Outcome
	if zero != NotVerified {
		t.Fatalf("the zero outcome is %s; a forgotten step would report as satisfied", zero)
	}
	if (Report{Steps: []Step{{Name: "forgotten"}}}).Ready() {
		t.Fatal("a step nobody answered leaves the instance ready")
	}
}

// The written report prints every step including the ones that passed, and its
// last line counts them per outcome. A report holding only problems cannot be
// told from a report of a run that examined three things, which is the same
// reason cmd/invariant prints what it did not run.
func TestTheWrittenReportSaysWhatItExamined(t *testing.T) {
	var out strings.Builder
	report := Run(with(t, "{}"))
	if err := report.Write(&out); err != nil {
		t.Fatal(err)
	}

	text := out.String()
	for _, s := range report.Steps {
		if !strings.Contains(text, s.Name) {
			t.Errorf("%s is in the report and not in what was written", s.Name)
		}
	}
	if !strings.Contains(text, "NOT VERIFIED") {
		t.Error("the written report does not carry the words a reader has to see")
	}
	for _, want := range []string{"5 step(s)", "0 passed", "1 failed", "4 NOT VERIFIED", "live=true", "ready=false"} {
		if !strings.Contains(text, want) {
			t.Errorf("the last line does not carry %q: %s", want, text)
		}
	}
}

// Counts adds up to the number of steps, so a reader of the last line is
// reading about the whole report rather than about part of it.
func TestTheCountsCoverEveryStep(t *testing.T) {
	report := Run(with(t, "{}"))
	total := 0
	for _, n := range report.Counts() {
		total += n
	}
	if total != len(report.Steps) {
		t.Fatalf("the counts add to %d over %d steps", total, len(report.Steps))
	}
}

func step(t *testing.T, r Report, name string) Step {
	t.Helper()
	for _, s := range r.Steps {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("there is no step called %q", name)
	return Step{}
}
