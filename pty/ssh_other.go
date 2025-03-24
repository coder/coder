//go:build !windows

package pty

import (
	"log"

	"github.com/gliderlabs/ssh"
	"github.com/u-root/u-root/pkg/termios"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
)

// terminalModeFlagNames maps the SSH terminal mode flags to mnemonic
// names used by the termios package.
var terminalModeFlagNames = map[uint8]string{
	gossh.VINTR:         "intr",
	gossh.VQUIT:         "quit",
	gossh.VERASE:        "erase",
	gossh.VKILL:         "kill",
	gossh.VEOF:          "eof",
	gossh.VEOL:          "eol",
	gossh.VEOL2:         "eol2",
	gossh.VSTART:        "start",
	gossh.VSTOP:         "stop",
	gossh.VSUSP:         "susp",
	gossh.VDSUSP:        "dsusp",
	gossh.VREPRINT:      "rprnt",
	gossh.VWERASE:       "werase",
	gossh.VLNEXT:        "lnext",
	gossh.VFLUSH:        "flush",
	gossh.VSWTCH:        "swtch",
	gossh.VSTATUS:       "status",
	gossh.VDISCARD:      "discard",
	gossh.IGNPAR:        "ignpar",
	gossh.PARMRK:        "parmrk",
	gossh.INPCK:         "inpck",
	gossh.ISTRIP:        "istrip",
	gossh.INLCR:         "inlcr",
	gossh.IGNCR:         "igncr",
	gossh.ICRNL:         "icrnl",
	gossh.IUCLC:         "iuclc",
	gossh.IXON:          "ixon",
	gossh.IXANY:         "ixany",
	gossh.IXOFF:         "ixoff",
	gossh.IMAXBEL:       "imaxbel",
	gossh.IUTF8:         "iutf8",
	gossh.ISIG:          "isig",
	gossh.ICANON:        "icanon",
	gossh.XCASE:         "xcase",
	gossh.ECHO:          "echo",
	gossh.ECHOE:         "echoe",
	gossh.ECHOK:         "echok",
	gossh.ECHONL:        "echonl",
	gossh.NOFLSH:        "noflsh",
	gossh.TOSTOP:        "tostop",
	gossh.IEXTEN:        "iexten",
	gossh.ECHOCTL:       "echoctl",
	gossh.ECHOKE:        "echoke",
	gossh.PENDIN:        "pendin",
	gossh.OPOST:         "opost",
	gossh.OLCUC:         "olcuc",
	gossh.ONLCR:         "onlcr",
	gossh.OCRNL:         "ocrnl",
	gossh.ONOCR:         "onocr",
	gossh.ONLRET:        "onlret",
	gossh.CS7:           "cs7",
	gossh.CS8:           "cs8",
	gossh.PARENB:        "parenb",
	gossh.PARODD:        "parodd",
	gossh.TTY_OP_ISPEED: "tty_op_ispeed",
	gossh.TTY_OP_OSPEED: "tty_op_ospeed",
}

// applyTerminalModesToFd applies the terminal settings from the SSH
// request to the given fd.
//
// This is based on code from Tailscale's tailssh package:
// https://github.com/tailscale/tailscale/blob/main/ssh/tailssh/incubator.go
func applyTerminalModesToFd(logger *log.Logger, fd uintptr, req ssh.Pty) error {
	// Get the current TTY configuration.
	tios, err := termios.GTTY(int(fd)) // #nosec G115 -- uintptr to int is safe for file descriptors
	if err != nil {
		return xerrors.Errorf("GTTY: %w", err)
	}

	// Apply the modes from the SSH request.
	tios.Row = req.Window.Height
	tios.Col = req.Window.Width

	for c, v := range req.Modes {
		if c == gossh.TTY_OP_ISPEED {
			tios.Ispeed = int(v)	// #nosec G115 -- uint32 to int is safe for TTY speeds
			continue
		}
		if c == gossh.TTY_OP_OSPEED {
			tios.Ospeed = int(v)	// #nosec G115 -- uint32 to int is safe for TTY speeds
			continue
		}
		k, ok := terminalModeFlagNames[c]
		if !ok {
			if logger != nil {
				logger.Printf("unknown terminal mode: %d", c)
			}
			continue
		}
		if _, ok := tios.CC[k]; ok {
			if v <= 255 { // Ensure value fits in uint8
				tios.CC[k] = uint8(v) // #nosec G115 -- value is checked to fit in uint8
			}
			continue
		}
		if _, ok := tios.Opts[k]; ok {
			tios.Opts[k] = v > 0
			continue
		}

		if logger != nil {
			logger.Printf("unsupported terminal mode: k=%s, c=%d, v=%d", k, c, v)
		}
	}
	// #nosec G115 -- int to int64 is safe for file descriptors
	// Save the new TTY configuration.
	if _, err := tios.STTY(int(fd)); err != nil {	// #nosec G115 -- uintptr to int is safe for file descriptors
		return xerrors.Errorf("STTY: %w", err)
	}

	return nil
}
