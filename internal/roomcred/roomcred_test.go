// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package roomcred

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/secret"
)

// Two keys of the minimum length, written out rather than generated, because a
// suite that makes its own keys needs a source of randomness and this package
// deliberately has none. They are test material and nothing signs anything real
// with them.
var (
	keyA = []byte("0123456789abcdef0123456789abcdef")
	keyB = []byte("fedcba9876543210fedcba9876543210")
)

// base is the instant every test starts from. time.Date is arithmetic on the
// calendar and reads no clock, which is what the guard package refuses.
var base = time.Date(2026, time.August, 6, 9, 0, 0, 0, time.UTC)

// lecture is the window a credential for a lecture starting at base gets: it
// opens fifteen minutes early and closes two hours later.
func lecture() Claims {
	return Claims{
		Conference: "conf-7f3a",
		Subject:    "part-91c2",
		Role:       "attendee",
		NotBefore:  base.Add(-15 * time.Minute),
		Expires:    base.Add(2 * time.Hour),
	}
}

func issuer(t *testing.T, key []byte) *Issuer {
	t.Helper()
	i, err := NewIssuer(key)
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	return i
}

func verifier(t *testing.T, key []byte, c clock.Clock) *Verifier {
	t.Helper()
	v, err := NewVerifier(key, c)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func TestACredentialRoundTripsThroughItsOwnVerifier(t *testing.T) {
	want := lecture()
	token, err := issuer(t, keyA).Issue(want)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := verifier(t, keyA, clock.NewTest(base)).Verify(token, want.Conference)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Conference != want.Conference || got.Subject != want.Subject || got.Role != want.Role {
		t.Errorf("claims came back as %+v, want %+v", got, want)
	}
	if !got.NotBefore.Equal(want.NotBefore) || !got.Expires.Equal(want.Expires) {
		t.Errorf("window came back as %s..%s, want %s..%s", got.NotBefore, got.Expires, want.NotBefore, want.Expires)
	}
}

// A credential travels in a link, so it has to survive being in a URL without
// being re-encoded. base64url without padding is what makes that true, and this
// asserts the alphabet rather than trusting the encoder.
func TestACredentialIsSafeToPutInALink(t *testing.T) {
	token, err := issuer(t, keyA).Issue(lecture())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	if i := strings.IndexFunc(token, func(r rune) bool { return !strings.ContainsRune(alphabet, r) }); i >= 0 {
		t.Errorf("token carries %q at %d, which is outside the base64url alphabet", token[i], i)
	}
}

func TestAKeyShorterThanTheMinimumIsRefusedOnBothSides(t *testing.T) {
	short := make([]byte, MinKeyBytes-1)

	if _, err := NewIssuer(short); !errors.Is(err, ErrKeyTooShort) {
		t.Errorf("NewIssuer with %d bytes returned %v, want ErrKeyTooShort", len(short), err)
	}
	if _, err := NewVerifier(short, clock.NewTest(base)); !errors.Is(err, ErrKeyTooShort) {
		t.Errorf("NewVerifier with %d bytes returned %v, want ErrKeyTooShort", len(short), err)
	}
	if _, err := NewIssuer(keyA); err != nil {
		t.Errorf("NewIssuer with %d bytes returned %v, want it accepted", len(keyA), err)
	}
}

func TestAVerifierWithoutAClockIsRefused(t *testing.T) {
	if _, err := NewVerifier(keyA, nil); err == nil {
		t.Error("NewVerifier accepted a nil clock; such a verifier cannot refuse an expired credential")
	}
}

func TestClaimsThatCannotBeMintedAreRefused(t *testing.T) {
	long := strings.Repeat("x", MaxFieldBytes+1)

	cases := map[string]func(Claims) Claims{
		"no conference":                     func(c Claims) Claims { c.Conference = ""; return c },
		"no role":                           func(c Claims) Claims { c.Role = ""; return c },
		"conference over the field maximum": func(c Claims) Claims { c.Conference = long; return c },
		"subject over the field maximum":    func(c Claims) Claims { c.Subject = long; return c },
		"role over the field maximum":       func(c Claims) Claims { c.Role = long; return c },
		// Each of these two leaves the other end of the window in a shape
		// nothing else objects to, so the missing end is the only thing being
		// refused. A pair of zero times would be refused by the case below it
		// instead, and would say nothing about this one.
		"no start": func(c Claims) Claims {
			c.NotBefore = time.Time{}
			c.Expires = time.Time{}.Add(time.Hour)
			return c
		},
		"no end": func(c Claims) Claims {
			c.NotBefore = time.Time{}.Add(-time.Hour)
			c.Expires = time.Time{}
			return c
		},
		"ends before it opens": func(c Claims) Claims { c.Expires = c.NotBefore.Add(-time.Second); return c },
		"ends when it opens":   func(c Claims) Claims { c.Expires = c.NotBefore; return c },
		"longer than the maximum lifetime": func(c Claims) Claims {
			c.Expires = c.NotBefore.Add(MaxLifetime + time.Second)
			return c
		},
	}

	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := issuer(t, keyA).Issue(spoil(lecture())); !errors.Is(err, ErrClaims) {
				t.Errorf("Issue returned %v, want ErrClaims", err)
			}
		})
	}

	// The window one second under the maximum is minted, so the case above is
	// the maximum biting rather than every long window being refused.
	c := lecture()
	c.Expires = c.NotBefore.Add(MaxLifetime)
	if _, err := issuer(t, keyA).Issue(c); err != nil {
		t.Errorf("Issue refused a window of exactly the maximum: %v", err)
	}
}

