// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package placement answers which unit carries a conference, and which unit
// carries a participant arriving at a conference that is already running.
//
// It is empty. Issues #57 and #58 fill it.
//
// docs/decisions/placement-seam.md fixes what it may read: three records, and
// nothing else is passed or reachable. This package holds no connection, opens
// no socket and reads no clock of its own, because it is the component most
// likely to be replaced and that is only cheap while its answer is a function
// of its inputs. A placer that can reach the network is one whose answer
// depends on when it was asked.
package placement
