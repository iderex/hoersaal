// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package roomcred mints and verifies the credential that admits somebody to
// one conference, in one role, for a bounded time, and to nothing else.
//
// It answers the second half of issue #33. The first half, the accounts the
// people who run a room hold, is in
// docs/decisions/accounts-and-room-credentials.md together with every number
// this package refuses against and the reason for each.
//
// A room credential is a bearer credential. Whoever holds the bytes may use
// them, and that is a property of a link sent to three hundred people rather
// than a defect in this package. What bounds the damage is written into the
// bytes: the conference it admits to, the role it grants, and the window it is
// usable in. Verify is asked which conference is being entered and refuses a
// credential minted for another one, so a credential cannot be replayed into a
// different room by a caller that forgot to compare.
//
// Nothing in a credential identifies a person. Subject is an identifier this
// installation made up, and the document says why a name or an address may not
// go in: the bytes travel in a link, and a link is in a browser history, a
// referrer header and whatever log sits between the reader and the service.
//
// This package makes no key. It is handed one, which is why it imports no
// source of randomness and does not appear in the exception the guard package
// carries for internal/random. Where an operator's key comes from is issue #86
// and waits on there being a configuration to forbid it in. How it is held is
// answered: the key is a secret.Bytes, so neither it nor the two structs that
// hold it print it under any verb.
package roomcred

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
	"github.com/iderex/hoersaal/internal/secret"
)

// The numbers this package refuses against. Each one is argued in
// docs/decisions/accounts-and-room-credentials.md; the reason is not restated
// here, so the two cannot drift into two different values.
const (
	// Version is the one credential layout that exists. It is the first byte,
	// so a later layout is told apart before anything else is read.
	Version = 1

	// MinKeyBytes is the shortest key this package will sign or verify with.
	MinKeyBytes = 32

	// MaxTokenBytes bounds what a stranger may hand Verify before any of it is
	// decoded.
	MaxTokenBytes = 1024

	// MaxFieldBytes bounds each of the three text fields inside a credential.
	MaxFieldBytes = 128

	// MaxLifetime is the longest window Issue will mint.
	MaxLifetime = 12 * time.Hour
)

// The refusals, as values rather than as strings, because admission on issue
// #35 has to tell them apart to decide what the client is told. Everything
// below ErrSignature is only ever returned for a credential whose signature
// already verified, so a stranger holding a forgery learns nothing about the
// contents from which refusal came back.
var (
	// ErrKeyTooShort is a key shorter than MinKeyBytes.
	ErrKeyTooShort = errors.New("roomcred: key is shorter than the minimum")

	// ErrClaims is a credential that may not be minted as asked.
	ErrClaims = errors.New("roomcred: the claims cannot be minted")

	// ErrMalformed is a token that is not a credential at all: too long, not
	// base64url, too short to hold a signature, or with a field running past
	// the end of what was handed over.
	ErrMalformed = errors.New("roomcred: malformed token")

	// ErrUnknownVersion is a credential in a layout this build does not know.
	ErrUnknownVersion = errors.New("roomcred: unknown credential version")

	// ErrSignature is a credential this key did not sign, or one whose bytes
	// were changed after it did.
	ErrSignature = errors.New("roomcred: signature does not verify")

	// ErrWrongConference is a credential minted for another room.
	ErrWrongConference = errors.New("roomcred: credential is for another conference")

	// ErrNotYetValid is a credential whose window has not opened.
	ErrNotYetValid = errors.New("roomcred: credential is not yet valid")

	// ErrExpired is a credential whose window has closed.
	ErrExpired = errors.New("roomcred: credential has expired")
)

// Claims is what a credential says. Every field is carried in the signed bytes,
// so none of them can be changed by whoever holds the token.
type Claims struct {
	// Conference is the room this admits to, and nothing else.
	Conference string

	// Subject is who the credential was minted for, as an identifier this
	// installation made up. It is empty for a credential put into a link that
	// was sent to a group, and the document says what that costs.
	Subject string

	// Role is the name of the role granted. What powers a name bundles is
	// issue #34; this package carries the name and gives it no meaning.
	Role string

	// NotBefore and Expires bound the window, to the second. A credential is
	// usable at NotBefore and at Expires, and outside them it is not.
	NotBefore time.Time
	Expires   time.Time
}