func TestACredentialForAnotherConferenceIsRefused(t *testing.T) {
	token, err := issuer(t, keyA).Issue(lecture())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	_, err = verifier(t, keyA, clock.NewTest(base)).Verify(token, "conf-0000")
	if !errors.Is(err, ErrWrongConference) {
		t.Errorf("Verify against another conference returned %v, want ErrWrongConference", err)
	}
}

// Verify takes the conference rather than returning the claims for the caller
// to compare, so a caller cannot forget. Asking with no conference at all is
// the shape that would slip past that, and it is refused.
func TestVerifyWithoutAConferenceIsRefused(t *testing.T) {
	token, err := issuer(t, keyA).Issue(lecture())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if _, err := verifier(t, keyA, clock.NewTest(base)).Verify(token, ""); !errors.Is(err, ErrClaims) {
		t.Errorf("Verify with no conference returned %v, want ErrClaims", err)
	}
}

func TestACredentialIsRefusedBeforeItsWindowAndAfterIt(t *testing.T) {
	c := lecture()
	token, err := issuer(t, keyA).Issue(c)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// A second before the window opens.
	early := clock.NewTest(c.NotBefore.Add(-time.Second))
	if _, err := verifier(t, keyA, early).Verify(token, c.Conference); !errors.Is(err, ErrNotYetValid) {
		t.Errorf("Verify a second early returned %v, want ErrNotYetValid", err)
	}

	// The same clock moved to the far end of the window and one second past it.
	late := clock.NewTest(c.NotBefore)
	late.Advance(c.Lifetime() + time.Second)
	if _, err := verifier(t, keyA, late).Verify(token, c.Conference); !errors.Is(err, ErrExpired) {
		t.Errorf("Verify a second late returned %v, want ErrExpired", err)
	}
}

func TestACredentialIsUsableAtBothEndsOfItsWindow(t *testing.T) {
	c := lecture()
	token, err := issuer(t, keyA).Issue(c)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	for name, at := range map[string]time.Time{
		"the instant it opens": c.NotBefore,
		"the instant it ends":  c.Expires,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier(t, keyA, clock.NewTest(at)).Verify(token, c.Conference); err != nil {
				t.Errorf("Verify at %s returned %v, want it accepted", at, err)
			}
		})
	}
}

func TestAnotherKeyDoesNotVerify(t *testing.T) {
	token, err := issuer(t, keyA).Issue(lecture())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	_, err = verifier(t, keyB, clock.NewTest(base)).Verify(token, lecture().Conference)
	if !errors.Is(err, ErrSignature) {
		t.Errorf("Verify under another key returned %v, want ErrSignature", err)
	}
}

// Every byte, not one byte. A signature that covered all of the payload except
// the version, or except the window, would pass a test that only tampered with
// the conference.
func TestChangingAnyByteOfACredentialRefusesIt(t *testing.T) {
	c := lecture()
	token, err := issuer(t, keyA).Issue(c)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding what Issue produced: %v", err)
	}

	v := verifier(t, keyA, clock.NewTest(base))
	for i := range raw {
		spoilt := append([]byte(nil), raw...)
		spoilt[i] ^= 0x01
		_, err := v.Verify(base64.RawURLEncoding.EncodeToString(spoilt), c.Conference)
		if !errors.Is(err, ErrSignature) {
			t.Errorf("flipping a bit of byte %d of %d returned %v, want ErrSignature", i, len(raw), err)
		}
	}
}

func TestATokenThatIsNotACredentialIsRefused(t *testing.T) {
	v := verifier(t, keyA, clock.NewTest(base))

	cases := map[string]string{
		"empty":                    "",
		"not base64url":            "not a token!!",
		"padded base64":            base64.StdEncoding.EncodeToString(make([]byte, 64)),
		"shorter than a signature": base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
		"exactly a signature":      base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		// Four over rather than one over. base64url refuses a length of one
		// more than a multiple of four on its own, so a token one byte over the
		// maximum would be refused by the decoder whether or not the size was
		// checked, and would say nothing about the size check.
		"longer than the maximum": strings.Repeat("A", MaxTokenBytes+4),
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(token, "conf-7f3a"); !errors.Is(err, ErrMalformed) {
				t.Errorf("Verify returned %v, want ErrMalformed", err)
			}
		})
	}
}

