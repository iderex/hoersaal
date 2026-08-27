// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package invariant holds the rules that are properties of the text rather than
// of the behaviour, in one table, and answers issue #95.
//
// Several rules on this board are of that kind: a thing that must not appear in
// a Go file anywhere except in the one place that owns it. Each of them was
// stated in the issue that wanted it and then left to whoever remembered it.
// Rules below is where they are counted, and each entry carries the issue it
// came from, so a reader who disagrees with a refusal is sent to the place the
// disagreement belongs rather than to this file.
//
// A rule in the table is in one of two states and the table says which. An
// enforced rule has a subject in this tree and a checker here, and the suite
// keeps a case that reds it. A declared rule has no subject yet, because the
// thing it would read is fixed by an issue that is still open, and it names that
// issue in Waiting. The command prints both lists, so a run that enforced three
// of five rules cannot be read as one that enforced five and found nothing.
// Declaring a rule that cannot yet run is the point rather than a placeholder:
// the day its subject arrives, the entry is already here with the issue that
// asked for it, instead of being remembered.
//
// One rule is enforced somewhere else and is listed here anyway. The machine
// clock and the sources of randomness are refused by internal/guard, which
// answers issue #27 and keeps the syntax-tree reading and its own fixtures. Its
// entry below calls that package rather than repeating it, because two
// implementations of one rule drift and the drift is silent. What this table
// claims is that the rules can be counted in one place and run by one command,
// not that every line of every checker is in this file.
//
// What this reads. Go files, through the syntax tree rather than through the
// text, so a word in a comment is not a finding and a call written across two
// lines is one. String literals are read for the rules whose subject is a name
// that travels as data, because a forwarding unit's vocabulary reaches this
// tree as a URL path or a JSON key long before it reaches it as an identifier.
//
// What it does not read, said here rather than found later. Nothing outside Go:
// a name in a workflow, in a document or in a fixture file is invisible to it.
// It has no type information, so it judges the name at the call site and not
// what that name resolves to; a participant identifier passed through a variable
// called v is not caught by the logging rule, and the rule is written for the
// mistake somebody actually makes rather than for the one somebody hides.
package invariant

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/iderex/hoersaal/internal/config"
	"github.com/iderex/hoersaal/internal/guard"
)

// The rule ids. They are printed with every refusal and they are what somebody
// searches for when they want to read the rule that stopped them, so they are
// stable strings rather than positions in the table.
const (
	RuleClockAndRandom     = "no-machine-clock-or-random-source"
	RuleForwardingUnitName = "no-forwarding-unit-name-outside-the-adapter"
	RuleLoggedIdentifier   = "no-personal-identifier-in-a-log-call"
	RuleClientDisplay      = "no-display-dependency-in-the-client-decision-layer"
	RuleConfigurationKey   = "no-configuration-key-outside-the-fixed-list"
)

// A Finding is one refusal, carrying enough to fix the code without opening this
// file.
type Finding struct {
	Path   string // from the repository root, forward slashes
	Line   int
	Rule   string
	Detail string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.Path, f.Line, f.Rule, f.Detail)
}

// A Rule is one property of the text. Issue is the issue that required it, and
// it is not optional: a rule nobody can trace to an argument is a preference.
// Waiting is empty for a rule that runs, and otherwise says what has to exist
// before it can, in a sentence naming the issue that will produce it.
type Rule struct {
	ID      string
	Issue   string
	Subject string
	Waiting string

	check func(path string, src []byte, file *ast.File, fset *token.FileSet) ([]Finding, error)
}

// Enforced says whether this rule reads anything today.
func (r Rule) Enforced() bool { return r.Waiting == "" }

