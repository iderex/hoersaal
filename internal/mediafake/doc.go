// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mediafake satisfies the media plane port without any media at all.
//
// It answers issue #42. It exists so the control plane can be exercised end to
// end on a machine with no camera, no microphone and no forwarding unit, which
// is what the whole suite runs on, and so a test can say what a unit did rather
// than inferring it from what arrived somewhere else.
//
// It is a bookkeeper rather than a stub, and the difference is what makes it
// worth having. It keeps the records a unit keeps and answers out of them, so a
// caller that admits a subscriber to a conference that was never opened gets the
// error a real unit would give, and a test cannot pass by doing something the
// real thing would refuse.
//
// The other half is that it can be told what to be. Its capacity signal, its
// latency, how much of a reception set it accepts, which operations it refuses,
// when it dies and when it comes back are all set by the test, so the scaling
// milestone can put a pool through a failure without owning a pool. Nothing here
// is left to chance, and nothing here sleeps: a latency is a wait on the clock
// the fake was handed, which a clock.Test finishes the moment a test advances
// it.
//
// Two fakes in one Fabric can carry one conference between them. Each answers
// with a reference the other accepts, and once both have linked, a subscriber on
// either unit may receive a source published on the other. That is what a
// cascade test needs and it needs no real unit anywhere, because the port
// declares the reference opaque to everything above it, so the shape the two
// fakes agree on is nobody else's business.
//
// It names no forwarding unit. A fake that borrowed the chosen unit's vocabulary
// would put that vocabulary above the boundary, which is the failure the three
// separate places in docs/repository-layout.md exist against.
//
// What it does not decide, and where the limit is. OpenConference answers
// ErrInvalid where the profile is not one the unit can serve, and which profiles
// are servable at all is the codec and layer policy on issue #48, which is not
// decided. So that arm is set by the test through Serves, and the neighbouring
// arm, a source outside the profile its conference was opened with, is a
// comparison against the fake's own record and is enforced. The Unit comment
// says the same thing where somebody writing a test will meet it.
package mediafake
