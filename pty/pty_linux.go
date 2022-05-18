// go:build linux

package pty

import "golang.org/x/sys/unix"

func (p *otherPty) EchoEnabled() (bool, error) {
	termios, err := unix.IoctlGetTermios(int(p.pty.Fd()), unix.TCGETS)
	if err != nil {
		return false, err
	}
	return (termios.Lflag & unix.ECHO) != 0, nil
}
