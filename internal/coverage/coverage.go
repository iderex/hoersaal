// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package coverage holds a bar over the packages where a gap in the tests costs
// the most, and reports the rest of the tree without holding it to anything.
//
// It answers issue #92. The shape is taken from the gate this board is measured
// against in docs/reference-gate-parity.md, which puts a high number on the
// modules that decide security outcomes and no number on the whole codebase. A
// figure over everything is easy to reach and says nothing, because the code
// that is trivial to cover is also the code where a gap costs least.
//
// # The surfaces
//
// Named by package path, in Surfaces below, each with the reason it is on the
// list. Everything in a named package is under the bar, so a file added to one
// falls under it without anybody editing anything, which is issue #92's first
// condition. Adding a new surface is editing the list, and that is deliberate:
// the two surfaces this issue names that do not exist yet, the authorisation
// decision on issue #34 and the admission path on issue #35, each arrive with a
// path of their own and a change that says which list they joined.
//
// # The bar, and where the number comes from
//
// BarTenths, which is 89.0 percentage points. It is derived rather than chosen,
// and the derivation is short enough to check.
//
// The lowest surface today is internal/placement, at 93 of 103 statements. One
// statement of that package is 100/103, which is 0.97 of a point, so a bar
// closer to it than that is a bar the first uncovered statement added to the
// tightest surface reds. That is the check this issue says it does not want: it
// asks for a bar set from what the code reaches, not one that forces tests
// written to touch lines. So the bar is that surface less one of its
// statements, 89.2, rounded down to the whole point.
//
// The figure this package prints for that surface is 90.2 and `go test -cover`
// prints 90.3 for the same numbers. Both are 93/103; this one truncates and the
// toolchain rounds. A bar is a floor, so a figure that rounded up would pass a
// package sitting below it, and the truncation is deliberate rather than a
// disagreement to be reconciled.
//
// What that costs, said rather than left to be inferred. A bar under every
// current figure is a bar nothing is currently held to, so what this refuses
// today is a fall of more than one statement in the tightest surface rather
// than any fall at all. The alternative was to raise internal/placement first
// and set the bar against the raised figure, which is work on that package and
// not on this one.
//
// The bar is one number over every surface rather than a number per surface.
// Issue #92 asks for a bar and gives the reason for it in one voice, and a bar
// per surface would have to be argued against that wording first.
//
// # What is refused, and what is only printed
//
// A surface below the bar is refused. A surface the profile holds no row for is
// refused as well, and that is the leg worth having: a run that measured less
// than it was asked to measure would otherwise report a clean tree, which is
// the shape this repository refuses everywhere else. A profile with no rows at
// all is refused for the same reason.
//
// Everything outside the surfaces is printed with its figure and gates nothing.
// That is issue #92's fourth condition, and it is also what stops the bar from
// quietly becoming a tree-wide number: the figures are in front of a reader
// without being in front of the gate.
package coverage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// BarTenths is the bar in tenths of a percentage point, so the comparison that
// decides a merge is integer arithmetic and not a float one. The number is
// derived in the package comment above, and it is written once here rather than
// as a percentage and a scaled copy that can drift apart.
const BarTenths = 890

// Bar is the same number as a percentage, for printing. Nothing compares
// against it.
func Bar() float64 { return float64(BarTenths) / 10 }

// The refusals.
var (
	// ErrProfile is a profile that cannot be read: no mode line, or a row this
	// package cannot parse.
	ErrProfile = errors.New("coverage: the profile cannot be read")

	// ErrNothingMeasured is a profile holding no statement at all, which would
	// otherwise judge every surface against an empty set and pass.
	ErrNothingMeasured = errors.New("coverage: the profile holds no statement")
)

// A Surface is a package under the bar, with the reason it is one. The reason
// is carried here rather than in a document, because it is printed beside the
// figure in every run and a reader meets it where the refusal is.
type Surface struct {
	// Path is the package's import path.
	Path string

	// Why is what a gap here costs.
	Why string
}

// Surfaces are the packages under the bar. Issue #92 names five and three
// exist; the other two arrive with their own path when they are built, and the
// package comment says which issues those are.
func Surfaces() []Surface {
	return []Surface{
		{
			Path: "github.com/iderex/hoersaal/internal/roomcred",
			Why:  "the credential handling: a gap here is somebody in a lecture they were not invited to",
		},
		{
			Path: "github.com/iderex/hoersaal/internal/wire",
			Why:  "the protocol decoder: it is the surface a stranger reaches first and hands arbitrary bytes",
		},
		{
			Path: "github.com/iderex/hoersaal/internal/placement",
			Why:  "the placement policy: a defect here costs an operator money and costs a room its lecture",
		},
		{
			Path: "github.com/iderex/hoersaal/internal/pool",
			Why:  "the pool: it decides which machines exist and admits the ones that carry other people's media",
		},
	}
}

// A Package is one package's statement count and how many of them a run
// reached.
type Package struct {
	// Path is the package's import path.
	Path string

	// Statements is how many statements the profile holds for it.
	Statements int

	// Covered is how many of those a run reached.
	Covered int
}

// Tenths is the coverage in tenths of a percentage point, truncated, which is
// the figure the bar is compared against. A package with no statement is a
// hundred per cent, because there is nothing in it a test could have missed.
func (p Package) Tenths() int {
	if p.Statements == 0 {
		return 1000
	}
	return p.Covered * 1000 / p.Statements
}

// Percent is the same figure for printing.
func (p Package) Percent() float64 { return float64(p.Tenths()) / 10 }

