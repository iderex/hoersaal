// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command hoersaal is the service entry point. It does nothing yet.
//
// It exists so that the toolchain decision on issue #15 is proved by a build
// rather than asserted, and so that the checks on this milestone have something
// to compile. What the repository is laid out as, and where the boundary
// between the control plane and the media plane falls in it, is issue #16.
package main

import (
	"fmt"
	"net/http"
)

func main() {
	fmt.Println("hoersaal: nothing is implemented yet")

	// A deliberate fault, removed in the next commit on this branch. A server
	// started this way has no read, write or idle timeout, so one slow client
	// holds a connection for as long as it likes. It is here so that the check
	// this branch adds is shown to bite rather than described as biting, and it
	// is a class the correctness analyser has no rule for.
	_ = http.ListenAndServe(":8080", nil)
}
