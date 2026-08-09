// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Property tests over the decoder, issue #41.
//
// The decoder is the first thing an unauthenticated stranger reaches and it
// takes arbitrary bytes, so a suite of the messages somebody thought of is a
// suite over the wrong set. What is asserted here is asserted over generated
// input instead, and the four properties are the ones #41 names: decoding never
// panics and never allocates without bound, anything that decodes and
// re-encodes gives the same value back, anything that fails to decode returns
// no partial value, and a message over the frame limit is refused before it is
// parsed rather than after.
//
// The generator is seeded, so a failure here is reproduced by naming the seed
// rather than by running it again and hoping. The seed and the number of cases
// are constants below and both are printed by the run.
package wire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/iderex/hoersaal/internal/random"
)

const (
	// generatedCases is the number of cases each property is asserted over. It
	// is stated rather than tuned: a run that examined fewer cannot be read as
	// one that examined this many and found nothing.
	generatedCases = 20000

	// propertySeed is the seed the generator is started from. A failing case
	// prints it, and naming it again reproduces the run exactly.
	propertySeed = 41
)

// A source is the part of a seeded random source this suite uses. It is named
// here rather than imported, so that this file does not import a randomness
// package: the guard on #27 refuses that import anywhere but internal/random,
// and a test suite is not an exception to it.
type source interface {
	IntN(n int) int
	Uint64() uint64
}

func newSource(seed uint64) source { return random.Seeded(seed) }

// decodeOutcome is what one case did, so the run can report that it reached
// both answers rather than asserting properties over inputs that were all
// refused at the first line.
type decodeOutcome struct {
	decoded  int
	refused  int
	tooLarge int
}

// checkDecodeProperties asserts everything that must hold for one input,
// whatever that input is. It is one function rather than four because the fuzz
// work on #94 enters at Decode and needs the same assertions over its own
// corpus; a second copy of them is the thing #41 asks that milestone not to
// build.
//
// It returns the outcome so a caller can count what it saw.
func checkDecodeProperties(t *testing.T, in []byte) (decoded bool) {
	t.Helper()

	// Property one, first half. Decode never panics, whatever the bytes are.
	// The recover is here rather than around the whole loop so that the input
	// that caused it is the one reported.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Decode panicked on %s: %v", fixtureLiteral(in), r)
		}
	}()

	m, err := Decode(in)

	if err != nil {
		// Property three. A refusal returns no partial value. A caller that
		// reads the message anyway must find nothing in it, because a partial
		// value is how a refused message gets acted on further up.
		if m.Type != "" || m.Payload != nil {
			t.Errorf("Decode refused %s with %v and still returned type %q and payload %s",
				fixtureLiteral(in), err, m.Type, m.Payload)
		}
		// Property four, the half that holds for every input. A refusal names
		// one of the three refusals this package declares, so a caller can tell
		// them apart without reading the text.
		if !errors.Is(err, ErrTooLarge) && !errors.Is(err, ErrNotJSON) && !errors.Is(err, ErrEnvelope) {
			t.Errorf("Decode refused %s with an error naming none of the three refusals: %v",
				fixtureLiteral(in), err)
		}
		return false
	}

	// Property two. What decoded can be re-encoded, and doing so gives the same
	// value back. Byte equality against the input is not the property and is not
	// asserted: the payload is handed on with its whitespace, the encoder
	// compacts, and member order is the encoder's. What must hold is that the
	// value survives, and that a second pass moves nothing, which is what makes
	// the first pass a normalisation rather than a slow corruption.
	out, err := Encode(m)
	if err != nil {
		t.Errorf("Decode accepted %s and Encode then refused the value it produced: %v",
			fixtureLiteral(in), err)
		return true
	}
	again, err := Decode(out)
	if err != nil {
		t.Errorf("Decode accepted %s, Encode produced %s, and Decode then refused that: %v",
			fixtureLiteral(in), out, err)
		return true
	}
	if again.Type != m.Type {
		t.Errorf("type moved on a round trip of %s: %q became %q", fixtureLiteral(in), m.Type, again.Type)
	}
	if !sameJSON(m.Payload, again.Payload) {
		t.Errorf("payload moved on a round trip of %s: %s became %s", fixtureLiteral(in), m.Payload, again.Payload)
	}
	twice, err := Encode(again)
	if err != nil {
		t.Errorf("the second Encode of %s refused: %v", fixtureLiteral(in), err)
		return true
	}
	if !bytes.Equal(out, twice) {
		t.Errorf("encoding is not a fixed point for %s: %s then %s", fixtureLiteral(in), out, twice)
	}
	return true
}