// Rules is the whole table. Adding a rule is adding an entry here and a case in
// the suite that reds it; there is no second list anywhere.
var Rules = []Rule{
	{
		ID:      RuleClockAndRandom,
		Issue:   "#27",
		Subject: "every Go file outside internal/clock/clock.go and internal/random/random.go",
		check:   checkClockAndRandom,
	},
	{
		ID:      RuleForwardingUnitName,
		Issue:   "#43",
		Subject: "every Go file outside internal/mediaunit and this package",
		check:   checkForwardingUnitName,
	},
	{
		ID:      RuleLoggedIdentifier,
		Issue:   "#85",
		Subject: "every call into log or log/slog, and every call on a value named logger, in any Go file",
		check:   checkLoggedIdentifier,
	},
	{
		ID:      RuleClientDisplay,
		Issue:   "#75",
		Subject: "the client decision layer, which does not exist",
		// WHAT THIS WAITED ON CHANGED AND THE SENTENCE HAD NOT. It said the
		// client platform was undecided on #74, so a display-carrying import
		// had no name and a list written then would have been a list against a
		// guess. #74 is decided and closed, and docs/decisions/client-platform.md
		// fixes the browser, so the vocabulary is no longer what is missing:
		//
		//	gh api repos/iderex/hoersaal/issues/74 --jq '[.number,.state,.state_reason]|@tsv'
		//	74	closed	completed
		//
		// It was found by re-running the command a neighbouring document pastes
		// and reading the line that had stopped being true, rather than by
		// anybody meeting the rule.
		//
		// What is missing instead is the subject. There is no client decision
		// layer, and the first file of one cannot arrive quietly: that document
		// names four top-level directories and internal/arch refuses a fifth,
		// so a client directory is a change to it first. That is #75, and it is
		// also where the language, the framework and the headless driver are
		// each the means check of the change that introduces them, which
		// client-platform.md says in as many words. This checker reads Go
		// through the syntax tree, so what it will be asked to read is a second
		// question that arrives with the layer rather than one this entry can
		// answer now.
		Waiting: "there is no client decision layer in this tree and the first file of one is a change to docs/repository-layout.md before it is a change to the tree, so this rule has no subject; #75 builds the layer and pins what its imports may be",
	},
	{
		ID:      RuleConfigurationKey,
		Issue:   "#82",
		Subject: "every string in a Go file outside internal/config that begins with one of the prefixes the four groups of docs/decisions/what-an-operator-may-set.md use",
		check:   checkConfigurationKey,
	},
}

// clockPlace and randomPlace repeat internal/guard's own exceptions so that the
// subject line above is true. They are asserted against that package in the
// suite rather than trusted.
const (
	clockPlace  = "internal/clock/clock.go"
	randomPlace = "internal/random/random.go"
)

// AdapterDir is the one directory allowed to name the chosen forwarding unit.
// docs/repository-layout.md calls that the first boundary and internal/arch
// refuses the import half of it; this is the half an import rule cannot see,
// which is the unit's vocabulary arriving as a name or as a string.
const AdapterDir = "internal/mediaunit"

// ConfigDir is the one directory that holds the configuration keys as data, and
// it is exempt from the rule that reads them for the same reason SelfDir is
// exempt from the forwarding unit rule: the list has to be written down
// somewhere, and the place it is written down cannot be judged against itself.
//
// What that exemption costs is the arm of the fourth condition of issue #82 an
// import rule cannot reach, and it is not left uncovered. Whether the keys that
// package accepts are the ones docs/decisions/what-an-operator-may-set.md fixes
// is asserted in that package's own suite, which reads the block in the
// document rather than holding a second copy of it. This rule is the other arm:
// a key named anywhere else in the tree that the loader would refuse.
const ConfigDir = "internal/config"

// SelfDir is this package, and it is exempt from the forwarding unit rule for
// the reason that it holds the vocabulary as data. The exemption is real and it
// is bounded to one directory: a name from the unit leaking into this package
// is invisible to this rule, and nothing else here is exempt from anything.
const SelfDir = "internal/invariant"

// forwardingUnitNames is the vocabulary of the unit docs/decisions/media-plane.md
// chose. It is short and every entry is unmistakable, because a rule that
// refused an ordinary word would be a rule that refuses correct code, and the
// first person to meet it would delete it rather than argue with it. A term the
// unit shares with the rest of the world, its health path among them, is
// deliberately absent for that reason.
var forwardingUnitNames = []string{
	"jitsi",
	"videobridge",
	"jvb",
	"colibri",
}

// personalNames are the words a log line must not carry, from issue #85: a
// participant name, an address, a credential. They are matched against the name
// at the call site, lowercased, as substrings, so ParticipantID and participantID
// are one entry rather than two.
var personalNames = []string{
	"participantid",
	"participantname",
	"identityid",
	"displayname",
	"roomtitle",
	"email",
	"address",
	"credential",
	"password",
	"secret",
	"token",
}

// loggerRoots are the names a logging call hangs off: the two standard packages,
// and the ordinary names a logger value carries. The rule stops there on
// purpose. fmt.Printf reaches the same stream and is not refused, because the
// commands in this tree print paths and counts with it all day and a rule that
// covered them would be turned off within a week.
var loggerRoots = map[string]bool{
	"log":    true,
	"slog":   true,
	"logger": true,
}

