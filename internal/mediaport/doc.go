// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mediaport is the interface the control plane uses to talk to a
// forwarding unit, and the vocabulary that goes with it.
//
// docs/decisions/media-plane-port.md specifies the eight operations, their
// arguments, their results and their errors, written before any implementation
// existed so that the interface is not shaped by the first thing that
// implemented it. This package is that specification transcribed, which is the
// first half of issue #42; the second half is the bookkeeper in mediafake, which
// is the only thing satisfying this interface today.
//
// The nouns are internal/domain's rather than this package's own. A conference,
// a participant, a source and a subscription already have types there, and the
// model's own comment says the set it returns from Reception is the set a caller
// hands to SetReception here. A second vocabulary for those four would be a
// translation layer whose only purpose is that two packages were written on
// different days.
//
// What this package adds to them is what lives only at the port: the profile a
// conference is opened with, the transport parameters an admission answers with,
// the reference and the identifier a link is made from, the fault notice, and
// the six errors. The transport parameters and the reference are opaque, which
// is a promise of the port and not an accident of their types, and it is what
// makes a fake possible: nothing above the port can tell a real unit's answer
// from a made-up one.
//
// This package sits above the boundary rather than inside the media plane. The
// control plane depends on it, and the property docs/repository-layout.md states
// is that the control plane goes on compiling when the media plane is gone,
// which is only true while the port is not part of what goes. internal/arch
// refuses the import that would break it, in the direction that matters here:
// the port may not reach for the fake.
//
// Nothing here may name a type, a constant or a field belonging to any
// particular forwarding unit. That vocabulary lives in mediaunit and nowhere
// else, and cmd/invariant refuses one written here.
package mediaport
