// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package pool

import (
	"testing"
	"time"
)

// TestZeroIsALoadAndNotARefusal pins the lower edge of what Report and Commit
// accept. An idle unit reports exactly zero and a placement that commits no
// bitrate commits exactly zero, and both are answers rather than mistakes. The
// mutation run on issue #93 found the edge held by nothing: a refusal of "zero
// or below" in place of "below zero" left the suite green, and under it an
// idle unit could never report.
func TestZeroIsALoadAndNotARefusal(t *testing.T) {
	p, _, id, _ := admitted(t, "unit-a")
	if err := p.Report(id, 0); err != nil {
		t.Fatalf("Report(0) = %v, and an idle unit reports exactly that", err)
	}
	if err := p.Commit(id, 0); err != nil {
		t.Fatalf("Commit(0) = %v, and a placement that commits nothing is still a placement", err)
	}
	row, ok := p.Unit(id)
	if !ok {
		t.Fatal("the unit is gone from the pool")
	}
	if row.Reported() != 0 || row.EffectiveLoad() != 0 {
		t.Errorf("after a report of zero and a commitment of zero the row reads %v reported and %v effective, want 0 and 0", row.Reported(), row.EffectiveLoad())
	}
}

// TestSweepRetiresADrainingUnitWhoseSignalWentStale is the draining half of
// what Sweep judges. A unit that is draining still carries people, and one
// that stops answering while it drains is as dead as an admitting one that
// stops answering. The mutation run on issue #93 found only the admitting half
// held: a sweep that skipped every draining unit left the suite green.
func TestSweepRetiresADrainingUnitWhoseSignalWentStale(t *testing.T) {
	const window = 30 * time.Second
	p, clk, id, u := admitted(t, "unit-a")
	u.SetLoad(0.4)
	p.Collect()
	if err := p.Drain(id, TheOperatorAsked()); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	clk.Advance(window + time.Second)
	retired, err := p.Sweep(window)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(retired) != 1 || retired[0] != id {
		t.Fatalf("Sweep retired %v, want [%s]: a draining unit that stopped answering is gone like any other", retired, id)
	}
	row, _ := p.Unit(id)
	if row.State() != Gone() || row.Cause() != TheSignalWentStale() {
		t.Errorf("the unit is %s because %q, want %s because %q", row.State(), row.Cause(), Gone(), TheSignalWentStale())
	}
}
