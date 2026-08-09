// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package arch refuses the imports docs/repository-layout.md forbids, and the
// top-level directory that document does not name.
//
// It answers issue #98. The rules are not argued here. Each one carries the
// section of docs/repository-layout.md that argued it, so a rule that turns out
// to be wrong is changed by changing the argument rather than by deleting an
// assertion, and a reader who disagrees with a refusal is sent to the place the
// disagreement belongs.
//
// It reads import paths out of the syntax tree, one file at a time, and the
// caller decides which file. That is what lets the suite keep a fixture per rule
// rather than waiting for somebody to write the mistake into the tree.
//
// The rules cover _test.go files as well, and that is deliberate rather than
// incidental. The realistic way the placer acquires a clock is a test that wants
// to control one, not a placer that reads the machine's; a rule that stopped at
// the production files would pass the case it exists for.
//
// What this package does not do. It reads imports, so a dependency taken any
// other way is outside it: a linkname, a build-tagged file this parse skips, or
// a package reached through a third one it is allowed to import. It says
// nothing about acyclicity, because the Go toolchain refuses an import cycle at
// compile time and a test asserting it would be a test of the compiler; what is
// left of that rule is direction between layers that form no cycle, and the two
// directions docs/repository-layout.md fixes are below. And it judges this
// repository's own arrangement, so it is a guard on the layout and not on the
// design the layout came from.
package arch

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ModulePath is this module, which every import of a package in this repository
// begins with. It is written here rather than read from go.mod because the rules
// below are about named packages and a rule table that resolved its own subject
// at run time would pass over a module nobody meant. TestTheModulePathIsTheOne
// InGoMod is what stops the two drifting apart.
const ModulePath = "github.com/iderex/hoersaal"

// TopLevel is the whole top level, from the "## The top level" section of
// docs/repository-layout.md, which ends with the sentence this list exists to
// enforce: a fifth directory is a change to that document before it is a change
// to the tree.
var TopLevel = []string{".github", "cmd", "docs", "internal"}

// A Finding is one refusal, with enough in it to fix the code without opening
// this file.
type Finding struct {
	Path   string // from the repository root, forward slashes; empty for a finding about the tree itself
	Line   int
	Detail string
}

func (f Finding) String() string {
	if f.Path == "" {
		return f.Detail
	}
	return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Detail)
}

// The packages the rules below are about, as directories from the repository
// root. A rule names one of these rather than matching a pattern, so adding a
// package does not silently acquire somebody else's rule.
const (
	dirDomain    = "internal/domain"
	dirMediaFake = "internal/mediafake"
	dirMediaPort = "internal/mediaport"
	dirMediaUnit = "internal/mediaunit"
	dirPlacement = "internal/placement"
	dirWiring    = "cmd/hoersaal"
)

// CheckFile refuses the imports src takes at path. path is from the repository
// root with forward slashes, and it is what decides which rules apply, because
// every rule here is about a named package rather than about a shape.
func CheckFile(path string, src []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	dir := packageDir(path)
	var findings []Finding
	for _, imp := range file.Imports {
		imported, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: unreadable import %s", path, imp.Path.Value)
		}
		line := fset.Position(imp.Pos()).Line
		for _, detail := range refusals(dir, imported) {
			findings = append(findings, Finding{Path: path, Line: line, Detail: detail})
		}
	}
	return findings, nil
}

// refusals is the whole rule table. Each entry answers for one importing
// directory and one imported path, and returns the sentence a reader gets,
// which names the document section that argued the rule.
func refusals(dir, imported string) []string {
	var out []string

	// The first boundary, docs/repository-layout.md, "The first boundary: the
	// control plane may not depend on the media plane". The adapter is the only
	// place allowed to name the chosen forwarding unit, and cmd/hoersaal is the
	// one caller that reaches it, so that removing the adapter leaves every
	// other package compiling.
	if inModule(imported) == dirMediaUnit && dir != dirMediaUnit && dir != dirWiring {
		out = append(out, fmt.Sprintf(
			"imports %s; the adapter is reached by %s and by nothing else, so that removing it leaves every other package compiling (docs/repository-layout.md, the first boundary)",
			imported, dirWiring))
	}

	// The same boundary, the direction that is not about the adapter. The port
	// is above the boundary and the fake is below it, so a port that reached for
	// the fake would put the boundary inside the thing it divides.
	if dir == dirMediaPort && inModule(imported) == dirMediaFake {
		out = append(out, fmt.Sprintf(
			"imports %s; the port sits above the boundary and the fake satisfies it from below, so the dependency runs the other way (docs/repository-layout.md, the first boundary)",
			imported))
	}

	// The second boundary, docs/repository-layout.md, "The second boundary: the
	// placer may not depend on the transport", and docs/decisions/placement-seam.md,
	// which fixes what the placer may read and argues for a small set rather than
	// an exclusion list. So the module half is an allow-list: the model, and
	// nothing else in this repository. That is what refuses internal/clock, which
	// is the near miss, because the placer takes its clock as an argument.
	if dir == dirPlacement {
		if in := inModule(imported); in != "" && in != dirDomain {
			out = append(out, fmt.Sprintf(
				"imports %s; the placer reads the three records docs/decisions/placement-seam.md names and takes everything else, its clock included, as an argument, so %s is the only package in this repository it may import (docs/repository-layout.md, the second boundary)",
				imported, dirDomain))
		}
		if reachesOutsideTheProcess(imported) {
			out = append(out, fmt.Sprintf(
				"imports %s; a placer that can reach the network, the file system or a store is one whose answer depends on when it was asked, and nobody can test that (docs/repository-layout.md, the second boundary)",
				imported))
		}
	}

	// The model. docs/repository-layout.md puts it under the flat rest of
	// internal, and issue #29 gave it the property: it has to compile with the
	// transport, the storage and the media plane gone. internal/domain keeps its
	// own copy of this assertion for the reason its comment gives; this is the
	// same rule read from the tree, so a file added to that package after
	// somebody deleted its local test is still refused.
	if dir == dirDomain && !isStandardLibrary(imported) {
		out = append(out, fmt.Sprintf(
			"imports %s; the model holds no transport, storage or media plane type and depends on nothing but the standard library (docs/repository-layout.md, the rest of internal, and issue #29)",
			imported))
	}

	return out
}