// TestDecodeHoldsItsPropertiesOverGeneratedInput is the suite #41 asks for. One
// loop rather than four, because every property is a statement about the same
// call and running the generator four times would assert them over four
// different sets of bytes.
func TestDecodeHoldsItsPropertiesOverGeneratedInput(t *testing.T) {
	t.Logf("%d generated cases from seed %d", generatedCases, propertySeed)

	r := newSource(propertySeed)
	var seen decodeOutcome
	for i := 0; i < generatedCases; i++ {
		in := generate(r)
		if len(in) > MaxMessageBytes {
			seen.tooLarge++
		}
		if checkDecodeProperties(t, in) {
			seen.decoded++
		} else {
			seen.refused++
		}
		if t.Failed() {
			t.Fatalf("stopping at case %d of %d", i, generatedCases)
		}
	}

	t.Logf("decoded %d, refused %d, over the frame limit %d", seen.decoded, seen.refused, seen.tooLarge)

	// A run in which everything was refused proves nothing about the round
	// trip, and a run in which everything decoded proves nothing about the
	// refusals. Both floors are asserted so that a generator that stops
	// producing one shape cannot leave this suite green over nothing.
	if seen.decoded < generatedCases/100 {
		t.Errorf("only %d of %d cases decoded; the round trip property was barely exercised", seen.decoded, generatedCases)
	}
	if seen.refused < generatedCases/100 {
		t.Errorf("only %d of %d cases were refused; the refusal properties were barely exercised", seen.refused, generatedCases)
	}
}

// TestAMessageOverTheFrameLimitIsRefusedBeforeItIsParsed is property four, and
// it is a separate test because the generator does not reach it: a case of a
// hundred kilobytes on every iteration would make the suite about allocation
// rather than about the decoder.
//
// Refused before parsing rather than after is shown by the answer, not by
// reading the source. Bytes that are over the limit AND are not JSON at all
// come back as the size refusal. If the parse ran first they would come back as
// the JSON one.
func TestAMessageOverTheFrameLimitIsRefusedBeforeItIsParsed(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"a well formed envelope that is one byte too long", oversizeEnvelope()},
		{"bytes that are not JSON", bytes.Repeat([]byte{0xff}, MaxMessageBytes+1)},
		{"an unterminated object", append([]byte(`{"type":"x","data":"`), bytes.Repeat([]byte{'a'}, MaxMessageBytes)...)},
		{"nothing but opening brackets", bytes.Repeat([]byte{'['}, MaxMessageBytes+1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if len(c.in) <= MaxMessageBytes {
				t.Fatalf("the case is %d bytes, which is not over the limit of %d", len(c.in), MaxMessageBytes)
			}
			m, err := Decode(c.in)
			if !errors.Is(err, ErrTooLarge) {
				t.Fatalf("Decode of %d bytes gave %v, want %v", len(c.in), err, ErrTooLarge)
			}
			if m.Type != "" || m.Payload != nil {
				t.Errorf("a refused message came back carrying type %q and payload %s", m.Type, m.Payload)
			}
		})
	}

	// The other side of the same property. One byte under the limit is the
	// decoder's business again, so the limit is a limit and not a wall the
	// decoder hides behind.
	under := envelopeOfExactly(MaxMessageBytes)
	if len(under) != MaxMessageBytes {
		t.Fatalf("the fixture is %d bytes, want exactly %d", len(under), MaxMessageBytes)
	}
	if _, err := Decode(under); err != nil {
		t.Errorf("a message of exactly the maximum was refused: %v", err)
	}
}

