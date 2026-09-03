// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package boundary

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestTreeIsClean is the check itself. It reads the repository it lives in and
// fails on anything the rule refuses. It passes today because nothing here
// speaks to anything, and that is the state docs/decisions/federation.md asks
// for rather than a state that makes the rule vacuous: every fixture below is a
// file the checker is asked about directly, so a rule that stopped refusing
// shows up here rather than waiting for somebody to write the mistake into the
// tree.
func TestTreeIsClean(t *testing.T) {
	findings, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

func check(t *testing.T, path, src string) []Finding {
	t.Helper()
	findings, err := CheckFile(path, []byte(src))
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return findings
}

func one(t *testing.T, path, src, wants string) Finding {
	t.Helper()
	findings := check(t, path, src)
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Detail, wants) {
		t.Errorf("the finding does not name %s: %s", wants, findings[0])
	}
	return findings[0]
}

func TestDiallingIsRefused(t *testing.T) {
	const src = `package p

import "net"

func f() (net.Conn, error) { return net.Dial("tcp", "elsewhere:443") }
`
	if got := one(t, "internal/pool/pool.go", src, "net.Dial"); got.Line != 5 {
		t.Errorf("want line 5, got %d", got.Line)
	}
}

func TestARenamedImportIsRefused(t *testing.T) {
	const src = `package p

import sockets "net"

func f() (sockets.Conn, error) { return sockets.Dial("tcp", "elsewhere:443") }
`
	one(t, "internal/pool/pool.go", src, "net.Dial")
}

// The rule is about origination. A service that could not listen could not be
// reached, and being reached is not a crossing.
func TestListeningIsNotRefused(t *testing.T) {
	const src = `package p

import "net"

func serve(l net.Listener) (net.Conn, error) {
	c, err := l.Accept()
	if err != nil {
		return nil, err
	}
	var addr net.Addr = c.RemoteAddr()
	_ = addr
	return c, nil
}

func listen() (net.Listener, error) { return net.Listen("tcp", ":8443") }
`
	if got := check(t, "internal/wire/listen.go", src); len(got) != 0 {
		t.Errorf("listening and accepting are not a crossing: %v", got)
	}
}

// Asking a resolver where a name lives is a question sent to something that is
// not this process, and it is how a host belonging to another installation would
// be turned into an address.
func TestAResolverQuestionIsRefused(t *testing.T) {
	const src = `package p

import "net"

func f(host string) ([]string, error) { return net.LookupHost(host) }
`
	one(t, "internal/pool/pool.go", src, "net.LookupHost")
}

func TestTheHTTPClientIsRefusedInEveryPositionItIsWritten(t *testing.T) {
	for name, src := range map[string]string{
		"a call": `package p

import "net/http"

func f() { http.Get("https://elsewhere/api") }
`,
		"a composite literal": `package p

import "net/http"

func f() { c := http.Client{}; _ = c }
`,
		"a value taken by name": `package p

import "net/http"

func f() { c := http.DefaultClient; _ = c }
`,
		"a struct field": `package p

import "net/http"

type unit struct{ to *http.Client }
`,
	} {
		if got := check(t, "internal/pool/pool.go", src); len(got) != 1 {
			t.Errorf("%s: want 1 finding, got %d: %v", name, len(got), got)
		}
	}
}

func TestServingHTTPIsNotRefused(t *testing.T) {
	const src = `package p

import "net/http"

func f(w http.ResponseWriter, r *http.Request) {
	s := &http.Server{Addr: ":8443"}
	_ = s
	w.WriteHeader(http.StatusNoContent)
	_ = r
}
`
	if got := check(t, "internal/wire/serve.go", src); len(got) != 0 {
		t.Errorf("serving is not a crossing: %v", got)
	}
}

func TestATLSDialIsRefused(t *testing.T) {
	const src = `package p

import "crypto/tls"

func f() { tls.Dial("tcp", "elsewhere:443", nil) }
`
	one(t, "internal/pool/pool.go", src, "crypto/tls.Dial")
}

func TestAPackageThatOnlySpeaksOutwardIsRefusedOnTheImport(t *testing.T) {
	const src = `package p

import "net/smtp"

func f() { smtp.SendMail("mail:25", nil, "a", nil, nil) }
`
	// Two findings would mean the import rule and the reference rule both fired
	// on one act, and net/smtp is only in the first table, so this asserts the
	// tables do not overlap as much as it asserts the refusal.
	if got := one(t, "internal/pool/pool.go", src, "net/smtp"); got.Line != 3 {
		t.Errorf("the finding is at line %d, want the import at line 3", got.Line)
	}
}

