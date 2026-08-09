// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mediaharness declares what the media integration harness needs before
// it can run and what it is the only place able to prove. It answers issue #51.
//
// Some properties cannot be shown without real media on a real network, and
// pretending otherwise is how a project ends up with a green suite and a service
// that does not work. The failure this package is written against is not the
// missing hardware, which is a fact of the machine. It is a harness that is
// skipped quietly: a suite everybody believes is running, whose absence looks
// exactly like a pass.
//
// So the two lists below are data rather than prose. Requirements is what the
// harness needs, each with a probe that answers on this machine. Properties is
// what only this harness can show, each naming the issue that owes it and the
// requirements it needs. The command prints both and exits non-zero when
// anything is missing, and it never reports a skip as a pass.
//
// What a probe answers, exactly, because the difference matters and is easy to
// lose. It answers whether this machine declares the thing, not whether the
// thing works. An endpoint named by an environment variable is a declaration; it
// is not a unit that answered. Nothing here dials, because a connection out of
// this process is made in internal/boundary and nowhere else, and that rule is
// worth more than a sharper probe. So a green probe set means the harness may
// start, and the run itself is what finds out whether the declarations were
// true.
//
// The names of the environment variables are here rather than in the command,
// because they are part of what an operator of this harness is told and the
// command only prints what this package decides.
package mediaharness

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// The requirement ids. They are printed by the command and they are what
// somebody searches for when a run tells them something is missing, so they are
// stable strings rather than positions in a list.
const (
	ReqForwardingUnit       = "forwarding-unit"
	ReqSecondForwardingUnit = "second-forwarding-unit"
	ReqBrowser              = "browser"
	ReqCaptureDevices       = "capture-devices"
	ReqShapedNetwork        = "shaped-network"
)

// The environment variables an operator of this harness sets. They are named
// rather than discovered, so a run that found nothing can tell somebody what to
// set instead of telling them to read the source.
const (
	EnvForwardingUnit       = "HOERSAAL_HARNESS_UNIT"
	EnvSecondForwardingUnit = "HOERSAAL_HARNESS_SECOND_UNIT"
	EnvBrowser              = "HOERSAAL_HARNESS_BROWSER"
	EnvCaptureDevices       = "HOERSAAL_HARNESS_DEVICES"
	EnvShapedNetwork        = "HOERSAAL_HARNESS_NETWORK"
)

// A Requirement is one thing the harness cannot run without. What is the thing,
// Because is why the harness needs it, and Missing is the sentence somebody gets
// when it is not there, which says what to do rather than what happened.
type Requirement struct {
	ID      string
	What    string
	Because string
	Missing string

	probe func(Environment) bool
}

// An Environment is the machine, as far as this package reads it. It is an
// argument rather than a package-level read so that the suite can hand over a
// machine that has everything and a machine that has nothing, and neither run
// depends on what the machine running the suite happens to hold.
type Environment struct {
	// Getenv reads a variable. Nil reads the real environment.
	Getenv func(string) string
	// LookPath answers whether an executable is on the path. Nil uses the real
	// one.
	LookPath func(string) (string, error)
}

func (e Environment) getenv(name string) string {
	if e.Getenv == nil {
		return os.Getenv(name)
	}
	return e.Getenv(name)
}

func (e Environment) lookPath(name string) bool {
	look := e.LookPath
	if look == nil {
		look = exec.LookPath
	}
	_, err := look(name)
	return err == nil
}

// declared says whether a variable names something. It is the whole of what most
// probes here can honestly ask.
func declared(name string) func(Environment) bool {
	return func(e Environment) bool { return strings.TrimSpace(e.getenv(name)) != "" }
}

