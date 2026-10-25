//go:build !windows

package cli

// terminalDevice is the controlling terminal of the process.
const terminalDevice = "/dev/tty"
