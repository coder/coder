//go:build windows

package main

import "syscall"

// Winsock errno values that the Go stdlib does not export. The Microsoft
// docs canonicalize these in WinError.h:
//
//	#define WSAECONNREFUSED 10061
//	#define WSAECONNRESET   10054
const (
	wsaeconnrefused syscall.Errno = 10061
	wsaeconnreset   syscall.Errno = 10054
)

// refusedErrnos are the errnos the Windows kernel returns when the
// listener sent RST during the TCP three-way handshake (no listener, or
// accept-queue overflow, which on Windows answers with RST rather than
// silently dropping the SYN).
//
// Critical: on Windows syscall.ECONNREFUSED is an "invented" Errno
// constant (APPLICATION_ERROR + N) that is NOT numerically equal to the
// real Winsock errno (10061) returned by ConnectEx. syscall.Errno.Is on
// Windows does not alias ECONNREFUSED to wsaeconnrefused (see
// src/syscall/syscall_windows.go: Errno.Is, which only handles a handful
// of oserror sentinels). Match the WSA* value directly.
var refusedErrnos = []error{wsaeconnrefused, syscall.ECONNREFUSED}

// resetErrnos are the errnos the Windows kernel returns when a peer
// aborts an already-established connection. syscall.WSAECONNRESET is in
// the stdlib (unlike WSAECONNREFUSED), but for symmetry and clarity we
// use our own constant.
var resetErrnos = []error{wsaeconnreset, syscall.WSAECONNRESET, syscall.ECONNRESET}
