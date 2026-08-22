// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package selfcheck runs the things an operator gets wrong, before the first
// lecture rather than during it, and answers issue #84.
//
// # The one rule this package exists for
//
// A step that could not be run reports as not verified, in those words, and
// never as passed. A self-check that reports success for a step it could not
// run is worse than no self-check, because it converts an absence of knowledge
// into a statement, and the operator acts on the statement.
//
// That is the same property the automated run over this repository holds
// itself to, which CONTRIBUTING.md states as a run saying what it examined: a
// leg that covered nothing says so rather than passing. This package is that
// sentence turned outward, at the operator instead of at a contributor.
//
// It has three outcomes and not two, and the third is the point. Passed is a
// step that ran and was satisfied. Failed is a step that ran and was not, and
// it names what to change. NotVerified is a step that did not run, and it names
// what is missing instead of what is wrong, because those send an operator to
// two different places.
//
// # What it can check today, and what it cannot
//
// The configuration is the one thing that exists to be checked, and it exists
// because issue #82 landed. Everything else this issue names is a resource this
// deployment does not have yet, so every one of those steps reports not
// verified with the issue that will produce it:
//
// The store, which docs/decisions/control-plane-state.md chooses and issue #30
// wires. Nothing in this tree opens one.
//
// The units listed, which issue #52 decides the reachability of and issue #56's
// pool records. Nothing in this tree contacts one.
//
// The key that signs room credentials. internal/roomcred can sign and verify,
// and there is no key: the settings list in
// docs/decisions/what-an-operator-may-set.md carries none, and issue #86 is
// where the sources of the secrets are decided.
//
// The listener, which is the address, the port and the certificate an operator
// may set. Nothing in this tree binds a socket.
//
// So four of the five steps report not verified on any deployment today, and a
// run that said otherwise would be the failure at the top of this comment. The
// count is not written into the report as a promise that it will stay four; the
// steps carry their own state and the report counts what it holds.
//
// # Liveness and readiness
//
// They are separate because they mean different things to whatever is watching,
// and the difference is a stated failure rather than a preference. A control
// plane that cannot reach its store is not ready, because it cannot admit
// anybody, and it is alive, because restarting it does not fix the store.
// Collapsing the two turns a store outage into a restart loop.
//
// Both are decisions here and neither is an endpoint. Nothing in this tree
// serves HTTP, so there is no path for a watcher to fetch and this package does
// not pretend there is one. What it holds is the answer; the transport for it
// arrives with the thing that listens.
//
// # What this package does not do
//
// It contacts nothing. Every step is a function of what it was handed, so the
// suite runs it with no network, no store and no device, and so a step that
// would have to contact something is exactly the step that reports not
// verified. The day one of those becomes real, the step takes what it needs as
// an argument like everything else in this tree does, and it stops reporting
// not verified because it ran rather than because somebody edited a table.
package selfcheck

import (
	"fmt"
	"io"
	"strings"

	"github.com/iderex/hoersaal/internal/config"
)

// An Outcome is what became of one step.
type Outcome int

const (
	// NotVerified is a step that did not run. It is the zero value on purpose:
	// a step somebody adds and forgets to answer reports as unknown rather
	// than as satisfied, which is the direction this package errs in.
	NotVerified Outcome = iota

	// Passed is a step that ran and was satisfied.
	Passed

	// Failed is a step that ran and was not satisfied.
	Failed
)

func (o Outcome) String() string {
	switch o {
	case Passed:
		return "passed"
	case Failed:
		return "failed"
	default:
		return "NOT VERIFIED"
	}
}

// A Step is one thing an operator gets wrong, and what became of it.
type Step struct {
	// Name is what was checked, in the operator's vocabulary rather than in
	// this repository's.
	Name string

	// Outcome is what became of it.
	Outcome Outcome

	// Detail says what to change where the step failed, what is missing where
	// it could not run, and what was found where it passed. It is never empty:
	// a step with no detail is one an operator cannot act on.
	Detail string
}

func (s Step) String() string { return fmt.Sprintf("%-12s %s: %s", s.Outcome, s.Name, s.Detail) }

// A Report is one run of the whole check.
type Report struct {
	Steps []Step
}

// Run answers what this deployment can and cannot be shown to do. It takes the
// settings rather than reading them, because loading is issue #82's and doing
// it twice is how two answers about one file come to exist.
func Run(settings config.Settings) Report {
	return Report{Steps: []Step{
		checkConfiguration(settings),
		checkListener(settings),
		checkStore(settings),
		checkUnits(settings),
		checkSigningKey(),
	}}
}