// A blank or dot import leaves no name in front of a call, so the reference rule
// has nothing to match on and would pass the file in silence.
func TestAnImportWithNoNameToReadIsRefused(t *testing.T) {
	for name, src := range map[string]string{
		"blank": `package p

import _ "net/http"
`,
		"dot": `package p

import . "net/http"

func f() { Get("https://elsewhere/api") }
`,
	} {
		if got := check(t, "internal/pool/pool.go", src); len(got) != 1 {
			t.Errorf("a %s import of net/http: want 1 finding, got %d: %v", name, len(got), got)
		}
	}
}

func TestTheWordInACommentOrAStringIsNotRefused(t *testing.T) {
	const src = `package p

// This package does not call net.Dial and does not want to.
const why = "net.Dial and http.Get are made in internal/boundary"
`
	if got := check(t, "internal/domain/domain.go", src); len(got) != 0 {
		t.Errorf("a mention is not a call: %v", got)
	}
}

// A method with one of these names on something that is not the package is not
// the package's function, and refusing it would make the rule unusable for
// anything that has a Dial method of its own.
func TestAMethodOnSomethingElseIsNotRefused(t *testing.T) {
	const src = `package p

type pool struct{}

func (pool) Dial() {}

func f(p pool) { p.Dial() }
`
	if got := check(t, "internal/pool/pool.go", src); len(got) != 0 {
		t.Errorf("a method of the same name is not the package's function: %v", got)
	}
}

// The exemption, which is the other half of the rule and would otherwise be
// untested: the named place may do what everywhere else may not.
func TestThePlaceMayOriginateAConnection(t *testing.T) {
	const src = `package boundary

import (
	"crypto/tls"
	"net"
	"net/http"
)

func f() {
	net.Dial("tcp", "unit:7000")
	tls.Dial("tcp", "unit:7000", nil)
	http.Get("https://unit/health")
}
`
	if got := check(t, Place+"/dial.go", src); len(got) != 0 {
		t.Errorf("the place itself is refused: %v", got)
	}
}

// Directly in the place rather than anywhere beneath it. A subdirectory is a
// package of its own and would inherit an exemption nobody argued for.
func TestASubdirectoryOfThePlaceIsNotThePlace(t *testing.T) {
	const src = `package inner

import "net"

func f() { net.Dial("tcp", "elsewhere:443") }
`
	one(t, Place+"/inner/inner.go", src, "net.Dial")
}

// A neighbouring directory whose name begins with the place's is not the place
// either, which is the one-character mistake this comparison exists against.
func TestADirectoryWhoseNameStartsWithThePlaceIsNotThePlace(t *testing.T) {
	const src = `package boundaries

import "net"

func f() { net.Dial("tcp", "elsewhere:443") }
`
	one(t, Place+"ish/boundaryish.go", src, "net.Dial")
}

// Every finding sends the reader to the document that decided the rule rather
// than restating it here, which is what internal/arch does and for the same
// reason: a rule that turns out to be wrong is changed by changing the argument.
func TestEveryFindingNamesTheDecision(t *testing.T) {
	const src = `package p

import (
	"net"
	"net/smtp"
)

func f() {
	net.Dial("tcp", "elsewhere:443")
	smtp.SendMail("mail:25", nil, "a", nil, nil)
}
`
	findings := check(t, "internal/pool/pool.go", src)
	if len(findings) != 2 {
		t.Fatalf("want 2 findings, got %d: %v", len(findings), findings)
	}
	for _, f := range findings {
		if !strings.Contains(f.Detail, "docs/decisions/federation.md") {
			t.Errorf("the finding does not send the reader to the decision: %s", f)
		}
		if !strings.Contains(f.Detail, Place) {
			t.Errorf("the finding does not name where the connection belongs: %s", f)
		}
	}
}

func TestAFileThatDoesNotParseIsAnErrorRatherThanAPass(t *testing.T) {
	if _, err := CheckFile("internal/pool/pool.go", []byte("package")); err == nil {
		t.Error("a file that does not parse was read as a file with nothing in it")
	}
}

// TestThePlaceItselfHoldsNoConnectionToday reads the files that are allowed to
// originate a connection as though they were not allowed to, and fails on any
// connection it finds. TestTreeIsClean cannot see this: the place is exempt
// from the rule by construction, so a dial written here passes it. What this
// asserts is the sentence docs/data-protection.md carries about the outbound
// connections this software can make, which is that today there are none, and
// a sentence like that is worth more asserted than written. The day the
// adapter on issue #43 lands its connection here, this test is the one that
// changes, and the list in that document changes with it.
func TestThePlaceItselfHoldsNoConnectionToday(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the place: %v", err)
	}
	read, found := 0, 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// #nosec G304 -- name came out of reading this package's own
		// directory, so what is opened is a file of the repository the test
		// runs in. Nothing reaches this from a caller choosing a path.
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		// Judged under a path outside the place, so the exemption that
		// isThePlace grants does not apply and the file is read like any
		// other.
		findings, err := CheckFile("elsewhere/"+name, src)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		read++
		found += len(findings)
		for _, f := range findings {
			t.Errorf("%s originates a connection, so the outbound list in docs/data-protection.md is no longer empty: %s", name, f)
		}
	}
	if read == 0 {
		t.Fatal("no file was read, so nothing was asserted")
	}
	t.Logf("files in the place read as though they were elsewhere: %d, connections found: %d", read, found)
}

