// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

package boundary

import (
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
