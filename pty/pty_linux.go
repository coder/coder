//go:build linux

package pty

import (
	"github.com/u-root/u-root/pkg/termios"
	"golang.org/x/sys/unix"
)

func (p *otherPty) EchoEnabled() (echo bool, err error) {
	err = p.control(p.pty, func(fd uintptr) error {
		t, err := termios.GetTermios(fd)
		if err != nil {
			return err
		}

		echo = (t.Lflag & unix.ECHO) != 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return echo, nil
}
