package util

import (
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{60 * time.Second, "1m"},
		{time.Hour, "1h"},
		{time.Hour + time.Minute + time.Second, "1h1m1s"},
		{1500 * time.Millisecond, "1s"}, // sub-second component is truncated
	}
	for _, c := range cases {
		if got := Duration(c.in); got != c.want {
			t.Errorf("Duration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
