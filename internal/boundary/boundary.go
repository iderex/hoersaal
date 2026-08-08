// Package boundary is the place a connection out of this process is made, and
// it refuses one made anywhere else.
//
// It answers the first condition of issue #104.
// docs/decisions/federation.md decides that no installation of this software
// talks to another one, and its "Where the boundary lives" section says that the
// boundary is in the code rather than in that sentence. This is the code. The
// package is empty of connections, which is what the decision says it should be,
// and it is the only place in this repository where writing one is not a
// finding, so the day somebody writes one it is written here and a reader
// looking for what this software talks to opens one directory.
//
// A wall rather than a shut door, and that is the decision's word rather than a
// softening of it. There is no setting, no build tag and no deployment in which
// this software reaches another installation, so there is nothing to turn on and
// nothing that a flag being off is holding back.
//
// What this reads. Every reference to a standard-library name that starts a
// connection or asks a resolver a question, in any position: a call, the type of
// a composite literal, a struct field, a value taken by name. It reads the
// syntax tree rather than the text, so the words in this comment are not
// findings and a call written across two lines is one.
//
// What it does not read, said here rather than discovered by somebody whose
// change passed:
//
//   - Accepting a connection. A listener is how a client reaches this service
//     and nothing crosses outward when one is accepted, so net.Listen and what
//     follows from it are not refused. The rule is about origination.
//   - A dependency. Nothing outside this module is refused by name, because the
//     graph is empty and a list written against an empty graph is a list written
//     against nothing. A library that dials on this package's behalf is outside
//     what this reads, and the import rules in internal/arch are what stand
//     between a package and a library it should not have.
//   - Starting a process. os/exec can reach anything this rule refuses, and it
//     is not named here, because refusing it is a different rule about a
//     different failure and it belongs to whatever issue wants it.
//   - The inbound half. The decision's own sentence is about the place an
//     identity asserted by another installation would enter, and it refuses
//     because no route accepts one. An absent route is an absence, and no
//     reading of the syntax tree finds an absence. That half stays prose until
//     there is an admission path for a test to drive, which is #35 and #36, and
//     it is the second condition of #104 rather than this one.
package boundary

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
)

// Place is the one directory whose files may originate a connection, as a path
// from the repository root with forward slashes. It is a directory rather than a
// file so that the seam can grow a second file without this constant becoming a
// list, and it is one entry because a second entry is the whole rule gone.
const Place = "internal/boundary"

// outboundPackages are refused on the import, because every exported name in
// them speaks to something that is not this process. Naming the package is
// enough and naming each function would be a list that drifts against the
// standard library.
var outboundPackages = map[string]string{
	"net/smtp": "sends mail to a server somewhere else",
	"net/rpc":  "calls a procedure in another process",
}

// outboundNames are refused on the reference rather than on the import, because
// the packages holding them are also how this service is reached. net is
// imported by anything that listens, and net/http by anything that serves.
//
// Each entry is the name as it is written after the package. A value is listed
// beside a function where taking the value is the whole act: http.DefaultClient
// is a client somebody is about to use, and net.Dialer is one they are about to
// build.
var outboundNames = map[string]map[string]bool{
	"net": {
		"Dial": true, "DialTimeout": true, "DialIP": true, "DialTCP": true,
		"DialUDP": true, "DialUnix": true, "Dialer": true,
		"Resolver": true, "DefaultResolver": true,
		"LookupHost": true, "LookupIP": true, "LookupAddr": true,
		"LookupCNAME": true, "LookupMX": true, "LookupNS": true,
		"LookupPort": true, "LookupSRV": true, "LookupTXT": true,
	},
	"net/http": {
		"Get": true, "Post": true, "Head": true, "PostForm": true,
		"Client": true, "DefaultClient": true,
		"Transport": true, "DefaultTransport": true,
	},
	"crypto/tls": {
		"Dial": true, "DialWithDialer": true, "Client": true,
	},
}

// A Finding is one refusal, with enough in it to fix the code without opening
// this file.
type Finding struct {
	Path   string // from the repository root, forward slashes
	Line   int
	Detail string
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Detail) }

// CheckFile refuses what src does at path. path is from the repository root with
// forward slashes, and it is what decides whether the file is the place.
//
// The rule covers _test.go files, for the reason internal/arch gives about its
// own: the realistic way this repository acquires an outbound path is a test
// that wants to prove something end to end, and a rule stopping at the
// production files would pass the case it exists for.
func CheckFile(path string, src []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	if isThePlace(path) {
		return nil, nil
	}

	at := func(pos token.Pos) int { return fset.Position(pos).Line }
	var findings []Finding

	// names maps what a package is called in this file to the import path it
	// came from, so a renamed import is refused for what it is rather than for
	// what it is spelled.
	names := map[string]string{}
	for _, imp := range file.Imports {
		imported, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: unreadable import %s", path, imp.Path.Value)
		}
		if why, refused := outboundPackages[imported]; refused {
			findings = append(findings, Finding{
				Path: path, Line: at(imp.Pos()),
				Detail: fmt.Sprintf(
					"imports %s, which %s; a connection out of this process is made in %s and nowhere else (docs/decisions/federation.md, where the boundary lives)",
					imported, why, Place),
			})
			continue
		}
		if _, watched := outboundNames[imported]; !watched {
			continue
		}
		name := imported[strings.LastIndex(imported, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		// A blank or dot import gives no name to write in front of a call, so
		// there is nothing for the reference rule below to match on. The blank
		// one takes no name at all and the dot one puts every name into this
		// file's scope, which is a shape this repository does not use and which
		// this rule would silently pass, so it is refused where it is written.
		if name == "_" || name == "." {
			findings = append(findings, Finding{
				Path: path, Line: at(imp.Pos()),
				Detail: fmt.Sprintf(
					"imports %s under %q, which leaves nothing for this rule to read; write the ordinary import so that a connection out of this process can be told from a listener (docs/decisions/federation.md, where the boundary lives)",
					imported, name),
			})
			continue
		}
		names[name] = imported
	}

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		imported, watched := names[pkg.Name]
		if !watched || !outboundNames[imported][sel.Sel.Name] {
			return true
		}
		findings = append(findings, Finding{
			Path: path, Line: at(sel.Pos()),
			Detail: fmt.Sprintf(
				"names %s.%s, which starts a connection out of this process; that is made in %s and nowhere else, so that what this software talks to is one directory to read (docs/decisions/federation.md, where the boundary lives)",
				imported, sel.Sel.Name, Place),
		})
		return true
	})

	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Line < findings[j].Line })
	return findings, nil
}

// isThePlace answers whether path is a file directly in Place. Directly, rather
// than anywhere beneath it, because a subdirectory is a package of its own and
// would inherit an exemption nobody argued for.
func isThePlace(path string) bool {
	return strings.LastIndex(path, "/") == len(Place) && strings.HasPrefix(path, Place+"/")
}

// CheckTree refuses everything under root. Directories whose name begins with a
// dot are not read, and neither is anything that is not a .go file.
func CheckTree(root string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := CheckFile(filepath.ToSlash(rel), src)
		if err != nil {
			return err
		}
		findings = append(findings, f...)
		return nil
	})
	return findings, err
}