// Requirements is the whole list. Adding one is adding an entry here; the
// command holds no second copy and neither does any document.
var Requirements = []Requirement{
	{
		ID:      ReqForwardingUnit,
		What:    "a running forwarding unit, with its control interface named by " + EnvForwardingUnit,
		Because: "everything here that is not the client needs a unit that actually forwards packets, which is the thing the fake in internal/mediafake deliberately is not",
		Missing: "start a forwarding unit and set " + EnvForwardingUnit + " to its control interface",
		probe:   declared(EnvForwardingUnit),
	},
	{
		ID:      ReqSecondForwardingUnit,
		What:    "a second forwarding unit, named by " + EnvSecondForwardingUnit,
		Because: "the cascade is a property of two units and one conference, and a single unit cannot show it however it is driven",
		Missing: "start a second forwarding unit and set " + EnvSecondForwardingUnit + " to its control interface",
		probe:   declared(EnvSecondForwardingUnit),
	},
	{
		ID:      ReqBrowser,
		What:    "a real browser, named by " + EnvBrowser + " or on the path",
		Because: "a browser's own media stack decides what a person actually experiences, and no headless substitute answers for it",
		Missing: "install a browser and set " + EnvBrowser + " to it, or put it on the path",
		probe: func(e Environment) bool {
			if declared(EnvBrowser)(e) {
				return true
			}
			for _, name := range browserNames {
				if e.lookPath(name) {
					return true
				}
			}
			return false
		},
	},
	{
		ID:      ReqCaptureDevices,
		What:    "a microphone and a camera, or the loopback devices standing in for them, named by " + EnvCaptureDevices,
		Because: "a publisher with no capture device publishes nothing, and a run with no publisher measures an empty room",
		Missing: "attach or configure capture devices and set " + EnvCaptureDevices + " to name them",
		probe:   declared(EnvCaptureDevices),
	},
	{
		ID:      ReqShapedNetwork,
		What:    "a network path the project controls, on which loss and delay can be applied, named by " + EnvShapedNetwork,
		Because: "real loss and real jitter are what separate this harness from the fast suite, and a path nobody can degrade proves nothing about degradation",
		Missing: "provide a path this project controls and set " + EnvShapedNetwork + " to it",
		probe:   declared(EnvShapedNetwork),
	},
}

// browserNames are the executables a browser is ordinarily called. The list is a
// convenience for somebody who already has one installed; EnvBrowser is the
// answer that does not depend on this list being current.
var browserNames = []string{"chromium", "chromium-browser", "google-chrome", "firefox"}

// A Property is something this harness is the only place able to show. Issue is
// the issue that owes it, Needs are the requirement ids it cannot be shown
// without, and What is the sentence a reader of the board gets.
type Property struct {
	ID    string
	Issue string
	What  string
	Needs []string
}

// Properties is what is covered only here. It is the list the readme and the
// board state, derived from this file rather than retyped, so the three cannot
// drift apart.
var Properties = []Property{
	{
		ID:    "real-codec-behaviour",
		Issue: "#43",
		What:  "the adapter driving a real unit, where the unit's own answers replace the ones the fake was written to give",
		Needs: []string{ReqForwardingUnit},
	},
	{
		ID:    "layer-distribution-under-mixed-capacity",
		Issue: "#45",
		What:  "which simulcast layer each subscriber actually receives in a room whose subscribers can take different amounts",
		Needs: []string{ReqForwardingUnit, ReqShapedNetwork, ReqCaptureDevices},
	},
	{
		ID:    "first-syllable-after-a-speaker-switch",
		Issue: "#47",
		What:  "how much of the first word survives the moment the forwarded speaker set changes",
		Needs: []string{ReqForwardingUnit, ReqCaptureDevices},
	},
	{
		ID:    "degradation-order-on-a-constrained-path",
		Issue: "#49",
		What:  "the order in which a subscriber's streams are actually reduced when their own path is constrained",
		Needs: []string{ReqForwardingUnit, ReqShapedNetwork},
	},
	{
		ID:    "capacity-signal-against-measured-quality",
		Issue: "#55",
		What:  "whether the capacity signal moves before quality does, which is a claim about two curves and not about one number",
		Needs: []string{ReqForwardingUnit, ReqCaptureDevices},
	},
	{
		ID:    "cross-unit-cascade",
		Issue: "#59",
		What:  "one conference carried by two units, what crosses the link between them, and what the extra hop costs",
		Needs: []string{ReqForwardingUnit, ReqSecondForwardingUnit, ReqShapedNetwork},
	},
	{
		ID:    "browser-client-behaviour",
		Issue: "#75",
		What:  "the parts of the client that a browser decides, which is everything below the layer that runs without one",
		Needs: []string{ReqBrowser, ReqForwardingUnit},
	},
	{
		ID:    "join-storm-timing-on-real-paths",
		Issue: "#71",
		What:  "the time from join to being in the room at the slow end, where the delay includes a real handshake",
		Needs: []string{ReqForwardingUnit, ReqShapedNetwork},
	},
	{
		ID:    "behaviour-past-the-ceiling",
		Issue: "#73",
		What:  "what happens to people already in the room when a ceiling is exceeded rather than approached",
		Needs: []string{ReqForwardingUnit, ReqShapedNetwork},
	},
	{
		ID:    "client-budget-lines-on-stated-hardware",
		Issue: "#81",
		What:  "the processor, the memory and the delay a client adds, on hardware described precisely enough to be bought again",
		Needs: []string{ReqBrowser, ReqCaptureDevices},
	},
}

