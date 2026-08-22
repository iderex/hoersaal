// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package secret

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// The secret every test below prints. It is written out rather than generated,
// for the reason internal/roomcred's suite gives for its own keys: a suite that
// makes its own needs a source of randomness, and this package neither has one
// nor needs one. It is thirty-two bytes, which is the length the one secret in
// the tree today is refused below.
var key = Bytes("swordfish-0123456789abcdef-key!!")

// The verbs. Every one fmt offers for a slice of bytes, plus the two that carry
// a width and a flag, because a verb with a flag takes a different path through
// fmt than the bare one and a Formatter that answered only the bare form would
// pass a suite that never wrote a flag.
var verbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%08x", "%-20v", "%.4s"}

// spellings is every way the bytes of key could arrive in a string. A test that
// looked only for the bytes themselves would pass on a hex dump of them, which
// is what %x writes and what half of a bug report is made of.
func spellings(b []byte) map[string]string {
	decimal := make([]string, 0, len(b))
	for _, c := range b {
		decimal = append(decimal, strconv.Itoa(int(c)))
	}
	return map[string]string{
		"the bytes themselves":  string(b),
		"lowercase hex":         hex.EncodeToString(b),
		"uppercase hex":         strings.ToUpper(hex.EncodeToString(b)),
		"standard base64":       base64.StdEncoding.EncodeToString(b),
		"base64url":             base64.RawURLEncoding.EncodeToString(b),
		"a list of byte values": strings.Join(decimal, " "),
	}
}

// revealed names the spelling found in out, or is empty. It is a name rather
// than a boolean so a failure says which spelling leaked rather than only that
// one did.
func revealed(out string, b []byte) string {
	for name, spelling := range spellings(b) {
		// A single byte spelled as a decimal number appears in unrelated
		// output by chance, so the shortest spelling is only evidence when
		// enough of it is present to be the secret rather than a coincidence.
		if len(spelling) >= 8 && strings.Contains(out, spelling) {
			return name
		}
	}
	return ""
}

func TestEveryVerbPrintsThePlaceholder(t *testing.T) {
	for _, verb := range verbs {
		got := fmt.Sprintf(verb, key)
		if got != Placeholder {
			t.Errorf("%s of a secret is %q, want %q", verb, got, Placeholder)
		}
	}
}

func TestNoVerbRevealsTheSecret(t *testing.T) {
	for _, verb := range verbs {
		out := fmt.Sprintf(verb, key)
		if name := revealed(out, key); name != "" {
			t.Errorf("%s of a secret contains it as %s: %q", verb, name, out)
		}
	}
}

// The unverbed printers take a different path through fmt from Sprintf, so they
// are asserted rather than assumed to follow from the verbs above.
func TestThePrintersWithoutAVerbDoNotRevealIt(t *testing.T) {
	for name, out := range map[string]string{
		"Sprint":           fmt.Sprint(key),
		"Sprintln":         fmt.Sprintln(key),
		"Sprint of a pair": fmt.Sprint(key, key),
	} {
		if !strings.Contains(out, Placeholder) {
			t.Errorf("%s of a secret is %q, want it to carry %q", name, out, Placeholder)
		}
		if leaked := revealed(out, key); leaked != "" {
			t.Errorf("%s of a secret contains it as %s: %q", name, leaked, out)
		}
	}
}

func TestTheEncodingsProduceThePlaceholder(t *testing.T) {
	text, err := key.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if string(text) != Placeholder {
		t.Errorf("MarshalText wrote %q, want %q", text, Placeholder)
	}

	// Inside a struct with an exported field, which is the shape a diagnostic
	// bundle is written from and the shape encoding/json's own rule for a slice
	// of bytes would otherwise apply to.
	out, err := json.Marshal(struct {
		Signing Bytes `json:"signing"`
	}{key})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if want := `{"signing":"` + Placeholder + `"}`; string(out) != want {
		t.Errorf("json.Marshal wrote %s, want %s", out, want)
	}
	if name := revealed(string(out), key); name != "" {
		t.Errorf("json.Marshal of a secret contains it as %s: %s", name, out)
	}
}

func TestRevealReturnsTheBytes(t *testing.T) {
	got := key.Reveal()
	if string(got) != "swordfish-0123456789abcdef-key!!" {
		t.Errorf("Reveal returned %d bytes that are not the secret", len(got))
	}
	if key.Len() != len(got) {
		t.Errorf("Len is %d and Reveal returned %d bytes", key.Len(), len(got))
	}
}

// The check that the spelling helper above is not vacuous. Every leak test in
// this file passes if revealed never finds anything, so revealed is asked to
// find each spelling in a string that plainly holds it.
func TestTheLeakDetectorFindsEverySpelling(t *testing.T) {
	for name, spelling := range spellings(key) {
		if got := revealed("prefix "+spelling+" suffix", key); got == "" {
			t.Errorf("a string carrying the secret as %s was read as carrying nothing", name)
		}
	}
	if got := revealed("nothing of the sort", key); got != "" {
		t.Errorf("a string carrying nothing was read as carrying %s", got)
	}
}
