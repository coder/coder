//go:build windows

package dbtestutil

import "syscall"

// Windows's syscall.ECONNREFUSED is an invented constant
// (APPLICATION_ERROR + N), not WSAECONNREFUSED (10061). syscall.Errno.Is
// on Windows does not alias the two. The Go stdlib exports
// syscall.WSAECONNRESET but not WSAECONNREFUSED, so declare it locally.
const (
	wsaeconnrefused syscall.Errno = 10061
	wsaeconnreset   syscall.Errno = 10054
)

var transientConnectErrnos = []error{
	wsaeconnrefused,
	wsaeconnreset,
	syscall.WSAECONNRESET,
	syscall.ECONNREFUSED,
	syscall.ECONNRESET,
}
