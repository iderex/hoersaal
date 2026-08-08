// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fuzzing

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/roomcred"
	"github.com/iderex/hoersaal/internal/wire"
)

// fuzzConference is the conference every credential target verifies against. It
// is a constant rather than a fuzzed value because the question these targets
// ask is what a stranger's bytes do, and the conference is not a stranger's to
// choose.
const fuzzConference = "lecture-hall-1"

// fuzzKey is the signing key the credential targets use. It is a fixture rather
// than a secret: nothing outside this file has it, and a key read from a
// generator would make a failing input impossible to reproduce, which is the one
// thing a fuzz fixture has to be.
var fuzzKey = bytes.Repeat([]byte("hoersaal-fuzz-key"), 4)

// fuzzNow is the moment the credential targets are judged at. Fixed, because a
// target reading the machine's clock finds inputs that fail on one afternoon and
// pass on the next, and this repository refuses a direct clock read for that
// reason.
var fuzzNow = time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

// FuzzSignallingEnvelope is the first thing an unauthenticated stranger
// touches. The bytes are one message, exactly as they arrive off the transport.
//
// What it asserts beyond not panicking is deliberately narrow. Whether a
// decoded message round-trips, and which inputs are refused for which reason,
// are the properties on issue #41 and are asserted there; repeating them here
// would make one defect two failures in two places. What is left is what only a
// fuzzer finds: a panic, an unbounded read, or a decode that succeeds while
// leaving the envelope in a state the package says it cannot be in.
func FuzzSignallingEnvelope(f *testing.F) {
	f.Add([]byte(`{"type":"join"}`))
	f.Add([]byte(`{"type":"join","data":{"conference":"lecture-hall-1"}}`))
	f.Add([]byte(`{"type":"join","type":"leave"}`))
	f.Add([]byte(`{"type":""}`))
	f.Add([]byte(`{"type":"join"} {"type":"join"}`))
	f.Add([]byte(`[{"type":"join"}]`))
	f.Add([]byte(`{"type":123}`))
	f.Add([]byte("\x00"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := wire.Decode(b)
		if err != nil {
			return
		}
		if m.Type == "" {
			t.Fatalf("decoded a message with no type from %q", b)
		}
		if len(m.Type) > wire.MaxTypeBytes {
			t.Fatalf("decoded a type of %d bytes, over the maximum of %d, from %q",
				len(m.Type), wire.MaxTypeBytes, b)
		}
		if len(b) > wire.MaxMessageBytes {
			t.Fatalf("decoded a message of %d bytes, over the maximum of %d",
				len(b), wire.MaxMessageBytes)
		}
	})
}

// FuzzRoomCredentialToken hands the verifier a token exactly as a stranger
// sends one: bytes, unsigned, arbitrary.
//
// The property is the one that matters at this door. A token this verifier
// accepts must name the conference it was asked about, and it must be inside
// its window. A success here at all would be an HMAC forgery, so the assertion
// is an oracle rather than a formality: if the fuzzer ever reaches it, the
// finding is the interesting one.
//
// The bound on this target is worth reading before its coverage figures are.
// Verify checks the signature before it reads anything inside the credential,
// so these bytes stop at the base64 decoding, the length checks and the
// comparison. The decoder behind that check is not reached from here at any
// duration, which is what the round-trip target below is for.
func FuzzRoomCredentialToken(f *testing.F) {
	issuer, err := roomcred.NewIssuer(fuzzKey)
	if err != nil {
		f.Fatalf("building the issuer: %v", err)
	}
	valid, err := issuer.Issue(roomcred.Claims{
		Conference: fuzzConference,
		Subject:    "somebody",
		Role:       "attendee",
		NotBefore:  fuzzNow.Add(-time.Minute),
		Expires:    fuzzNow.Add(time.Hour),
	})
	if err != nil {
		f.Fatalf("minting the seed credential: %v", err)
	}
	f.Add([]byte(valid))
	f.Add([]byte(""))
	f.Add([]byte("not base64url at all"))
	f.Add([]byte("AAAA"))
	f.Add(bytes.Repeat([]byte("A"), roomcred.MaxTokenBytes+1))

	verifier, err := roomcred.NewVerifier(fuzzKey, clock.NewTest(fuzzNow))
	if err != nil {
		f.Fatalf("building the verifier: %v", err)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		c, err := verifier.Verify(string(b), fuzzConference)
		if err != nil {
			return
		}
		if c.Conference != fuzzConference {
			t.Fatalf("a credential naming %q was accepted for %q", c.Conference, fuzzConference)
		}
		if fuzzNow.Before(c.NotBefore) || fuzzNow.After(c.Expires) {
			t.Fatalf("a credential valid from %s to %s was accepted at %s",
				c.NotBefore, c.Expires, fuzzNow)
		}
	})
}

// FuzzRoomCredentialRoundTrip reaches the decoder the target above cannot,
// through the mint that a holder of the key would use, and it is not a stranger
// surface. The bytes become the three text fields of a credential, which is
// where a length, an encoding or a separator is decided.
//
// The property is that a credential says what it was minted to say. A mint and
// a verify that disagree is the defect this target exists for, and it is the
// one that would let a name, a role or a conference change on the way through.
func FuzzRoomCredentialRoundTrip(f *testing.F) {
	f.Add([]byte("lecture-hall-1\x00somebody\x00attendee"))
	f.Add([]byte("\x00\x00"))
	f.Add([]byte("a\x00b\x00c"))
	f.Add(bytes.Repeat([]byte("x"), roomcred.MaxFieldBytes*3))

	issuer, err := roomcred.NewIssuer(fuzzKey)
	if err != nil {
		f.Fatalf("building the issuer: %v", err)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		conference, subject, role := three(b)
		claims := roomcred.Claims{
			Conference: conference,
			Subject:    subject,
			Role:       role,
			NotBefore:  fuzzNow.Add(-time.Minute),
			Expires:    fuzzNow.Add(time.Hour),
		}
		token, err := issuer.Issue(claims)
		if err != nil {
			// Refused at the mint, which is the issuer's own rule about what it
			// will sign, and not a defect.
			if errors.Is(err, roomcred.ErrClaims) {
				return
			}
			t.Fatalf("minting %q refused for a reason the issuer does not declare: %v", b, err)
		}

		verifier, err := roomcred.NewVerifier(fuzzKey, clock.NewTest(fuzzNow))
		if err != nil {
			t.Fatalf("building the verifier: %v", err)
		}
		got, err := verifier.Verify(token, conference)
		if err != nil {
			t.Fatalf("a credential this key minted did not verify: %v", err)
		}
		if got.Conference != conference || got.Subject != subject || got.Role != role {
			t.Fatalf("minted (%q, %q, %q) and verified (%q, %q, %q)",
				conference, subject, role, got.Conference, got.Subject, got.Role)
		}
	})
}

// three splits the fuzzer's bytes into the three text fields, on the first two
// zero bytes. A separator the fuzzer can also produce inside a field is the
// point: a credential whose fields survive one containing the separator is the
// case a hand-written test would not have thought to write.
func three(b []byte) (string, string, string) {
	parts := bytes.SplitN(b, []byte{0}, 3)
	for len(parts) < 3 {
		parts = append(parts, nil)
	}
	return string(parts[0]), string(parts[1]), string(parts[2])
}
