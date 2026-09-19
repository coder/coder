//go:build linux

package agentegress

import "golang.org/x/sys/unix"

func dropCapabilityBoundingSet(option uintptr, arg2 uintptr, arg3 uintptr, arg4 uintptr, arg5 uintptr) error {
	return unix.Prctl(int(option), arg2, arg3, arg4, arg5)
}

func dropNetAdmin(prctl func(uintptr, uintptr, uintptr, uintptr, uintptr) error) error {
	return prctl(unix.PR_CAPBSET_DROP, unix.CAP_NET_ADMIN, 0, 0, 0)
}
