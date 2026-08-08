// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

package srcheader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// good is a file that passes, built from the constants rather than typed out, so
// a test that agrees with the code because both were edited together is not
// possible here: change Holder and every fixture below moves with it.
func good(marker, licence string) string {
	return marker + " " + CopyrightTag + " " + Holder + "\n" +
		marker + " " + LicenceTag + " " + licence + "\n"
}

// TestTreeIsClean is the check itself. It reads the repository it lives in and
// fails on any covered file that does not open with the identifier its directory
// owes.
func TestTreeIsClean(t *testing.T) {
	findings, _, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

// TestTheWalkReadTheTree is the leg that stops a green run over nothing. A walk
// that read no file refuses nothing and looks exactly like a walk that read the
// tree and found it clean.
//
// It asserts a file from each directory the rule names rather than a count,
// because a count is a number that has to be edited every time a file is added
// and is therefore a number nobody trusts. What it is really asserting is that
// the walk reaches a dotted directory and a nested one, which are the two ways
// this walk has to be wrong to pass over the tree.
func TestTheWalkReadTheTree(t *testing.T) {
	_, read, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range read {
		seen[p] = true
	}
	for _, want := range []string{
		".github/workflows/unit.yml",
		"cmd/hoersaal/main.go",
		"internal/srcheader/srcheader.go",
	} {
		if !seen[want] {
			t.Errorf("the walk did not read %s, so it is not covering the tree it reports on", want)
		}
	}
	for _, p := range read {
		if !Covers(p) {
			t.Errorf("the walk read %s, which Covers says is not a file this judges", p)
		}
	}
}

// TestAFileFromTheTreeGoesThroughTheFixtureRoute checks that the route the
// fixtures below take is the route the tree takes. A fixture suite that passes
// against a function the real walk does not call proves nothing about the tree.
func TestAFileFromTheTreeGoesThroughTheFixtureRoute(t *testing.T) {
	src, err := os.ReadFile("srcheader.go")
	if err != nil {
		t.Fatalf("reading a file the walk covers: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("the file the walk covers is empty")
	}
	if f := CheckFile("internal/srcheader/srcheader.go", src); len(f) != 0 {
		t.Errorf("a file from the tree was refused by the fixture route: %v", f)
	}
}

// TestTheHeaderIsRefusedWhenItIsWrong is the proof that the rules bite. Each
// case is a file this check exists to stop reaching the tree, and each is a
// mistake somebody makes rather than one invented to fill the table: the header
// left off a new file, the header written under the package comment, the
// neighbouring SPDX identifier, and a new directory nobody declared.
func TestTheHeaderIsRefusedWhenItIsWrong(t *testing.T) {
	cases := []struct {
		name string
		path string
		src  string
		want string // a fragment of the sentence the refusal has to carry
	}{
		{
			name: "no identifier at all",
			path: "internal/placement/doc.go",
			src:  "// Package placement decides where a conference goes.\npackage placement\n",
			want: "does not open with the identifier",
		},
		{
			name: "the identifier under the package comment",
			path: "internal/placement/doc.go",
			src: "// Package placement decides where a conference goes.\npackage placement\n\n" +
				good("//", "AGPL-3.0-only"),
			want: "rather than on the first two lines",
		},
		{
			name: "or-later rather than only",
			path: "internal/placement/doc.go",
			src:  good("//", "AGPL-3.0-or-later"),
			want: `declares "AGPL-3.0-or-later"`,
		},
		{
			name: "a holder nobody assigned to",
			path: "internal/placement/doc.go",
			src:  "// " + CopyrightTag + " Nils Lehnen\n// " + LicenceTag + " AGPL-3.0-only\n",
			want: "as the copyright holder",
		},
		{
			name: "the two lines in the other order",
			path: "internal/placement/doc.go",
			src:  "// " + LicenceTag + " AGPL-3.0-only\n// " + CopyrightTag + " " + Holder + "\n",
			want: "the order is",
		},
		{
			name: "the copyright line and nothing under it",
			path: "internal/placement/doc.go",
			src:  "// " + CopyrightTag + " " + Holder + "\npackage placement\n",
			want: "and no " + LicenceTag + " on the second",
		},
		{
			name: "a Go comment marker in a workflow",
			path: ".github/workflows/unit.yml",
			src:  good("//", "AGPL-3.0-only"),
			want: "does not open with the identifier",
		},
		{
			name: "a directory no entry names",
			path: "clients/js/index.go",
			src:  good("//", "AGPL-3.0-only"),
			want: "no entry in srcheader.Licences names",
		},
		{
			name: "a file at the repository root",
			path: "tool.go",
			src:  good("//", "AGPL-3.0-only"),
			want: "no entry in srcheader.Licences names",
		},
		{
			name: "a file with one line in it",
			path: "internal/placement/doc.go",
			src:  "// " + CopyrightTag + " " + Holder,
			want: "too short",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := CheckFile(c.path, []byte(c.src))
			if len(findings) == 0 {
				t.Fatalf("accepted %s, which the rule exists to refuse", c.path)
			}
			joined := ""
			for _, f := range findings {
				joined += f.String() + "\n"
			}
			if !strings.Contains(joined, c.want) {
				t.Errorf("the refusal does not say %q; it says:\n%s", c.want, joined)
			}
		})
	}
}

// TestTheHeaderIsAcceptedWhenItIsRight is the other half of the same proof. A
// check that refuses everything is not a check, and the cases here are the two
// markers, the trailing space an editor leaves behind, and a file that carries
// the package comment under the header the way the tree's files do.
func TestTheHeaderIsAcceptedWhenItIsRight(t *testing.T) {
	cases := []struct {
		name string
		path string
		src  string
	}{
		{
			name: "a Go file",
			path: "internal/placement/doc.go",
			src:  good("//", "AGPL-3.0-only") + "\n// Package placement decides where a conference goes.\npackage placement\n",
		},
		{
			name: "a workflow",
			path: ".github/workflows/unit.yml",
			src:  good("#", "AGPL-3.0-only") + "\nname: Unit\n",
		},
		{
			name: "a command",
			path: "cmd/hoersaal/main.go",
			src:  good("//", "AGPL-3.0-only") + "\npackage main\n",
		},
		{
			name: "a trailing space an editor left",
			path: "internal/placement/doc.go",
			src:  "// " + CopyrightTag + " " + Holder + "  \n// " + LicenceTag + " AGPL-3.0-only \n",
		},
		{
			name: "carriage returns from a checkout that converted them",
			path: "internal/placement/doc.go",
			src:  strings.ReplaceAll(good("//", "AGPL-3.0-only"), "\n", "\r\n"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if f := CheckFile(c.path, []byte(c.src)); len(f) != 0 {
				t.Errorf("refused a file that carries the identifier: %v", f)
			}
		})
	}
}

// TestAFileTheCheckDoesNotCoverIsNotJudged keeps the boundary of the check where
// the package comment says it is. A markdown document with no header has to pass
// through this function untouched, because the alternative is a check that
// quietly grows a rule the documentation says it does not have.
func TestAFileTheCheckDoesNotCoverIsNotJudged(t *testing.T) {
	for _, p := range []string{"README.md", "go.mod", ".gitattributes", "docs/decisions/room-topology.md"} {
		if Covers(p) {
			t.Errorf("Covers says %s is judged here and the package comment says it is not", p)
		}
		if f := CheckFile(p, []byte("no identifier anywhere in this file\n")); len(f) != 0 {
			t.Errorf("%s was refused and it is not a file this check covers: %v", p, f)
		}
	}
}

// TestTheLicenceEveryDirectoryCarriesIsTheOneTheTreeHolds stops the map above
// from drifting away from LICENSE. Every entry says AGPL today, and the file at
// the root is what says whether that is still true. It reads the licence's own
// title rather than a copy of it kept here.
func TestTheLicenceEveryDirectoryCarriesIsTheOneTheTreeHolds(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "LICENSE"))
	if err != nil {
		t.Fatalf("reading LICENSE: %v", err)
	}
	title := "GNU AFFERO GENERAL PUBLIC LICENSE"
	version := "Version 3"
	if !strings.Contains(string(src), title) || !strings.Contains(string(src), version) {
		t.Fatalf("LICENSE is not %s %s, and every entry in srcheader.Licences says it is", title, version)
	}
	for dir, id := range Licences {
		if !strings.HasPrefix(id, "AGPL-3.0") {
			t.Errorf("%s/ carries %q and LICENSE is %s %s", dir, id, title, version)
		}
	}
}

// TestEveryTopLevelDirectoryTheTreeHoldsHasARule is the direction the map cannot
// state for itself. A new top-level directory of source with no entry is refused
// file by file above, which is a message per file; this says it once, at the
// directory, which is where the missing decision actually is.
func TestEveryTopLevelDirectoryTheTreeHoldsHasARule(t *testing.T) {
	_, read, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range read {
		seen[topLevel(p)] = true
	}
	for dir := range seen {
		if _, ok := Licences[dir]; !ok {
			t.Errorf("%s/ holds source and srcheader.Licences does not say what licence it carries", dir)
		}
	}
	for dir := range Licences {
		if !seen[dir] {
			t.Errorf("srcheader.Licences names %s/ and the tree holds no source there, so the rule is about nothing", dir)
		}
	}
}
