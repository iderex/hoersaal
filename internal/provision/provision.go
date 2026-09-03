// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package provision is the seam through which the scaling loop asks for the
// machine a forwarding unit runs on and gives one back. It answers issue #63
// as far as a driver can without the loop that calls it, and
// docs/decisions/provisioning-driver.md is the decision this package is the
// code of; nothing here is argued twice.
//
// Three operations and no more. Ask asks for one machine of a stated size and
// answers at once with a handle, or with ErrNoCapacity when no more is
// available at any price, which is a terminal answer the loop reports rather
// than retries. Exists reports every machine the driver holds and whether each
// one is there yet, so that a control plane coming back can reconcile what it
// recorded against what the driver actually made, which is what
// docs/decisions/control-plane-state.md asks of this seam. Remove gives a
// machine back.
//
// Ask does not wait. A machine that takes minutes to appear is the ordinary
// case for a driver that makes one, and a call that blocked for it would put a
// sleep at the centre of a loop this repository refuses to let sleep. So Ask
// records and answers, and Exists is where the machine's arrival is read, on
// whatever clock the driver was handed. That is what lets the fakes in
// internal/provisionfake run a slow driver against a controlled clock in no
// time at all.
//
// What a driver does not know. Whether the unit on a machine answers the port
// is the pool's question, read through registration and health, and a driver
// that answered it would be a second source of a fact the pool is authoritative
// for. Exists on a Machine means the machine, not the unit.
//
// Two drivers live here and the second is the absence of one. Listed hands out
// machines the operator wrote into provisioner.machines, in that order, and
// starts nothing on them: the operator started the unit there and it registers
// itself with the installation's key, so this driver reaches no network and is
// testable by anybody. None is the fixed-pool case, where asking always fails
// with ErrNoCapacity and the pool is whatever registered. No driver for a
// rented machine ships in this tree, which is the answer to entry 3 of issue #1
// and is not this package's to reopen.
package provision

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/iderex/hoersaal/internal/config"
	"github.com/iderex/hoersaal/internal/placement"
)

// A Size is what the caller asks for: the egress the unit on the machine should
// be able to commit, in bits per second, and zero asks for any machine at all.
// It is the one quantity an operator already states about a unit, in
// unit.egress-ceiling, and the capacity model on issue #54 derives everything
// else, so a driver is not asked for a number nobody holds.
type Size struct {
	Egress int64
}

// A Handle is one machine a driver holds, with the identifier the pool knows
// the unit on it by. Machine is the name as the operator or the driver wrote
// it, and it is the whole of what would be handed to internal/boundary on the
// day something dials; nothing here does.
type Handle struct {
	Unit    placement.UnitID
	Machine string
}

// A Machine is one row of what a driver holds: the handle, and whether the
// machine exists yet. A machine that exists is one the unit on it may register
// from; whether it has is the pool's answer and not this one.
type Machine struct {
	Handle Handle
	Exists bool
}

// The errors, in the three classes docs/decisions/provisioning-driver.md
// fixes. ErrNoCapacity is terminal: the loop reports it and does not ask again
// until something changes. ErrUnknown and the constructor refusals are the
// caller's mistake. Anything else a driver returns is transient and may pass.
var (
	// ErrNoCapacity is the answer that means no more machine is available at
	// any price: every listed machine is in use, or there is no driver.
	ErrNoCapacity = errors.New("no more capacity is available at any price")

	// ErrUnknown is a handle this driver never handed out or has already taken
	// back.
	ErrUnknown = errors.New("no machine is held under that handle")

	// ErrNoMachines is a listed driver built over nothing.
	ErrNoMachines = errors.New("a listed driver needs at least one machine")

	// ErrEmptyMachine is a listed machine with no name, which nothing could
	// reach.
	ErrEmptyMachine = errors.New("a listed machine has no name")

	// ErrDuplicate is a machine listed twice, which would be handed out twice
	// under one identifier.
	ErrDuplicate = errors.New("a machine is listed twice")

	// ErrUnknownDriver is a driver name the configuration accepted and this
	// package has no code for, which is a defect in one of the two lists.
	ErrUnknownDriver = errors.New("no driver of that name is built")
)

