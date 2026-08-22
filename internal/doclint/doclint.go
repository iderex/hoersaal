// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package doclint judges the documents in this repository the way the code is
// judged, which is what issue #97 is about.
//
// This project has more prose than most, because the decisions are the product
// as much as the code is. A document that drifts is the same defect as code that
// drifts, and until now nothing read one.
//
// Two things are checked and they are not equally important.
//
// The references are the point. A decision document naming a file that no longer
// exists is a document nobody can follow, and the arrangement on this board
// depends on documents pointing at each other and at the tree: the layout
// document names the packages, the decisions name each other, and
// docs/reference-gate-parity.md names most of the workflows. A moved file turns
// that arrangement into decoration and nothing says so.
//
// The formatting is the smaller half, and it is here so that a change to a
// document is a change to its meaning rather than a rewrap. What it refuses is
// listed at each rule below.
//
// What is inside the check and what is outside it, because a checker whose edge
// is not stated reads as complete when it is not.
//
// Inside: every tracked Markdown file in this repository, and every reference
// from one of them into this repository.
//
// Outside, and deliberately: any address beginning http:// or https://. Nothing
// here fetches one. A link checker that followed them would turn an unrelated
// change red the day somebody else's site moved, and this repository's documents
// point at other repositories a great deal. The command that runs this prints
// how many were not followed, so an absence of failures is not read as an
// absence of external links.
//
// Also outside: a path reference whose first element is not one of the
// directories this repository's own layout fixes. docs/reference-gate-parity.md
// names scripts in another repository, and CONTRIBUTING.md names branches like
// ci/build-check, and both look exactly like paths. Refusing those would be the
// checker refusing correct prose, so the rule is bounded and the bound is
// PathRefTopLevel below.
package doclint

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The rule names. They are printed with every refusal and they are what somebody
// searches for when they want to read the rule that stopped them, so they are
// stable strings rather than positions in a list.
const (
	RuleTrailingWhitespace = "no-trailing-whitespace"
	RuleHardTab            = "no-hard-tab-in-prose"
	RuleFinalNewline       = "one-final-newline"
	RuleBlankRun           = "no-two-blank-lines"
	RuleFenceClosed        = "fence-is-closed"
	RuleHeadingLevel       = "heading-level-does-not-skip"
	RuleLinkResolves       = "link-resolves"
	RulePathResolves       = "path-reference-resolves"
)

// PathRefTopLevel is the bound on the path rule: a backticked token is read as a
// reference into this repository only when it starts with one of these. They are
// the top level docs/repository-layout.md fixes, and internal/arch is what
// refuses a fifth one, so this list cannot quietly fall behind the tree.
var PathRefTopLevel = []string{".github", "cmd", "docs", "internal"}

// PathRefExtensions are the file suffixes this tree holds. A token carrying one
// of them is a file reference wherever it appears.
var PathRefExtensions = []string{".go", ".md", ".yml", ".yaml", ".mod"}

// PackageTopLevel are the two directories whose entries are packages, so a token
// under one of them with no extension is a directory reference rather than a
// name that merely looks like one. docs/ is not in this list on purpose:
// CONTRIBUTING.md names the branch docs/contribution-guide, which is not a path
// and never was, and a rule that could not tell the two apart would refuse the
// guide for describing its own convention.
var PackageTopLevel = []string{"cmd", "internal"}

// A Finding is one refusal, carrying enough to fix the document without opening
// this file.
type Finding struct {
	Path   string // from the repository root, forward slashes
	Line   int
	Rule   string
	Detail string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.Path, f.Line, f.Rule, f.Detail)
}

// A Reference is one pointer out of a document, as found. Resolved says whether
// anything in this repository answers to it, and External says the check did not
// try, which are different statements and are never collapsed.
type Reference struct {
	Path     string // the document it was found in
	Line     int
	Target   string
	External bool
	Resolved bool
}

var (
	// A Markdown inline link. The target stops at the first closing bracket,
	// which is what Markdown itself does for a target holding no parentheses,
	// and a target holding one is not a shape this repository writes.
	linkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)[^)]*\)`)

	// A backticked token with at least one separator in it. Whether it is a
	// path at all is decided by IsPathReference and not here.
	tokenPattern = regexp.MustCompile("`([A-Za-z0-9_.][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.-]+)+)`")

	// An address written on its own in prose. Trailing punctuation is left out
	// so that a sentence ending in a full stop does not put one in the address.
	bareURLPattern = regexp.MustCompile(`https?://[^\s)>\]]*[^\s)>\].,;:]`)
)

