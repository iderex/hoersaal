// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package coverage

import (
	"errors"
	"strings"
	"testing"
)

// The fixtures are profiles written here rather than produced by running this
// repository's own suite. A row judged against the real tree proves the state
// of the tree on the day it ran and not the guard, and the figures this tree
// reaches move with every change to it.
//
// One row per statement, so a fixture's arithmetic is countable by eye.

// The four surface paths, named here for what each surface is rather than for
// its package. A constant whose name carries "cred" is read by the security
// analyser as a hardcoded credential, and this file holds none.
const (
	admission = "github.com/iderex/hoersaal/internal/roomcred"
	decoder   = "github.com/iderex/hoersaal/internal/wire"
	placer    = "github.com/iderex/hoersaal/internal/placement"
	units     = "github.com/iderex/hoersaal/internal/pool"
)

// profile writes a mode line and then one row per statement for each package,
// covered of them reached.
func profile(t *testing.T, of map[string][2]int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("mode: set\n")
	line := 1
	for _, pkg := range []string{admission, decoder, placer, units, "github.com/iderex/hoersaal/internal/domain"} {
		counts, held := of[pkg]
		if !held {
			continue
		}
		statements, covered := counts[0], counts[1]
		if covered > statements {
			t.Fatalf("fixture for %s covers %d of %d statements", pkg, covered, statements)
		}
		for i := range statements {
			hit := 0
			if i < covered {
				hit = 1
			}
			line++
			b.WriteString(pkg + "/file.go:" + itoa(line) + ".1," + itoa(line) + ".2 1 " + itoa(hit) + "\n")
		}
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// everySurfaceAtTheBar is the fixture the refusing cases below start from: each
// surface exactly at the bar, so one uncovered statement is the whole
// difference between a green run and a red one.
func everySurfaceAtTheBar() map[string][2]int {
	return map[string][2]int{
		admission: {1000, 890},
		decoder:   {1000, 890},
		placer:    {1000, 890},
		units:     {1000, 890},
	}
}

func judge(t *testing.T, of map[string][2]int) Report {
	t.Helper()
	packages, err := Parse(strings.NewReader(profile(t, of)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return Judge(packages)
}

func TestASurfaceExactlyAtTheBarPasses(t *testing.T) {
	r := judge(t, everySurfaceAtTheBar())
	if len(r.Refusals) != 0 {
		t.Fatalf("a surface at the bar was refused: %v", r.Refusals)
	}
	for _, j := range r.Surfaces {
		if !j.Meets() {
			t.Fatalf("%s at %.1f%% does not meet a bar of %.1f%%", j.Surface.Path, j.Package.Percent(), Bar())
		}
	}
}

func TestOneStatementBelowTheBarIsRefused(t *testing.T) {
	for _, surface := range Surfaces() {
		t.Run(surface.Path, func(t *testing.T) {
			of := everySurfaceAtTheBar()
			counts := of[surface.Path]
			counts[1]--
			of[surface.Path] = counts

			r := judge(t, of)
			if len(r.Refusals) != 1 {
				t.Fatalf("one statement below the bar produced %d refusal(s): %v", len(r.Refusals), r.Refusals)
			}
			if !strings.Contains(r.Refusals[0], surface.Path) {
				t.Fatalf("the refusal is %q and does not name %s", r.Refusals[0], surface.Path)
			}
			if !strings.Contains(r.Refusals[0], surface.Why) {
				t.Fatalf("the refusal is %q and does not say what a gap there costs", r.Refusals[0])
			}
		})
	}
}

// A surface the run did not measure is the case a coverage check gets wrong
// quietly: the profile is short, every figure it does hold is fine, and the
// verdict is green over a smaller set than the one the bar names.
func TestASurfaceTheProfileDidNotMeasureIsRefused(t *testing.T) {
	of := everySurfaceAtTheBar()
	delete(of, decoder)

	r := judge(t, of)
	if len(r.Refusals) != 1 {
		t.Fatalf("an unmeasured surface produced %d refusal(s): %v", len(r.Refusals), r.Refusals)
	}
	if !strings.Contains(r.Refusals[0], decoder) || !strings.Contains(r.Refusals[0], "no row") {
		t.Fatalf("the refusal is %q, want it to say the profile held no row for %s", r.Refusals[0], decoder)
	}
	for _, j := range r.Surfaces {
		if j.Surface.Path == decoder && j.Meets() {
			t.Fatal("an unmeasured surface met the bar")
		}
	}
}

func TestEverythingOutsideTheSurfacesIsReportedAndGatesNothing(t *testing.T) {
	of := everySurfaceAtTheBar()
	of["github.com/iderex/hoersaal/internal/domain"] = [2]int{100, 1}

	r := judge(t, of)
	if len(r.Refusals) != 0 {
		t.Fatalf("a package outside the surfaces was gated on: %v", r.Refusals)
	}
	if len(r.Rest) != 1 || r.Rest[0].Path != "github.com/iderex/hoersaal/internal/domain" {
		t.Fatalf("the rest of the tree is %v, want the one package outside the surfaces", r.Rest)
	}
	if r.Rest[0].Percent() != 1.0 {
		t.Fatalf("the reported figure is %.1f%%, want 1.0%%", r.Rest[0].Percent())
	}

	var out strings.Builder
	if err := r.Write(&out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(out.String(), "reported and not gated") {
		t.Fatalf("the report does not say the rest of the tree is not gated:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "internal/domain") {
		t.Fatalf("the report does not name the package it did not gate on:\n%s", out.String())
	}
}

func TestTheReportSaysWhatItExamined(t *testing.T) {
	r := judge(t, everySurfaceAtTheBar())
	var out strings.Builder
	if err := r.Write(&out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, want := range []string{"the bar is 89.0%", "4 surface(s)", "at or above", "every surface is at or above the bar."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the report does not carry %q:\n%s", want, out.String())
		}
	}
}

func TestAProfileThatMeasuredNothingIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want error
	}{
		{"nothing at all", "", ErrProfile},
		{"no mode line", "github.com/iderex/hoersaal/internal/pool/pool.go:1.1,1.2 1 1\n", ErrProfile},
		{"a mode line and no row", "mode: set\n", ErrNothingMeasured},
		{"a row with two fields", "mode: set\nfoo/bar.go:1.1,1.2 1\n", ErrProfile},
		{"a row with no file position", "mode: set\nbar.go 1 1\n", ErrProfile},
		{"a row whose file names no package", "mode: set\nbar.go:1.1,1.2 1 1\n", ErrProfile},
		{"a statement count that is not a number", "mode: set\na/bar.go:1.1,1.2 x 1\n", ErrProfile},
		{"a hit count that is not a number", "mode: set\na/bar.go:1.1,1.2 1 x\n", ErrProfile},
		{"a negative statement count", "mode: set\na/bar.go:1.1,1.2 -1 1\n", ErrProfile},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(c.in)); !errors.Is(err, c.want) {
				t.Fatalf("Parse of %s = %v, want %v", c.name, err, c.want)
			}
		})
	}
}

func TestABlankLineInAProfileIsNotARow(t *testing.T) {
	in := "mode: set\n\ngithub.com/iderex/hoersaal/internal/pool/pool.go:1.1,1.2 3 1\n\n"
	packages, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := packages[units].Statements; got != 3 {
		t.Fatalf("the profile holds %d statement(s) for the pool, want 3", got)
	}
}

func TestABlockReachedMoreThanOnceCountsOnce(t *testing.T) {
	in := "mode: count\n" +
		"github.com/iderex/hoersaal/internal/pool/a.go:1.1,1.2 2 7\n" +
		"github.com/iderex/hoersaal/internal/pool/a.go:3.1,3.2 2 0\n"
	packages, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := packages[units]
	if p.Statements != 4 || p.Covered != 2 {
		t.Fatalf("the pool is %d/%d, want 2/4", p.Covered, p.Statements)
	}
	if p.Percent() != 50.0 {
		t.Fatalf("the figure is %.1f%%, want 50.0%%", p.Percent())
	}
}

// A package with no statement is a hundred per cent rather than a division by
// zero, because there is nothing in it a test could have missed. It is here
// because the arithmetic is the thing a reader of a coverage check distrusts.
func TestAPackageWithNoStatementIsNotADivisionByZero(t *testing.T) {
	if got := (Package{Path: units}).Tenths(); got != 1000 {
		t.Fatalf("a package with no statement is %d tenths, want 1000", got)
	}
}

func TestTheFigureIsTruncatedRatherThanRounded(t *testing.T) {
	// 8 of 9 is 88.88 per cent, which rounds to 88.9 and truncates to 88.8. A
	// bar is a floor, so a figure that rounded up would pass a package that is
	// below it.
	p := Package{Path: units, Statements: 9, Covered: 8}
	if got := p.Tenths(); got != 888 {
		t.Fatalf("8 of 9 is %d tenths, want 888", got)
	}
}

func TestEverySurfaceCarriesAReasonAndAPathInThisModule(t *testing.T) {
	for _, s := range Surfaces() {
		if !strings.HasPrefix(s.Path, "github.com/iderex/hoersaal/") {
			t.Fatalf("%q is not a package in this module", s.Path)
		}
		if s.Why == "" {
			t.Fatalf("%s carries no reason, and the reason is what a reader meets at the refusal", s.Path)
		}
	}
}

func TestTheBarIsOneNumber(t *testing.T) {
	if Bar() != float64(BarTenths)/10 {
		t.Fatalf("Bar is %.1f and BarTenths is %d, and the two are supposed to be one number", Bar(), BarTenths)
	}
}