// Lifetime is the window the claims ask for.
func (c Claims) Lifetime() time.Duration { return c.Expires.Sub(c.NotBefore) }

// An Issuer mints credentials with one key.
type Issuer struct{ key secret.Bytes }

// NewIssuer refuses a key shorter than MinKeyBytes. The key is copied, so a
// caller that reuses its buffer cannot change what this issuer signs with.
func NewIssuer(key secret.Bytes) (*Issuer, error) {
	if key.Len() < MinKeyBytes {
		return nil, fmt.Errorf("%w: %d bytes, minimum is %d", ErrKeyTooShort, key.Len(), MinKeyBytes)
	}
	return &Issuer{key: append(secret.Bytes(nil), key...)}, nil
}

// Format writes a placeholder for every verb.
//
// The key is an unexported field, and fmt prints an unexported field by
// reflection without reaching the methods on its type, because it may not take
// the value out to call one. So secret.Bytes protects itself and cannot protect
// the struct around it, and the struct around it says so here. Every verb
// rather than a String, for the reason secret.Format gives: fmt prints the
// fields themselves for the verbs a Stringer does not cover.
func (i Issuer) Format(f fmt.State, verb rune) {
	io.WriteString(f, "roomcred.Issuer{key: "+secret.Placeholder+"}")
}

// Issue mints a credential for c. It refuses claims it may not mint rather than
// minting something a verifier would then have to make sense of.
func (i *Issuer) Issue(c Claims) (string, error) {
	if c.Conference == "" {
		return "", fmt.Errorf("%w: no conference", ErrClaims)
	}
	if c.Role == "" {
		return "", fmt.Errorf("%w: no role", ErrClaims)
	}
	// A slice rather than a map, so the field named in the error is the same
	// one on every run. Two fields over the maximum is not a case worth two
	// messages, and a message that moves between runs is one nobody can search
	// a log for.
	for _, f := range []struct {
		name  string
		value string
	}{
		{"conference", c.Conference},
		{"subject", c.Subject},
		{"role", c.Role},
	} {
		if len(f.value) > MaxFieldBytes {
			return "", fmt.Errorf("%w: %s is %d bytes, maximum is %d", ErrClaims, f.name, len(f.value), MaxFieldBytes)
		}
	}
	if c.NotBefore.IsZero() || c.Expires.IsZero() {
		return "", fmt.Errorf("%w: the window is not bounded at both ends", ErrClaims)
	}
	if !c.Expires.After(c.NotBefore) {
		return "", fmt.Errorf("%w: the window ends before it opens", ErrClaims)
	}
	if c.Lifetime() > MaxLifetime {
		return "", fmt.Errorf("%w: a lifetime of %s is longer than the maximum of %s", ErrClaims, c.Lifetime(), MaxLifetime)
	}

	payload := encode(c)
	mac := sign(i.key.Reveal(), payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac...)), nil
}

// A Verifier reads credentials with one key and one clock.
type Verifier struct {
	key   secret.Bytes
	clock clock.Clock
}

// NewVerifier refuses a key shorter than MinKeyBytes and refuses to be built
// without a clock, because a verifier with no clock is one that cannot refuse
// an expired credential and would pass every other test in this package.
func NewVerifier(key secret.Bytes, c clock.Clock) (*Verifier, error) {
	if key.Len() < MinKeyBytes {
		return nil, fmt.Errorf("%w: %d bytes, minimum is %d", ErrKeyTooShort, key.Len(), MinKeyBytes)
	}
	if c == nil {
		return nil, errors.New("roomcred: a verifier needs a clock")
	}
	return &Verifier{key: append(secret.Bytes(nil), key...), clock: c}, nil
}

// Format writes a placeholder for every verb, for the reason Issuer.Format
// gives. The clock is left out of it as well: a verifier printed while a test
// is failing should say which verifier it is and not what time it thinks it is,
// and the clock is the caller's to print.
func (v Verifier) Format(f fmt.State, verb rune) {
	io.WriteString(f, "roomcred.Verifier{key: "+secret.Placeholder+"}")
}