// Exists is how a reference is resolved. It takes a path from the repository
// root and answers whether the tree holds it, as a file or as a directory. The
// suite hands it a fixture so that a rule is proved without a tree on disk.
type Exists func(repoPath string) bool

// CheckFile refuses everything wrong with one document. path is from the
// repository root with forward slashes. exists answers for the tree the document
// lives in.
func CheckFile(path string, src []byte, exists Exists) ([]Finding, []Reference) {
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	findings := checkFormatting(path, text)
	f, refs := checkReferences(path, text, exists)
	return append(findings, f...), refs
}

// checkFormatting is the smaller half. Each rule refuses a thing that makes a
// diff say something other than what changed.
func checkFormatting(docPath, text string) []Finding {
	var findings []Finding

	if !strings.HasSuffix(text, "\n") {
		findings = append(findings, Finding{Path: docPath, Line: countLines(text), Rule: RuleFinalNewline,
			Detail: "the file does not end with a newline, so the last line of every diff that touches it is reported as changed"})
	} else if strings.HasSuffix(text, "\n\n") {
		findings = append(findings, Finding{Path: docPath, Line: countLines(text), Rule: RuleFinalNewline,
			Detail: "the file ends with a blank line; one newline ends the last line and a second one is a line nobody wrote"})
	}

	if i := strings.Index(text, "\n\n\n"); i >= 0 {
		findings = append(findings, Finding{Path: docPath, Line: countLines(text[:i]) + 2, Rule: RuleBlankRun,
			Detail: "two blank lines in a row; one separates paragraphs and a second is invisible in the rendered document and visible in every diff"})
	}

	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	fenced := false
	previousHeading := 0
	for i, line := range lines {
		n := i + 1

		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}

		if trimmed := strings.TrimRight(line, " \t"); trimmed != line {
			findings = append(findings, Finding{Path: docPath, Line: n, Rule: RuleTrailingWhitespace,
				Detail: "trailing whitespace, which no reader sees and every diff does"})
		}

		indented := strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
		if strings.Contains(line, "\t") && !fenced && !indented {
			findings = append(findings, Finding{Path: docPath, Line: n, Rule: RuleHardTab,
				Detail: "a hard tab outside a code block, which is a different width in every viewer"})
		}
		if fenced || indented {
			continue
		}

		if level, ok := headingLevel(line); ok {
			if previousHeading > 0 && level > previousHeading+1 {
				findings = append(findings, Finding{Path: docPath, Line: n, Rule: RuleHeadingLevel,
					Detail: fmt.Sprintf("a level %d heading under a level %d one; a skipped level is a section a reader and a screen reader both place wrongly", level, previousHeading)})
			}
			previousHeading = level
		}
	}
	if fenced {
		findings = append(findings, Finding{Path: docPath, Line: len(lines), Rule: RuleFenceClosed,
			Detail: "a fenced code block is never closed, so everything under it renders as code"})
	}

	return findings
}

// checkReferences is the half the issue says matters most. Every pointer out of
// the document is collected, whether or not it is judged, so that what was not
// judged can be counted rather than assumed empty.
func checkReferences(docPath, text string, exists Exists) ([]Finding, []Reference) {
	var findings []Finding
	var refs []Reference

	dir := path.Dir(docPath)
	if dir == "." {
		dir = ""
	}

	fenced := false
	for i, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		n := i + 1
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}

		// A code block is where command output lives, and command output names
		// paths and addresses that were true when the command ran rather than
		// ones this repository holds. So references are read out of prose only.
		if fenced || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}

		for _, m := range linkPattern.FindAllStringSubmatch(line, -1) {
			target := m[1]
			if IsExternal(target) {
				refs = append(refs, Reference{Path: docPath, Line: n, Target: target, External: true})
				continue
			}
			resolved := exists(resolveAgainst(dir, target))
			refs = append(refs, Reference{Path: docPath, Line: n, Target: target, Resolved: resolved})
			if !resolved {
				findings = append(findings, Finding{Path: docPath, Line: n, Rule: RuleLinkResolves,
					Detail: fmt.Sprintf("links to %q and the tree holds nothing at %q", target, resolveAgainst(dir, target))})
			}
		}

		// An address written on its own, without brackets around it, which is
		// how this repository's documents point at other projects. It is never
		// a finding and it is always counted, because the thing a reader has to
		// be able to see is how much of what a document points at this check
		// did not look at.
		for _, m := range bareURLPattern.FindAllString(line, -1) {
			if strings.Contains(line, "]("+m) {
				continue
			}
			refs = append(refs, Reference{Path: docPath, Line: n, Target: m, External: true})
		}

		for _, m := range tokenPattern.FindAllStringSubmatch(line, -1) {
			token := m[1]
			if !IsPathReference(token) {
				continue
			}
			resolved := exists(token)
			refs = append(refs, Reference{Path: docPath, Line: n, Target: token, Resolved: resolved})
			if !resolved {
				findings = append(findings, Finding{Path: docPath, Line: n, Rule: RulePathResolves,
					Detail: fmt.Sprintf("names %q and the tree holds nothing there", token)})
			}
		}
	}

	return findings, refs
}

