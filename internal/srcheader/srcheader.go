// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package srcheader refuses a source file that does not open by naming its
// copyright holder and its licence, which is what issue #101 is about.
//
// LICENSE at the root covers this repository. It does not travel with a file
// that leaves it, and files here demonstrably leave: three of the workflows in
// this tree were taken from another repository along with their prose, which
// docs/reference-gate-parity.md records under "What this document does not
// enumerate", down to the issue numbers that belong to that other board. A file
// arriving somewhere else with no identifier on it is a file whose terms the
// person holding it has to guess.
//
// The identifier is two lines in the SPDX form, first in the file, before
// anything else including the package comment. First matters: a header below
// the package comment is invisible to whoever copies the first screen of a file,
// which is the case the rule exists for, so it is refused with the line it was
// actually found on rather than accepted as near enough.
//
// The licence is read per directory rather than assumed for the whole tree. The
// second and more permissive licence for the client libraries and the generated
// protocol definitions is still open on issue #1, and those directories do not
// exist yet; a single tree-wide constant would turn the day that is answered
// into a rewrite of every header in the repository. Licences below is where a
// new top-level directory declares what it carries, and a directory with no
// entry is refused rather than defaulted, so the answer is written down rather
// than inherited.
//
// What it does not cover, named here rather than left to be discovered. The
// languages the tree holds are Go and the workflow YAML, and both are covered.
// Markdown is not: a document is prose that renders, the identifier would either
// be shown to a reader or hidden in a comment nobody opens, and a document that
// travels alone carries its own sentences saying where it came from. go.mod is
// not: it is written by the toolchain, and build.yml refuses a build that
// changes it, so a comment placed there is a comment the next `go mod tidy` may
// move. .gitattributes and .gitignore are not: they are declarations about this
// checkout that mean nothing outside it. And the fuzz corpus under testdata is
// not, because a corpus entry is a byte sequence a decoder is asked about and a
// comment in it is a different question.
//
// The check is a test rather than a workflow of its own. It reads the tree it
// lives in, so it runs wherever the suite runs, and the fixtures below are what
// prove each rule refuses rather than a pull request that happens to trip one.
package srcheader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Holder is the copyright holder every header names.
//
// It is the contributors rather than a person or a company because nothing in
// this repository collects an assignment. CONTRIBUTING.md says of the sign-off
// that it is not a copyright assignment and not a contributor licence
// agreement, so every contributor keeps their own copyright, and a header naming
// one name would assert an assignment the tree refuses to ask for.
//
// There is no year in it, and the omission is deliberate. The SPDX form makes
// the year optional. A year in a header is a number that is wrong the following
// January in every file nobody edited, and no reading of the tree can tell a
// stale year from a correct one, so the check would either police a number it
// cannot judge or ignore the field it demanded.
const Holder = "The hoersaal contributors"

// The two tags, in the order they have to appear. They are the SPDX names
// rather than names invented here, because a machine reading a vendored copy of
// one of these files is looking for these strings and not for ours.
const (
	CopyrightTag = "SPDX-FileCopyrightText:"
	LicenceTag   = "SPDX-License-Identifier:"
)

// Licences is the rule, one entry per top-level directory. A source file under
// a directory with no entry here is refused, and adding a directory is adding a
// line to this map in the change that creates it.
//
// Every entry says AGPL-3.0-only today. `-only` rather than `-or-later` because
// that is what the tree says and no more: the licence entry on issue #1 was
// answered "AGPL-3.0", LICENSE holds the GNU AGPL version 3 text, and no file in
// this repository offers the reader a later version. Writing `-or-later` here
// would grant, in a header, a term nobody decided.
var Licences = map[string]string{
	".github":  "AGPL-3.0-only",
	"cmd":      "AGPL-3.0-only",
	"internal": "AGPL-3.0-only",
}

// Markers is the line comment each covered language opens a comment with, keyed
// by file extension. A file whose extension is not here is not a file this
// package judges, and Covers is the one place that decision is made.
var Markers = map[string]string{
	".go":   "//",
	".yml":  "#",
	".yaml": "#",
}

// GitDir is git's own storage. It is skipped by name rather than by a rule about
// dotted directories, because .github is dotted too and is one of the three the
// rule is about.
const GitDir = ".git"

// TestdataDir holds the fuzz corpus, whose entries are byte sequences rather
// than source.
const TestdataDir = "testdata"

// A Finding is one refusal, carrying enough to fix the file without opening this
// one.
type Finding struct {
	Path   string // from the repository root, forward slashes
	Line   int    // 1 for a header that should be at the top and is not there
	Detail string
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Detail) }

// Covers says whether path is a file this package judges. It is exported
// because the walk and the suite have to agree on the answer, and two copies of
// the extension list would not.
func Covers(path string) bool {
	_, ok := Markers[filepath.Ext(path)]
	return ok
}