// A Driver is what the scaling loop holds. Every implementation answers Ask
// without waiting, reports the whole of what it holds from Exists, and refuses
// a Remove of what it does not hold.
type Driver interface {
	// Ask asks for one machine of at least the stated size and answers at once
	// with a handle, with ErrNoCapacity when none is available at any price, or
	// with a transient error the caller may retry.
	Ask(size Size) (Handle, error)

	// Exists reports every machine this driver holds, in a stable order, with
	// whether each one is there yet.
	Exists() ([]Machine, error)

	// Remove gives a machine back. It refuses a handle the driver does not
	// hold with ErrUnknown rather than accepting it quietly.
	Remove(h Handle) error
}

// None is the fixed-pool driver: there is nothing to ask, so Ask always refuses
// with ErrNoCapacity, Exists holds nothing, and Remove holds nothing to give
// back. It is the driver config.DriverNone names and the default configuration
// runs under.
type None struct{}

// Ask always answers ErrNoCapacity, because a fixed pool has no more at any
// price.
func (None) Ask(Size) (Handle, error) {
	return Handle{}, fmt.Errorf("no driver is configured: %w", ErrNoCapacity)
}

// Exists holds nothing: a fixed pool's units registered on their own.
func (None) Exists() ([]Machine, error) { return nil, nil }

// Remove refuses, because nothing was ever handed out.
func (None) Remove(h Handle) error {
	return fmt.Errorf("%q: %w", h.Machine, ErrUnknown)
}

// Listed hands out the machines the operator listed, in the order they were
// written, and takes them back. It starts nothing: the unit on each machine is
// the operator's to run, and it registers itself with the pool under the
// machine's name, which is why the handle's unit identifier is that name.
type Listed struct {
	mu       sync.Mutex
	machines []string
	inUse    map[string]bool
}

// NewListed refuses an empty list, an empty name and a name listed twice, so
// that every machine it can hand out is one that can be reached and is handed
// out once.
func NewListed(machines []string) (*Listed, error) {
	if len(machines) == 0 {
		return nil, ErrNoMachines
	}
	seen := make(map[string]bool, len(machines))
	for _, m := range machines {
		if strings.TrimSpace(m) == "" {
			return nil, ErrEmptyMachine
		}
		if seen[m] {
			return nil, fmt.Errorf("%q: %w", m, ErrDuplicate)
		}
		seen[m] = true
		if _, err := placement.NewUnitID(m); err != nil {
			return nil, fmt.Errorf("%q: %w", m, err)
		}
	}
	return &Listed{
		machines: append([]string(nil), machines...),
		inUse:    make(map[string]bool, len(machines)),
	}, nil
}

// Ask hands out the first listed machine not in use. The size is not read: a
// listed machine is what it is, and whether it is large enough is what the
// capacity signal finds out, which docs/decisions/provisioning-driver.md says
// where the choice is argued.
func (l *Listed) Ask(Size) (Handle, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.machines {
		if l.inUse[m] {
			continue
		}
		l.inUse[m] = true
		return l.handle(m), nil
	}
	return Handle{}, fmt.Errorf("%d listed machine(s), every one in use: %w", len(l.machines), ErrNoCapacity)
}

// Exists reports the machines in use, in the listed order. A listed machine
// exists from the moment it is handed out, because it was there before the
// driver was.
func (l *Listed) Exists() ([]Machine, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Machine
	for _, m := range l.machines {
		if l.inUse[m] {
			out = append(out, Machine{Handle: l.handle(m), Exists: true})
		}
	}
	return out, nil
}

// Remove takes a machine back so it may be handed out again.
func (l *Listed) Remove(h Handle) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.inUse[h.Machine] {
		return fmt.Errorf("%q: %w", h.Machine, ErrUnknown)
	}
	delete(l.inUse, h.Machine)
	return nil
}

// handle builds the handle for a listed name. The identifier cannot fail here
// because NewListed refused every name it could fail on.
func (l *Listed) handle(m string) Handle {
	id, _ := placement.NewUnitID(m)
	return Handle{Unit: id, Machine: m}
}

// Open answers with the driver a validated configuration names. It is the one
// place the name in the configuration meets the code for it, so a name the
// configuration accepts and this package cannot build is refused here by name
// rather than reaching a loop as a nil.
func Open(s config.Settings) (Driver, error) {
	switch s.ProvisionerDriver {
	case config.DriverNone:
		return None{}, nil
	case config.DriverListed:
		return NewListed(s.ProvisionerMachines)
	}
	return nil, fmt.Errorf("%q: %w", s.ProvisionerDriver, ErrUnknownDriver)
}