// CheckFile runs every enforced rule over src. path is from the repository root
// with forward slashes, and it is what decides which exceptions apply.
func CheckFile(path string, src []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, rule := range Rules {
		if !rule.Enforced() {
			continue
		}
		found, err := rule.check(path, src, file, fset)
		if err != nil {
			return nil, err
		}
		findings = append(findings, found...)
	}
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Line < findings[j].Line })
	return findings, nil
}

// checkClockAndRandom hands the file to internal/guard and relabels what comes
// back. The rule is #27's and the enforcement is #27's; what this adds is that
// it is counted here with the others and run by one command.
func checkClockAndRandom(path string, src []byte, _ *ast.File, _ *token.FileSet) ([]Finding, error) {
	found, err := guard.CheckFile(path, src)
	if err != nil {
		return nil, err
	}
	out := make([]Finding, 0, len(found))
	for _, f := range found {
		out = append(out, Finding{
			Path: f.Path, Line: f.Line, Rule: RuleClockAndRandom,
			Detail: f.Detail + " (issue #27, refused by internal/guard)",
		})
	}
	return out, nil
}

// checkForwardingUnitName refuses the chosen unit's vocabulary anywhere but the
// adapter. It reads identifiers, selector names, import paths and the contents
// of string literals, because the way this vocabulary actually escapes is as a
// URL path or a JSON key rather than as a Go name.
func checkForwardingUnitName(path string, _ []byte, file *ast.File, fset *token.FileSet) ([]Finding, error) {
	dir := packageDir(path)
	if dir == AdapterDir || dir == SelfDir {
		return nil, nil
	}

	var findings []Finding
	refuse := func(pos token.Pos, where, text string) {
		for _, name := range forwardingUnitNames {
			if !strings.Contains(strings.ToLower(text), name) {
				continue
			}
			findings = append(findings, Finding{
				Path: path, Line: fset.Position(pos).Line, Rule: RuleForwardingUnitName,
				Detail: fmt.Sprintf(
					"%s names %q, which belongs to the forwarding unit docs/decisions/media-plane.md chose; that vocabulary stops at %s, so that swapping the unit is a change to one directory (issue #43)",
					where, name, AdapterDir),
			})
			return
		}
	}

	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: unreadable import %s", path, imp.Path.Value)
		}
		refuse(imp.Pos(), "the import", p)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			refuse(v.Pos(), "the name", v.Name)
		case *ast.BasicLit:
			if v.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(v.Value)
			if err != nil {
				text = v.Value
			}
			refuse(v.Pos(), "the string", text)
		}
		return true
	})
	return findings, nil
}

// checkConfigurationKey refuses a configuration key that internal/config would
// not accept. It reads string literals, because a key travels as data: it is
// what an operator writes in a file, what a message names back to them, and
// what a test fixture holds, and it reaches Go as a string in every one of
// those rather than as an identifier.
//
// The reach is the shape a key has and the prefixes the four groups use, and
// that is a bound rather than a detail. What is inside it: a misspelling of a
// real key, and a key nobody declared under a group that exists. What is
// outside it: a key invented under a sixth prefix, and a key built by joining
// two strings. The mistake this is written for is the one somebody actually
// makes, which is typing a name that looks like the others.
func checkConfigurationKey(path string, _ []byte, file *ast.File, fset *token.FileSet) ([]Finding, error) {
	if packageDir(path) == ConfigDir {
		return nil, nil
	}

	accepted := map[string]bool{}
	for _, key := range config.Keys() {
		accepted[key] = true
	}

	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil || accepted[text] || !looksLikeAKey(text) {
			return true
		}
		findings = append(findings, Finding{
			Path: path, Line: fset.Position(lit.Pos()).Line, Rule: RuleConfigurationKey,
			Detail: fmt.Sprintf(
				"%q reads as a configuration key and %s does not accept it; %s fixes what an operator may set and a key outside that list is a typo the operator never hears about (issue #82)",
				text, ConfigDir, config.TheList),
		})
		return false
	})
	return findings, nil
}

