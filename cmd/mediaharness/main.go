// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command mediaharness is the media integration harness, and it is named for
// what it is. It answers issue #51.
//
// It is not part of the check that runs on every pull request. Nothing in
// .github/workflows invokes it, it needs hardware and a network the project
// controls, and its results are recorded rather than gating. The name is the
// point: a harness called the integration tests, run rarely and quietly skipped
// when the hardware is unavailable, becomes a suite everybody believes is
// running.
//
// So this refuses to skip. On a machine that does not have what it needs it
// prints what was missing, prints what each missing thing would take, prints
// which properties are therefore not shown and who owes them, and exits
// non-zero. A run that could not happen and a run that happened and found
// nothing leave different exit codes and different output, which is the whole
// of what this command is for today.
//
// Everything it decides is in internal/mediaharness. This is wiring: it reads
// the machine, prints, and chooses the exit code.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/iderex/hoersaal/internal/mediaharness"
)

func main() {
	results := mediaharness.Probe(mediaharness.Environment{})

	fmt.Printf("requirements: %d\n", len(results))
	for _, r := range results {
		state := "missing"
		if r.Present {
			state = "present"
		}
		fmt.Printf("  %-24s %-8s %s\n", r.Requirement.ID, state, r.Requirement.What)
	}

	fmt.Printf("properties covered only by this harness: %d\n", len(mediaharness.Properties))
	for _, p := range mediaharness.Properties {
		fmt.Printf("  %-40s %s  %s\n", p.ID, p.Issue, p.What)
	}

	code, reason := mediaharness.Verdict(results)

	missing := mediaharness.Missing(results)
	if len(missing) == 0 {
		// The declarations are all present. What runs after this point is the
		// harness itself, and it does not exist yet: there is no adapter to
		// drive a unit with, and no client to put in a browser. This is refused
		// rather than left as a green exit, because a command that printed a
		// clean requirement list and stopped would be read as a run.
		fmt.Println("every requirement is declared on this machine.")
		fmt.Fprintln(os.Stderr, reason)
		os.Exit(code)
	}

	fmt.Printf("missing: %d\n", len(missing))
	for _, r := range results {
		if r.Present {
			continue
		}
		fmt.Printf("  %s\n", r.Requirement.ID)
		fmt.Printf("      needed because %s\n", r.Requirement.Because)
		fmt.Printf("      to provide it: %s\n", r.Requirement.Missing)
	}

	blocked := mediaharness.Blocked(results)
	ids := make([]string, 0, len(blocked))
	for id := range blocked {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("properties not shown by this run: %d of %d\n", len(ids), len(mediaharness.Properties))
	for _, id := range ids {
		fmt.Printf("  %s (owed by %s), stopped by %v\n", id, issueFor(id), blocked[id])
	}

	fmt.Fprintln(os.Stderr, reason)
	os.Exit(code)
}

func issueFor(id string) string {
	for _, p := range mediaharness.Properties {
		if p.ID == id {
			return p.Issue
		}
	}
	return "no issue"
}
