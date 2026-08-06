package guard

import (
	"net"
	"testing"
)

// TestAToolkitOpensTheDisplay is temporary and is removed in a following commit
// on this branch. It is the near-miss on issue #18: a test that reaches for a
// display, written the way a toolkit reaches for one, so the unit check is shown
// to red rather than assumed to.
func TestAToolkitOpensTheDisplay(t *testing.T) {
	conn, err := net.Dial("unix", "/tmp/.X11-unix/X0")
	if err != nil {
		t.Fatalf("opening the display: %v", err)
	}
	defer conn.Close()
}
