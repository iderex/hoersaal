// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package wire

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
)

var start = time.Date(2026, time.August, 6, 9, 0, 0, 0, time.UTC)

// stuckConn is a client that stopped reading. Its first Write blocks until the
// connection is closed, and it reports what happened to it.
type stuckConn struct {
	entered chan struct{} // one value, the first time Write is called
	release chan struct{} // closed by Close, which is what unblocks Write

	mu      sync.Mutex
	written int
	closed  bool
}

func newStuckConn() *stuckConn {
	return &stuckConn{entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (c *stuckConn) Write(message []byte) error {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	<-c.release
	c.mu.Lock()
	defer c.mu.Unlock()
	c.written++
	return nil
}

func (c *stuckConn) Close() error {
	c.mu.Lock()
	already := c.closed
	c.closed = true
	c.mu.Unlock()
	if !already {
		close(c.release)
	}
	return nil
}

func (c *stuckConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *stuckConn) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.written
}

// countingConn takes everything immediately and counts it.
type countingConn struct {
	mu       sync.Mutex
	messages [][]byte
	closed   bool
}

func (c *countingConn) Write(message []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, message)
	return nil
}

func (c *countingConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *countingConn) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

// failingConn refuses every write.
type failingConn struct {
	err    error
	closed bool
	mu     sync.Mutex
}

func (c *failingConn) Write([]byte) error { return c.err }

