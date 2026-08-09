// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"strings"
	"testing"
)

// TestIsObjectNameAcceptsWhatGitWritesAndNothingElse holds the shape check to
// both directions. The accepted cases are the two lengths git writes; the
// refused ones are each a single character away from an accepted one, because
// the mistake this exists against is a value that looks close enough to pass a
// glance.
func TestIsObjectNameAcceptsWhatGitWritesAndNothingElse(t *testing.T) {
	sha1 := strings.Repeat("a", 40)
	sha256 := strings.Repeat("0", 64)

	for _, s := range []string{sha1, sha256, "0123456789abcdef" + strings.Repeat("0", 24)} {
		if !isObjectName(s) {
			t.Errorf("%q is an object name and was refused", s)
		}
	}

	for name, s := range map[string]string{
		"empty":                         "",
		"one short":                     strings.Repeat("a", 39),
		"one long":                      strings.Repeat("a", 41),
		"upper case":                    strings.Repeat("A", 40),
		"not hexadecimal":               strings.Repeat("g", 40),
		"a leading dash":                "-" + strings.Repeat("a", 39),
		"an option of the right length": "--all" + strings.Repeat("a", 35),
		"a ref name":                    "refs/heads/main",
	} {
		if isObjectName(s) {
			t.Errorf("%s: %q was accepted as an object name", name, s)
		}
	}
}

// TestCommitRangeRefusesAnEndThatIsNotAnObjectName is the guard at the site
// that matters: nothing reaches git until both ends have passed. It runs no git
// command, because the refusal happens before one is built, and that is the
// property being asserted rather than a convenience of the test.
//
// The error names the environment variable rather than only the value, since
// the person reading it is looking at a workflow and not at this file.
func TestCommitRangeRefusesAnEndThatIsNotAnObjectName(t *testing.T) {
	good := strings.Repeat("a", 40)

	for _, c := range []struct {
		base, head, names string
	}{
		{"--all", good, "PR_BASE_SHA"},
		{good, "--all", "PR_HEAD_SHA"},
		{"", good, "PR_BASE_SHA"},
		{good, "refs/heads/main", "PR_HEAD_SHA"},
	} {
		commits, err := commitRange(c.base, c.head)
		if err == nil {
			t.Errorf("commitRange(%q, %q) returned %d commit(s) and no error; the end that is not an object name should have been refused", c.base, c.head, len(commits))
			continue
		}
		if !strings.Contains(err.Error(), c.names) {
			t.Errorf("commitRange(%q, %q) refused with %q, which does not name %s", c.base, c.head, err, c.names)
		}
	}
}
