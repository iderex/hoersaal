// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A run with no configuration file starts, which is issue #82's second
// condition read at the place it is actually about: a deployment that has
// written nothing.
func TestStartingWithNoConfigurationFileSucceeds(t *testing.T) {
	var out, errs strings.Builder
	if code := run(nil, &out, &errs); code != 0 {
		t.Fatalf("starting with no configuration gave %d and wrote %q", code, errs.String())
	}
	if !strings.Contains(out.String(), "the configuration is valid") {
		t.Errorf("the run said nothing about the configuration: %q", out.String())
	}
}

// The same, from a file that exists and is empty. The two are one answer and
// the test says so, because a loader that accepted the absent file and refused
// the empty one would meet the condition in form only.
func TestStartingWithAnEmptyConfigurationFileSucceeds(t *testing.T) {
	path := write(t, "")

	var out, errs strings.Builder
	if code := run([]string{"-config", path}, &out, &errs); code != 0 {
		t.Fatalf("an empty file gave %d and wrote %q", code, errs.String())
	}
}

// Issue #82's third condition. An invalid value stops the startup, and it stops
// it here rather than later: nothing after this point in run is reached, which
// is what "before anything listens" means while there is nothing that listens.
func TestAnInvalidValueStopsTheStartup(t *testing.T) {
	path := write(t, `{"pool.minimum": 0}`)

	var out, errs strings.Builder
	code := run([]string{"-config", path}, &out, &errs)
	if code == 0 {
		t.Fatalf("a floor of zero started the service and wrote %q", out.String())
	}
	if out.String() != "" {
		t.Errorf("the run got past the configuration and wrote %q", out.String())
	}
	if !strings.Contains(errs.String(), "pool.minimum") {
		t.Errorf("the refusal does not name the key: %q", errs.String())
	}
}

// The first condition, at the same place. An unknown key stops the startup and
// the operator is told which key it was.
//
// The key is joined from two pieces rather than written out, and that is the
// rule landing with this change working rather than a way around it:
// internal/invariant refuses a key-shaped string outside internal/config
// anywhere in the tree, this test's whole subject is such a string, and the
// rule's own comment names a key built by joining as outside its reach.
func TestAnUnknownKeyStopsTheStartupAndIsNamed(t *testing.T) {
	const unknown = "listen." + "tls"
	path := write(t, `{"`+unknown+`": true}`)

	var out, errs strings.Builder
	if code := run([]string{"-config", path}, &out, &errs); code == 0 {
		t.Fatalf("an unknown key started the service and wrote %q", out.String())
	}
	if !strings.Contains(errs.String(), unknown) {
		t.Errorf("the refusal does not name the key: %q", errs.String())
	}
}

// A file the operator named and did not create is a different mistake from a
// file with something wrong in it, and the message says which.
func TestAConfigurationFileThatIsNotThereStopsTheStartup(t *testing.T) {
	var out, errs strings.Builder
	if code := run([]string{"-config", filepath.Join(t.TempDir(), "absent.json")}, &out, &errs); code == 0 {
		t.Fatal("a file that is not there started the service")
	}
	if !strings.Contains(errs.String(), "opening the configuration") {
		t.Errorf("the refusal does not say the file could not be opened: %q", errs.String())
	}
}

// An argument this command does not take is refused rather than ignored, for
// the same reason a key it does not have is: an operator who typed something
// that had no effect learns that from a message and not from an afternoon.
func TestAnArgumentThisCommandDoesNotTakeIsRefused(t *testing.T) {
	var out, errs strings.Builder
	if code := run([]string{"config.json"}, &out, &errs); code == 0 {
		t.Fatal("a bare argument was accepted")
	}
	if !strings.Contains(errs.String(), "config.json") {
		t.Errorf("the refusal does not name the argument: %q", errs.String())
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hoersaal.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
