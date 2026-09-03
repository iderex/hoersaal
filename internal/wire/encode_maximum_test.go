// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package wire

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestEncodeAcceptsAMessageOfExactlyTheMaximum pins the upper edge of what
// Encode emits. Decode already has its case at exactly the maximum; Encode had
// one byte over and nothing at the line, and the mutation run on issue #93
// found that refusing a message of exactly MaxMessageBytes on the way out left
// the suite green. A message that decodes at the maximum has to encode at it
// too, or this service accepts what it cannot then forward.
func TestEncodeAcceptsAMessageOfExactlyTheMaximum(t *testing.T) {
	frame := func(n int) json.RawMessage {
		payload, err := json.Marshal(map[string]string{"x": strings.Repeat("y", n)})
		if err != nil {
			t.Fatalf("building the case: %v", err)
		}
		return payload
	}
	// The overhead is measured off an empty payload rather than typed, so the
	// case stays at the line when the envelope's shape moves.
	empty, err := Encode(Message{Type: "join", Payload: frame(0)})
	if err != nil {
		t.Fatalf("Encode of the empty case: %v", err)
	}
	filler := MaxMessageBytes - len(empty)

	exact, err := Encode(Message{Type: "join", Payload: frame(filler)})
	if err != nil {
		t.Fatalf("Encode refused a message of exactly the maximum: %v", err)
	}
	if len(exact) != MaxMessageBytes {
		t.Fatalf("the case is %d bytes, so it is not testing what it claims", len(exact))
	}
	if _, err := Decode(exact); err != nil {
		t.Errorf("what Encode emitted at the maximum, Decode refused: %v", err)
	}

	if _, err := Encode(Message{Type: "join", Payload: frame(filler + 1)}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Encode one byte over the maximum returned %v, want ErrTooLarge", err)
	}
}
