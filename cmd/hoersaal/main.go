// Command hoersaal is the service entry point. It does nothing yet.
//
// It exists so that the toolchain decision on issue #15 is proved by a build
// rather than asserted, and so that the checks on this milestone have something
// to compile. What the repository is laid out as, and where the boundary
// between the control plane and the media plane falls in it, is issue #16.
package main

import (
	"fmt"
	"strings"
)

func main() {
	banner := "  hoersaal: nothing is implemented yet  "
	//lint:ignore SA4017 deliberate, and this reason is the thing under test on issue #22
	strings.TrimSpace(banner)
	fmt.Println(banner)
}
