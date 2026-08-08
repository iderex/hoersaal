// Package secret holds a value that is worth stealing, in a type whose
// formatting produces a placeholder instead of the value.
//
// It answers the second condition of issue #86. The other three conditions
// there need a configuration, a rotation with a live session to have an effect
// on, and a diagnostic bundle, and none of the three exists yet. This one needs
// nothing but the type, and it gets more expensive with every caller that holds
// a secret as a plain slice, which is the reason it is not waiting for them.
//
// What this is for. A rule saying not to log a secret is obeyed by everybody
// who has read it, and the log line that leaks one is written by somebody who
// was printing the struct that happens to contain it. So the refusal is in the
// type rather than in the rule: every formatting verb produces Placeholder, and
// so do the two encodings a diagnostic bundle is most likely to be written in.
//
// What it does not do, stated because a reader who assumes otherwise is worse
// off than one who knows. It is not a container that keeps the bytes out of a
// core dump, out of swap or out of another process. It does not erase them; Go
// moves values and a slice this type held may outlive it in memory. And it does
// not stop code that has the secret from writing it somewhere on purpose, which
// is what Reveal is named to make visible at the site rather than prevented.
//
// The residual worth knowing about is in fmt rather than here. A secret held in
// an unexported field of another struct is printed by fmt's reflection without
// this type's methods being reached, because fmt cannot call a method on a
// value it may not take out. So a type that holds one carries its own
// formatting, which is what roomcred.Issuer and roomcred.Verifier do, and
// TestPrintingTheHolderOfTheKeyDoesNotRevealIt is what says so.
package secret

import (
	"fmt"
	"io"
)

// Placeholder is what formatting a secret produces. It is one string for every
// verb and every encoding, so a reader who meets it in a log knows what it is
// without having to know which verb wrote it.
const Placeholder = "[redacted]"

// Bytes is a secret held as bytes.
//
// It is a defined slice type rather than a struct wrapping one, so a []byte
// assigns into it and out of it without a conversion. That is a deliberate
// trade. It makes adopting the type a change to a declaration rather than to
// every caller, and it means the type is a guarantee about formatting rather
// than about access: nothing stops code that holds one from assigning it to a
// []byte and printing that. Reveal exists so the code that has to do it says
// so, and the assignment that skips Reveal is the case this type does not
// catch.
type Bytes []byte

// Format writes Placeholder whatever the verb is.
//
// Every verb, rather than the handful a Stringer covers, because fmt reaches a
// Stringer for v, s, q, x and X and prints the bytes themselves for d, and a
// secret printed as a list of decimal numbers is a secret printed. Implementing
// Formatter is the only way to answer for the verbs nobody thought of.
func (b Bytes) Format(f fmt.State, verb rune) {
	io.WriteString(f, Placeholder)
}

// String is Placeholder. Format already answers for fmt, so this is for the
// code that calls String itself.
func (b Bytes) String() string { return Placeholder }

// GoString is Placeholder, for the same reason String is.
func (b Bytes) GoString() string { return Placeholder }

// MarshalText writes Placeholder, which covers every encoder that asks for text
// and is what stops a secret reaching a file somebody attaches to a bug report.
func (b Bytes) MarshalText() ([]byte, error) { return []byte(Placeholder), nil }

// MarshalJSON writes Placeholder as a JSON string.
//
// It is here rather than left to MarshalText because encoding/json has a rule
// of its own for a slice of bytes, which is to write it as base64, and base64 of
// a signing key is the signing key.
func (b Bytes) MarshalJSON() ([]byte, error) { return []byte(`"` + Placeholder + `"`), nil }

// Reveal returns the bytes.
//
// It is the named way out, so that a search for Reveal finds every place the
// value leaves this type on purpose. It returns the slice this value is over
// rather than a copy: a caller that wants one takes it, and a copy made here
// would be a second place the secret lives on every call.
func (b Bytes) Reveal() []byte { return []byte(b) }

// Len is the number of bytes, which is the one fact about a secret that is safe
// to print and is what a refusal for a key that is too short has to say.
func (b Bytes) Len() int { return len(b) }
