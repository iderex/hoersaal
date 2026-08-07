// Package faultprobe is a deliberate fault. It exists for one commit, to show
// that the code scanning job on issue #90 refuses a finding rather than
// reporting it, and the commit after this one removes it.
package faultprobe

import "strconv"

// ParticipantCount reads a participant count a stranger sent and narrows it,
// which is the fault: the value is parsed at sixty-four bits and used at
// thirty-two, so a number above the smaller range wraps into a small or
// negative count instead of being refused.
func ParticipantCount(raw string) (int32, error) {
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return int32(parsed), nil
}
