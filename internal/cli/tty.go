package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// IsTTY reports whether stdin is an interactive terminal. Per DESIGN.md's
// "Agent mode" section, no TTY on stdin means non-interactive: no command
// may prompt, taking its safe default or failing with ExitWouldPrompt
// instead.
//
// This deliberately doesn't use the common os.ModeCharDevice heuristic:
// /dev/null (what `< /dev/null` redirects stdin to, and what an agent
// harness with no stdin at all typically wires up) is itself a character
// device, so that check reports "interactive" for exactly the
// non-interactive case this function exists to detect. A real ioctl that
// reads terminal attributes only succeeds on an actual tty/pty.
func IsTTY() bool {
	_, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), ttyIoctlRequest)
	return err == nil
}

// confirm prompts the user with a yes/no question on stderr, reading the
// answer from stdin. Callers must check IsTTY() first — confirm itself
// does not, since some callers need ExitWouldPrompt instead of a prompt
// when stdin isn't a terminal.
func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')

	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
