// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package roomcred

import (
	"strings"
	"testing"

	"github.com/iderex/hoersaal/internal/clock"
)

// TestAFieldOfExactlyTheMaximumIsMintedAndRead pins the field maximum as
// inclusive on both sides of the credential. The suite already refuses one
// byte over it; nothing held the byte at it, and the mutation run on issue #93
// found that refusing a field of exactly MaxFieldBytes, at minting or at
// reading, left the suite green. A round trip is what closes both, because the
// verifier reads the same length the issuer wrote.
func TestAFieldOfExactlyTheMaximumIsMintedAndRead(t *testing.T) {
	edge := strings.Repeat("x", MaxFieldBytes)
	cases := map[string]func(*Claims){
		"conference": func(c *Claims) { c.Conference = edge },
		"subject":    func(c *Claims) { c.Subject = edge },
		"role":       func(c *Claims) { c.Role = edge },
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			want := lecture()
			set(&want)
			token, err := issuer(t, keyA).Issue(want)
			if err != nil {
				t.Fatalf("Issue refused a %s of exactly %d bytes: %v", name, MaxFieldBytes, err)
			}
			got, err := verifier(t, keyA, clock.NewTest(base)).Verify(token, want.Conference)
			if err != nil {
				t.Fatalf("Verify refused a %s of exactly %d bytes: %v", name, MaxFieldBytes, err)
			}
			if got.Conference != want.Conference || got.Subject != want.Subject || got.Role != want.Role {
				t.Errorf("claims came back as %+v, want %+v", got, want)
			}
		})
	}
}