// Verify reads token as a credential admitting to conference. It returns what
// the credential says only when this key signed it, the layout is one this
// build knows, it names that conference, and the clock is inside its window.
//
// The signature is checked before anything inside the credential is read, so
// nothing a forger writes into a token reaches a decision here.
func (v *Verifier) Verify(token string, conference string) (Claims, error) {
	if conference == "" {
		return Claims{}, fmt.Errorf("%w: no conference to verify against", ErrClaims)
	}
	if len(token) > MaxTokenBytes {
		return Claims{}, fmt.Errorf("%w: %d bytes, maximum is %d", ErrMalformed, len(token), MaxTokenBytes)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: not base64url", ErrMalformed)
	}
	if len(raw) <= sha256.Size {
		return Claims{}, fmt.Errorf("%w: too short to hold a signature", ErrMalformed)
	}
	payload, mac := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	if !hmac.Equal(mac, sign(v.key.Reveal(), payload)) {
		return Claims{}, ErrSignature
	}

	c, err := decode(payload)
	if err != nil {
		return Claims{}, err
	}
	if c.Conference != conference {
		return Claims{}, ErrWrongConference
	}

	now := v.clock.Now()
	if now.Before(c.NotBefore) {
		return Claims{}, ErrNotYetValid
	}
	if now.After(c.Expires) {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// sign is the one place a signature is made, so the two sides cannot disagree
// about what is covered. What is covered is the whole payload, first byte to
// last, which is every field including the version.
func sign(key, payload []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(payload)
	return m.Sum(nil)
}

// encode lays out the signed bytes: the version, then each text field with its
// length in front of it, then the window as two counts of seconds. Lengths are
// in front so a reader never has to guess where a field ends, which is the
// property the malformed cases below are refused by.
func encode(c Claims) []byte {
	out := make([]byte, 0, 64+len(c.Conference)+len(c.Subject)+len(c.Role))
	out = append(out, Version)
	for _, s := range []string{c.Conference, c.Subject, c.Role} {
		out = binary.BigEndian.AppendUint16(out, uint16(len(s)))
		out = append(out, s...)
	}
	out = binary.BigEndian.AppendUint64(out, uint64(c.NotBefore.Unix()))
	out = binary.BigEndian.AppendUint64(out, uint64(c.Expires.Unix()))
	return out
}

// decode reads what encode wrote. It is reached only for bytes whose signature
// already verified, and it still refuses a payload that does not fit together,
// because a signed thing that this build cannot read is a bug rather than a
// reason to guess.
func decode(payload []byte) (Claims, error) {
	if len(payload) < 1 {
		return Claims{}, fmt.Errorf("%w: empty payload", ErrMalformed)
	}
	if payload[0] != Version {
		return Claims{}, fmt.Errorf("%w: version %d", ErrUnknownVersion, payload[0])
	}
	rest := payload[1:]

	fields := make([]string, 3)
	for i := range fields {
		if len(rest) < 2 {
			return Claims{}, fmt.Errorf("%w: a field length runs past the end", ErrMalformed)
		}
		n := int(binary.BigEndian.Uint16(rest))
		rest = rest[2:]
		if n > MaxFieldBytes {
			return Claims{}, fmt.Errorf("%w: a field of %d bytes, maximum is %d", ErrMalformed, n, MaxFieldBytes)
		}
		if len(rest) < n {
			return Claims{}, fmt.Errorf("%w: a field runs past the end", ErrMalformed)
		}
		fields[i] = string(rest[:n])
		rest = rest[n:]
	}
	if len(rest) != 16 {
		return Claims{}, fmt.Errorf("%w: the window is %d bytes rather than 16", ErrMalformed, len(rest))
	}

	return Claims{
		Conference: fields[0],
		Subject:    fields[1],
		Role:       fields[2],
		NotBefore:  time.Unix(int64(binary.BigEndian.Uint64(rest[:8])), 0).UTC(),
		Expires:    time.Unix(int64(binary.BigEndian.Uint64(rest[8:])), 0).UTC(),
	}, nil
}
