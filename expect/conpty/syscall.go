//go:build windows
// +build windows

// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package conpty

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procResizePseudoConsole = kernel32.NewProc("ResizePseudoConsole")
	procCreatePseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	procClosePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
)

func createPseudoConsole(consoleSize uintptr, ptyIn windows.Handle, ptyOut windows.Handle, hpCon *windows.Handle) (err error) {
	r1, _, e1 := procCreatePseudoConsole.Call(
		consoleSize,
		uintptr(ptyIn),
		uintptr(ptyOut),
		0,
		uintptr(unsafe.Pointer(hpCon)),
	)

	if r1 != 0 { // !S_OK
		err = e1
	}
	return
}

func resizePseudoConsole(handle windows.Handle, consoleSize uintptr) (err error) {
	r1, _, e1 := procResizePseudoConsole.Call(uintptr(handle), consoleSize)
	if r1 != 0 { // !S_OK
		err = e1
	}
	return
}

func closePseudoConsole(handle windows.Handle) (err error) {
	r1, _, e1 := procClosePseudoConsole.Call(uintptr(handle))
	if r1 == 0 {
		err = e1
	}

	return
}
