package arch

import (
	"os"
	"strings"
	"testing"
)

// TestTreeIsClean is the check itself. It reads the repository it lives in and
// fails on any import the layout refuses and on a top level that has drifted
// from the document.
func TestTreeIsClean(t *testing.T) {
	findings, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

// TestTheTreeWasActuallyRead is the leg that stops a green run over nothing. A
// walk that read no Go file refuses nothing and looks exactly like a walk that
// read the tree and found it clean.
func TestTheTreeWasActuallyRead(t *testing.T) {
	src, err := os.ReadFile("../placement/doc.go")
	if err != nil {
		t.Fatalf("reading a file the walk covers: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("the file the walk covers is empty")
	}
	findings, err := CheckFile("internal/placement/doc.go", src)
	if err != nil {
		t.Fatalf("checking a file from the tree: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("a file from the tree was refused by the fixture route: %v", findings)
	}
}

// TestTheModulePathIsTheOneInGoMod stops the constant this package matches
// against from drifting away from the module it is supposed to be about. Every
// rule that names a package in this repository goes through that constant, so a
// wrong one turns the whole table into a set of rules about somebody else's code
// and every one of them passes.
func TestTheModulePathIsTheOneInGoMod(t *testing.T) {
	src, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	declared := ""
	for _, line := range strings.Split(string(src), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			declared = strings.TrimSpace(rest)
			break
		}
	}
	if declared == "" {
		t.Fatal("go.mod declares no module path")
	}
	if declared != ModulePath {
		t.Errorf("go.mod declares %s and this package matches against %s", declared, ModulePath)
	}
}

// The rest of this file proves the rules bite. Each fixture is one file the
// checker is asked about directly, and each refusal is paired with the
// neighbouring legal case, because a rule that refuses everything passes the
// first half of this and is not a rule.

func check(t *testing.T, path, src string) []Finding {
	t.Helper()
	findings, err := CheckFile(path, []byte(src))
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return findings
}

func one(t *testing.T, findings []Finding, want string) {
	t.Helper()
	if len(findings) != 1 {
		t.Fatalf("wanted one finding, got %d: %v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Detail, want) {
		t.Errorf("the finding does not say why: %s", findings[0].Detail)
	}
}

func none(t *testing.T, findings []Finding) {
	t.Helper()
	if len(findings) != 0 {
		t.Errorf("wanted no finding, got %v", findings)
	}
}

// The first boundary. A package under internal reaching the adapter is the
// dependency that makes removing the media plane stop compiling the control
// plane, which is the property the whole arrangement exists to hold.

func TestAControlPlanePackageReachingTheAdapterIsRefused(t *testing.T) {
	one(t, check(t, "internal/roomcred/roomcred.go", `package roomcred

import _ "`+ModulePath+`/internal/mediaunit"
`), "reached by cmd/hoersaal and by nothing else")
}

// The near miss, and the one somebody actually writes. The fake is the package
// most tempted by the adapter's vocabulary, because it is imitating the thing
// the adapter wraps, and a fake that borrows it puts that vocabulary above the
// boundary.
func TestTheFakeReachingTheAdapterIsRefused(t *testing.T) {
	one(t, check(t, "internal/mediafake/fake.go", `package mediafake

import _ "`+ModulePath+`/internal/mediaunit"
`), "reached by cmd/hoersaal and by nothing else")
}

// The other near miss. A test is a file in the package and reaches for whatever
// makes the test convenient, so a rule that stopped at production files would
// pass the case it exists for.
func TestATestReachingTheAdapterIsRefusedToo(t *testing.T) {
	one(t, check(t, "internal/mediafake/fake_test.go", `package mediafake

import _ "`+ModulePath+`/internal/mediaunit"
`), "reached by cmd/hoersaal and by nothing else")
}

func TestTheAdapterMayImportItself(t *testing.T) {
	none(t, check(t, "internal/mediaunit/unit.go", `package mediaunit

import _ "`+ModulePath+`/internal/mediaport"
`))
}

func TestTheWiringMayReachTheAdapter(t *testing.T) {
	none(t, check(t, "cmd/hoersaal/main.go", `package main

import _ "`+ModulePath+`/internal/mediaunit"

func main() {}
`))
}

func TestThePortReachingTheFakeIsRefused(t *testing.T) {
	one(t, check(t, "internal/mediaport/port.go", `package mediaport

import _ "`+ModulePath+`/internal/mediafake"
`), "the dependency runs the other way")
}

func TestTheFakeMayReachThePort(t *testing.T) {
	none(t, check(t, "internal/mediafake/fake.go", `package mediafake

import _ "`+ModulePath+`/internal/mediaport"
`))
}

// The second boundary. The placer's rule has two halves and each is shown
// separately, because they are refused for different reasons and a reader who
// trips one should not be sent to the other's argument.

func TestThePlacerReachingTheClockIsRefused(t *testing.T) {
	one(t, check(t, "internal/placement/place.go", `package placement

import _ "`+ModulePath+`/internal/clock"
`), "takes everything else, its clock included, as an argument")
}

// The near miss for this one. A placer test wants a controllable clock more than
// a placer does, so this is the file the rule is really written against.
func TestThePlacerTestReachingTheClockIsRefusedToo(t *testing.T) {
	one(t, check(t, "internal/placement/place_test.go", `package placement

import _ "`+ModulePath+`/internal/clock"
`), "takes everything else, its clock included, as an argument")
}

func TestThePlacerDiallingIsRefused(t *testing.T) {
	one(t, check(t, "internal/placement/place.go", `package placement

import _ "net"
`), "depends on when it was asked")
}

func TestThePlacerReadingTheFileSystemIsRefused(t *testing.T) {
	one(t, check(t, "internal/placement/place.go", `package placement

import _ "os"
`), "depends on when it was asked")
}

func TestThePlacerReachingAStoreIsRefused(t *testing.T) {
	one(t, check(t, "internal/placement/place.go", `package placement

import _ "database/sql"
`), "depends on when it was asked")
}

func TestThePlacerMayHoldTheModel(t *testing.T) {
	none(t, check(t, "internal/placement/place.go", `package placement

import _ "`+ModulePath+`/internal/domain"
`))
}

// The exemption, which is the point of the rule rather than a hole in it. An
// address a placer was handed is one of the three records it reads; an address a
// placer dials is the thing being refused, and the two are different packages.
func TestThePlacerMayHoldAnAddressAsAValue(t *testing.T) {
	none(t, check(t, "internal/placement/place.go", `package placement

import (
	_ "net/netip"
	_ "sort"
)
`))
}

func TestThePlacerMayUseTheStandardLibraryOtherwise(t *testing.T) {
	none(t, check(t, "internal/placement/place.go", `package placement

import (
	_ "errors"
	_ "fmt"
	_ "time"
)
`))
}

// The model. internal/domain asserts this about itself for the reason its own
// comment gives; this is the same rule read from the tree, so a file added to
// that package after somebody deleted the local test is still refused.

func TestTheModelReachingAnotherPackageIsRefused(t *testing.T) {
	one(t, check(t, "internal/domain/domain.go", `package domain

import _ "`+ModulePath+`/internal/wire"
`), "depends on nothing but the standard library")
}

func TestTheModelReachingOutsideTheModuleIsRefused(t *testing.T) {
	one(t, check(t, "internal/domain/domain.go", `package domain

import _ "golang.org/x/text/language"
`), "depends on nothing but the standard library")
}

func TestTheModelMayUseTheStandardLibrary(t *testing.T) {
	none(t, check(t, "internal/domain/domain.go", `package domain

import (
	_ "errors"
	_ "sort"
)
`))
}

// Every other package is unruled on purpose. docs/repository-layout.md says the
// rest of internal is flat and named for what it holds, and inventing a rule
// here that the document does not carry would be a rule with no argument behind
// it.
func TestAPackageWithNoRuleIsNotRefused(t *testing.T) {
	none(t, check(t, "internal/wire/wire.go", `package wire

import (
	_ "encoding/json"
	_ "net/http"

	_ "`+ModulePath+`/internal/domain"
)
`))
}

// The top level, and the fourth condition on issue #98: the set cannot silently
// fall behind the tree.

func TestANewTopLevelDirectoryIsRefused(t *testing.T) {
	findings := CheckTopLevel([]string{".github", "cmd", "docs", "internal", "pkg"})
	if len(findings) != 1 {
		t.Fatalf("wanted one finding, got %d: %v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Detail, "pkg/") {
		t.Errorf("the finding does not name the directory: %s", findings[0].Detail)
	}
}

func TestATopLevelDirectoryThatWentIsRefusedToo(t *testing.T) {
	findings := CheckTopLevel([]string{".github", "cmd", "internal"})
	if len(findings) != 1 {
		t.Fatalf("wanted one finding, got %d: %v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Detail, "docs/") {
		t.Errorf("the finding does not name the directory: %s", findings[0].Detail)
	}
}

func TestTheTopLevelTheDocumentNamesIsNotRefused(t *testing.T) {
	if findings := CheckTopLevel(TopLevel); len(findings) != 0 {
		t.Errorf("the document's own list was refused: %v", findings)
	}
}