// TestTheServiceStartsNoProcessThatCouldReachOut walks the import closure of
// the service binary inside this module and refuses os/exec anywhere in it.
// The rule in this package reads connections and says in its own comment that
// a process started to make one is outside what it reads. For the service
// that gap is closed here rather than left: cmd/hoersaal and everything it
// links start no process, so there is no route by which something other than
// this binary contacts an endpoint on its behalf.
//
// It is the closure of the binary and not the whole tree on purpose. Two
// commands in this repository do start processes, and both are tooling that
// runs on this repository rather than on a deployment: cmd/prhygiene runs git
// over a pull request, and cmd/mediaharness starts what the harness drives.
// Neither is linked into the service, and the walk is what shows that rather
// than a list that says so.
func TestTheServiceStartsNoProcessThatCouldReachOut(t *testing.T) {
	packages, findings, err := closure("../..", "cmd/hoersaal", modulePath(t))
	if err != nil {
		t.Fatalf("walking the closure: %v", err)
	}
	if len(packages) < 2 {
		t.Fatalf("the walk reached %d package(s), so it did not follow an import and asserts nothing", len(packages))
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
	t.Logf("packages linked into the service: %s", strings.Join(packages, " "))
}

// TestTheClosureWalkRefusesAProcessStart is the proof that the walk above
// bites, against a module made for the purpose: the start package imports a
// second package of the same module, and that second one imports os/exec. The
// finding has to be in the second package, because a walk that only read the
// start directory would pass a service that reaches out one import away.
func TestTheClosureWalkRefusesAProcessStart(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("cmd/svc/main.go", "package main\n\nimport _ \"example.test/mod/internal/deep\"\n\nfunc main() {}\n")
	write("internal/deep/deep.go", "package deep\n\nimport \"os/exec\"\n\nvar _ = exec.Command\n")
	write("internal/deep/deep_test.go", "package deep\n\nimport \"os/exec\"\n\nvar _ = exec.Command\n")

	packages, findings, err := closure(root, "cmd/svc", "example.test/mod")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(packages, " "); got != "cmd/svc internal/deep" {
		t.Errorf("the walk should reach both packages in order, got %q", got)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, for the production file and not the test file, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "internal/deep/deep.go" || findings[0].Line != 3 {
		t.Errorf("the finding should sit on the import in the second package, got %s", findings[0])
	}

	// One change away: the same module with the import gone refuses nothing.
	write("internal/deep/deep.go", "package deep\n\nvar Nothing = 0\n")
	if _, findings, err = closure(root, "cmd/svc", "example.test/mod"); err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("a module that starts no process should pass, got %v", findings)
	}
}

// closure walks the production files of start and of every package of module
// that they import, directly or through another, and answers with the packages
// it reached in the order it reached them and a finding for every import of
// os/exec. _test.go files are not read, because a test is not linked into the
// binary and the question is what the binary can do.
func closure(root, start, module string) ([]string, []Finding, error) {
	var packages []string
	var findings []Finding
	seen := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if seen[dir] {
			continue
		}
		seen[dir] = true
		packages = append(packages, dir)

		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return nil, nil, err
		}
		var next []string
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := dir + "/" + name
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(path)), nil, parser.ImportsOnly)
			if err != nil {
				return nil, nil, err
			}
			for _, imp := range file.Imports {
				imported, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: unreadable import %s", path, imp.Path.Value)
				}
				if imported == "os/exec" {
					findings = append(findings, Finding{
						Path: path, Line: fset.Position(imp.Pos()).Line,
						Detail: "imports os/exec, which can start a process that reaches what this rule refuses; the service links nothing that starts one (docs/data-protection.md, the outbound connections)",
					})
				}
				if strings.HasPrefix(imported, module+"/") {
					next = append(next, strings.TrimPrefix(imported, module+"/"))
				}
			}
		}
		sort.Strings(next)
		queue = append(queue, next...)
	}
	return packages, findings, nil
}

// modulePath reads the module this repository is, from go.mod rather than from
// a constant here, so that the walk above follows the imports the toolchain
// follows and not the ones a test remembered.
func modulePath(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	t.Fatal("go.mod names no module")
	return ""
}