// A Result is one requirement and what the probe found.
type Result struct {
	Requirement Requirement
	Present     bool
}

// Probe answers every requirement against env. The order is the order of
// Requirements, so a reader comparing two runs is comparing two lists in the
// same order.
func Probe(env Environment) []Result {
	out := make([]Result, 0, len(Requirements))
	for _, r := range Requirements {
		out = append(out, Result{Requirement: r, Present: r.probe(env)})
	}
	return out
}

// Missing is the requirement ids that were not found, in order.
func Missing(results []Result) []string {
	var out []string
	for _, r := range results {
		if !r.Present {
			out = append(out, r.Requirement.ID)
		}
	}
	return out
}

// Blocked is the properties that cannot be shown because something they need is
// missing, each with the requirement ids that stopped it. A property with
// nothing missing is not in the map.
func Blocked(results []Result) map[string][]string {
	absent := map[string]bool{}
	for _, id := range Missing(results) {
		absent[id] = true
	}
	out := map[string][]string{}
	for _, p := range Properties {
		var stoppers []string
		for _, need := range p.Needs {
			if absent[need] {
				stoppers = append(stoppers, need)
			}
		}
		if len(stoppers) > 0 {
			sort.Strings(stoppers)
			out[p.ID] = stoppers
		}
	}
	return out
}

// The exit codes. They are here rather than in the command because the rule
// this issue turns on is about the code a run leaves, and a rule that lived in
// main would be a rule no test could delete and watch fail.
const (
	// ExitRan is a harness that ran. Nothing returns it today and the constant
	// is here so that the day something does, the case is already named.
	ExitRan = 0
	// ExitDidNotRun is the case this command exists for: something it needs is
	// not on this machine. Non-zero rather than a skip, because a skipped
	// harness and a passing one look the same from outside.
	ExitDidNotRun = 1
	// ExitListIsBroken is a property naming a requirement nobody declared, which
	// makes the property list a list of nothing while reading like a full one.
	ExitListIsBroken = 2
	// ExitNothingToDrive is a machine that has everything and a harness with
	// nothing to drive, because the adapter is #43 and the client is #75. It is
	// separate from ExitDidNotRun so that an operator who provided the hardware
	// is not told they are missing it.
	ExitNothingToDrive = 3
)

// Verdict is the exit code one run leaves and the sentence that goes with it.
// There is no path to ExitRan today and that is the honest state rather than an
// oversight: nothing here drives anything yet.
func Verdict(results []Result) (int, string) {
	if bad := EveryNeedIsARequirement(); len(bad) > 0 {
		return ExitListIsBroken, "a property names something that is not a requirement, so the list of what is covered only here covers nothing: " + strings.Join(bad, "; ")
	}
	if len(Missing(results)) > 0 {
		return ExitDidNotRun, "this harness did not run. What is above is what running it would have required, and nothing here is evidence about any property named in it."
	}
	return ExitNothingToDrive, "nothing was run: the harness has nothing to drive yet, because the adapter is #43 and the client is #75"
}

// EveryNeedIsARequirement reports a property naming a requirement that does not
// exist, which is how a list stops being a list of anything. It returns the
// offending pairs rather than a boolean, because the caller has to say which.
func EveryNeedIsARequirement() []string {
	known := map[string]bool{}
	for _, r := range Requirements {
		known[r.ID] = true
	}
	var bad []string
	for _, p := range Properties {
		for _, need := range p.Needs {
			if !known[need] {
				bad = append(bad, fmt.Sprintf("%s needs %q, which is not a requirement", p.ID, need))
			}
		}
	}
	return bad
}
