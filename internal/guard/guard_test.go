// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package guard

import (
	"strings"
	"testing"
)

// TestTreeIsClean is the check itself. It reads the repository it lives in and
// fails on anything the rules refuse.
func TestTreeIsClean(t *testing.T) {
	findings, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

// The rest of this file proves the rules bite. Each fixture is one file the
// checker is asked about directly, so a rule that stopped refusing shows up
// here rather than waiting for somebody to write the mistake.

func check(t *testing.T, path, src string) []Finding {
	t.Helper()
	findings, err := CheckFile(path, []byte(src))
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return findings
}

func TestDirectClockReadIsRefused(t *testing.T) {
	const src = `package p

import "time"

func f() time.Time { return time.Now() }
`
	findings := check(t, "internal/pool/pool.go", src)
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Detail, "time.Now") {
		t.Errorf("the finding does not name the call: %s", findings[0])
	}
	if findings[0].Line != 5 {
		t.Errorf("want line 5, got %d", findings[0].Line)
	}
}

func TestRenamedTimeImportIsRefused(t *testing.T) {
	const src = `package p

import clockpkg "time"

func f() clockpkg.Time { return clockpkg.Now() }
`
	if got := check(t, "internal/pool/pool.go", src); len(got) != 1 {
		t.Fatalf("want 1 finding for a renamed import, got %d: %v", len(got), got)
	}
}

func TestDurationArithmeticIsNotRefused(t *testing.T) {
	const src = `package p

import "time"

const window = 2 * time.Minute

func f(t time.Time) time.Time { return t.Add(window) }
`
	if got := check(t, "internal/pool/pool.go", src); len(got) != 0 {
		t.Errorf("holding a duration is not a clock read: %v", got)
	}
}

func TestTheWordInACommentOrStringIsNotRefused(t *testing.T) {
	const src = `package p

// time.Now() is what this package does not call.
const s = "time.Now()"
`
	if got := check(t, "internal/pool/pool.go", src); len(got) != 0 {
		t.Errorf("a mention is not a call: %v", got)
	}
}

func TestTheNamedPlaceMayReadTheClock(t *testing.T) {
	const src = `package clock

import "time"

func now() time.Time { return time.Now() }
`
	if got := check(t, "internal/clock/clock.go", src); len(got) != 0 {
		t.Errorf("the one named place is allowed to read the clock: %v", got)
	}
}

func TestATestMayNotReadTheClockEither(t *testing.T) {
	const src = `package p

import "time"

func TestSomething(t *testing.T) { _ = time.Now() }
`
	if got := check(t, "internal/pool/pool_test.go", src); len(got) != 1 {
		t.Fatalf("want 1 finding in a test file, got %d: %v", len(got), got)
	}
}

func TestSleepIsRefusedEverywhere(t *testing.T) {
	const src = `package p

import "time"

func f() { time.Sleep(time.Second) }
`
	for _, path := range []string{
		"internal/pool/pool.go",
		"internal/pool/pool_test.go",
		"internal/clock/clock.go",
	} {
		got := check(t, path, src)
		if len(got) != 1 {
			t.Fatalf("want 1 finding in %s, got %d: %v", path, len(got), got)
		}
		if !strings.Contains(got[0].Detail, "time.Sleep") {
			t.Errorf("%s: the finding does not name the call: %s", path, got[0])
		}
	}
}

func TestARandomSourceOutsideTheNamedPlaceIsRefused(t *testing.T) {
	for _, imp := range []string{"math/rand", "math/rand/v2", "crypto/rand"} {
		src := "package p\n\nimport \"" + imp + "\"\n\nvar _ = rand.Uint64\n"
		got := check(t, "internal/pool/pool.go", src)
		if len(got) != 1 {
			t.Fatalf("want 1 finding for %s, got %d: %v", imp, len(got), got)
		}
		if !strings.Contains(got[0].Detail, imp) {
			t.Errorf("the finding does not name the import: %s", got[0])
		}
	}
}

func TestTheNamedPlaceMayMakeRandomness(t *testing.T) {
	const src = `package random

import "math/rand/v2"

var _ = rand.Uint64
`
	if got := check(t, "internal/random/random.go", src); len(got) != 0 {
		t.Errorf("the one named place is allowed to make a source: %v", got)
	}
}
