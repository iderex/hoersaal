// Package wire reads what a client sends to the control plane and writes what
// the control plane sends back.
//
// It answers the framing half of issue #31. The transport it sits inside, the
// reason for each choice and every number below are in
// docs/decisions/signalling-transport.md.
//
// Decode is a function from bytes to a message or an error. It takes no
// connection, holds nothing between calls and reaches nothing outside itself,
// because this is the first thing an unauthenticated stranger touches and the
// only honest way to test that is to hand it bytes. The property suite on issue
// #41 and the fuzzing on issue #94 both enter here rather than building a
// second door.
//
// What this package does not do is give a message meaning. It reads the
// envelope, refuses everything that is not one, and hands the payload on
// untouched to whoever owns that message type. The types themselves belong to
// the exchanges that use them, which are issues #35 and #37, and the version
// this envelope is spoken at is issue #32.
package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// The numbers this package refuses against. Each one is argued in
// docs/decisions/signalling-transport.md rather than here, so the two cannot
// drift into two different values.
const (
	// MaxMessageBytes is the largest message a client may send. A message over
	// it is refused without being parsed.
	MaxMessageBytes = 64 * 1024

	// MaxTypeBytes is the longest message type name this envelope may carry.
	MaxTypeBytes = 64
)

// The refusals, as values, because what a client is told when its message is
// rejected is a decision the exchange above this one makes and it cannot make
// that decision from a string.
var (
	// ErrTooLarge is a message over MaxMessageBytes.
	ErrTooLarge = errors.New("wire: message is larger than the maximum")

	// ErrNotJSON is a message that is not one well-formed JSON object.
	ErrNotJSON = errors.New("wire: message is not a JSON object")

	// ErrEnvelope is a message that is a JSON object and not an envelope: a
	// member this envelope does not have, one member given twice, a member of
	// the wrong kind, no type, or a type that is empty or too long.
	ErrEnvelope = errors.New("wire: message is not an envelope")
)

// The two members an envelope has, and the only two. A message carrying
// anything else is refused rather than ignored, so a client that misspells a
// member is told rather than quietly having it dropped.
const (
	memberType = "type"
	memberData = "data"
)

// A Message is one envelope. Payload is the bytes of the data member exactly as
// they arrived, so the type that owns them decodes them itself and this package
// never has to know what they mean. It is nil where the envelope carried none.
type Message struct {
	Type    string
	Payload json.RawMessage
}

// envelope is the shape the members are read into once the structure has been
// refused. The tag names are the wire names.
// A message with nothing to carry has no data member at all rather than one
// holding null. Without that, encoding a message with no payload and reading it
// back gives a payload of the four bytes "null", so the two directions of this
// package disagree about the same message.
type envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Decode reads one message. It is pure: the same bytes give the same answer,
// and it neither reads nor writes anything outside its arguments.
//
// It refuses in this order: the size, then the structure, then the values. The
// size comes first so that nothing a stranger sends decides how much work this
// does.
func Decode(b []byte) (Message, error) {
	if len(b) > MaxMessageBytes {
		return Message{}, fmt.Errorf("%w: %d bytes, maximum is %d", ErrTooLarge, len(b), MaxMessageBytes)
	}
	if err := structure(b); err != nil {
		return Message{}, err
	}

	var e envelope
	if err := json.Unmarshal(b, &e); err != nil {
		// The structure is already known to be one object of known members, so
		// what is left for this to catch is a member of the wrong kind, which
		// is an envelope fault rather than a JSON one.
		return Message{}, fmt.Errorf("%w: %s", ErrEnvelope, err)
	}
	if e.Type == "" {
		return Message{}, fmt.Errorf("%w: no type", ErrEnvelope)
	}
	if len(e.Type) > MaxTypeBytes {
		return Message{}, fmt.Errorf("%w: a type of %d bytes, maximum is %d", ErrEnvelope, len(e.Type), MaxTypeBytes)
	}

	return Message{Type: e.Type, Payload: e.Data}, nil
}

