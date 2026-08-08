// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

package wire

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAnEnvelopeRoundTrips(t *testing.T) {
	want := Message{Type: "join", Payload: json.RawMessage(`{"conference":"conf-7f3a"}`)}

	b, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Type != want.Type {
		t.Errorf("type came back as %q, want %q", got.Type, want.Type)
	}
	if string(got.Payload) != string(want.Payload) {
		t.Errorf("payload came back as %s, want %s", got.Payload, want.Payload)
	}
}

// The payload is handed on exactly as it arrived, because the type that owns it
// is what decides what it means, and a layer that re-encoded it on the way
// through would decide some of that here.
func TestThePayloadIsHandedOnUntouched(t *testing.T) {
	const payload = `{ "b" : 1,  "a": [2, 3] }`
	m, err := Decode([]byte(`{"type":"x","data":` + payload + `}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if string(m.Payload) != payload {
		t.Errorf("payload came back as %s, want %s", m.Payload, payload)
	}
}

// A message with nothing to carry survives both directions unchanged. This is
// the case that found the encoder writing a data member holding null, which
// came back as a payload of four bytes and made the two directions of this
// package disagree about one message.
func TestAMessageWithNoPayloadRoundTrips(t *testing.T) {
	b, err := Encode(Message{Type: "leave"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(b) != `{"type":"leave"}` {
		t.Errorf("encoded as %s, want %s", b, `{"type":"leave"}`)
	}

	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Payload != nil {
		t.Errorf("payload came back as %q, want nothing", m.Payload)
	}
}

func TestAnEnvelopeWithNoDataIsRead(t *testing.T) {
	m, err := Decode([]byte(`{"type":"leave"}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Type != "leave" {
		t.Errorf("type came back as %q, want %q", m.Type, "leave")
	}
	if m.Payload != nil {
		t.Errorf("payload came back as %s, want nothing", m.Payload)
	}
}

// Decode is a function of its argument and nothing else. Bytes that come back
// as a message are the same message every time, and bytes that are refused are
// refused the same way, which is the property the generated cases on #41 will
// be run against.
func TestDecodeAnswersTheSameWayEveryTime(t *testing.T) {
	for _, in := range []string{
		`{"type":"join","data":{"a":1}}`,
		`{"type":"","data":null}`,
		`not json`,
		`{"type":"x","type":"y"}`,
	} {
		first, firstErr := Decode([]byte(in))
		for i := 0; i < 4; i++ {
			got, err := Decode([]byte(in))
			if (err == nil) != (firstErr == nil) {
				t.Fatalf("%s: run %d returned %v, first returned %v", in, i, err, firstErr)
			}
			if err != nil {
				if err.Error() != firstErr.Error() {
					t.Errorf("%s: run %d refused with %v, first refused with %v", in, i, err, firstErr)
				}
				continue
			}
			if got.Type != first.Type || string(got.Payload) != string(first.Payload) {
				t.Errorf("%s: run %d gave %+v, first gave %+v", in, i, got, first)
			}
		}
	}
}

// Decode holds nothing between calls, so a message read after a refused one
// carries nothing from it.
func TestARefusedMessageLeavesNothingBehind(t *testing.T) {
	if _, err := Decode([]byte(`{"type":"join","data":{"secret":1}`)); err == nil {
		t.Fatal("a truncated message was accepted")
	}
	m, err := Decode([]byte(`{"type":"leave"}`))
	if err != nil {
		t.Fatalf("Decode after a refusal: %v", err)
	}
	if m.Payload != nil {
		t.Errorf("payload came back as %s after a refused message, want nothing", m.Payload)
	}
}

func TestAMessageOverTheMaximumIsRefusedWithoutBeingParsed(t *testing.T) {
	// Valid JSON, so nothing but the size can be what refuses it.
	big := `{"type":"join","data":{"x":"` + strings.Repeat("y", MaxMessageBytes) + `"}}`
	if _, err := Decode([]byte(big)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Decode of %d bytes returned %v, want ErrTooLarge", len(big), err)
	}

	// One byte under the maximum, so the case above is the size biting rather
	// than every large message being refused.
	filler := MaxMessageBytes - len(`{"type":"join","data":{"x":""}}`) - 1
	fits := `{"type":"join","data":{"x":"` + strings.Repeat("y", filler) + `"}}`
	if len(fits) != MaxMessageBytes-1 {
		t.Fatalf("the case is %d bytes, so it is not testing what it claims", len(fits))
	}
	if _, err := Decode([]byte(fits)); err != nil {
		t.Errorf("Decode of %d bytes returned %v, want it accepted", len(fits), err)
	}
}

func TestSomethingThatIsNotOneJSONObjectIsRefused(t *testing.T) {
	cases := map[string]string{
		"empty":                    "",
		"not json at all":          "join now",
		"a bare string":            `"join"`,
		"a number":                 `17`,
		"an array":                 `[{"type":"join"}]`,
		"null":                     `null`,
		"truncated":                `{"type":"join"`,
		"a second object after it": `{"type":"join"}{"type":"leave"}`,
		"a value after it":         `{"type":"join"} 17`,
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(in)); !errors.Is(err, ErrNotJSON) {
				t.Errorf("Decode returned %v, want ErrNotJSON", err)
			}
		})
	}
}

