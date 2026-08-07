// Package textbytes reads the checkout and refuses a carriage return in a text
// file, which is what issue #26 is about.
//
// The subject is the checkout rather than the index, on purpose. What a test
// reads is the file on disk, so the failure this guards against is text that
// reaches a test differently than it left the author, and the file on disk is
// where that difference shows. The attributes in .gitattributes are what makes
// the two agree on every platform; this package is what says whether they did.
//
// The attributes alone are not enough, which is why a guard exists beside them.
// `text=auto` normalises a carriage return that is followed by a line feed and
// leaves a lone one alone, so a stray carriage return in the middle of a line
// survives every checkout of every platform and no attribute refuses it. And an
// attribute line added later can turn normalisation off for a path without
// anybody noticing, after which the pair comes back too.
//
// Which paths are binary is read from .gitattributes rather than held here, so
// the declaration has one home. A file whose declared suffix is binary is not
// read at all, and a file that is not declared but holds a zero byte is treated
// as binary by content, which is the rule git itself applies under `text=auto`.
package textbytes

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// binaryScan is how far into a file the content heuristic looks for a zero
// byte. It is git's own window, so a file this package treats as binary by
// content is one git would have treated the same way.
const binaryScan = 8000

// attributesFile is the declaration this package reads. It sits at the root of
// the repository and nowhere else: a second one deeper in the tree would change
// what git does for the paths under it while this package went on reporting the
// root's answer, so a nested one is refused rather than merged into the set.
const attributesFile = ".gitattributes"

// A Finding is one refusal, with enough in it to fix the file without opening
// this package.
type Finding struct {
	Path   string // from the repository root, forward slashes
	Line   int    // 1 for a finding about the file rather than a place in it
	Detail string
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Detail) }

// BinarySuffixes reads the patterns .gitattributes declares binary and returns
// their suffixes, lowercased and including the dot.
//
// It reads only the form this repository writes, `*.suffix`, and it refuses a
// binary declaration in any other form rather than guessing at it. A pattern
// this package cannot read is one whose files it would judge as text while git
// stored them raw, and silently judging the wrong set is the failure the whole
// package exists against.
func BinarySuffixes(attrs []byte) ([]string, []Finding) {
	var suffixes []string
	var findings []Finding
	for i, raw := range strings.Split(string(attrs), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pattern, declared := fields[0], fields[1:]
		if !declaresBinary(declared) {
			continue
		}
		suffix, ok := strings.CutPrefix(pattern, "*")
		if !ok || suffix == "" || !strings.HasPrefix(suffix, ".") || strings.ContainsAny(suffix, "*?[/") {
			findings = append(findings, Finding{
				Path: attributesFile, Line: i + 1,
				Detail: fmt.Sprintf("declares %q binary in a form this guard cannot read; it reads *.suffix and nothing else, and a pattern it cannot read is one whose files it would judge as text", pattern),
			})
			continue
		}
		suffixes = append(suffixes, strings.ToLower(suffix))
	}
	return suffixes, findings
}

// declaresBinary says whether the attributes on one line take a path out of
// normalisation. `binary` is the macro and `-text` is what the macro expands
// to, so both spellings are the same declaration and both are read.
func declaresBinary(attrs []string) bool {
	for _, a := range attrs {
		if a == "binary" || a == "-text" {
			return true
		}
	}
	return false
}

// CheckFile refuses what src carries at path. path is from the repository root
// with forward slashes, and binary holds the suffixes BinarySuffixes returned.
//
// One finding per file, at the first carriage return, because a file that
// picked one up on a checkout has one on every line and a finding per line
// would bury every other refusal in the run.
func CheckFile(path string, src []byte, binary []string) []Finding {
	lower := strings.ToLower(path)
	for _, suffix := range binary {
		if strings.HasSuffix(lower, suffix) {
			return nil
		}
	}
	head := src
	if len(head) > binaryScan {
		head = head[:binaryScan]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return nil
	}

	at := bytes.IndexByte(src, '\r')
	if at < 0 {
		return nil
	}
	line := bytes.Count(src[:at], []byte{'\n'}) + 1
	total := bytes.Count(src, []byte{'\r'})
	kind := "a carriage return with no line feed after it, which no attribute normalises away"
	if at+1 < len(src) && src[at+1] == '\n' {
		kind = "a carriage return before a line feed, so this checkout did not honour the text attributes"
	}
	return []Finding{{
		Path: path, Line: line,
		Detail: fmt.Sprintf("carries %s (%d in the file); tracked text is stored and checked out with line feeds only", kind, total),
	}}
}

// CheckTree refuses everything under root. The git directory is not read and
// everything else is, including directories whose name begins with a dot,
// because the workflow files are tracked text and carry the same risk as the
// source.
//
// What it reads is the checkout and not the tracked set, so a file somebody has
// left in their working directory is judged like any other. That is wider than
// the rule needs to be and it is the safe direction: a file in the checkout is
// a file a test can open.
func CheckTree(root string) ([]Finding, error) {
	attrs, err := os.ReadFile(filepath.Join(root, attributesFile))
	if err != nil {
		if os.IsNotExist(err) {
			return []Finding{{
				Path: attributesFile, Line: 1,
				Detail: "is missing; without it a checkout decides the line endings from a personal setting and every file in the tree is whatever that setting made it",
			}}, nil
		}
		return nil, err
	}
	binary, findings := BinarySuffixes(attrs)

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if d.Name() == attributesFile && filepath.ToSlash(rel) != attributesFile {
			findings = append(findings, Finding{
				Path: filepath.ToSlash(rel), Line: 1,
				Detail: "is a second attributes file; it would change what git does for the paths under it while this guard went on reporting the root's answer, so the declaration stays in one place",
			})
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		findings = append(findings, CheckFile(filepath.ToSlash(rel), src, binary)...)
		return nil
	})
	return findings, err
}