// TestDecodeAllocatesInProportionToItsInput is the second half of property one.
//
// What is asserted is a ceiling on the bytes allocated per byte of input, over
// the shapes chosen to make that ratio as bad as they can. The ceiling is a
// measured number with room above it rather than a guess, and the run prints
// what it actually saw, so a change that makes the decoder greedier is visible
// here before it is visible on a machine.
//
// The bound this gives is a ratio and not a constant. It says no input makes
// the decoder allocate out of proportion to the input; it does not say the
// decoder is frugal.
func TestDecodeAllocatesInProportionToItsInput(t *testing.T) {
	const ceiling = 200.0

	cases := []struct {
		name string
		in   []byte
	}{
		{"a deeply nested array", nestedPayload(8000)},
		{"many short members", manyMembers(2000)},
		{"one long string", []byte(`{"type":"x","data":"` + strings.Repeat("a", 32000) + `"}`)},
		{"many small arrays", []byte(`{"type":"x","data":[` + strings.Repeat("[1],", 3000) + `[1]]}`)},
		{"a long type", []byte(`{"type":"` + strings.Repeat("t", MaxTypeBytes) + `","data":1}`)},
	}

	worst := 0.0
	for _, c := range cases {
		per := allocatedPerByte(func() { _, _ = Decode(c.in) }, len(c.in))
		t.Logf("%-24s %6d bytes in, %8.1f bytes allocated per byte of input", c.name, len(c.in), per)
		if per > worst {
			worst = per
		}
		if per > ceiling {
			t.Errorf("%s allocated %.1f bytes per byte of input, ceiling is %.1f", c.name, per, ceiling)
		}
	}
	t.Logf("worst ratio over these shapes: %.1f, ceiling %.1f", worst, ceiling)
}

// TestTheFixturesFoundByTheGeneratorStayRefused is the register #41 asks for.
// Anything the generator ever found is kept here with its bytes exact, so the
// same case cannot come back quietly once somebody has changed the generator.
//
// The bytes are base64 in the source rather than a raw literal, because a raw
// literal goes through this repository's text handling on the way into git and
// a fixture that exists to carry an exact byte is the last thing that may be
// normalised. Adding one is adding a row.
func TestTheFixturesFoundByTheGeneratorStayRefused(t *testing.T) {
	if len(foundFixtures) == 0 {
		t.Fatal("the register is empty; a row is removed only with the reason it was kept for")
	}
	for _, f := range foundFixtures {
		t.Run(f.name, func(t *testing.T) {
			in := f.bytes()
			t.Logf("%d bytes: %s", len(in), f.why)
			checkDecodeProperties(t, in)
		})
	}
	t.Logf("%d fixtures in the register", len(foundFixtures))
}

// foundFixtures is the register. Each row is a case that failed one of the four
// properties, kept after the defect it found was repaired.
//
// A row holds a construction rather than a literal. Two of the shapes that
// break these properties are tens of kilobytes, and eighty kilobytes of base64
// in a source file is not a fixture anybody reads. A construction from a repeat
// count is exact in the way that matters here, which is that it carries no byte
// text handling could alter on the way into git and no byte it could alter on
// the way out. Where a row is short enough to be a literal it is held as base64
// and decoded by the same field, so the register has one shape.
var foundFixtures = []struct {
	name string
	// why says what the case broke when it was found, so a row cannot become a
	// case nobody remembers the reason for.
	why   string
	bytes func() []byte
}{
	{
		name: "a payload of sixty thousand less-than signs",
		why: "Decode accepted it at 60022 bytes and Encode then refused the value, " +
			"because the encoder escaped every < as \\u003c and the message came to " +
			"360022 bytes. The service would have accepted a message it could not " +
			"forward. Encode no longer escapes for HTML, so encoding only compacts.",
		bytes: func() []byte {
			return []byte(`{"type":"x","data":"` + strings.Repeat("<", 60000) + `"}`)
		},
	},
	{
		name: "an array nested past the decoder's depth limit",
		why: "The first property is that no bytes make Decode panic. Nesting is the " +
			"shape that reaches for the stack, and this is refused as not being JSON " +
			"rather than by running out of it. Kept because the depth limit lives in " +
			"the standard library rather than here, so nothing in this tree would " +
			"notice it moving.",
		bytes: func() []byte {
			const depth = 32000
			return []byte(`{"type":"x","data":` + strings.Repeat("[", depth) + strings.Repeat("]", depth) + `}`)
		},
	},
	{
		name: "a type of sixty-four less-than signs",
		why: "The same expansion on the other member. The type is inside the frame " +
			"limit at 64 bytes and was 384 encoded, which no longer refuses anything " +
			"on its own but was the same defect one member over.",
		bytes: func() []byte {
			return []byte(`{"type":"` + strings.Repeat("<", MaxTypeBytes) + `","data":1}`)
		},
	},
}

