//go:build windows

package main

import (
	"os"
	"time"
)

type resources struct {
	UserCPU, SystemCPU time.Duration
	MemoryBytes        int64
	Available          bool
}

func resourceUsage(*os.ProcessState) resources { return resources{} }
