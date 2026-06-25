// Package util holds small helpers shared across helm-spray.
package util

import (
	"strings"
	"time"
)

// Duration formats a duration for human-readable output, truncated to whole
// seconds, dropping a trailing zero-second or zero-minute component (e.g. a
// one-hour duration renders as "1h" rather than "1h0m0s").
func Duration(d time.Duration) string {
	d = d.Truncate(time.Second)
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}
