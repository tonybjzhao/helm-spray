// Package log provides the small, indented, "[spray]"-prefixed logging used
// throughout helm-spray. The level passed to Info controls indentation (a
// presentation concern conveying nesting) rather than severity. The output
// destinations are package variables so tests can capture what is written.
package log

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// out and errOut are the destinations for Info and Error. Both default to
// os.Stderr so that stdout is reserved for machine-readable output (for example
// the "--output json" deployment plan). They are variables so tests can capture
// what is written.
var (
	out    io.Writer = os.Stderr
	errOut io.Writer = os.Stderr
)

const prefix = "[spray] "

// indentForLevel returns the indentation prefix for a message level. Level 1 is
// unindented; deeper levels are progressively indented to convey nesting.
func indentForLevel(level int) string {
	switch level {
	case 2:
		return "  > "
	case 3:
		return "    o "
	case 4:
		return "      - "
	default:
		if level >= 5 {
			return "        . "
		}
		return ""
	}
}

// Info writes an indented, "[spray]"-prefixed message to the info destination,
// using fmt.Printf formatting semantics. Dynamic content must be passed as an
// argument (e.g. Info(1, "%s", value)), never embedded in the format string.
func Info(level int, format string, args ...interface{}) {
	_, _ = fmt.Fprintln(out, prefix+indentForLevel(level)+fmt.Sprintf(format, args...))
}

// Error writes a "[spray]" error message to the error destination, using
// fmt.Printf formatting semantics.
func Error(format string, args ...interface{}) {
	_, _ = fmt.Fprintln(errOut, fmt.Sprintf(format, args...))
}

// WithNumberedLines writes a multi-line string with each line numbered, at the
// given indentation level. The content is treated as data (never as a format
// string), so characters such as "%" in the content are printed verbatim.
func WithNumberedLines(level int, str string) {
	numberOfLines := strings.Count(str, "\n")
	if len(str) > 0 && !strings.HasSuffix(str, "\n") {
		numberOfLines++
	}
	width := len(strconv.Itoa(numberOfLines))
	lineFormat := fmt.Sprintf("[%%%dd] %%s", width)

	scanner := bufio.NewScanner(strings.NewReader(str))
	lineNbr := 0
	for scanner.Scan() {
		Info(level, lineFormat, lineNbr, scanner.Text())
		lineNbr++
	}
}
