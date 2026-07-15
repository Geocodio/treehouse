package cli

import "golang.org/x/sys/unix"

// ttyIoctlRequest is the ioctl request that reads terminal attributes on
// Linux; see tty.go for why this needs to be a real ioctl rather than an
// os.ModeCharDevice check.
const ttyIoctlRequest = unix.TCGETS
