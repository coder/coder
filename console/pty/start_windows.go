//go:build windows
// +build windows

package pty

import (
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func runPty(cmd *exec.Cmd) (Pty, error) {
	fullPath, err := exec.LookPath(cmd.Path)
	if err != nil {
		return nil, err
	}
	pathPtr, err := windows.UTF16PtrFromString(fullPath)
	if err != nil {
		return nil, err
	}
	argsPtr, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return nil, err
	}
	if cmd.Dir == "" {
		cmd.Dir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	dirPtr, err := windows.UTF16PtrFromString(cmd.Dir)
	if err != nil {
		return nil, err
	}
	pty, err := newPty()
	if err != nil {
		return nil, err
	}
	winPty := pty.(*ptyWindows)

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	err = attrs.Update(22|0x00020000, unsafe.Pointer(winPty.console), unsafe.Sizeof(winPty.console))
	if err != nil {
		return nil, err
	}

	startupInfo := &windows.StartupInfoEx{}
	startupInfo.StartupInfo.Cb = uint32(unsafe.Sizeof(*startupInfo))
	startupInfo.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	startupInfo.ProcThreadAttributeList = attrs.List()
	var processInfo windows.ProcessInformation
	err = windows.CreateProcess(
		pathPtr,
		argsPtr,
		nil,
		nil,
		false,
		// https://docs.microsoft.com/en-us/windows/win32/procthread/process-creation-flags#create_unicode_environment
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT,
		// Environment variables can come later!
		nil,
		dirPtr,
		&startupInfo.StartupInfo,
		&processInfo,
	)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(windows.Handle(processInfo.Thread))

	return pty, nil
}