// fileSuffixes are the endings this rule does not read, because a file name and
// a configuration key are the same shape and this tree names files in Go all
// day. That is measured rather than supposed: the first run of this rule over
// the tree refused "unit.yml" in internal/textbytes' own suite, which is a
// workflow and not a setting.
//
// The cost is stated rather than hidden. A key typed as listen.yml would pass
// this rule, and nothing else would catch it either. That is a narrower hole
// than a rule somebody deletes the first week because it refuses correct code.
var fileSuffixes = []string{".go", ".json", ".md", ".mod", ".sum", ".txt", ".yaml", ".yml"}

// looksLikeAKey says whether text is written the way a key on the list is: one
// of the group prefixes, then lowercase letters, digits and hyphens, and
// nothing else at all.
//
// The shape is what keeps a format string out. The other run of this rule over
// the tree refused "pool.Pool{key: " in internal/pool, which begins with a
// group prefix and is a fragment of Go being printed; a capital, a brace and a
// space each put it outside the shape.
func looksLikeAKey(text string) bool {
	inside := false
	for _, prefix := range config.Prefixes() {
		if strings.HasPrefix(text, prefix) {
			inside = true
			break
		}
	}
	if !inside {
		return false
	}
	for _, suffix := range fileSuffixes {
		if strings.HasSuffix(text, suffix) {
			return false
		}
	}

	name := text[strings.Index(text, ".")+1:]
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// checkLoggedIdentifier refuses a log call that carries something naming a
// person. What it judges is the name written at the call site, because there is
// no type information here, and the mistake this is written against is somebody
// reaching for the identifier they already have rather than somebody hiding one
// behind a variable called v.
func checkLoggedIdentifier(path string, _ []byte, file *ast.File, fset *token.FileSet) ([]Finding, error) {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isLoggerRoot(sel.X) {
			return true
		}
		for _, arg := range call.Args {
			name, word := personalNameIn(arg)
			if name == "" {
				continue
			}
			findings = append(findings, Finding{
				Path: path, Line: fset.Position(arg.Pos()).Line, Rule: RuleLoggedIdentifier,
				Detail: fmt.Sprintf(
					"the log call takes %s, whose name carries %q; a log from this service is read in a support ticket and kept for a year, so what identifies a person stays out of it (issue #85)",
					name, word),
			})
		}
		return true
	})
	return findings, nil
}

// isLoggerRoot says whether e is the thing a logging call hangs off. It walks
// down a selector chain, so both slog.Info and h.logger.Info are answered by the
// same rule.
func isLoggerRoot(e ast.Expr) bool {
	for {
		switch v := e.(type) {
		case *ast.Ident:
			return loggerRoots[strings.ToLower(v.Name)]
		case *ast.SelectorExpr:
			if loggerRoots[strings.ToLower(v.Sel.Name)] {
				return true
			}
			e = v.X
		case *ast.CallExpr:
			e = v.Fun
		default:
			return false
		}
	}
}

// personalNameIn reports the first name inside e that carries one of the words
// above, with the word it carried. It reads names and not string contents: a
// message saying the word address is prose, and the argument beside it is the
// finding.
func personalNameIn(e ast.Expr) (string, string) {
	var name, word string
	ast.Inspect(e, func(n ast.Node) bool {
		if name != "" {
			return false
		}
		var candidate string
		switch v := n.(type) {
		case *ast.Ident:
			candidate = v.Name
		case *ast.SelectorExpr:
			candidate = v.Sel.Name
		default:
			return true
		}
		for _, p := range personalNames {
			if strings.Contains(strings.ToLower(candidate), p) {
				name, word = candidate, p
				return false
			}
		}
		return true
	})
	return name, word
}

// packageDir is the directory a path sits in, from the repository root, with
// forward slashes and no trailing separator.
func packageDir(path string) string {
	path = filepath.ToSlash(path)
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}

// CheckTree runs every enforced rule over every Go file under root and returns
// the refusals and the files it read. The file list is returned rather than
// counted here, because a run that read nothing and a clean run leave the same
// verdict and only the list tells them apart.
func CheckTree(root string) ([]Finding, []string, error) {
	var findings []Finding
	var files []string

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
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
		rel = filepath.ToSlash(rel)

		// #nosec G304 -- p is a path this walk produced from root, so what is
		// opened is the checkout the command was pointed at. Nothing reaches
		// this from a request or from a caller choosing a file.
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		found, err := CheckFile(rel, src)
		if err != nil {
			return err
		}
		files = append(files, rel)
		findings = append(findings, found...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Strings(files)
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, files, nil
}