// A Judged surface is a surface with the figure the run gave it.
type Judged struct {
	// Surface is which one.
	Surface Surface

	// Package is what the profile held for it. Measured is false where the
	// profile held nothing, in which case Package is empty.
	Package Package

	// Measured is whether the profile held any row for this surface.
	Measured bool
}

// Meets is whether this surface is at or above the bar. A surface the profile
// did not measure never meets it.
func (j Judged) Meets() bool { return j.Measured && j.Package.Tenths() >= BarTenths }

// A Report is one run's verdict: the surfaces with their figures, everything
// else with its figure and no verdict, and the refusals.
type Report struct {
	// Surfaces are the packages under the bar, in the order Surfaces gives.
	Surfaces []Judged

	// Rest is every other package the profile held, by path. It gates nothing.
	Rest []Package

	// Refusals is one sentence per surface that failed, empty where none did.
	Refusals []string
}

// Parse reads a Go coverage profile and answers one Package per package.
//
// It reads the package out of the file path in each row rather than asking the
// toolchain, because the profile is the whole input and a checker that had to
// resolve packages against a module would be judging the machine it ran on as
// well as the profile it was handed.
func Parse(r io.Reader) (map[string]Package, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	if !s.Scan() {
		return nil, fmt.Errorf("no first line: %w", ErrProfile)
	}
	if !strings.HasPrefix(s.Text(), "mode:") {
		return nil, fmt.Errorf("the first line is %q rather than a mode line: %w", s.Text(), ErrProfile)
	}

	out := map[string]Package{}
	for n := 2; s.Scan(); n++ {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		pkg, statements, count, err := row(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		p := out[pkg]
		p.Path = pkg
		p.Statements += statements
		if count > 0 {
			p.Covered += statements
		}
		out[pkg] = p
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("reading the profile: %w", err)
	}
	if len(out) == 0 {
		return nil, ErrNothingMeasured
	}
	return out, nil
}

// row reads one profile line: a file position, the number of statements in the
// block, and how many times the run reached it.
func row(line string) (pkg string, statements, count int, err error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", 0, 0, fmt.Errorf("%q has %d field(s) rather than three: %w", line, len(fields), ErrProfile)
	}
	colon := strings.LastIndex(fields[0], ":")
	if colon < 1 {
		return "", 0, 0, fmt.Errorf("%q names no file position: %w", fields[0], ErrProfile)
	}
	file := fields[0][:colon]
	dir := path.Dir(file)
	if dir == "." || dir == "/" {
		return "", 0, 0, fmt.Errorf("%q names no package: %w", file, ErrProfile)
	}
	if statements, err = strconv.Atoi(fields[1]); err != nil || statements < 0 {
		return "", 0, 0, fmt.Errorf("%q is not a statement count: %w", fields[1], ErrProfile)
	}
	if count, err = strconv.Atoi(fields[2]); err != nil || count < 0 {
		return "", 0, 0, fmt.Errorf("%q is not a hit count: %w", fields[2], ErrProfile)
	}
	return dir, statements, count, nil
}

// Judge holds the surfaces to the bar and leaves everything else alone.
func Judge(packages map[string]Package) Report {
	var r Report
	named := map[string]bool{}
	for _, s := range Surfaces() {
		named[s.Path] = true
		p, measured := packages[s.Path]
		j := Judged{Surface: s, Package: p, Measured: measured}
		r.Surfaces = append(r.Surfaces, j)
		switch {
		case !measured:
			r.Refusals = append(r.Refusals, fmt.Sprintf(
				"%s is under the bar and the profile holds no row for it, so this run measured less than it was asked to and cannot be read as a pass",
				s.Path))
		case !j.Meets():
			r.Refusals = append(r.Refusals, fmt.Sprintf(
				"%s is at %.1f%% and the bar is %.1f%%: %s",
				s.Path, p.Percent(), Bar(), s.Why))
		}
	}
	for p, v := range packages {
		if !named[p] {
			r.Rest = append(r.Rest, v)
		}
	}
	sort.Slice(r.Rest, func(i, j int) bool { return r.Rest[i].Path < r.Rest[j].Path })
	return r
}

// Write prints what the run examined, which surfaces were held to the bar and
// at what figure, and every other package with no verdict attached. A run that
// printed only a verdict could not be told from one that judged a smaller set.
func (r Report) Write(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "the bar is %.1f%% and it is held over %d surface(s):\n", Bar(), len(r.Surfaces))
	for _, j := range r.Surfaces {
		if !j.Measured {
			fmt.Fprintf(&b, "  NOT MEASURED  %s\n", j.Surface.Path)
			continue
		}
		verdict := "under the bar"
		if j.Meets() {
			verdict = "at or above"
		}
		fmt.Fprintf(&b, "  %5.1f%%  %s  %s (%d/%d statements)\n",
			j.Package.Percent(), verdict, j.Surface.Path, j.Package.Covered, j.Package.Statements)
	}
	fmt.Fprintf(&b, "the rest of the tree, reported and not gated, %d package(s):\n", len(r.Rest))
	for _, p := range r.Rest {
		fmt.Fprintf(&b, "  %5.1f%%  %s (%d/%d statements)\n", p.Percent(), p.Path, p.Covered, p.Statements)
	}
	if len(r.Refusals) == 0 {
		fmt.Fprintf(&b, "every surface is at or above the bar.\n")
	} else {
		for _, s := range r.Refusals {
			fmt.Fprintf(&b, "REFUSED: %s\n", s)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