// GitDir is git's own storage and is not part of the layout this repository
// decides, so it is not judged against the document.
//
// It is named rather than covered by a rule about dotted names, because .github
// is dotted too and is one of the four. And it is a specific defect rather than a
// tidiness: .git is a directory in a clone and a file in a linked worktree, so a
// rule that read it would give two different answers for one commit depending on
// how the checkout was made. That is how this arrived, on a run that was green on
// one checkout and red on another.
const GitDir = ".git"

// CheckTopLevel refuses a top-level entry docs/repository-layout.md does not
// name. names are the directory names directly under the repository root.
//
// The missing direction is refused too. A document naming four directories and a
// tree holding three is the same drift read from the other side, and a rule that
// only counted upwards would go green the moment somebody deleted one.
func CheckTopLevel(names []string) []Finding {
	named := map[string]bool{}
	for _, n := range TopLevel {
		named[n] = true
	}
	present := map[string]bool{}
	for _, n := range names {
		if n == GitDir {
			continue
		}
		present[n] = true
	}

	var findings []Finding
	var extra []string
	for n := range present {
		if !named[n] {
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	for _, n := range extra {
		findings = append(findings, Finding{Detail: fmt.Sprintf(
			"%s/ is at the top level and docs/repository-layout.md does not name it; a fifth directory is a change to that document before it is a change to the tree",
			n)})
	}

	var missing []string
	for _, n := range TopLevel {
		if !present[n] {
			missing = append(missing, n)
		}
	}
	for _, n := range missing {
		findings = append(findings, Finding{Detail: fmt.Sprintf(
			"docs/repository-layout.md names %s/ at the top level and the tree does not hold it; the document and the tree have to say the same thing in both directions",
			n)})
	}
	return findings
}

// CheckTree refuses everything under root. Directories whose name begins with a
// dot are not read, which is the same reading internal/guard does, so the
// automated run under .github is outside what the import rules judge; the top
// level itself is read from the directory entries rather than from the walk, so
// a dotted directory there is still named.
func CheckTree(root string) ([]Finding, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	findings := CheckTopLevel(names)

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
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

// packageDir is the directory a file belongs to, from the repository root. A
// file at the root belongs to no directory and matches no rule below, which is
// correct: every rule here names a package under cmd/ or internal/.
func packageDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ""
	}
	return p[:i]
}

// inModule is the directory an import names inside this repository, or the empty
// string for an import of anything else. The trailing separator matters: a
// module whose path is a prefix of this one is a different module.
func inModule(imported string) string {
	if !strings.HasPrefix(imported, ModulePath+"/") {
		return ""
	}
	return strings.TrimPrefix(imported, ModulePath+"/")
}

// isStandardLibrary uses the rule the toolchain uses: the first element of an
// import path outside the standard library holds a dot, because it begins with
// a domain.
func isStandardLibrary(imported string) bool {
	return !strings.Contains(strings.Split(imported, "/")[0], ".")
}

// reachesOutsideTheProcess is the standard-library half of the placer's rule.
//
// It is an exclusion list where the module half is an allow-list, and the two
// are asymmetric on purpose. The packages in this repository are a set this
// repository decides, so naming the one the placer may have is possible and is
// what docs/decisions/placement-seam.md asks for. The standard library is not,
// and an allow-list over it would refuse the next release's arithmetic.
//
// net/netip and net/url are exempt and the exemption is the point of the rule
// rather than a hole in it. Both are values and parsing: a placer holding the
// address of a unit is reading one of the three records it is given, and a
// placer dialling that address is the thing being refused.
func reachesOutsideTheProcess(imported string) bool {
	switch imported {
	case "net/netip", "net/url":
		return false
	case "net", "syscall", "os", "database/sql":
		return true
	}
	for _, prefix := range []string{"net/", "os/", "database/sql/", "syscall/"} {
		if strings.HasPrefix(imported, prefix) {
			return true
		}
	}
	return false
}
