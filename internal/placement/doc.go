// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package placement answers which unit carries a conference, and which unit
// carries a participant arriving at a conference that is already running.
//
// It answers the first of those two questions. Issue #57 is the policy for a
// conference that is on no unit yet, and it is here, argued in
// docs/decisions/new-conference-placement.md and implemented as Naive. Issue #58
// is the arriving participant, and nothing here answers it: PlaceConference is
// the only question this package takes, and the third refusal reason the seam
// names, the conference reaching its unit ceiling, arrives with that issue
// because a conference on no unit cannot have reached one.
//
// The records the placer reads are declared here rather than taken from the pool
// on issue #56, because the rule below leaves nowhere else for them: the model
// holds no unit, and this package may import nothing else from this repository.
// So the pool fills in a record this package declares, which is the direction
// that keeps a placement a function of what the placer was handed.
//
// docs/decisions/placement-seam.md fixes what it may read: three records, and
// nothing else is passed or reachable. This package holds no connection, opens
// no socket and reads no clock of its own, because it is the component most
// likely to be replaced and that is only cheap while its answer is a function
// of its inputs. A placer that can reach the network is one whose answer
// depends on when it was asked.
package placement
