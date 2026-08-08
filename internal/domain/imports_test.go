// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestTheModelDependsOnNothingButTheLanguage is the enforced half of the fourth
// condition on issue #29: the model has to compile with the transport, the
// storage and the media plane directories removed. Rather than asserting that
// about directories which do not exist yet, this reads every file of this package
// and refuses an import outside the standard library, which is the property that
// makes the removal safe whatever those directories end up being called.
//
// The rule is that the first element of an import path with a dot in it is not in
// the standard library, which is the same rule the toolchain uses. It covers the
// suite as well as the code, so a test that reached for another layer is refused
// too, and the reason is that a fixture from somewhere else is how a model's
// vocabulary quietly acquires a dependency on the thing it is supposed to be
// independent of.
//
// What it does not catch. It reads import paths, so anything reaching another
// layer without importing it, a linkname or a build-tagged file this parse skips,
// is outside it. It reads this directory only. The general form over the whole
// tree, boundary by boundary, is issue #98.
func TestTheModelDependsOnNothingButTheLanguage(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	examined := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", e.Name(), err)
		}
		examined++
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unreadable import %s", e.Name(), imp.Path.Value)
			}
			if strings.Contains(strings.Split(path, "/")[0], ".") {
				t.Errorf("%s imports %s; this package holds no transport, storage or media plane type and depends on nothing in this module", e.Name(), path)
			}
		}
	}
	// A green run over nothing examined would say the package is clean when it
	// was never read, which is the same shape as the count leg in the unit job.
	if examined == 0 {
		t.Fatal("no Go file in this directory was examined, so this test refused nothing")
	}
	t.Logf("files examined: %d", examined)
}
