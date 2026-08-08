// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mediafake satisfies the media plane port without any media at all.
//
// It is empty. Issue #42 fills it. It exists so the control plane can be
// exercised end to end on a machine with no camera, no microphone and no
// forwarding unit, which is what the whole suite runs on, and so a test can say
// what a unit did rather than inferring it from what arrived somewhere else.
//
// It names no forwarding unit. A fake that borrowed the chosen unit's
// vocabulary would put that vocabulary above the boundary, which is the failure
// the three separate places in docs/repository-layout.md exist against.
package mediafake
