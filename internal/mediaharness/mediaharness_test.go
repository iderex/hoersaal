// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package mediaharness

import (
	"errors"
	"sort"
	"strings"
	"testing"
)

// bare is a machine that declares nothing and has nothing on its path, which is
// every machine the fast suite runs on. It is the case this harness exists to
// report honestly rather than the case it fails on.
var bare = Environment{
	Getenv:   func(string) string { return "" },
	LookPath: func(string) (string, error) { return "", errors.New("not on the path") },
}

// equipped declares everything. It is a fixture and not a claim about any real
// machine: a declared endpoint is not a unit that answered, which is what the
// package comment says and what this fixture is bounded by.
var equipped = Environment{
	Getenv:   func(string) string { return "declared" },
	LookPath: func(string) (string, error) { return "", errors.New("not on the path") },
}

func TestABareMachineIsMissingEverythingAndSaysWhat(t *testing.T) {
	results := Probe(bare)
	if len(results) != len(Requirements) {
		t.Fatalf("probed %d of %d requirements", len(results), len(Requirements))
	}
	missing := Missing(results)
	if len(missing) != len(Requirements) {
		t.Fatalf("a machine declaring nothing should be missing every requirement, missing %d of %d", len(missing), len(Requirements))
	}
	for _, r := range results {
		if strings.TrimSpace(r.Requirement.Missing) == "" {
			t.Errorf("%s is missing and the run does not say what would provide it", r.Requirement.ID)
		}
		if strings.TrimSpace(r.Requirement.Because) == "" {
			t.Errorf("%s is missing and the run does not say why it is needed", r.Requirement.ID)
		}
	}
}

func TestADeclaredMachineIsMissingNothing(t *testing.T) {
	missing := Missing(Probe(equipped))
	if len(missing) != 0 {
		t.Fatalf("every requirement is declared and these were still missing: %v", missing)
	}
}

// The browser is the one requirement with a second way of being satisfied, and
// the fallback is the reason the list of executable names exists. Both halves
// are asserted, because a fallback nobody tests is a fallback that has stopped
// working.
func TestTheBrowserIsFoundOnThePathAsWellAsByName(t *testing.T) {
	onlyBrowser := Environment{
		Getenv: func(string) string { return "" },
		LookPath: func(name string) (string, error) {
			if name == browserNames[0] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not on the path")
		},
	}
	missing := Missing(Probe(onlyBrowser))
	for _, id := range missing {
		if id == ReqBrowser {
			t.Fatal("a browser on the path was not found")
		}
	}
	if len(missing) != len(Requirements)-1 {
		t.Fatalf("only the browser was available and %d of %d requirements were reported present", len(Requirements)-len(missing), len(Requirements))
	}
}

// Every property is stopped on a bare machine. A property that came out
// unblocked there would be one this harness is not actually needed for, which is
// a claim in the wrong list rather than a good result.
func TestNoPropertyIsShownOnABareMachine(t *testing.T) {
	blocked := Blocked(Probe(bare))
	if len(blocked) != len(Properties) {
		t.Fatalf("%d of %d properties were reported unblocked on a machine that has nothing", len(Properties)-len(blocked), len(Properties))
	}
	for _, p := range Properties {
		if len(blocked[p.ID]) == 0 {
			t.Errorf("%s is blocked and nothing is named as stopping it", p.ID)
		}
	}
}

// Losing one requirement blocks exactly the properties that named it. This is
// the leg that says the two lists are joined rather than merely both present.
func TestLosingOneRequirementBlocksThePropertiesThatNeedIt(t *testing.T) {
	for _, req := range Requirements {
		lost := req.ID
		env := Environment{
			Getenv: func(name string) string {
				if name == envFor(lost) {
					return ""
				}
				return "declared"
			},
			LookPath: func(string) (string, error) { return "", errors.New("not on the path") },
		}
		blocked := Blocked(Probe(env))

		want := map[string]bool{}
		for _, p := range Properties {
			for _, need := range p.Needs {
				if need == lost {
					want[p.ID] = true
				}
			}
		}
		if len(blocked) != len(want) {
			t.Errorf("without %s, %d properties were blocked and %d name it", lost, len(blocked), len(want))
		}
		for id := range blocked {
			if !want[id] {
				t.Errorf("without %s, %s was blocked and does not name it", lost, id)
			}
		}
	}
}