// The size is refused before the bytes are decoded, so a stranger cannot make
// this process do work proportional to what they sent. A token of the maximum
// length still reaches the decoder, which is what tells the two apart.
func TestATokenOfTheMaximumLengthIsStillRead(t *testing.T) {
	v := verifier(t, keyA, clock.NewTest(base))
	token := strings.Repeat("A", MaxTokenBytes)

	if _, err := v.Verify(token, "conf-7f3a"); !errors.Is(err, ErrSignature) {
		t.Errorf("a token of exactly the maximum returned %v, want it read and refused as ErrSignature", err)
	}
}

// Nothing inside a credential is read until this key's signature verifies. The
// case is a token that is both forged and expired: an implementation that read
// the window first would return ErrExpired and would be telling a forger which
// of their guesses was closest.
func TestTheSignatureIsCheckedBeforeAnythingInsideIsRead(t *testing.T) {
	c := lecture()
	c.Expires = base.Add(-time.Hour)
	c.NotBefore = base.Add(-2 * time.Hour)

	forged := base64.RawURLEncoding.EncodeToString(append(encode(c), make([]byte, 32)...))
	if _, err := verifier(t, keyA, clock.NewTest(base)).Verify(forged, c.Conference); !errors.Is(err, ErrSignature) {
		t.Errorf("a forged and expired token returned %v, want ErrSignature", err)
	}
}

// A payload this build cannot read, signed by the right key. These are the
// cases the length prefixes exist for, and they are reachable only by somebody
// holding the key, which is why they are a bug rather than an attack.
func TestASignedPayloadThisBuildCannotReadIsRefused(t *testing.T) {
	good := encode(lecture())

	field := func(s string) []byte {
		// #nosec G115 -- every string handed to this helper is written a few
		// lines below and none is longer than a sentence.
		out := binary.BigEndian.AppendUint16(nil, uint16(len(s)))
		return append(out, s...)
	}
	window := func() []byte {
		// #nosec G115 -- base is a fixed instant in this file, so both counts
		// of seconds are known at the time the test is read.
		out := binary.BigEndian.AppendUint64(nil, uint64(base.Unix()))
		// #nosec G115 -- the other end of the same fixed window.
		return binary.BigEndian.AppendUint64(out, uint64(base.Add(time.Hour).Unix()))
	}
	// A field whose length says more bytes follow than actually do.
	overrun := []byte{Version}
	overrun = append(overrun, binary.BigEndian.AppendUint16(nil, 8)...)
	overrun = append(overrun, "abc"...)

	// A field longer than a credential may carry, which encode cannot produce
	// because Issue refuses it, so the decoder is the second place it is
	// refused rather than the only one. The rest of this payload is well formed
	// on purpose: a truncated one would be refused by the length checks below
	// and would say nothing about the maximum.
	oversize := []byte{Version}
	oversize = append(oversize, field(strings.Repeat("x", MaxFieldBytes+1))...)
	oversize = append(oversize, field("")...)
	oversize = append(oversize, field("attendee")...)
	oversize = append(oversize, window()...)

	cases := map[string][]byte{
		"empty":                       {},
		"an unknown version":          append([]byte{Version + 1}, good[1:]...),
		"a field length past the end": overrun,
		"a field over the maximum":    oversize,
		"nothing after the version":   {Version},
		"a window of the wrong size": func() []byte {
			out := []byte{Version}
			out = append(out, field("conf-7f3a")...)
			out = append(out, field("")...)
			out = append(out, field("attendee")...)
			return append(out, window()[:12]...)
		}(),
		"bytes after the window": append(append([]byte(nil), good...), 0x00),
	}

	v := verifier(t, keyA, clock.NewTest(base))
	i := issuer(t, keyA)
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			token := base64.RawURLEncoding.EncodeToString(append(append([]byte(nil), payload...), sign(i.key, payload)...))
			_, err := v.Verify(token, "conf-7f3a")
			if !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrUnknownVersion) {
				t.Errorf("Verify returned %v, want ErrMalformed or ErrUnknownVersion", err)
			}
		})
	}
}

