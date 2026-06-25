package log

import (
	"bytes"
	"strings"
	"testing"
)

// capture redirects the package output to buffers and returns a restore func.
func capture(t *testing.T) (info, errs *bytes.Buffer, restore func()) {
	t.Helper()
	info, errs = &bytes.Buffer{}, &bytes.Buffer{}
	origOut, origErr := out, errOut
	out, errOut = info, errs
	return info, errs, func() { out, errOut = origOut, origErr }
}

func TestInfoFormatsAndIndents(t *testing.T) {
	info, _, restore := capture(t)
	defer restore()

	Info(1, "hello %s", "world")
	if got, want := info.String(), "[spray] hello world\n"; got != want {
		t.Errorf("level 1: got %q want %q", got, want)
	}

	info.Reset()
	Info(2, "nested")
	if got, want := info.String(), "[spray]   > nested\n"; got != want {
		t.Errorf("level 2: got %q want %q", got, want)
	}
}

func TestErrorGoesToErrorWriter(t *testing.T) {
	info, errs, restore := capture(t)
	defer restore()

	Error("boom: %d", 42)
	if got, want := errs.String(), "boom: 42\n"; got != want {
		t.Errorf("error output: got %q want %q", got, want)
	}
	if info.Len() != 0 {
		t.Errorf("error must not write to the info destination, got %q", info.String())
	}
}

func TestWithNumberedLinesTreatsContentAsData(t *testing.T) {
	info, _, restore := capture(t)
	defer restore()

	WithNumberedLines(1, "first\n100% second\nthird")
	got := info.String()
	for _, want := range []string{"[0] first", "[1] 100% second", "[2] third"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; full output:\n%s", want, got)
		}
	}
}