func TestEveryPropertyNamesAnIssueAndARequirementThatExists(t *testing.T) {
	if len(Properties) == 0 {
		t.Fatal("nothing is claimed to be covered only here, which makes this harness unnecessary")
	}
	if bad := EveryNeedIsARequirement(); len(bad) > 0 {
		t.Errorf("a property names something that is not a requirement: %s", strings.Join(bad, "; "))
	}
	seen := map[string]bool{}
	for _, p := range Properties {
		if seen[p.ID] {
			t.Errorf("%s appears twice", p.ID)
		}
		seen[p.ID] = true
		if !strings.HasPrefix(p.Issue, "#") {
			t.Errorf("%s names no issue, so nobody owes it", p.ID)
		}
		if strings.TrimSpace(p.What) == "" {
			t.Errorf("%s says nothing about what it covers", p.ID)
		}
		if len(p.Needs) == 0 {
			t.Errorf("%s needs nothing, so it is not a property of this harness", p.ID)
		}
	}
}

func TestEveryRequirementIsNeededByAProperty(t *testing.T) {
	needed := map[string]bool{}
	for _, p := range Properties {
		for _, n := range p.Needs {
			needed[n] = true
		}
	}
	var idle []string
	for _, r := range Requirements {
		if !needed[r.ID] {
			idle = append(idle, r.ID)
		}
	}
	sort.Strings(idle)
	if len(idle) > 0 {
		t.Errorf("these are demanded of an operator and no property needs them: %v", idle)
	}
}

// The rule this issue turns on: a machine without the hardware leaves a non-zero
// code and a sentence saying what was missing, rather than a skip. Both the bare
// machine and the fully declared one are asserted, because the two leave
// different codes for different reasons and collapsing them is how a run that
// could not happen starts reading like one that did.
func TestNoRunOnThisMachineIsEverReportedAsASuccess(t *testing.T) {
	bareCode, bareWhy := Verdict(Probe(bare))
	if bareCode != ExitDidNotRun {
		t.Errorf("a machine with none of the hardware left code %d, and the case this harness exists for is %d", bareCode, ExitDidNotRun)
	}
	if !strings.Contains(bareWhy, "did not run") {
		t.Errorf("the sentence for a machine that could not run does not say so: %q", bareWhy)
	}

	fullCode, fullWhy := Verdict(Probe(equipped))
	if fullCode != ExitNothingToDrive {
		t.Errorf("a machine declaring everything left code %d, and there is nothing to drive yet, which is %d", fullCode, ExitNothingToDrive)
	}
	if !strings.Contains(fullWhy, "nothing was run") {
		t.Errorf("the sentence for a machine with nothing to drive does not say so: %q", fullWhy)
	}

	for _, env := range []Environment{bare, equipped} {
		if code, _ := Verdict(Probe(env)); code == ExitRan {
			t.Error("a run was reported as a success, and nothing here has run anything")
		}
	}
}

// envFor is the variable a requirement is declared by. It is here rather than on
// Requirement because it is a fact the suite needs and not one the command
// prints, and putting it in the table would invite a probe that reads it instead
// of the probe the entry actually holds.
func envFor(id string) string {
	switch id {
	case ReqForwardingUnit:
		return EnvForwardingUnit
	case ReqSecondForwardingUnit:
		return EnvSecondForwardingUnit
	case ReqBrowser:
		return EnvBrowser
	case ReqCaptureDevices:
		return EnvCaptureDevices
	case ReqShapedNetwork:
		return EnvShapedNetwork
	}
	return ""
}

// The switch above is a second list beside Requirements, and a second list is
// what drifts. This is what refuses one that has fallen behind.
func TestEveryRequirementHasAVariable(t *testing.T) {
	for _, r := range Requirements {
		if envFor(r.ID) == "" {
			t.Errorf("%s has no variable in the suite's own map, so the leg that removes it one at a time silently stops removing anything", r.ID)
		}
	}
}