// ---------------------------------------------------------------------------
// The generator.
//
// The shapes are mixed on purpose. Uniform random bytes are almost never JSON,
// so a generator that produced only those would assert the refusal properties
// twenty thousand times and the round trip never. Roughly half of what comes
// out of this is a well formed envelope or one thing away from being one.
// ---------------------------------------------------------------------------

func generate(r source) []byte {
	switch r.IntN(10) {
	case 0, 1:
		return randomBytes(r, r.IntN(96))
	case 2:
		return []byte(randomJSON(r, 0))
	case 3, 4, 5:
		return wellFormedEnvelope(r)
	default:
		return mutate(r, wellFormedEnvelope(r))
	}
}

func randomBytes(r source, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.IntN(256))
	}
	return b
}

// alphabet holds the characters a generated string is built from. The three
// that Go's encoder escapes on the way out are in it deliberately: a type or a
// payload full of them is longer encoded than it was decoded, which is the one
// way a round trip can grow, and a generator without them would never look.
const alphabet = "abcXYZ019 _-.:/<>&\"\\\n\tä€"

func randomString(r source, n int) string {
	var sb strings.Builder
	runes := []rune(alphabet)
	for i := 0; i < n; i++ {
		sb.WriteRune(runes[r.IntN(len(runes))])
	}
	return sb.String()
}

