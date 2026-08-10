// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command coverage reads a Go coverage profile and holds the surfaces in
// internal/coverage to the bar. It answers issue #92.
//
// It is wiring and it decides nothing. Which packages are under the bar, what
// the bar is and where the number came from are in internal/coverage, where the
// suite exercises each refusal against a fixture rather than against whatever
// the tree happens to reach today.
//
// The profile is a file this command is given, and producing it is the caller's
// half: `go test -coverprofile`. That split is on purpose. A command that ran
// the suite itself would be a second way to run it, with its own flags, and the
// two would drift; and it would have to launch a subprocess, which this tree
// keeps to the one place that already needs one.
package main

import (
	"fmt"
	"os"

	"github.com/iderex/hoersaal/internal/coverage"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: coverage <profile>\n")
		os.Exit(2)
	}

	// #nosec G304 -- the path is this command's only argument, given by
	// whoever started it, and the file it names is a coverage profile the same
	// caller has just written. There is no untrusted path here to constrain.
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening %s: %v\n", os.Args[1], err)
		os.Exit(2)
	}
	defer func() { _ = f.Close() }()

	packages, err := coverage.Parse(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "::error::%v\n", err)
		os.Exit(2)
	}

	report := coverage.Judge(packages)
	if err := report.Write(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "writing the report: %v\n", err)
		os.Exit(2)
	}

	for _, r := range report.Refusals {
		fmt.Fprintf(os.Stderr, "::error::%s\n", r)
	}
	if len(report.Refusals) > 0 {
		os.Exit(1)
	}
}