// Trailing whitespace is not a second value. This is here so that the refusal
// above cannot be satisfied by refusing everything after the closing brace.
func TestWhitespaceAfterTheEnvelopeIsAccepted(t *testing.T) {
	if _, err := Decode([]byte("{\"type\":\"join\"}\n  \t")); err != nil {
		t.Errorf("Decode returned %v, want it accepted", err)
	}
}

func TestAJSONObjectThatIsNotAnEnvelopeIsRefused(t *testing.T) {
	cases := map[string]string{
		"no type":                 `{"data":{"a":1}}`,
		"an empty type":           `{"type":""}`,
		"a type that is a number": `{"type":17}`,
		"a type given twice":      `{"type":"join","type":"leave"}`,
		"data given twice":        `{"type":"join","data":{"a":1},"data":{"a":2}}`,
		"a member we do not have": `{"type":"join","extra":1}`,
		"a misspelled member":     `{"type":"join","dat":{"a":1}}`,
		"a type over the maximum": `{"type":"` + strings.Repeat("t", MaxTypeBytes+1) + `"}`,
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(in)); !errors.Is(err, ErrEnvelope) {
				t.Errorf("Decode returned %v, want ErrEnvelope", err)
			}
		})
	}

	// A type of exactly the maximum, so the case above is the maximum biting.
	at := `{"type":"` + strings.Repeat("t", MaxTypeBytes) + `"}`
	if _, err := Decode([]byte(at)); err != nil {
		t.Errorf("a type of exactly the maximum returned %v, want it accepted", err)
	}
}

// A member given twice is refused rather than resolved. Go's decoder takes the
// last one, so an envelope naming one type to this reader and another to a
// reader written differently would otherwise pass through here.
func TestARepeatedMemberIsRefusedRatherThanResolved(t *testing.T) {
	m, err := Decode([]byte(`{"type":"join","type":"kick"}`))
	if err == nil {
		t.Fatalf("a repeated member was accepted as %+v", m)
	}
	if !errors.Is(err, ErrEnvelope) {
		t.Errorf("Decode returned %v, want ErrEnvelope", err)
	}
	if strings.Contains(err.Error(), "kick") && !strings.Contains(err.Error(), "type") {
		t.Errorf("the refusal quotes the value rather than naming the member: %v", err)
	}
}

func TestEncodeRefusesWhatDecodeWouldNotAccept(t *testing.T) {
	cases := map[string]Message{
		"no type":                 {Type: ""},
		"a type over the maximum": {Type: strings.Repeat("t", MaxTypeBytes+1)},
		"a payload that is not JSON": {
			Type:    "join",
			Payload: json.RawMessage(`{"a":`),
		},
	}

	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Encode(m); err == nil {
				t.Error("Encode accepted it")
			}
		})
	}
}

func TestEncodeRefusesAMessageOverTheMaximum(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"x": strings.Repeat("y", MaxMessageBytes)})
	if err != nil {
		t.Fatalf("building the case: %v", err)
	}
	if _, err := Encode(Message{Type: "join", Payload: payload}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Encode returned %v, want ErrTooLarge", err)
	}
}
