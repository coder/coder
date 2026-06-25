//go:build !windows

package dbtestutil

import "syscall"

var transientConnectErrnos = []error{
	syscall.ECONNREFUSED,
	syscall.ECONNRESET,
}
