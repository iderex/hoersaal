// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package admission_test

import (
	"sort"
	"testing"
	"time"
)

// TestAdmissionsGrantedInOneInstantAreSweptInIdentifierOrder pins the tiebreak
// Sweep promises: grants made at one clock reading come back ordered by
// participant identifier, ascending, so what a caller records reads the same
// way on every run rather than in map order. The mutation run on issue #93
// found nothing held the direction: a sweep answering the same grants in
// descending order left the suite green.
func TestAdmissionsGrantedInOneInstantAreSweptInIdentifierOrder(t *testing.T) {
	b := newBench(t, withWindow(time.Minute))

	// Three grants with no clock movement between them, so GrantedAt is one
	// instant for all three and only the identifier can order them.
	for range 3 {
		if _, ok := b.admit(b.request(b.credential(attendee), false)).Granted(); !ok {
			t.Fatal("the admission was refused")
		}
	}

	b.clock.Advance(time.Minute)
	swept := b.desk.Sweep()
	if len(swept) != 3 {
		t.Fatalf("swept %d admissions, want 3", len(swept))
	}
	for i := 1; i < len(swept); i++ {
		if !swept[i].GrantedAt.Equal(swept[0].GrantedAt) {
			t.Fatalf("the grants are not in one instant, so this case is not testing the tiebreak")
		}
	}

	got := make([]string, 0, len(swept))
	for _, a := range swept {
		got = append(got, a.Participant.String())
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("swept in the order %v, and admissions granted in one instant are answered by identifier, ascending", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] == got[i] {
			t.Errorf("two swept admissions carry the identifier %q, so the order between them is meaningless", got[i])
		}
	}
}
