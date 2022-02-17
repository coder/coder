//go:build windows
// +build windows

package pty

import (
	"os"
	"os/exec"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Allocates a PTY and starts the specified command attached to it.
// See: https://docs.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session#creating-the-hosted-process
func startPty(cmd *exec.Cmd) (PTY, error) {
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
	err = attrs.Update(0x20016, unsafe.Pointer(winPty.console), unsafe.Sizeof(winPty.console))
	if err != nil {
		return nil, err
	}

	startupInfo := &windows.StartupInfoEx{}
	startupInfo.ProcThreadAttributeList = attrs.List()
	startupInfo.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	startupInfo.StartupInfo.Cb = uint32(unsafe.Sizeof(*startupInfo))
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
		createEnvBlock([]string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}),
		dirPtr,
		&startupInfo.StartupInfo,
		&processInfo,
	)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(processInfo.Thread)
	defer windows.CloseHandle(processInfo.Process)

	return pty, nil
}

// Taken from: https://github.com/microsoft/hcsshim/blob/7fbdca16f91de8792371ba22b7305bf4ca84170a/internal/exec/exec.go#L476
func createEnvBlock(envv []string) *uint16 {
	if len(envv) == 0 {
		return &utf16.Encode([]rune("\x00\x00"))[0]
	}
	length := 0
	for _, s := range envv {
		length += len(s) + 1
	}
	length += 1

	b := make([]byte, length)
	i := 0
	for _, s := range envv {
		l := len(s)
		copy(b[i:i+l], []byte(s))
		copy(b[i+l:i+l+1], []byte{0})
		i = i + l + 1
	}
	copy(b[i:i+1], []byte{0})

	return &utf16.Encode([]rune(string(b)))[0]
}
