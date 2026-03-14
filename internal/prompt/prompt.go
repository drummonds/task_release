package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	reader io.Reader = os.Stdin
	writer io.Writer = os.Stdout
)

// SetIO overrides stdin/stdout for testing.
func SetIO(r io.Reader, w io.Writer) {
	reader = r
	writer = w
}

// Confirm asks a yes/no question. Default is yes.
func Confirm(msg string) bool {
	_, _ = fmt.Fprintf(writer, "%s [Y/n] ", msg)
	s := bufio.NewScanner(reader)
	if !s.Scan() {
		return false
	}
	ans := strings.TrimSpace(strings.ToLower(s.Text()))
	return ans == "" || ans == "y" || ans == "yes"
}

// AskString prompts for a string with a default value.
func AskString(msg, def string) string {
	if def != "" {
		_, _ = fmt.Fprintf(writer, "%s [%s]: ", msg, def)
	} else {
		_, _ = fmt.Fprintf(writer, "%s: ", msg)
	}
	s := bufio.NewScanner(reader)
	if !s.Scan() {
		return def
	}
	ans := strings.TrimSpace(s.Text())
	if ans == "" {
		return def
	}
	return ans
}

// AutoConfirm controls whether prompts auto-accept defaults.
var AutoConfirm bool

// ConfirmOrAuto returns true immediately if AutoConfirm is set.
func ConfirmOrAuto(msg string) bool {
	if AutoConfirm {
		_, _ = fmt.Fprintf(writer, "%s [Y/n] y (auto)\n", msg)
		return true
	}
	return Confirm(msg)
}

// AskStringOrAuto returns the default immediately if AutoConfirm is set.
func AskStringOrAuto(msg, def string) string {
	if AutoConfirm {
		_, _ = fmt.Fprintf(writer, "%s [%s]: %s (auto)\n", msg, def, def)
		return def
	}
	return AskString(msg, def)
}