// CheckFile refuses the opening of src at path. path is from the repository root
// with forward slashes, and its first element is what decides which licence the
// file owes.
func CheckFile(path string, src []byte) []Finding {
	marker, ok := Markers[filepath.Ext(path)]
	if !ok {
		return nil
	}

	dir := topLevel(path)
	want, ok := Licences[dir]
	if !ok {
		return []Finding{{Path: path, Line: 1, Detail: fmt.Sprintf(
			"no entry in srcheader.Licences names %q, so nothing says which licence a file here carries; a new top-level directory declares what it carries in the change that creates it",
			dir)}}
	}

	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")

	// The header somewhere other than the top is the near miss this reports by
	// name. Somebody who writes the two lines under the package comment has done
	// the work and still leaves the first screen of the file saying nothing.
	if elsewhere := findTag(lines, marker, CopyrightTag); elsewhere > 2 {
		return []Finding{{Path: path, Line: elsewhere,
			Detail: "the identifier is here rather than on the first two lines of the file; a reader who copies the top of a file has to be able to see it, so it opens the file and nothing precedes it"}}
	}

	got, findings := headerLines(path, lines, marker)
	if len(findings) > 0 {
		return findings
	}

	if got.holder != Holder {
		findings = append(findings, Finding{Path: path, Line: 1, Detail: fmt.Sprintf(
			"names %q as the copyright holder and every file here names %q; nothing in this repository collects an assignment, so the holder is the contributors",
			got.holder, Holder)})
	}
	if got.licence != want {
		findings = append(findings, Finding{Path: path, Line: 2, Detail: fmt.Sprintf(
			"declares %q and %s/ carries %q; the identifier a file leaves with has to be the one the directory it came from is under",
			got.licence, dir, want)})
	}
	return findings
}

// header is the two values one file's opening declares.
type header struct {
	holder  string
	licence string
}

// headerLines reads the first two lines and refuses everything that is not the
// pair, with a separate sentence per way of getting it wrong. The order is one
// of them: a file whose two lines are swapped has both tags and is still not the
// form a tool reading a vendored copy looks for.
func headerLines(path string, lines []string, marker string) (header, []Finding) {
	if len(lines) < 2 {
		return header{}, []Finding{{Path: path, Line: 1, Detail: fmt.Sprintf(
			"is too short to open with the two identifier lines; every source file begins with %s %s and %s %s",
			marker, CopyrightTag, marker, LicenceTag)}}
	}

	holder, holderOK := field(lines[0], marker, CopyrightTag)
	licence, licenceOK := field(lines[1], marker, LicenceTag)

	switch {
	case holderOK && licenceOK:
		return header{holder: holder, licence: licence}, nil
	case !holderOK && !licenceOK:
		if _, ok := field(lines[0], marker, LicenceTag); ok {
			return header{}, []Finding{{Path: path, Line: 1, Detail: fmt.Sprintf(
				"opens with the licence line and then the copyright line; the order is %s first, because that is the order the form is read in",
				CopyrightTag)}}
		}
		return header{}, []Finding{{Path: path, Line: 1, Detail: fmt.Sprintf(
			"does not open with the identifier; the first two lines of a source file are %q and %q",
			marker+" "+CopyrightTag+" "+Holder,
			marker+" "+LicenceTag+" "+licenceFor(path))}}
	case !holderOK:
		return header{}, []Finding{{Path: path, Line: 1, Detail: fmt.Sprintf(
			"has %s on the second line and no %s on the first", LicenceTag, CopyrightTag)}}
	default:
		return header{}, []Finding{{Path: path, Line: 2, Detail: fmt.Sprintf(
			"has %s on the first line and no %s on the second", CopyrightTag, LicenceTag)}}
	}
}

// licenceFor is the identifier path owes, for use in a message. It answers with
// the empty string for a directory no entry names, and the caller that would
// reach that case has already refused the file for the missing entry.
func licenceFor(path string) string { return Licences[topLevel(path)] }

// field reads the value a tagged comment line carries, and says whether the line
// was that comment at all. Trailing space is tolerated on the way in and the
// value is not, so a header with a stray space at the end of a line passes and
// one with a doubled space inside the holder does not: the first is invisible in
// every editor and the second is a different string.
func field(line, marker, tag string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimRight(line, " \t"), marker+" "+tag)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// findTag is the line number a tag appears on, or zero. It is what turns a
// header written in the wrong place into a message naming the place.
func findTag(lines []string, marker, tag string) int {
	for i, line := range lines {
		if _, ok := field(line, marker, tag); ok {
			return i + 1
		}
	}
	return 0
}

// CheckTree refuses every covered file under root and says how many it read.
//
// The count is returned rather than logged because a walk that read nothing
// refuses nothing and is indistinguishable from a walk that read the tree and
// found it clean. The suite is where that becomes a failure.
func CheckTree(root string) (findings []Finding, read []string, err error) {
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == GitDir || d.Name() == TestdataDir {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(rel)
		if !Covers(slashed) {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		read = append(read, slashed)
		findings = append(findings, CheckFile(slashed, src)...)
		return nil
	})
	sort.Strings(read)
	return findings, read, err
}

// topLevel is the first element of a path from the repository root. A file at
// the root has none, and it is answered with the empty string, which no entry in
// Licences names, so such a file is refused rather than quietly exempt.
func topLevel(p string) string {
	if i := strings.Index(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}
