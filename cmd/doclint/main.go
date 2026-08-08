// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command doclint judges the documents in this repository and exits non-zero on
// any refusal. It answers issue #97.
//
// It is wiring and it decides nothing. Walking the tree and printing what the
// run examined is here; every rule is in internal/doclint, where the suite
// exercises each one against a fixture rather than waiting for somebody to write
// the mistake into a document.
//
// What it prints before any verdict is what it read: how many documents, how
// many references it resolved, and how many it did not follow. A run that
// covered nothing and a run that covered everything and found nothing leave the
// same exit code, and the difference between them is only visible in that line.
package main

import (
	"fmt"
	"os"

	"github.com/iderex/hoersaal/internal/doclint"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	findings, documents, references, err := doclint.CheckTree(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading %s: %v\n", root, err)
		os.Exit(2)
	}

	external := doclint.CountExternal(references)
	fmt.Printf("documents read: %d\n", len(documents))
	for _, d := range documents {
		fmt.Printf("  %s\n", d)
	}
	fmt.Printf("references found: %d\n", len(references))
	fmt.Printf("  resolved against this repository: %d\n", len(references)-external)
	fmt.Printf("  not followed, because they leave it: %d\n", external)

	// A run over no document refuses nothing and exits zero, which reads
	// exactly like a clean tree. It is a failure here rather than a green tick.
	if len(documents) == 0 {
		fmt.Fprintf(os.Stderr, "::error::no document was read under %s, so this run judged nothing\n", root)
		os.Exit(2)
	}

	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "::error file=%s,line=%d::%s: %s\n", f.Path, f.Line, f.Rule, f.Detail)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "%d refusal(s)\n", len(findings))
		os.Exit(1)
	}

	fmt.Println("every document is formatted as the rules ask and every reference into this repository resolves.")
}
