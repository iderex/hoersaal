// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mediaport is the interface the control plane uses to talk to a
// forwarding unit, and the vocabulary that goes with it.
//
// It is empty. docs/decisions/media-plane-port.md specifies the eight
// operations, their arguments, their results and their errors, written before
// any implementation existed so that the interface is not shaped by the first
// thing that implemented it. Transcribing that specification into this package
// is the first half of issue #42, which also builds the fake in mediafake.
//
// This package sits above the boundary rather than inside the media plane. The
// control plane depends on it, and the property docs/repository-layout.md
// states is that the control plane goes on compiling when the media plane is
// gone, which is only true while the port is not part of what goes.
//
// Nothing here may name a type, a constant or a field belonging to any
// particular forwarding unit. That vocabulary lives in mediaunit and nowhere
// else.
package mediaport