// decode called with nothing at all. This is the one refusal in the package
// that Verify cannot reach: Verify has already refused anything at or under the
// size of a signature by the time it calls decode, so the payload it hands over
// always holds at least the version byte. The case is here rather than left
// unproven because decode is what a second credential layout would be added to,
// and the guard is what stops that change from turning an empty payload into a
// panic.
func TestDecodeRefusesAnEmptyPayload(t *testing.T) {
	if _, err := decode(nil); !errors.Is(err, ErrMalformed) {
		t.Errorf("decode(nil) returned %v, want ErrMalformed", err)
	}
}

// A credential in a link sent to a group names nobody. That case has to work,
// because refusing it would push an installation into putting something
// identifying in there instead.
func TestACredentialWithNoSubjectIsMintedAndRead(t *testing.T) {
	c := lecture()
	c.Subject = ""

	token, err := issuer(t, keyA).Issue(c)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := verifier(t, keyA, clock.NewTest(base)).Verify(token, c.Conference)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Subject != "" {
		t.Errorf("subject came back as %q, want it empty", got.Subject)
	}
}

// The window is carried as whole seconds, so a caller handing in finer
// precision gets the second back rather than what it asked for. Stated here
// because it is the kind of difference that is otherwise found by a test
// elsewhere failing an equality it had no reason to doubt.
func TestTheWindowIsCarriedToTheSecond(t *testing.T) {
	c := lecture()
	c.NotBefore = c.NotBefore.Add(400 * time.Millisecond)
	c.Expires = c.Expires.Add(600 * time.Millisecond)

	token, err := issuer(t, keyA).Issue(c)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := verifier(t, keyA, clock.NewTest(base)).Verify(token, c.Conference)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !got.NotBefore.Equal(c.NotBefore.Truncate(time.Second)) {
		t.Errorf("NotBefore came back as %s, want %s", got.NotBefore, c.NotBefore.Truncate(time.Second))
	}
	if !got.Expires.Equal(c.Expires.Truncate(time.Second)) {
		t.Errorf("Expires came back as %s, want %s", got.Expires, c.Expires.Truncate(time.Second))
	}
}

// A role is a name here and nothing else, and the name is inside the signature.
// The pair of assertions is what says the role cannot be edited by the holder
// while still being carried faithfully.
func TestTheRoleIsCarriedAndCannotBeChangedByTheHolder(t *testing.T) {
	c := lecture()
	c.Role = "presenter"

	token, err := issuer(t, keyA).Issue(c)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	v := verifier(t, keyA, clock.NewTest(base))

	got, err := v.Verify(token, c.Conference)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Role != "presenter" {
		t.Errorf("role came back as %q, want %q", got.Role, "presenter")
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding what Issue produced: %v", err)
	}
	// "presenter" and "moderator" are the same length, so this edits the role
	// without moving anything else in the payload.
	edited := strings.Replace(string(raw), "presenter", "moderator", 1)
	if edited == string(raw) {
		t.Fatal("the role was not found in the token bytes, so this case is not testing what it claims")
	}
	if _, err := v.Verify(base64.RawURLEncoding.EncodeToString([]byte(edited)), c.Conference); !errors.Is(err, ErrSignature) {
		t.Errorf("a token with the role edited returned %v, want ErrSignature", err)
	}
}

// The key is held in an unexported field, which fmt prints by reflection
// without reaching the methods on secret.Bytes. So the two structs that hold it
// answer for themselves, and this is the test that says they do. It prints them
// as a value and as a pointer, because a method with a value receiver is
// reached through both and a reader should not have to know that.
func TestPrintingTheHolderOfTheKeyDoesNotRevealIt(t *testing.T) {
	i, err := NewIssuer(keyA)
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	v, err := NewVerifier(keyA, clock.NewTest(base))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// %d is the verb a Stringer does not cover, and it is the one that prints a
	// key as a list of numbers. %#v is the one a bug report is most often
	// written with.
	verbs := []string{"%v", "%+v", "%#v", "%s", "%d", "%x"}
	for _, subject := range []struct {
		name  string
		value any
	}{
		{"an issuer", *i},
		{"an issuer through a pointer", i},
		{"a verifier", *v},
		{"a verifier through a pointer", v},
	} {
		for _, verb := range verbs {
			out := fmt.Sprintf(verb, subject.value)
			if !strings.Contains(out, secret.Placeholder) {
				t.Errorf("%s of %s is %q, want it to carry %q", verb, subject.name, out, secret.Placeholder)
			}
			for name, spelling := range map[string]string{
				"the bytes themselves":  string(keyA),
				"lowercase hex":         hex.EncodeToString(keyA),
				"a list of byte values": strings.Trim(strings.Join(strings.Fields(fmt.Sprint([]byte(keyA))), " "), "[]"),
			} {
				if strings.Contains(out, spelling) {
					t.Errorf("%s of %s contains the key as %s: %q", verb, subject.name, name, out)
				}
			}
		}
	}
}