// checkConfiguration is the one step that runs. A Settings only exists by
// coming through config.Load, which judges every field, so what is left to say
// is what the operator is running with rather than whether it is valid.
//
// The certificate is the exception and it is a failure rather than an absence.
// An operator who has set an address and a port and no certificate has a
// deployment that cannot serve HTTPS, and that is a thing to change rather than
// a thing this software has not got round to. Issue #82 records why there is no
// default for it.
func checkConfiguration(s config.Settings) Step {
	if s.ListenCertificate == "" {
		return Step{
			Name:    "the configuration",
			Outcome: Failed,
			Detail: fmt.Sprintf(
				"it loaded, and %s names no certificate, so this deployment can serve no HTTPS listener; set it to a certificate this machine holds (%s)",
				config.KeyListenCertificate, config.TheList),
		}
	}
	return Step{
		Name:    "the configuration",
		Outcome: Passed,
		Detail: fmt.Sprintf("it loaded and every value was judged; the pool floor is %d and the ceiling is %d",
			s.PoolMinimum, s.PoolMaximum),
	}
}

// checkListener is the port an operator most often has wrong, and nothing here
// binds one. This is the step most likely to be mistaken for a running check,
// so it says which issue owes the socket.
func checkListener(s config.Settings) Step {
	where := s.ListenAddress
	if where == "" {
		where = "every interface"
	}
	return Step{
		Name:    "the listener",
		Outcome: NotVerified,
		Detail: fmt.Sprintf(
			"%s port %d was not bound, because nothing in this build listens on a socket; the signalling transport that will is issue #35",
			where, s.ListenPort),
	}
}

// checkStore names the file the operator gave rather than reporting on it. The
// path being set is not the store answering, and reporting the first as the
// second is exactly the collapse this package exists against.
func checkStore(s config.Settings) Step {
	return Step{
		Name:    "the store",
		Outcome: NotVerified,
		Detail: fmt.Sprintf(
			"%q was not opened, because nothing in this build opens a store; docs/decisions/control-plane-state.md chooses one and issue #30 wires it",
			s.StorePath),
	}
}

// checkUnits counts what the operator listed and contacts none of it.
func checkUnits(s config.Settings) Step {
	return Step{
		Name:    "the units",
		Outcome: NotVerified,
		Detail: fmt.Sprintf(
			"%d machine(s) are listed and none was contacted, because nothing in this build reaches a unit; what reachable means is issue #52 and the pool that records it is issue #56",
			len(s.ProvisionerMachines)),
	}
}

// checkSigningKey is not verified for a reason that is not the same as the
// others: internal/roomcred can sign and verify today, and there is no key for
// it to do so with. A key is not on the settings list, so there is nothing to
// hand this step, and inventing one here would check this package against
// itself.
func checkSigningKey() Step {
	return Step{
		Name:    "the credential key",
		Outcome: NotVerified,
		Detail:  "no credential was signed or verified, because no key reaches this build; the settings list carries none and where the secrets come from is issue #86",
	}
}

// Live says whether this instance should be left running. It is false only for
// a state a restart repairs, and there is no such state in this build.
//
// The answer is a constant today and this is not a placeholder for one that
// will be computed later without anybody noticing: what would make it false is
// a resource inside this process that a restart rebuilds, and this process
// holds none.
func (r Report) Live() bool { return true }

// Ready says whether this instance should be sent participants. It is false
// while any step is unsatisfied, and a step that could not be run counts as
// unsatisfied.
//
// That is the stated difference issue #84's first condition asks for, and the
// case it is written from is the store: a control plane that cannot reach its
// store is not ready, because it cannot admit anybody, and it is alive, because
// restarting it does not fix the store.
//
// Not verified counting against readiness is the conservative direction and it
// is deliberate. A deployment that cannot show it can do its job is one that
// should not be sent three hundred people, and today that means this build is
// never ready, which is true and is the point.
func (r Report) Ready() bool {
	for _, s := range r.Steps {
		if s.Outcome != Passed {
			return false
		}
	}
	return true
}

// Counts is how many steps ended in each outcome, so a reader of one line knows
// whether a clean run examined anything.
func (r Report) Counts() map[Outcome]int {
	out := map[Outcome]int{}
	for _, s := range r.Steps {
		out[s.Outcome]++
	}
	return out
}

// Write prints the report. Every step is printed, including the ones that
// passed, because a report holding only problems cannot be told from a report
// of a run that examined three things.
//
// The last line is the count per outcome rather than a verdict, for the same
// reason: "no failures" over five steps of which four did not run is a sentence
// that misleads on its own.
func (r Report) Write(w io.Writer) error {
	for _, s := range r.Steps {
		if _, err := fmt.Fprintf(w, "  %s\n", s); err != nil {
			return err
		}
	}

	counts := r.Counts()
	parts := make([]string, 0, 3)
	for _, o := range []Outcome{Passed, Failed, NotVerified} {
		parts = append(parts, fmt.Sprintf("%d %s", counts[o], o))
	}

	_, err := fmt.Fprintf(w, "%d step(s): %s. live=%t ready=%t\n",
		len(r.Steps), strings.Join(parts, ", "), r.Live(), r.Ready())
	return err
}