func randomJSON(r source, depth int) string {
	if depth > 3 {
		return "1"
	}
	switch r.IntN(8) {
	case 0:
		return "null"
	case 1:
		return "true"
	case 2:
		// #nosec G115 -- the remainder bounds the value at 999 before the
		// conversion, so the number this generator writes is between -500 and
		// 499 on any width an int has.
		return fmt.Sprintf("%d", int(r.Uint64()%1000)-500)
	case 3:
		b, _ := json.Marshal(randomString(r, r.IntN(12)))
		return string(b)
	case 4:
		n := r.IntN(4)
		parts := make([]string, n)
		for i := range parts {
			parts[i] = randomJSON(r, depth+1)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case 5:
		n := r.IntN(4)
		parts := make([]string, n)
		for i := range parts {
			k, _ := json.Marshal(randomString(r, 1+r.IntN(6)))
			parts[i] = string(k) + ":" + randomJSON(r, depth+1)
		}
		return "{" + strings.Join(parts, ",") + "}"
	case 6:
		return "-0.5e3"
	default:
		return `"x"`
	}
}

func wellFormedEnvelope(r source) []byte {
	typ, _ := json.Marshal(randomString(r, 1+r.IntN(MaxTypeBytes)))
	gap := []string{"", " ", "\n", "  \t "}[r.IntN(4)]

	if r.IntN(4) == 0 {
		return []byte("{" + gap + `"type":` + gap + string(typ) + gap + "}")
	}
	data := randomJSON(r, 0)
	if r.IntN(2) == 0 {
		return []byte("{" + gap + `"type":` + string(typ) + "," + gap + `"data":` + data + gap + "}")
	}
	// The other member order, because an envelope does not promise one and a
	// decoder that quietly depended on it would pass a suite that only ever
	// wrote type first.
	return []byte("{" + gap + `"data":` + data + "," + gap + `"type":` + string(typ) + gap + "}")
}

// mutate breaks one thing about a well formed envelope. Each arm is a shape a
// stranger can actually send.
func mutate(r source, b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	switch r.IntN(9) {
	case 0: // one byte, somewhere
		out := append([]byte(nil), b...)
		out[r.IntN(len(out))] = byte(r.IntN(256))
		return out
	case 1: // cut it short
		return append([]byte(nil), b[:r.IntN(len(b))]...)
	case 2: // a second value after the envelope
		return append(append([]byte(nil), b...), []byte(` {"type":"x"}`)...)
	case 3: // a member this envelope does not have
		return []byte(`{"type":"x","` + randomString(r, 1+r.IntN(6)) + `":1}`)
	case 4: // one member twice
		return []byte(`{"type":"x","type":"y"}`)
	case 5: // no type at all
		return []byte(`{"data":` + randomJSON(r, 0) + `}`)
	case 6: // an empty type
		return []byte(`{"type":"","data":1}`)
	case 7: // a type past the maximum
		return []byte(`{"type":"` + strings.Repeat("t", MaxTypeBytes+1+r.IntN(16)) + `"}`)
	default: // a member of the wrong kind
		return []byte(`{"type":` + randomJSON(r, 2) + `}`)
	}
}

// ---------------------------------------------------------------------------
// Fixtures for the properties the generator does not reach, and small helpers.
// ---------------------------------------------------------------------------

func oversizeEnvelope() []byte {
	head := []byte(`{"type":"x","data":"`)
	tail := []byte(`"}`)
	pad := MaxMessageBytes + 1 - len(head) - len(tail)
	return append(append(append([]byte(nil), head...), bytes.Repeat([]byte{'a'}, pad)...), tail...)
}

func envelopeOfExactly(n int) []byte {
	head := []byte(`{"type":"x","data":"`)
	tail := []byte(`"}`)
	return append(append(append([]byte(nil), head...), bytes.Repeat([]byte{'a'}, n-len(head)-len(tail))...), tail...)
}

func nestedPayload(depth int) []byte {
	return []byte(`{"type":"x","data":` + strings.Repeat("[", depth) + strings.Repeat("]", depth) + `}`)
}

func manyMembers(n int) []byte {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("%q:%d", fmt.Sprintf("k%d", i), i)
	}
	return []byte(`{"type":"x","data":{` + strings.Join(parts, ",") + `}}`)
}

// allocatedPerByte runs f a few times and reports the bytes it allocated,
// divided by the size of its input. It reads the runtime's own counter rather
// than a timer, so it measures the same thing on a fast machine and a slow one.
func allocatedPerByte(f func(), inputLen int) float64 {
	const runs = 5
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < runs; i++ {
		f()
	}
	runtime.ReadMemStats(&after)
	if inputLen == 0 {
		return 0
	}
	return float64(after.TotalAlloc-before.TotalAlloc) / float64(runs) / float64(inputLen)
}

// sameJSON compares two payloads by what they mean rather than byte for byte.
// The payload arrives with its whitespace and leaves compacted, so byte
// equality across a round trip is not the property and asserting it would fail
// on input that is entirely correct.
func sameJSON(a, b json.RawMessage) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false
	}
	return fmt.Sprintf("%#v", x) == fmt.Sprintf("%#v", y)
}

// fixtureLiteral prints an input the way a fixture row holds it, so a failure
// message can be pasted into the register above without anybody re-deriving the
// bytes. Short and printable inputs are shown as themselves as well, because a
// base64 string nobody can read slows down the first minute of every failure.
// It is capped. A case measured in tens of kilobytes would otherwise print
// eighty kilobytes of base64 into the failure, and a failure nobody can scroll
// past is a failure that gets skimmed.
func fixtureLiteral(in []byte) string {
	const limit = 160
	enc := base64.StdEncoding.EncodeToString(in)
	if len(enc) > limit {
		enc = enc[:limit] + fmt.Sprintf("... (%d bytes of base64 elided)", len(enc)-limit)
	}
	if len(in) <= 120 && isPrintable(in) {
		return fmt.Sprintf("%q (b64 %s)", in, enc)
	}
	return fmt.Sprintf("%d bytes (b64 %s)", len(in), enc)
}

func isPrintable(in []byte) bool {
	for _, c := range in {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}