// IsExternal says whether a link target is one this check does not follow. The
// answer is by scheme rather than by whether a fetch succeeds, so it is the same
// answer on a machine with no network.
func IsExternal(target string) bool {
	for _, scheme := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(target, scheme) {
			return true
		}
	}
	return false
}

// IsPathReference decides whether a backticked token is a reference into this
// repository. It is the bound the package comment describes, in one place, so
// that widening it is one edit and the argument for the width is beside it.
func IsPathReference(token string) bool {
	first, rest, ok := strings.Cut(token, "/")
	if !ok || rest == "" {
		return false
	}
	if !contains(PathRefTopLevel, first) {
		return false
	}
	for _, ext := range PathRefExtensions {
		if strings.HasSuffix(token, ext) {
			return true
		}
	}
	// No extension. Under cmd/ and internal/ the entries are packages, so the
	// token is a directory and has to be one. Elsewhere it is as likely to be a
	// branch name, and refusing those would refuse correct prose.
	return contains(PackageTopLevel, first)
}

// resolveAgainst turns a link target into a path from the repository root. The
// fragment is dropped because a link to a heading in a file is a link to the
// file, and this check does not read headings out of a document it is pointed at.
func resolveAgainst(dir, target string) string {
	if i := strings.IndexAny(target, "#?"); i >= 0 {
		target = target[:i]
	}
	if target == "" {
		return dir
	}
	if dir == "" {
		return path.Clean(target)
	}
	return path.Clean(path.Join(dir, target))
}

func headingLevel(line string) (int, bool) {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 || i >= len(line) || line[i] != ' ' {
		return 0, false
	}
	if strings.TrimSpace(line[i:]) == "" {
		return 0, false
	}
	return i, true
}

func countLines(s string) int { return strings.Count(s, "\n") + 1 }

func contains(set []string, s string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}
	return false
}

// GitDir is git's own storage, which holds no document this repository decides.
const GitDir = ".git"

// Extension is the suffix of a document this check reads.
const Extension = ".md"

// CheckTree refuses every document under root and returns what it read.
//
// documents comes back so that a run over nothing is a failure rather than a
// clean bill. references comes back so that the external ones can be counted and
// printed, because "no broken links" and "no links" look identical in a green
// run and are different statements.
func CheckTree(root string) (findings []Finding, documents []string, references []Reference, err error) {
	exists := func(repoPath string) bool {
		_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(repoPath)))
		return statErr == nil
	}

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == GitDir {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), Extension) {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(rel)
		// #nosec G304 -- p is a path this walk produced from root, so what is
		// opened is the checkout the command was pointed at. Nothing reaches
		// this from a request or from a caller choosing a file.
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		documents = append(documents, slashed)
		f, refs := CheckFile(slashed, src, exists)
		findings = append(findings, f...)
		references = append(references, refs...)
		return nil
	})

	sort.Strings(documents)
	return findings, documents, references, err
}

// CountExternal is how many references were not followed. It is a function
// rather than a line in the caller because two callers report it, and a count
// derived twice is a count that can disagree with itself.
func CountExternal(refs []Reference) int {
	n := 0
	for _, r := range refs {
		if r.External {
			n++
		}
	}
	return n
}
