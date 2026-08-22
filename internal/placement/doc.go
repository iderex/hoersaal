// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package placement answers which unit carries a conference, and which unit
// carries a participant arriving at a conference that is already running.
//
// It answers both. Issue #57 is the policy for a conference that is on no unit
// yet, argued in docs/decisions/new-conference-placement.md. Issue #58 is the
// participant arriving at a conference that is already running, and it is where
// the third refusal reason the seam names arrives, because a conference on no
// unit cannot have reached its unit ceiling. One policy answers both, as Naive,
// behind two interfaces: the questions take different records and the second is
// the only one with a conference to keep together.
//
// One thing this package holds that no measurement supports yet, said here
// rather than left to be found. The unit ceiling is passed in rather than
// derived, because the bound in docs/decisions/room-topology.md is over two
// figures neither of which is in any record the placer is handed and neither of
// which has a value on this board. Issue #59 is where that figure comes from.
// Until it does, a caller filling the record in is choosing the number, and
// nothing here can tell a derived ceiling from a guessed one.
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
