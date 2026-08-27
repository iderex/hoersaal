// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mediaunit is the adapter between the media plane port and the
// forwarding unit this service actually runs.
//
// It is empty. Issue #43 fills it.
//
// THIS COMMENT SAID THE UNIT WAS NOT CHOSEN YET AND IT IS. It waited for "issue
// #5's successor work", and #5 is closed as completed with an answer:
//
//	gh api repos/iderex/hoersaal/issues/5 --jq '[.number,.state,.state_reason]|@tsv'
//	5	closed	completed
//
// docs/decisions/media-plane.md names Jitsi Videobridge, driven as a separate
// process over the control interface it already publishes, and that document
// rather than this comment is where the choice is argued. So what stands between
// this directory and code is the work on #43, not a decision anybody still owes.
// It was found by reading this package against the decisions it points at.
//
// It is the only package in this repository allowed to name a type, a constant
// or a field belonging to that unit, and the only one allowed to import its
// libraries. It is reached by cmd/hoersaal and by nothing else, so removing this
// directory leaves every other package compiling and every test passing. That
// property is what docs/repository-layout.md calls the first boundary, and it is
// the one worth running rather than reading.
//
// # Two constraints the adapter meets before it writes a line, and one of them
// is not obvious
//
// The unit answers over a network and this package may not dial it.
// internal/boundary is the one directory whose files may originate a connection
// and it exempts no other, this one included, which its own Place constant
// fixes. So the call that reaches the unit is made there and the adapter hands
// it what to call, which is the arrangement that keeps a reader looking for what
// this software talks to opening one directory.
//
// The vocabulary therefore crosses that line, and it crosses as data rather than
// as a literal. internal/invariant refuses the unit's words anywhere but this
// directory, and it reads the contents of string literals as well as identifiers
// precisely because a unit's name escapes as a URL path long before it escapes
// as a Go name. A path belonging to the unit written into internal/boundary is
// refused by that rule and is meant to be; what is passed there is a value this
// package built.
//
// Neither is a contradiction between the two boundaries and neither is decided
// here. They are the shape the first line of this adapter has to have, written
// down so that #43 meets them at the design rather than at a red gate.
package mediaunit
