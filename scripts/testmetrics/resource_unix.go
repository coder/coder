//go:build !windows

package main

import (
	"os"
	"runtime"
	"syscall"
	"time"
)

type resources struct {
	UserCPU, SystemCPU time.Duration
	MemoryBytes        int64
	Available          bool
}

func resourceUsage(state *os.ProcessState) resources {
	if state == nil {
		return resources{}
	}
	rusage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || rusage == nil {
		return resources{}
	}
	user := time.Duration(rusage.Utime.Sec)*time.Second + time.Duration(rusage.Utime.Usec)*time.Microsecond
	system := time.Duration(rusage.Stime.Sec)*time.Second + time.Duration(rusage.Stime.Usec)*time.Microsecond
	memory := rusage.Maxrss
	if runtime.GOOS == "linux" {
		memory *= 1024
	}
	return resources{UserCPU: user, SystemCPU: system, MemoryBytes: memory, Available: true}
}
