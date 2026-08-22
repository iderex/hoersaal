// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package wire

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/iderex/hoersaal/internal/clock"
)

// The numbers the outbound side refuses against, argued in
// docs/decisions/signalling-transport.md.
const (
	// MaxQueued is how many messages may be waiting for one client before that
	// client is disconnected. It is a count of messages rather than of bytes
	// because what this bounds is how far behind a client may fall, and the
	// messages this service sends are all small.
	MaxQueued = 64

	// WriteTimeout is how long one write to one client may take before that
	// client is disconnected.
	WriteTimeout = 10 * time.Second
)

var (
	// ErrTooSlow is a client that has stopped reading. Its queue reached
	// MaxQueued, so it is disconnected rather than allowed to grow a buffer in
	// this process until the machine dies.
	ErrTooSlow = errors.New("wire: the client is not reading")

	// ErrWriteTimeout is a write to one client that did not finish inside
	// WriteTimeout. It is the same failure as ErrTooSlow seen from the other
	// end, and it is a separate value because a deployment seeing one and not
	// the other is looking at a different problem.
	ErrWriteTimeout = errors.New("wire: a write to the client did not finish in time")

	// ErrSenderClosed is a send to a client that is already gone.
	ErrSenderClosed = errors.New("wire: the sender is closed")
)

// A Conn is what a Sender writes to. It is an interface with two operations
// because that is all the outbound side needs, and because a suite that has to
// stand up a real connection to prove backpressure proves it on a good day
// only.
//
// Write sends one message. It may block, and a Sender is built to survive one
// that blocks forever.
type Conn interface {
	Write(message []byte) error
	Close() error
}

// A Sender holds the outbound queue for one client and is the only thing that
// writes to it.
//
// What it exists to refuse is one client's slowness turning into this process's
// memory. A client that stops reading fills its queue and is disconnected, and
// the queue is a fixed size decided when the Sender is made rather than
// whatever the client's behaviour produces.
type Sender struct {
	conn  Conn
	clock clock.Clock

	queue chan []byte

	stop      chan struct{} // closed when this Sender is finished with, for any reason
	done      chan struct{} // closed by the writer as it stops
	stopOnce  sync.Once
	closeOnce sync.Once

	mu     sync.Mutex
	reason error // why this Sender stopped, nil while it is running
}

// NewSender starts the one goroutine that writes to conn and returns the Sender
// that feeds it. Nothing else may write to conn afterwards.
func NewSender(conn Conn, c clock.Clock) *Sender {
	s := &Sender{
		conn:  conn,
		clock: c,
		queue: make(chan []byte, MaxQueued),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	go s.run()
	return s
}

// Send puts one message on this client's queue. It never blocks: either the
// message is queued, or the queue is full and the client is disconnected before
// this returns.
//
// The message must not be changed afterwards. It is written from another
// goroutine, and a caller that reuses a buffer sends something nobody wrote.
func (s *Sender) Send(message []byte) error {
	select {
	case <-s.stop:
		return s.failure()
	default:
	}

	select {
	case s.queue <- message:
		return nil
	default:
		s.finish(ErrTooSlow)
		return ErrTooSlow
	}
}

// Close disconnects the client for a reason that is not a fault. A Sender
// already stopped keeps the reason it stopped for, because the first answer is
// the one that explains the rest.
//
// It waits for the writer to finish what it is doing and to write what is
// already queued, because a message that was accepted has been promised. What
// bounds that wait is WriteTimeout for each message still to go, so a caller
// closing a client that has stopped reading waits for the timeout rather than
// returning at once. A caller that does not want to wait at all has a client
// that is a fault rather than a departure, and that path is the one that closes
// the connection under the writer.
func (s *Sender) Close() error {
	s.finish(nil)
	<-s.done
	return nil
}

// Done is closed once this Sender has stopped and the connection is closed.
func (s *Sender) Done() <-chan struct{} { return s.done }

// Err is why this Sender stopped, or nil while it is running or where it was
// closed deliberately.
func (s *Sender) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

// run is the only writer. It takes messages in the order they were queued and
// stops at the first failure, because a connection that has failed one write
// has failed.
func (s *Sender) run() {
	defer close(s.done)
	defer s.closeConnection()

	for {
		// Stopping is looked at first and on its own. A select over both would
		// choose between them at random while the queue still held messages,
		// so whether a disconnected client's backlog was written to a closed
		// connection would depend on the run rather than on the decision.
		select {
		case <-s.stop:
			s.drain()
			return
		default:
		}

		select {
		case message := <-s.queue:
			if err := s.write(message); err != nil {
				s.finish(err)
				return
			}
		case <-s.stop:
			s.drain()
			return
		}
	}
}

// drain writes what is already queued, and is reached only where this Sender
// was closed deliberately. A message that was accepted has been promised, and
// dropping it because the room was closing is the kind of loss nobody can
// account for afterwards.
//
// It is not reached where the Sender stopped on a fault. There the client is
// either not reading or not there, and writing to it would be waiting on the
// thing that already failed.
func (s *Sender) drain() {
	if s.Err() != nil {
		return
	}
	for {
		select {
		case message := <-s.queue:
			if err := s.write(message); err != nil {
				s.record(err)
				return
			}
		default:
			return
		}
	}
}

// write sends one message and gives up on it after WriteTimeout.
//
// The wait is registered before the write starts rather than after, so a caller
// that has seen the write begin knows the timeout is already running. That
// ordering is what lets a test move a clock and get a decided answer instead of
// a race.
//
// A write that never returns leaves its goroutine parked on the connection for
// as long as the connection holds it. That is a real residual and it is stated
// here: this package cannot cancel a Write it was handed, and the alternative,
// requiring every Conn to carry a deadline, would put the whole of the
// behaviour into whatever implements the interface.
func (s *Sender) write(message []byte) error {
	timeout := s.clock.After(WriteTimeout)

	written := make(chan error, 1)
	go func() { written <- s.conn.Write(message) }()

	select {
	case err := <-written:
		if err != nil {
			return fmt.Errorf("wire: writing to the client: %w", err)
		}
		return nil
	case <-timeout:
		return ErrWriteTimeout
	}
}

// finish stops the writer for a reason decided outside it. It closes the
// connection itself rather than waiting for the writer to notice, because the
// case this exists for is a writer stuck inside a Write that is not coming
// back, and closing the connection under it is what ends that. It waits for
// nothing, so a caller inside Send is not held up by the connection that is
// already the problem.
func (s *Sender) finish(reason error) {
	s.stopOnce.Do(func() {
		s.record(reason)
		close(s.stop)
		if reason != nil {
			s.closeConnection()
		}
	})
}

// closeConnection closes the connection once, however many things decide it is
// time. The error is dropped: this is the end of the connection either way, and
// nothing above can act on it.
func (s *Sender) closeConnection() {
	s.closeOnce.Do(func() { _ = s.conn.Close() })
}

// record keeps the first reason and never replaces it.
func (s *Sender) record(reason error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reason == nil {
		s.reason = reason
	}
}

// failure is what Send returns once this Sender has stopped: the reason it
// stopped where there is one, and ErrSenderClosed where it was closed
// deliberately.
func (s *Sender) failure() error {
	if err := s.Err(); err != nil {
		return err
	}
	return ErrSenderClosed
}