// structure refuses everything about the shape of a message that the value
// decoding above cannot see: that it is one object rather than some other JSON
// value, that nothing follows it, that every member is one this envelope has,
// and that no member is given twice.
//
// The repeated member is the one worth naming. Go's decoder takes the last of
// them and says nothing, so two readers of one message can disagree about what
// it said, and the reader that matters is whichever one an attacker did not
// think of. The scan is over the top-level object, which is the whole of what
// this package decodes; a payload belongs to the type that owns it and is
// refused there.
func structure(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))

	open, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotJSON, err)
	}
	if d, ok := open.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("%w: it begins with %v", ErrNotJSON, open)
	}

	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return fmt.Errorf("%w: %s", ErrNotJSON, err)
		}
		name, ok := key.(string)
		if !ok {
			return fmt.Errorf("%w: a member name that is not a string", ErrNotJSON)
		}
		if name != memberType && name != memberData {
			return fmt.Errorf("%w: the member %q", ErrEnvelope, name)
		}
		if seen[name] {
			return fmt.Errorf("%w: the member %q is given twice", ErrEnvelope, name)
		}
		seen[name] = true

		// The value, whatever it is. Decoding into an empty interface consumes
		// exactly one value however deeply it nests.
		var skip any
		if err := dec.Decode(&skip); err != nil {
			return fmt.Errorf("%w: %s", ErrNotJSON, err)
		}
	}

	// The closing brace. Where it is missing the object was truncated, and this
	// is the only place that sees it: the decoder answers the question after it
	// with the end of the input either way, so a truncated message would
	// otherwise reach the value decoding below and be refused as an envelope
	// fault rather than as JSON that stops in the middle.
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("%w: the object is not closed", ErrNotJSON)
	}

	// Nothing after it. Whitespace is not anything.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: a second value follows the envelope", ErrNotJSON)
	}
	return nil
}

// Encode writes one message. It is here rather than beside the code that sends,
// so that what this service emits and what it accepts are described in one file
// and a change to either is visible against the other.
func Encode(m Message) ([]byte, error) {
	if m.Type == "" {
		return nil, fmt.Errorf("%w: no type", ErrEnvelope)
	}
	if len(m.Type) > MaxTypeBytes {
		return nil, fmt.Errorf("%w: a type of %d bytes, maximum is %d", ErrEnvelope, len(m.Type), MaxTypeBytes)
	}
	if m.Payload != nil && !json.Valid(m.Payload) {
		return nil, fmt.Errorf("%w: the payload is not JSON", ErrNotJSON)
	}

	// The encoder is built rather than json.Marshal called, for one setting.
	// json.Marshal escapes <, > and & so that its output is safe to drop into
	// an HTML document. Nothing here goes into one: this is a JSON message in a
	// WebSocket frame, and the escaping buys nothing while costing six bytes
	// for every one of those characters a payload holds.
	//
	// That cost is not cosmetic. A message a stranger sends that is comfortably
	// under MaxMessageBytes and is mostly those three characters grows past it
	// on the way back out, so this service would accept a message it could not
	// then forward. The property suite on #41 found it: a payload of sixty
	// thousand less-than signs decodes at 60022 bytes and re-encoded at 360022.
	// With the escaping off, encoding only ever compacts, so anything that
	// decoded can be encoded.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(envelope{Type: m.Type, Data: m.Payload}); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotJSON, err)
	}
	// Encoder.Encode writes a trailing newline and Marshal does not. One
	// message is one frame, so the newline is not a separator here and would be
	// a byte on the wire that means nothing.
	b := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if len(b) > MaxMessageBytes {
		return nil, fmt.Errorf("%w: %d bytes, maximum is %d", ErrTooLarge, len(b), MaxMessageBytes)
	}
	return b, nil
}
