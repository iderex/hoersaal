// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command invariant runs the rules in internal/invariant over the Go files in
// this repository and exits non-zero on any refusal. It answers issue #95.
//
// It is wiring and it decides nothing. Every rule, every exception and every
// reason is in internal/invariant, where the suite keeps a case per rule rather
// than waiting for somebody to write the mistake into the tree.
//
// What it prints before any verdict is what it examined: the rules it ran, the
// rules it did not run and what each of those waits on, and how many files it
// read. A run that enforced three rules of five and a run that enforced five and
// found nothing leave the same exit code, and the difference between them is
// only visible in those lines.
package main

import (
	"fmt"
	"os"

	"github.com/iderex/hoersaal/internal/invariant"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	var ran, waiting []invariant.Rule
	for _, r := range invariant.Rules {
		if r.Enforced() {
			ran = append(ran, r)
			continue
		}
		waiting = append(waiting, r)
	}

	fmt.Printf("rules declared: %d\n", len(invariant.Rules))
	fmt.Printf("rules run: %d\n", len(ran))
	for _, r := range ran {
		fmt.Printf("  %s (%s), over %s\n", r.ID, r.Issue, r.Subject)
	}
	fmt.Printf("rules not run: %d\n", len(waiting))
	for _, r := range waiting {
		fmt.Printf("  %s (%s): %s\n", r.ID, r.Issue, r.Waiting)
	}

	findings, files, err := invariant.CheckTree(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading %s: %v\n", root, err)
		os.Exit(2)
	}
	fmt.Printf("Go files read: %d\n", len(files))

	// A run over no file refuses nothing and exits zero, which reads exactly
	// like a clean tree. It is a failure here rather than a green tick.
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "::error::no Go file was read under %s, so this run judged nothing\n", root)
		os.Exit(2)
	}

	// A run in which every rule is waiting would print a clean verdict over a
	// tree nothing read. There is no such state today and there is no reason to
	// find out about the first one from a green tick.
	if len(ran) == 0 {
		fmt.Fprintf(os.Stderr, "::error::every rule is waiting on something, so this run enforced nothing\n")
		os.Exit(2)
	}

	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "::error file=%s,line=%d::%s: %s\n", f.Path, f.Line, f.Rule, f.Detail)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "%d refusal(s)\n", len(findings))
		os.Exit(1)
	}

	fmt.Printf("every Go file in this tree passes the %d rule(s) that run.\n", len(ran))
}