func (c *failingConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// This is the condition on #31: a client that stops reading is disconnected
// rather than buffered without bound. The queue is a fixed size, so what is
// held for one client is bounded by MaxQueued whatever that client does, and
// the send that would have gone past it disconnects instead of growing.
func TestAClientThatStopsReadingIsDisconnectedRatherThanBuffered(t *testing.T) {
	conn := newStuckConn()
	s := NewSender(conn, clock.NewTest(start))

	// The writer takes the first message and blocks inside the connection, so
	// what can be accepted after that is the queue and nothing more.
	if err := s.Send([]byte("first")); err != nil {
		t.Fatalf("the first send returned %v", err)
	}
	<-conn.entered

	accepted := 0
	var refusal error
	for i := 0; i < MaxQueued+2; i++ {
		if err := s.Send([]byte("more")); err != nil {
			refusal = err
			break
		}
		accepted++
	}

	if refusal == nil {
		t.Fatalf("%d sends were accepted with the client not reading and none was refused", accepted)
	}
	if !errors.Is(refusal, ErrTooSlow) {
		t.Errorf("the send that was refused returned %v, want ErrTooSlow", refusal)
	}
	if accepted > MaxQueued {
		t.Errorf("%d messages were accepted after the writer took one, which is more than the queue holds (%d)", accepted, MaxQueued)
	}
	if !conn.isClosed() {
		t.Error("the connection was not closed, so the client was buffered rather than disconnected")
	}
	if !errors.Is(s.Err(), ErrTooSlow) {
		t.Errorf("the sender stopped with %v, want ErrTooSlow", s.Err())
	}

	<-s.Done()
	if err := s.Send([]byte("after")); !errors.Is(err, ErrTooSlow) {
		t.Errorf("a send after the disconnection returned %v, want ErrTooSlow", err)
	}

	// The backlog is dropped rather than written to a connection that has been
	// closed under a client that was not reading it. What reached the client is
	// the one message that was already in flight when it stopped.
	if n := conn.count(); n != 1 {
		t.Errorf("%d messages reached the client after the disconnection, want the one that was in flight", n)
	}
}

// The queue is not the only bound. A connection that accepts a write and then
// never finishes it holds the writer rather than the queue, and the wait
// registered before the write is what ends that.
func TestAWriteThatDoesNotFinishDisconnectsTheClient(t *testing.T) {
	conn := newStuckConn()
	tc := clock.NewTest(start)
	s := NewSender(conn, tc)

	if err := s.Send([]byte("first")); err != nil {
		t.Fatalf("the first send returned %v", err)
	}
	// The wait is registered before the write starts, so seeing the write begin
	// means the wait already exists and advancing the clock is decided rather
	// than a race.
	<-conn.entered
	if tc.Waiting() != 1 {
		t.Fatalf("%d waits are registered, want 1, so this case is not testing what it claims", tc.Waiting())
	}

	tc.Advance(WriteTimeout)
	<-s.Done()

	if !errors.Is(s.Err(), ErrWriteTimeout) {
		t.Errorf("the sender stopped with %v, want ErrWriteTimeout", s.Err())
	}
	if !conn.isClosed() {
		t.Error("the connection was not closed after the write timed out")
	}
}

// A write one instant short of the timeout is not a timeout. Without this, a
// sender that gave up immediately would pass the case above.
func TestAWriteInsideTheTimeoutIsNotADisconnection(t *testing.T) {
	conn := newStuckConn()
	tc := clock.NewTest(start)
	s := NewSender(conn, tc)

	if err := s.Send([]byte("first")); err != nil {
		t.Fatalf("the first send returned %v", err)
	}
	<-conn.entered

	tc.Advance(WriteTimeout - time.Nanosecond)
	if err := s.Err(); err != nil {
		t.Errorf("the sender stopped with %v before the timeout came due", err)
	}
	select {
	case <-s.Done():
		t.Error("the sender stopped before the timeout came due")
	default:
	}
}

func TestMessagesReachTheClientInTheOrderTheyWereSent(t *testing.T) {
	conn := &countingConn{}
	s := NewSender(conn, clock.NewTest(start))

	for _, m := range [][]byte{[]byte("one"), []byte("two"), []byte("three")} {
		if err := s.Send(m); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.messages) != 3 {
		t.Fatalf("the client received %d messages, want 3", len(conn.messages))
	}
	for i, want := range []string{"one", "two", "three"} {
		if string(conn.messages[i]) != want {
			t.Errorf("message %d was %q, want %q", i, conn.messages[i], want)
		}
	}
}

func TestAConnectionThatRefusesAWriteStopsTheSender(t *testing.T) {
	refused := errors.New("the connection is gone")
	conn := &failingConn{err: refused}
	s := NewSender(conn, clock.NewTest(start))

	if err := s.Send([]byte("one")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	<-s.Done()

	if !errors.Is(s.Err(), refused) {
		t.Errorf("the sender stopped with %v, want the connection's own error", s.Err())
	}
	if err := s.Send([]byte("two")); !errors.Is(err, refused) {
		t.Errorf("a send after the failure returned %v, want the connection's own error", err)
	}
}

func TestClosingIsNotAFailure(t *testing.T) {
	conn := &countingConn{}
	s := NewSender(conn, clock.NewTest(start))

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Err(); err != nil {
		t.Errorf("a closed sender reports %v, want nothing", err)
	}
	if err := s.Send([]byte("after")); !errors.Is(err, ErrSenderClosed) {
		t.Errorf("a send after Close returned %v, want ErrSenderClosed", err)
	}
	if !conn.closed {
		t.Error("the connection was not closed")
	}
	if conn.count() != 0 {
		t.Errorf("%d messages reached the client, want none", conn.count())
	}
}

// Closing twice, and closing something that already stopped on its own, keep
// the first answer. A second Close that reported success would tell a caller
// the connection ended cleanly when it did not.
func TestTheFirstReasonIsTheOneThatIsKept(t *testing.T) {
	conn := newStuckConn()
	s := NewSender(conn, clock.NewTest(start))

	if err := s.Send([]byte("first")); err != nil {
		t.Fatalf("the first send returned %v", err)
	}
	<-conn.entered
	for i := 0; i < MaxQueued+2; i++ {
		if err := s.Send([]byte("more")); err != nil {
			break
		}
	}
	if !errors.Is(s.Err(), ErrTooSlow) {
		t.Fatalf("the sender stopped with %v, want ErrTooSlow, so this case is not testing what it claims", s.Err())
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !errors.Is(s.Err(), ErrTooSlow) {
		t.Errorf("after Close the sender reports %v, want the reason it stopped for", s.Err())
	}
}
