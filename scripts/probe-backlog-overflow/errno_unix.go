//go:build !windows

package main

import "syscall"

// refusedErrnos are the errnos a Unix kernel returns when the listener
// sent RST during the TCP three-way handshake (no listener, or accept
// queue full with tcp_abort_on_overflow=1).
var refusedErrnos = []error{syscall.ECONNREFUSED}

// resetErrnos are the errnos a Unix kernel returns when a peer aborts an
// already-established connection. The probe never gets past handshake,
// so this should not be observed; included for symmetry.
var resetErrnos = []error{syscall.ECONNRESET}
