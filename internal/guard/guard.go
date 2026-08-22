// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package guard reads the tree and refuses the things issue #27 says must not
// appear in it: a direct read of the machine's clock, a source of randomness
// made outside the one place that makes them, and a sleep anywhere at all.
//
// It works on the syntax tree rather than on the text, so a mention in a comment
// or in a string is not a finding and a call written across two lines is.
package guard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// clockReads are the calls into the time package that read or wait on the
// machine's clock. Everything else in that package is arithmetic on durations
// and instants, which anybody may use.
var clockReads = map[string]bool{
	"Now":       true,
	"Since":     true,
	"Until":     true,
	"After":     true,
	"AfterFunc": true,
	"Tick":      true,
	"NewTicker": true,
	"NewTimer":  true,
	"Sleep":     true,
}

// randomSources are the packages that make randomness.
var randomSources = map[string]bool{
	"math/rand":    true,
	"math/rand/v2": true,
	"crypto/rand":  true,
}

// clockPlace and randomPlace are the files allowed to do what the rules above
// refuse. They are paths from the repository root with forward slashes, and the
// list is short on purpose: a second entry is a decision somebody argues for,
// not a line somebody adds.
const (
	clockPlace  = "internal/clock/clock.go"
	randomPlace = "internal/random/random.go"
)

// A Finding is one refusal, with enough in it to fix the code without opening
// the checker.
type Finding struct {
	Path   string // from the repository root, forward slashes
	Line   int
	Detail string
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Detail) }

// CheckFile refuses what src does at path. path is from the repository root,
// with forward slashes, and decides which exceptions apply.
func CheckFile(path string, src []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	isTest := strings.HasSuffix(path, "_test.go")
	var findings []Finding
	at := func(pos token.Pos) int { return fset.Position(pos).Line }

	// The import of a randomness package is the finding, not its use, because a
	// file that imports one has a source in it however it is spelled later.
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || !randomSources[p] {
			continue
		}
		if path == randomPlace {
			continue
		}
		findings = append(findings, Finding{
			Path: path, Line: at(imp.Pos()),
			Detail: fmt.Sprintf("imports %s; a source of randomness comes from internal/random, seeded, so a failing run can be reproduced", p),
		})
	}

	// The time package is imported by anything that holds a duration, so here
	// the call is the finding.
	timeNames := map[string]bool{}
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != "time" {
			continue
		}
		name := "time"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		timeNames[name] = true
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || !timeNames[pkg.Name] || !clockReads[sel.Sel.Name] {
			return true
		}

		// Sleep is refused everywhere, including in tests and including in the
		// file that is otherwise allowed to read the clock. A suite that sleeps
		// is the thing issue #27 is about.
		if sel.Sel.Name == "Sleep" {
			findings = append(findings, Finding{
				Path: path, Line: at(call.Pos()),
				Detail: "calls time.Sleep; a test waits by advancing a clock.Test and a service waits on a Clock",
			})
			return true
		}
		if path == clockPlace {
			return true
		}
		if isTest {
			// A test that reads the machine's clock is measuring the machine,
			// not the code, and it is the same defect as one that sleeps.
			findings = append(findings, Finding{
				Path: path, Line: at(call.Pos()),
				Detail: fmt.Sprintf("calls time.%s; a test is given a clock.Test and controls it", sel.Sel.Name),
			})
			return true
		}
		findings = append(findings, Finding{
			Path: path, Line: at(call.Pos()),
			Detail: fmt.Sprintf("calls time.%s; production code takes a clock.Clock and is handed one", sel.Sel.Name),
		})
		return true
	})

	return findings, nil
}

// CheckTree refuses everything under root. Directories whose name begins with a
// dot are not read, and neither is anything that is not a .go file.
func CheckTree(root string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		// #nosec G304 -- p is a path this walk produced from root, so what is
		// opened is the checkout the command was pointed at. Nothing reaches
		// this from a request or from a caller choosing a file.
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := CheckFile(filepath.ToSlash(rel), src)
		if err != nil {
			return err
		}
		findings = append(findings, f...)
		return nil
	})
	return findings, err
}
